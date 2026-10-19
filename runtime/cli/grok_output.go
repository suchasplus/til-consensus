package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const maxGrokJSONLLineBytes = 10 * 1024 * 1024

// GrokResultError is a failed native Grok result, even when its process exits
// successfully. Cancellation reasons unwrap to context.Canceled. A process
// interrupted before emitting its end event must be classified by the caller's
// context and exit status instead; partial text never establishes completion.
type GrokResultError struct {
	EventType  string
	StopReason string
	Message    string
}

func (e *GrokResultError) Error() string {
	if e == nil {
		return "grok result failed"
	}
	status := e.EventType
	if e.StopReason != "" {
		status += " stopReason=" + e.StopReason
	}
	message := "grok result failed: " + strings.TrimSpace(status)
	if e.Message != "" {
		message += ": " + e.Message
	}
	return message
}

func (e *GrokResultError) Unwrap() error {
	if e != nil {
		switch strings.ToLower(strings.TrimSpace(e.StopReason)) {
		case "cancelled", "canceled", "interrupted":
			return context.Canceled
		}
	}
	return nil
}

type grokCLIEvent struct {
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Data             *string         `json:"data"`
	StopReason       string          `json:"stopReason"`
	Message          string          `json:"message"`
	StructuredOutput json.RawMessage `json:"structuredOutput"`
}

// normalizeGrokCLIOutput accepts Grok's native json and streaming-json formats.
// streaming-messages-json uses another protocol and is intentionally rejected.
// Only end_turn is complete: max_tokens, refusal and unknown reasons are errors.
func normalizeGrokCLIOutput(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("grok protocol error: no result envelope")
	}
	var event grokCLIEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err == nil {
		switch event.Type {
		case "", "end", "error":
			return grokResultOutput(event, "")
		default:
			return "", fmt.Errorf("grok protocol error: no native end envelope; got %q", event.Type)
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxGrokJSONLLineBytes)
	var text strings.Builder
	var terminal *grokCLIEvent
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if terminal != nil {
			return "", fmt.Errorf("grok jsonl protocol error at line %d: event after end", lineNumber)
		}
		var current grokCLIEvent
		if err := json.Unmarshal([]byte(line), &current); err != nil {
			return "", fmt.Errorf("grok jsonl protocol error at line %d: %w", lineNumber, err)
		}
		switch current.Type {
		case "":
			return "", fmt.Errorf("grok jsonl protocol error at line %d: missing event type", lineNumber)
		case "text":
			if current.Data == nil {
				return "", fmt.Errorf("grok jsonl protocol error at line %d: text is missing data", lineNumber)
			}
			// Native data fields are deltas. Repeated chunks are significant;
			// prefix or equality heuristics would corrupt legitimate output.
			text.WriteString(*current.Data)
		case "error":
			return "", &GrokResultError{EventType: current.Type, StopReason: current.StopReason, Message: current.Message}
		case "end":
			terminal = &current
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("grok jsonl protocol error: %w", err)
	}
	if terminal == nil {
		return "", fmt.Errorf("grok protocol error: no end envelope; assistant text is incomplete")
	}
	return grokResultOutput(*terminal, text.String())
}

func grokResultOutput(event grokCLIEvent, streamedText string) (string, error) {
	if event.Type == "error" {
		return "", &GrokResultError{EventType: event.Type, StopReason: event.StopReason, Message: event.Message}
	}
	if event.StopReason == "" {
		return "", fmt.Errorf("grok protocol error: result is missing stopReason")
	}
	if event.StopReason != "end_turn" {
		return "", &GrokResultError{EventType: event.Type, StopReason: event.StopReason, Message: event.Message}
	}
	structured := bytes.TrimSpace(event.StructuredOutput)
	if len(structured) != 0 && !bytes.Equal(structured, []byte("null")) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, structured); err != nil {
			return "", fmt.Errorf("grok protocol error: invalid structuredOutput: %w", err)
		}
		return compact.String(), nil
	}
	// A terminal text snapshot is authoritative. Never append it to deltas:
	// doing so would duplicate the final answer if a CLI emits both surfaces.
	if strings.TrimSpace(event.Text) != "" {
		return event.Text, nil
	}
	if strings.TrimSpace(streamedText) != "" {
		return streamedText, nil
	}
	return "", fmt.Errorf("grok protocol error: successful result has no structuredOutput or text")
}
