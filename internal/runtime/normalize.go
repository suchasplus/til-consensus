package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TaskOutputJSONSchema(task consensus.Task) map[string]any {
	switch task.(type) {
	case consensus.ProposalTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "claims"},
		}
	case consensus.ChallengeTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "tickets"},
		}
	case consensus.SemanticVerificationTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "results"},
		}
	case consensus.ArbiterTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "taskVerdict", "decisions"},
		}
	case consensus.ReportTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary"},
		}
	default:
		return map[string]any{
			"type":     "object",
			"required": []string{"fullResponse", "summary"},
		}
	}
}

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
	switch task.(type) {
	case consensus.ProposalTask:
		var output consensus.ProposalOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("proposal output missing summary")
		}
		return consensus.ProposalTaskResult{Output: output}, nil
	case consensus.ChallengeTask:
		var output consensus.ChallengeOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode challenge output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("challenge output missing summary")
		}
		return consensus.ChallengeTaskResult{Output: output}, nil
	case consensus.SemanticVerificationTask:
		var output consensus.SemanticVerificationOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode semantic verification output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("semantic verification output missing summary")
		}
		return consensus.SemanticVerificationTaskResult{Output: output}, nil
	case consensus.ArbiterTask:
		var output consensus.ArbiterTaskOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode arbiter output: %w", err)
		}
		if output.TaskVerdict == "" {
			return nil, fmt.Errorf("arbiter output missing taskVerdict")
		}
		return consensus.ArbiterTaskResult{Output: output}, nil
	case consensus.ReportTask:
		var output consensus.AdjudicationReport
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode report output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("report output missing summary")
		}
		return consensus.ReportTaskResult{Output: output}, nil
	case consensus.ActionTask:
		var output consensus.ActionExecution
		if err := json.Unmarshal(payload, &output); err == nil && strings.TrimSpace(output.FullResponse) != "" {
			if output.Summary == "" {
				output.Summary = output.FullResponse
			}
			return consensus.ActionTaskResult{Output: output}, nil
		}
		text := strings.TrimSpace(string(payload))
		if text == "" {
			text = "{}"
		}
		return consensus.ActionTaskResult{Output: consensus.ActionExecution{
			FullResponse: text,
			Summary:      truncateSummary(text),
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported task type")
	}
}

func truncateSummary(text string) string {
	if len(text) <= 200 {
		return text
	}
	return text[:200] + "..."
}
