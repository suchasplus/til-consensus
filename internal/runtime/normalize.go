package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type initialRoundContent struct {
	FullResponse    string                     `json:"fullResponse"`
	Summary         string                     `json:"summary"`
	TaskTitle       string                     `json:"taskTitle"`
	ExtractedClaims []consensus.ExtractedClaim `json:"extractedClaims"`
	Judgements      []consensus.ClaimJudgement `json:"judgements"`
}

type debateRoundContent struct {
	FullResponse    string                     `json:"fullResponse"`
	Summary         string                     `json:"summary"`
	ExtractedClaims []consensus.ExtractedClaim `json:"extractedClaims"`
	Judgements      []consensus.ClaimJudgement `json:"judgements"`
}

type finalVoteRoundContent struct {
	FullResponse string                     `json:"fullResponse"`
	Summary      string                     `json:"summary"`
	Judgements   []consensus.ClaimJudgement `json:"judgements"`
	ClaimVotes   []consensus.ClaimVoteInput `json:"claimVotes"`
}

func TaskOutputJSONSchema(task consensus.Task) map[string]any {
	switch value := task.(type) {
	case consensus.RoundTask:
		if value.Phase == consensus.PhaseInitial {
			return map[string]any{
				"type":     "object",
				"required": []string{"fullResponse", "summary", "taskTitle", "extractedClaims", "judgements"},
			}
		}
		if value.Phase == consensus.PhaseDebate {
			return map[string]any{
				"type":     "object",
				"required": []string{"fullResponse", "summary", "judgements"},
			}
		}
		return map[string]any{
			"type":     "object",
			"required": []string{"fullResponse", "summary", "judgements", "claimVotes"},
		}
	case consensus.ReportTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"mode", "traceIncluded", "traceLevel", "finalSummary", "representativeSpeech"},
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
		summary := text
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
	switch value := task.(type) {
	case consensus.RoundTask:
		switch value.Phase {
		case consensus.PhaseInitial:
			var content initialRoundContent
			if err := json.Unmarshal(payload, &content); err != nil {
				return nil, fmt.Errorf("decode initial round output: %w", err)
			}
			if err := validateInitialRoundContent(content); err != nil {
				return nil, err
			}
			return consensus.RoundTaskResult{
				Output: consensus.ParticipantRoundOutput{
					ParticipantID:   value.ParticipantID,
					Phase:           value.Phase,
					Round:           value.Round,
					FullResponse:    content.FullResponse,
					Summary:         content.Summary,
					TaskTitle:       content.TaskTitle,
					ExtractedClaims: content.ExtractedClaims,
					Judgements:      content.Judgements,
				},
			}, nil
		case consensus.PhaseDebate:
			var content debateRoundContent
			if err := json.Unmarshal(payload, &content); err != nil {
				return nil, fmt.Errorf("decode debate round output: %w", err)
			}
			if strings.TrimSpace(content.FullResponse) == "" || strings.TrimSpace(content.Summary) == "" || len(content.Judgements) == 0 {
				return nil, fmt.Errorf("debate round output missing required fields")
			}
			return consensus.RoundTaskResult{
				Output: consensus.ParticipantRoundOutput{
					ParticipantID:   value.ParticipantID,
					Phase:           value.Phase,
					Round:           value.Round,
					FullResponse:    content.FullResponse,
					Summary:         content.Summary,
					ExtractedClaims: content.ExtractedClaims,
					Judgements:      content.Judgements,
				},
			}, nil
		default:
			var content finalVoteRoundContent
			if err := json.Unmarshal(payload, &content); err != nil {
				return nil, fmt.Errorf("decode final vote output: %w", err)
			}
			if strings.TrimSpace(content.FullResponse) == "" || strings.TrimSpace(content.Summary) == "" || len(content.ClaimVotes) == 0 {
				return nil, fmt.Errorf("final vote output missing required fields")
			}
			return consensus.RoundTaskResult{
				Output: consensus.ParticipantRoundOutput{
					ParticipantID: value.ParticipantID,
					Phase:         value.Phase,
					Round:         value.Round,
					FullResponse:  content.FullResponse,
					Summary:       content.Summary,
					Judgements:    content.Judgements,
					ClaimVotes:    content.ClaimVotes,
				},
			}, nil
		}
	case consensus.ReportTask:
		var report consensus.FinalReport
		if err := json.Unmarshal(payload, &report); err != nil {
			return nil, fmt.Errorf("decode report output: %w", err)
		}
		if strings.TrimSpace(report.FinalSummary) == "" || strings.TrimSpace(report.RepresentativeSpeech) == "" {
			return nil, fmt.Errorf("report output missing required fields")
		}
		return consensus.ReportTaskResult{Output: report}, nil
	case consensus.ActionTask:
		var action consensus.ActionExecution
		if err := json.Unmarshal(payload, &action); err == nil && strings.TrimSpace(action.FullResponse) != "" && strings.TrimSpace(action.Summary) != "" {
			return consensus.ActionTaskResult{Output: action}, nil
		}
		text := strings.TrimSpace(string(payload))
		if text == "" {
			text = "{}"
		}
		summary := text
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		return consensus.ActionTaskResult{
			Output: consensus.ActionExecution{
				FullResponse: text,
				Summary:      summary,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported task type")
	}
}

func validateInitialRoundContent(content initialRoundContent) error {
	if strings.TrimSpace(content.FullResponse) == "" {
		return fmt.Errorf("initial round fullResponse is required")
	}
	if strings.TrimSpace(content.Summary) == "" {
		return fmt.Errorf("initial round summary is required")
	}
	if strings.TrimSpace(content.TaskTitle) == "" {
		return fmt.Errorf("initial round taskTitle is required")
	}
	return nil
}
