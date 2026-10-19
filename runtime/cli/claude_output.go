package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const maxClaudeJSONLLineBytes = 10 * 1024 * 1024

// ClaudeResultError represents a failed result envelope, including runs whose
// process exited with code zero. Status comes from the protocol, not the shell.
type ClaudeResultError struct {
	Subtype        string
	TerminalReason string
	Messages       []string
}

func (e *ClaudeResultError) Error() string {
	if e == nil {
		return "claude result failed"
	}
	status := e.Subtype
	if e.TerminalReason != "" {
		status += " terminal_reason=" + e.TerminalReason
	}
	message := "claude result failed: " + strings.TrimSpace(status)
	if len(e.Messages) != 0 {
		message += ": " + strings.Join(e.Messages, "; ")
	}
	return message
}

func (e *ClaudeResultError) Unwrap() error {
	if e == nil {
		return nil
	}
	status := strings.ToLower(e.Subtype + " " + e.TerminalReason)
	for _, marker := range []string{"abort", "cancel", "interrupt"} {
		if strings.Contains(status, marker) {
			return context.Canceled
		}
	}
	return nil
}

type claudeCLIEvent struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          *bool           `json:"is_error"`
	TerminalReason   string          `json:"terminal_reason"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Errors           []string        `json:"errors"`
	Error            json.RawMessage `json:"error"`
}

// normalizeClaudeCLIOutput accepts --output-format json and stream-json. It
// never treats partial assistant text, a zero exit status, or an absent result
// envelope as successful completion. A caller must request a JSON output mode.
func normalizeClaudeCLIOutput(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("claude protocol error: no result envelope")
	}
	var event claudeCLIEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err == nil {
		return claudeResultOutput(event)
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxClaudeJSONLLineBytes)
	var result *claudeCLIEvent
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var current claudeCLIEvent
		if err := json.Unmarshal([]byte(line), &current); err != nil {
			return "", fmt.Errorf("claude jsonl protocol error at line %d: %w", lineNumber, err)
		}
		if current.Type == "" {
			return "", fmt.Errorf("claude jsonl protocol error at line %d: missing event type", lineNumber)
		}
		if current.Type != "result" {
			continue
		}
		if _, err := claudeResultOutput(current); err != nil {
			return "", err
		}
		result = &current
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("claude jsonl protocol error: %w", err)
	}
	if result == nil {
		return "", fmt.Errorf("claude protocol error: no result envelope; assistant text is incomplete")
	}
	return claudeResultOutput(*result)
}

func claudeResultOutput(event claudeCLIEvent) (string, error) {
	if event.Type != "result" {
		return "", fmt.Errorf("claude protocol error: expected result envelope, got %q", event.Type)
	}
	if event.IsError == nil || event.Subtype == "" {
		return "", fmt.Errorf("claude protocol error: result is missing is_error or subtype")
	}
	terminal := strings.TrimSpace(event.TerminalReason)
	// Older CLI versions do not report terminal_reason. A known success subtype
	// plus is_error=false remains required; unknown nonempty terminal states fail.
	if *event.IsError || event.Subtype != "success" || (terminal != "" && terminal != "completed") || len(event.Errors) != 0 || hasClaudeError(event.Error) {
		messages := append([]string(nil), event.Errors...)
		if hasClaudeError(event.Error) {
			messages = append(messages, string(event.Error))
		}
		return "", &ClaudeResultError{Subtype: event.Subtype, TerminalReason: terminal, Messages: messages}
	}
	if len(event.StructuredOutput) != 0 && !bytes.Equal(bytes.TrimSpace(event.StructuredOutput), []byte("null")) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, event.StructuredOutput); err != nil {
			return "", fmt.Errorf("claude protocol error: invalid structured_output: %w", err)
		}
		return compact.String(), nil
	}
	if strings.TrimSpace(event.Result) != "" {
		return event.Result, nil
	}
	return "", fmt.Errorf("claude protocol error: successful result has no structured_output or text")
}

func hasClaudeError(raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)
	return len(value) != 0 && !bytes.Equal(value, []byte("null")) && !bytes.Equal(value, []byte(`""`))
}
