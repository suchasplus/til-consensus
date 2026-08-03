package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

func NormalizeTaskOutput(task consensus.Task, raw any) (consensus.TaskResult, error) {
	switch value := raw.(type) {
	case string:
		return NormalizeTaskOutputFromText(task, value)
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal raw task output: %w", err)
		}
		return normalizeTaskOutputFromJSON(task, payload)
	}
}

func StrictDecodeTaskOutput(task consensus.Task, raw any) (consensus.TaskResult, error) {
	switch value := raw.(type) {
	case string:
		payload, err := StrictJSONObjectBytes(value)
		if err != nil {
			return nil, err
		}
		return decodeTaskOutputFromJSON(task, payload)
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal raw task output: %w", err)
		}
		return decodeTaskOutputFromJSON(task, payload)
	}
}

func NormalizeTaskOutputFromText(task consensus.Task, text string) (consensus.TaskResult, error) {
	if _, ok := task.(consensus.ActionTask); ok {
		summary := strings.TrimSpace(text)
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		return consensus.ActionTaskResult{
			Output: consensus.ActionExecution{
				FullResponse: text,
				Summary:      summary,
			},
		}, nil
	}
	value, err := ParseJSONObject(text)
	if err != nil {
		return nil, err
	}
	return NormalizeTaskOutput(task, value)
}

func normalizeTaskOutputFromJSON(task consensus.Task, payload []byte) (consensus.TaskResult, error) {
	normalizedPayload, err := normalizeTaskOutputPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("normalize task output payload: %w", err)
	}
	return decodeTaskOutputFromJSON(task, normalizedPayload)
}

func normalizeTaskOutputPayload(payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data: %w", err)
	}

	coerceNumericStrings(value)

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func coerceNumericStrings(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			switch raw := item.(type) {
			case string:
				if _, ok := floatOutputKeys[key]; ok {
					if parsed, ok := parseFlexibleFloat(raw); ok {
						typed[key] = parsed
						continue
					}
				}
				if _, ok := intOutputKeys[key]; ok {
					if parsed, ok := parseFlexibleInt(raw); ok {
						typed[key] = parsed
						continue
					}
				}
			}
			coerceNumericStrings(item)
		}
	case []any:
		for _, item := range typed {
			coerceNumericStrings(item)
		}
	}
}

func parseFlexibleFloat(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if strings.HasSuffix(trimmed, "%") {
		percent := strings.TrimSpace(strings.TrimSuffix(trimmed, "%"))
		value, err := strconv.ParseFloat(percent, 64)
		if err != nil {
			return 0, false
		}
		return value / 100, true
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseFlexibleInt(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return value, true
	}
	floatValue, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	intValue := int64(floatValue)
	if float64(intValue) != floatValue {
		return 0, false
	}
	return intValue, true
}
