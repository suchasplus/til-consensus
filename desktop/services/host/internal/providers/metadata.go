package providers

import (
	"encoding/json"
	"strings"
)

// OutputMetadata reads provider metadata only, never assistant answer JSON or
// reasoning content. Usage values replace the prior snapshot; they are not summed.
func OutputMetadata(raw string) (string, map[string]any, *float64) {
	var model string
	var usage map[string]any
	var cost *float64
	for _, line := range strings.Split(raw, "\n") {
		var root map[string]any
		if json.Unmarshal([]byte(line), &root) != nil {
			continue
		}
		objects := []map[string]any{root}
		for _, key := range []string{"init", "result", "message"} {
			if nested, ok := root[key].(map[string]any); ok {
				objects = append(objects, nested)
			}
		}
		for _, object := range objects {
			if value, ok := object["model"].(string); ok && value != "" {
				model = value
			}
			if value, ok := object["usage"].(map[string]any); ok {
				usage = value
			}
			if value, ok := object["total_cost_usd"].(float64); ok {
				copy := value
				cost = &copy
			}
			if models, ok := object["modelUsage"].(map[string]any); ok && len(models) == 1 {
				for key := range models {
					model = key
				}
			}
		}
	}
	return model, usage, cost
}

// ReportedReasoning accepts only Codex's thread/start response field. No request
// echo, model suffix, assistant text or organization default counts as a report.
func ReportedReasoning(raw string) string {
	var found string
	for _, line := range strings.Split(raw, "\n") {
		var packet struct {
			ID     json.RawMessage `json:"id"`
			Result struct {
				Thread          json.RawMessage `json:"thread"`
				ReasoningEffort string          `json:"reasoningEffort"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &packet) == nil && len(packet.ID) > 0 && len(packet.Result.Thread) > 0 && packet.Result.ReasoningEffort != "" {
			found = packet.Result.ReasoningEffort
		}
	}
	return found
}

// RedactJSON removes known credential values from diagnostic JSON without
// breaking JSON escaping. It never reads or persists CLI authentication files.
func RedactJSON(data []byte) []byte {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return []byte(RedactSensitive(string(data)))
	}
	var visit func(any) any
	visit = func(v any) any {
		switch x := v.(type) {
		case string:
			return RedactSensitive(x)
		case map[string]any:
			for key, item := range x {
				x[key] = visit(item)
			}
		case []any:
			for i, item := range x {
				x[i] = visit(item)
			}
		}
		return v
	}
	clean, err := json.MarshalIndent(visit(value), "", "  ")
	if err != nil {
		return []byte("{\"error\":\"无法安全序列化诊断\"}")
	}
	return clean
}
