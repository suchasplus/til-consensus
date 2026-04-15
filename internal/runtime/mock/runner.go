package mock

import (
	"context"
	"errors"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func RunTask(ctx context.Context, task consensus.Task, agent config.AgentConfig, provider config.ProviderConfig) (any, error) {
	action := resolveAction(provider, task, agent.ID)
	if action.Delay.Duration > 0 {
		timer := time.NewTimer(action.Delay.Duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	switch action.Behavior {
	case "timeout":
		<-ctx.Done()
		return nil, ctx.Err()
	case "error":
		if action.Error != "" {
			return nil, errors.New(action.Error)
		}
		return nil, errors.New("mock error")
	case "malformed":
		return "not json", nil
	default:
		return buildDeterministic(task, agent), nil
	}
}

func resolveAction(provider config.ProviderConfig, task consensus.Task, agentID string) config.MockAction {
	scenario := provider.Participants[agentID]
	switch value := task.(type) {
	case consensus.RoundTask:
		switch value.Phase {
		case consensus.PhaseInitial:
			if scenario.Initial.Behavior != "" {
				return scenario.Initial
			}
		case consensus.PhaseDebate:
			if scenario.Debate.Behavior != "" {
				return scenario.Debate
			}
		default:
			if scenario.FinalVote.Behavior != "" {
				return scenario.FinalVote
			}
		}
	case consensus.ReportTask:
		if scenario.Report.Behavior != "" {
			return scenario.Report
		}
	default:
		if scenario.Action.Behavior != "" {
			return scenario.Action
		}
	}
	return config.MockAction{Behavior: fallbackBehavior(provider.Behavior), Delay: provider.Delay, Error: provider.Error}
}

func fallbackBehavior(value string) string {
	if value == "" {
		return "deterministic"
	}
	return value
}

func buildDeterministic(task consensus.Task, agent config.AgentConfig) any {
	switch value := task.(type) {
	case consensus.ReportTask:
		return consensus.FinalReport{
			Mode:                 "representative",
			TraceIncluded:        len(value.Input.Rounds) > 0,
			TraceLevel:           consensus.TraceLevelCompact,
			FinalSummary:         "Mock summary by " + agent.ID,
			RepresentativeSpeech: "Mock representative speech by " + agent.ID,
		}
	case consensus.ActionTask:
		return consensus.ActionExecution{
			FullResponse: "Action completed by " + agent.ID,
			Summary:      "Action completed by " + agent.ID,
		}
	case consensus.RoundTask:
		if value.Phase == consensus.PhaseInitial {
			return map[string]any{
				"fullResponse": "Initial analysis from " + agent.ID,
				"summary":      "Initial position from " + agent.ID,
				"taskTitle":    "Mock debate topic from " + agent.ID,
				"extractedClaims": []map[string]any{
					{
						"title":     "Proposal from " + agent.ID,
						"statement": agent.ID + " recommends a concrete next step.",
						"category":  "pro",
					},
				},
				"judgements": []any{},
			}
		}
		if value.Phase == consensus.PhaseDebate {
			judgements := make([]map[string]any, 0, len(value.ClaimCatalog))
			for _, claim := range value.ClaimCatalog {
				judgements = append(judgements, map[string]any{
					"claimId":    claim.ClaimID,
					"stance":     "agree",
					"confidence": 0.9,
					"rationale":  agent.ID + " agrees with " + claim.ClaimID,
				})
			}
			return map[string]any{
				"fullResponse":    "Debate response from " + agent.ID,
				"summary":         "Debate stance from " + agent.ID,
				"judgements":      judgements,
				"extractedClaims": []any{},
			}
		}
		judgements := make([]map[string]any, 0, len(value.ClaimCatalog))
		votes := make([]map[string]any, 0, len(value.ClaimCatalog))
		for _, claim := range value.ClaimCatalog {
			judgements = append(judgements, map[string]any{
				"claimId":    claim.ClaimID,
				"stance":     "agree",
				"confidence": 0.95,
				"rationale":  agent.ID + " accepts " + claim.ClaimID,
			})
			votes = append(votes, map[string]any{
				"claimId": claim.ClaimID,
				"vote":    "accept",
				"reason":  agent.ID + " accepts " + claim.ClaimID,
			})
		}
		return map[string]any{
			"fullResponse": "Final vote from " + agent.ID,
			"summary":      "Final vote from " + agent.ID,
			"judgements":   judgements,
			"claimVotes":   votes,
		}
	default:
		return map[string]any{}
	}
}
