package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

const maxPiJSONLLineBytes = 10 * 1024 * 1024

type piJSONLEvent struct {
	Type                  string `json:"type"`
	AssistantMessageEvent *struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
	} `json:"assistantMessageEvent"`
	Message *struct {
		Role         string `json:"role"`
		ErrorMessage string `json:"errorMessage"`
	} `json:"message"`
}

func reducePiJSONLOutput(raw string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxPiJSONLLineBytes)
	var output strings.Builder
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event piJSONLEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("pi jsonl protocol error at line %d: %w", lineNumber, err)
		}
		if event.Type == "message_end" && event.Message != nil && event.Message.Role == "assistant" && strings.TrimSpace(event.Message.ErrorMessage) != "" {
			return "", fmt.Errorf("pi assistant error: %s", strings.TrimSpace(event.Message.ErrorMessage))
		}
		if event.Type == "message_update" && event.AssistantMessageEvent != nil && event.AssistantMessageEvent.Type == "text_delta" {
			output.WriteString(event.AssistantMessageEvent.Delta)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("pi jsonl protocol error: %w", err)
	}
	if strings.TrimSpace(output.String()) == "" {
		return "", fmt.Errorf("pi returned no assistant text")
	}
	return output.String(), nil
}
