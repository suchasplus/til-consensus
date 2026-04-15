package mock

import (
	"context"
	"errors"
	"fmt"
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
	switch task.(type) {
	case consensus.ProposalTask:
		if scenario.Propose.Behavior != "" {
			return scenario.Propose
		}
	case consensus.ChallengeTask:
		if scenario.Challenge.Behavior != "" {
			return scenario.Challenge
		}
	case consensus.SemanticVerificationTask:
		if scenario.SemanticVerify.Behavior != "" {
			return scenario.SemanticVerify
		}
	case consensus.ArbiterTask:
		if scenario.Arbiter.Behavior != "" {
			return scenario.Arbiter
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
	case consensus.ProposalTask:
		claims := make([]map[string]any, 0, value.MaxClaims)
		limit := value.MaxClaims
		if limit <= 0 {
			limit = 1
		}
		for idx := 0; idx < limit; idx++ {
			claims = append(claims, map[string]any{
				"title":     fmt.Sprintf("Claim %d from %s", idx+1, agent.ID),
				"statement": fmt.Sprintf("%s says the task should be evaluated claim-by-claim (%d)", agent.ID, idx+1),
				"scope":     value.Scope,
			})
		}
		return map[string]any{
			"summary": "proposal by " + agent.ID,
			"claims":  claims,
		}
	case consensus.ChallengeTask:
		tickets := make([]map[string]any, 0, len(value.Claims))
		for _, claim := range value.Claims {
			tickets = append(tickets, map[string]any{
				"claimId":         claim.ClaimID,
				"statement":       "Need stronger evidence for " + claim.ClaimID,
				"kind":            "evidence-gap",
				"requestedChecks": []string{"workspace_snapshot"},
			})
		}
		return map[string]any{
			"summary": "challenge by " + agent.ID,
			"tickets": tickets,
		}
	case consensus.SemanticVerificationTask:
		return map[string]any{
			"summary": "semantic verification by " + agent.ID,
			"results": []map[string]any{
				{
					"claimId":    value.Claim.ClaimID,
					"verdict":    "supported",
					"confidence": 0.7,
					"rationale":  agent.ID + " finds the claim plausible",
				},
			},
		}
	case consensus.ArbiterTask:
		decisions := make([]map[string]any, 0, len(value.Claims))
		for _, claim := range value.Claims {
			decisions = append(decisions, map[string]any{
				"claimId":    claim.ClaimID,
				"verdict":    "supported",
				"confidence": 0.8,
				"rationale":  agent.ID + " supports " + claim.ClaimID,
			})
		}
		return map[string]any{
			"summary":     "arbiter summary by " + agent.ID,
			"taskVerdict": "supported",
			"decisions":   decisions,
		}
	case consensus.ReportTask:
		return consensus.AdjudicationReport{
			Summary:     "Report by " + agent.ID,
			Highlights:  []string{"highlight from " + agent.ID},
			NextActions: []string{"next action from " + agent.ID},
		}
	case consensus.ActionTask:
		return consensus.ActionExecution{
			FullResponse: "Action completed by " + agent.ID,
			Summary:      "Action completed by " + agent.ID,
		}
	default:
		return map[string]any{}
	}
}
