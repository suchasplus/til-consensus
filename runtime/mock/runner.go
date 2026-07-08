package mock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
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
	case consensus.InitialProposalTask:
		if scenario.Propose.Behavior != "" {
			return scenario.Propose
		}
	case consensus.ChallengeTask:
		if scenario.Challenge.Behavior != "" {
			return scenario.Challenge
		}
	case consensus.ReviseTask:
		if scenario.Propose.Behavior != "" {
			return scenario.Propose
		}
	case consensus.DebateRoundTask:
		if scenario.Challenge.Behavior != "" {
			return scenario.Challenge
		}
	case consensus.FinalVoteTask:
		if scenario.Challenge.Behavior != "" {
			return scenario.Challenge
		}
	case consensus.SemanticVerificationTask:
		if scenario.SemanticVerify.Behavior != "" {
			return scenario.SemanticVerify
		}
	case consensus.DelphiQuestionnaireTask:
		if scenario.Propose.Behavior != "" {
			return scenario.Propose
		}
	case consensus.DelphiRevisionTask:
		if scenario.Challenge.Behavior != "" {
			return scenario.Challenge
		}
	case consensus.DelphiFacilitatorSummaryTask:
		if scenario.Report.Behavior != "" {
			return scenario.Report
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
				"title":      fmt.Sprintf("Claim %d from %s", idx+1, agent.ID),
				"statement":  fmt.Sprintf("%s says the task should be evaluated claim-by-claim (%d)", agent.ID, idx+1),
				"scope":      value.Scope,
				"claimType":  "inference",
				"confidence": 0.6,
			})
		}
		return map[string]any{
			"summary": "proposal by " + agent.ID,
			"claims":  claims,
		}
	case consensus.InitialProposalTask:
		claims := make([]map[string]any, 0, value.MaxClaims)
		limit := value.MaxClaims
		if limit <= 0 {
			limit = 1
		}
		for idx := 0; idx < limit; idx++ {
			claims = append(claims, map[string]any{
				"title":     fmt.Sprintf("Initial claim %d from %s", idx+1, agent.ID),
				"statement": fmt.Sprintf("%s initial position %d", agent.ID, idx+1),
			})
		}
		return map[string]any{
			"summary": "initial proposal by " + agent.ID,
			"claims":  claims,
		}
	case consensus.ChallengeTask:
		requestedChecks := []string{"semantic"}
		suggestedMethod := "补充更具体的一手材料或比较依据"
		if consensus.DetectTaskType(value.TaskSpec) == consensus.CaseTaskTypeCoding {
			requestedChecks = []string{"workspace_snapshot"}
			suggestedMethod = "补充可复现材料"
		}
		tickets := make([]map[string]any, 0, len(value.Claims))
		for _, claim := range value.Claims {
			tickets = append(tickets, map[string]any{
				"claimId":                      claim.ClaimID,
				"statement":                    "Need stronger evidence for " + claim.ClaimID,
				"kind":                         "evidence-gap",
				"attackType":                   "insufficient_evidence",
				"severity":                     "medium",
				"requestedChecks":              requestedChecks,
				"suggestedFalsificationMethod": suggestedMethod,
			})
		}
		return map[string]any{
			"summary": "challenge by " + agent.ID,
			"tickets": tickets,
		}
	case consensus.ReviseTask:
		revisions := make([]map[string]any, 0, len(value.Claims))
		for _, claim := range value.Claims {
			revisions = append(revisions, map[string]any{
				"targetClaimId":   claim.ClaimID,
				"action":          "downgrade_confidence",
				"confidenceDelta": -0.1,
				"caveats":         []string{"需要更多一手证据"},
				"reason":          "mock revision by " + agent.ID,
			})
		}
		return map[string]any{
			"summary":   "revision by " + agent.ID,
			"revisions": revisions,
		}
	case consensus.DebateRoundTask:
		judgements := make([]map[string]any, 0, len(value.PeerClaims))
		for _, claim := range value.PeerClaims {
			judgements = append(judgements, map[string]any{
				"claimId":   claim.ClaimID,
				"judgement": "agree",
				"rationale": agent.ID + " agrees with " + claim.ClaimID,
			})
		}
		return map[string]any{
			"summary":    "debate round by " + agent.ID,
			"judgements": judgements,
		}
	case consensus.SemanticDedupTask:
		return map[string]any{
			"summary": "semantic dedup by " + agent.ID,
			"merges":  []map[string]any{},
		}
	case consensus.SynthesisTask:
		statement := "Synthesized recommendation by " + agent.ID + " covering " + fmt.Sprintf("%d", len(value.Claims)) + " claims."
		if value.Draft != nil {
			statement = value.Draft.Statement + " (integrated " + fmt.Sprintf("%d", len(value.Amendments)) + " amendments)"
		}
		return map[string]any{
			"summary": "synthesis by " + agent.ID,
			"claim": map[string]any{
				"title":     "Panel synthesis",
				"statement": statement,
			},
		}
	case consensus.FinalVoteTask:
		votes := make([]map[string]any, 0, len(value.Claims))
		for idx, claim := range value.Claims {
			// Deterministic but calibrated scores: uniform ballots are
			// rejected by the vote-calibration contract, and accept labels
			// must stay coherent (confidence >= 0.5).
			confidence := 0.95 - 0.03*float64(idx)
			if confidence < 0.55 {
				confidence = 0.55
			}
			votes = append(votes, map[string]any{
				"claimId":    claim.ClaimID,
				"vote":       "accept",
				"confidence": confidence,
				"rationale":  agent.ID + " accepts " + claim.ClaimID,
			})
		}
		return map[string]any{
			"summary": "final vote by " + agent.ID,
			"votes":   votes,
		}
	case consensus.SemanticVerificationTask:
		return map[string]any{
			"summary": "semantic verification by " + agent.ID,
			"results": []map[string]any{
				{
					"claimId":    value.Claim.ClaimID,
					"verdict":    "supported",
					"confidence": 0.7,
					"rationale":  "supported_core: " + agent.ID + " finds the current claim core plausible. | missing_or_conflict: none beyond already stated caveats. | verdict_reason: supported is stronger than insufficient_evidence because the mock record is intentionally affirmative.",
				},
			},
		}
	case consensus.ArbiterTask:
		openChallenges := make(map[string]bool, len(value.Challenges))
		for _, ticket := range value.Challenges {
			if ticket.Status == consensus.ChallengeStatusOpen {
				openChallenges[ticket.ClaimID] = true
			}
		}
		nonPassingFindings := make(map[string]bool, len(value.Findings))
		for _, finding := range value.Findings {
			if finding.Status != consensus.VerificationStatusPassed {
				nonPassingFindings[finding.ClaimID] = true
			}
		}
		decisions := make([]map[string]any, 0, len(value.Claims))
		records := make([]map[string]any, 0, len(value.Claims))
		claimVerdicts := make([]consensus.ClaimNode, 0, len(value.Claims))
		for _, claim := range value.Claims {
			verdict := consensus.ClaimVerdictSupported
			disposition := consensus.ClaimDispositionKeep
			confidence := 0.8
			rationale := agent.ID + " supports " + claim.ClaimID
			actionability := "ready"
			if openChallenges[claim.ClaimID] || nonPassingFindings[claim.ClaimID] {
				verdict = consensus.ClaimVerdictUndetermined
				disposition = consensus.ClaimDispositionUnresolved
				confidence = 0.45
				rationale = agent.ID + " keeps " + claim.ClaimID + " unresolved until stronger evidence lands"
				actionability = "blocked"
			}
			decisions = append(decisions, map[string]any{
				"claimId":    claim.ClaimID,
				"verdict":    verdict,
				"confidence": confidence,
				"rationale":  rationale,
			})
			records = append(records, map[string]any{
				"targetClaimId":   claim.ClaimID,
				"disposition":     disposition,
				"rationale":       rationale,
				"finalConfidence": confidence,
				"actionability":   actionability,
			})
			claimVerdicts = append(claimVerdicts, consensus.ClaimNode{Verdict: verdict})
		}
		return map[string]any{
			"summary":     "arbiter summary by " + agent.ID,
			"taskVerdict": consensus.DetermineTaskVerdict(claimVerdicts),
			"decisions":   decisions,
			"records":     records,
		}
	case consensus.ReportTask:
		return consensus.AdjudicationReport{
			Summary:             "Report by " + agent.ID,
			Highlights:          []string{"highlight from " + agent.ID},
			RetainedClaims:      []string{"retained by " + agent.ID},
			DowngradedClaims:    []string{"downgraded by " + agent.ID},
			UnresolvedQuestions: []string{"unresolved by " + agent.ID},
			NextActions:         []string{"next action from " + agent.ID},
		}
	case consensus.DelphiQuestionnaireTask:
		return map[string]any{
			"summary": "delphi questionnaire by " + agent.ID,
			"responses": []map[string]any{
				{
					"statement": "Option from " + agent.ID,
					"rating":    4.0,
					"rationale": "favored by " + agent.ID,
				},
			},
		}
	case consensus.DelphiRevisionTask:
		statementID := ""
		if len(value.StatementSummaries) > 0 {
			statementID = value.StatementSummaries[0].StatementID
		}
		return map[string]any{
			"summary": "delphi revision by " + agent.ID,
			"responses": []map[string]any{
				{
					"statementId": statementID,
					"rating":      4.0,
					"rationale":   "revised by " + agent.ID,
				},
			},
		}
	case consensus.DelphiFacilitatorSummaryTask:
		recommendation := "Option from facilitator"
		if len(value.StatementSummaries) > 0 && value.StatementSummaries[0].Statement != "" {
			recommendation = value.StatementSummaries[0].Statement
		}
		return map[string]any{
			"summary":        "facilitator summary by " + agent.ID,
			"recommendation": recommendation,
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
