package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

// exec --json reports complete items, not the app-server text delta protocol.
func normalizeCodexExecOutput(raw string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	completed := false
	text := ""
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event struct {
			Type    string          `json:"type"`
			Error   json.RawMessage `json:"error"`
			Message string          `json:"message"`
			Item    struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", fmt.Errorf("codex JSONL protocol: %w", err)
		}
		switch event.Type {
		case "turn.failed", "error":
			return "", fmt.Errorf("codex task failed: %s %s", event.Message, event.Error)
		case "turn.completed":
			completed = true
		case "item.completed":
			if event.Item.Type == "agent_message" {
				text = event.Item.Text
			}
		case "turn.started":
			if completed {
				return "", fmt.Errorf("codex one-shot returned multiple turns")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if !completed || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("codex task has no completed turn and final agent message")
	}
	return text, nil
}
