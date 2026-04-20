package telemetry

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func BuildRunTelemetry(result consensus.RunResult, summary ComplianceSummaryFile, artifactsDir string, now time.Time) RunTelemetryFile {
	file := RunTelemetryFile{
		Version:     1,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		RequestID:   result.RequestID,
		SessionID:   result.SessionID,
		Mode:        result.Mode,
		Providers:   collectProviders(summary.Entries),
		TaskSummary: aggregateTaskSummary(summary.Entries),
		Result: RunTelemetryResult{
			PrimaryResult: primaryResult(result),
			TaskVerdict:   taskVerdict(result),
			TerminalState: result.TerminalState,
		},
		Timing: RunTelemetryTiming{
			ElapsedMs: result.Metrics.ElapsedMs,
		},
		SourceSummary: RunTelemetrySourceSummary{
			ComplianceSummaryPath: filepath.Join(artifactsDir, "strict-compliance-summary.json"),
		},
	}

	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		file.WorkflowSummary, file.VerificationSummary = buildAdjudicationSummary(result)
	case consensus.WorkflowModeFreeDebate:
		file.WorkflowSummary = buildFreeDebateSummary(result)
	case consensus.WorkflowModeDelphi:
		file.WorkflowSummary = buildDelphiSummary(result)
	}
	return file
}

func buildAdjudicationSummary(result consensus.RunResult) (WorkflowSummary, VerificationSummary) {
	out := WorkflowSummary{
		ObservationCount: len(result.Observations),
	}
	verify := VerificationSummary{}
	if result.Adjudication == nil {
		return out, verify
	}
	out.Claims = len(result.Adjudication.ClaimGraph)
	out.ChallengeCount = len(result.Adjudication.ChallengeTickets)
	for _, claim := range result.Adjudication.ClaimGraph {
		switch claim.Verdict {
		case consensus.ClaimVerdictSupported:
			out.SupportedClaims++
		case consensus.ClaimVerdictRefuted:
			out.RefutedClaims++
		case consensus.ClaimVerdictInsufficientEvidence:
			out.InsufficientClaims++
		case consensus.ClaimVerdictUndetermined:
			out.UndeterminedClaims++
		}
		switch claim.Disposition {
		case consensus.ClaimDispositionKeep:
			out.KeepClaims++
		case consensus.ClaimDispositionKeepWithCaveat:
			out.KeepWithCaveatClaims++
		case consensus.ClaimDispositionUnresolved:
			out.UnresolvedClaims++
		case consensus.ClaimDispositionReject:
			out.RejectClaims++
		}
	}
	for _, item := range result.Adjudication.VerificationResults {
		switch item.Status {
		case consensus.VerificationStatusPassed:
			verify.Passed++
		case consensus.VerificationStatusFailed:
			verify.Failed++
		default:
			verify.Inconclusive++
		}
	}
	return out, verify
}

func buildFreeDebateSummary(result consensus.RunResult) WorkflowSummary {
	out := WorkflowSummary{
		ObservationCount: len(result.Observations),
	}
	if result.FreeDebate == nil {
		return out
	}
	out.FreeDebateRoundCount = len(result.FreeDebate.Rounds)
	out.FreeDebateClaimCount = len(result.FreeDebate.Claims)
	out.FreeDebateVoteCount = len(result.FreeDebate.Votes)
	return out
}

func buildDelphiSummary(result consensus.RunResult) WorkflowSummary {
	out := WorkflowSummary{
		ObservationCount: len(result.Observations),
	}
	if result.Delphi == nil {
		return out
	}
	out.DelphiRoundCount = len(result.Delphi.Rounds)
	out.DelphiStatementCount = len(result.Delphi.Statements)
	return out
}

func collectProviders(entries []ComplianceSummaryEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(entries))
	for _, item := range entries {
		label := item.Provider
		if item.ProviderModel != "" {
			label = fmt.Sprintf("%s/%s", label, item.ProviderModel)
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	slices.Sort(out)
	return out
}

func aggregateTaskSummary(entries []ComplianceSummaryEntry) []RunTaskSummary {
	if len(entries) == 0 {
		return nil
	}
	index := map[consensus.TaskKind]*RunTaskSummary{}
	order := make([]consensus.TaskKind, 0, len(entries))
	for _, item := range entries {
		entry := index[item.TaskKind]
		if entry == nil {
			entry = &RunTaskSummary{TaskKind: item.TaskKind}
			index[item.TaskKind] = entry
			order = append(order, item.TaskKind)
		}
		entry.Total += item.Total
		entry.Strict += item.Strict
		entry.Normalized += item.Normalized
		entry.Repaired += item.Repaired
		entry.Failed += item.Failed
	}
	slices.Sort(order)
	out := make([]RunTaskSummary, 0, len(order))
	for _, kind := range order {
		out = append(out, *index[kind])
	}
	return out
}

func primaryResult(result consensus.RunResult) string {
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.TerminalState != "" && result.TerminalState != consensus.TerminalStateCompleted {
			return string(result.TerminalState)
		}
		if result.Adjudication != nil {
			return string(result.Adjudication.TaskVerdict)
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			return string(result.FreeDebate.Outcome)
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi != nil {
			if result.Delphi.Recommendation != "" {
				return result.Delphi.Recommendation
			}
			return fmt.Sprintf("consensus=%.2f", result.Delphi.ConsensusLevel)
		}
	}
	return ""
}

func taskVerdict(result consensus.RunResult) string {
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.Adjudication != nil {
			return string(result.Adjudication.TaskVerdict)
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			return string(consensus.TaskVerdictFromDebateOutcome(result.FreeDebate.Outcome))
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi != nil {
			return string(consensus.TaskVerdictFromDelphi(result.Delphi))
		}
	}
	return ""
}
