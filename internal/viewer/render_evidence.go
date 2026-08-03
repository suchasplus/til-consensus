package viewer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

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
			return firstNonEmpty(result.Delphi.Recommendation, fmt.Sprintf("consensus=%.2f", result.Delphi.ConsensusLevel))
		}
	}
	return ""
}

func countClaimViews(claims []consensus.ClaimNode) map[string]int {
	counts := map[string]int{
		string(consensus.ClaimVerdictSupported):            0,
		string(consensus.ClaimVerdictRefuted):              0,
		string(consensus.ClaimVerdictInsufficientEvidence): 0,
		string(consensus.ClaimVerdictUndetermined):         0,
	}
	for _, claim := range claims {
		counts[string(claim.Verdict)]++
	}
	return counts
}

func workspacePaths(snapshot *consensus.WorkspaceSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	return snapshot.Paths
}

func sortClaims(claims []consensus.ClaimNode) []consensus.ClaimNode {
	out := append([]consensus.ClaimNode(nil), claims...)
	slices.SortFunc(out, func(left, right consensus.ClaimNode) int {
		if left.Verdict != right.Verdict {
			return cmpVerdict(left.Verdict) - cmpVerdict(right.Verdict)
		}
		return strings.Compare(left.ClaimID, right.ClaimID)
	})
	return out
}

func cmpVerdict(verdict consensus.ClaimVerdict) int {
	switch verdict {
	case consensus.ClaimVerdictSupported:
		return 0
	case consensus.ClaimVerdictUndetermined:
		return 1
	case consensus.ClaimVerdictInsufficientEvidence:
		return 2
	case consensus.ClaimVerdictRefuted:
		return 3
	default:
		return 4
	}
}

func extractVerifications(entries []consensus.EvidenceRecord) []VerificationView {
	out := make([]VerificationView, 0)
	for _, entry := range entries {
		switch entry.Kind {
		case consensus.EvidenceKindDeterministicCheck, consensus.EvidenceKindSemanticVerification:
			item := VerificationView{
				EntryID:        entry.EntryID,
				VerificationID: entry.VerificationID,
				ClaimID:        entry.ClaimID,
				ChallengeID:    entry.ChallengeID,
				Summary:        entry.Summary,
			}
			if entry.Artifact != nil {
				item.ArtifactPath = entry.Artifact.Path
			}
			if kind, ok := entry.Metadata["kind"].(string); ok {
				item.Kind = kind
			}
			if status, ok := entry.Metadata["status"].(string); ok {
				item.Status = consensus.VerificationStatus(status)
			}
			if failureCode, ok := entry.Metadata["failureCode"].(string); ok {
				item.FailureCode = failureCode
			}
			if verdict, ok := entry.Metadata["verdictSuggestion"].(string); ok {
				item.VerdictSuggestion = consensus.ClaimVerdict(verdict)
			}
			if confidence, ok := entry.Metadata["confidence"].(float64); ok {
				item.Confidence = confidence
			}
			out = append(out, item)
		}
	}
	return out
}

func limitVerifications(items []VerificationView, limit int) []VerificationView {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func extractArtifacts(entries []consensus.ArtifactManifestEntry, runDir string) []ArtifactView {
	out := make([]ArtifactView, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		path := displayCompanionPath(runDir, entry.Artifact.Path)
		key := strings.Join([]string{string(entry.Kind), path}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ArtifactView{
			Seq:          entry.Seq,
			EntryID:      entry.EntryID,
			ClaimID:      entry.ClaimID,
			ChallengeID:  entry.ChallengeID,
			Kind:         entry.Kind,
			ProducerRole: entry.ProducerRole,
			Path:         path,
			Hash:         entry.Artifact.Hash,
			MediaType:    entry.Artifact.MediaType,
		})
	}
	return out
}

func limitArtifacts(items []ArtifactView, limit int) []ArtifactView {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func buildRisks(result consensus.RunResult, ledger []consensus.EvidenceRecord) []RiskView {
	out := make([]RiskView, 0)
	if result.Adjudication != nil {
		if result.TerminalState != "" {
			out = append(out, RiskView{Category: "terminal_state", TargetID: result.RequestID, Summary: string(result.TerminalState)})
		}
		for _, ticket := range result.Adjudication.ChallengeTickets {
			if ticket.Status == consensus.ChallengeStatusOpen {
				out = append(out, RiskView{Category: "open challenge", TargetID: ticket.ClaimID, Summary: ticket.Statement})
			}
		}
		for _, claim := range result.Adjudication.ClaimGraph {
			if claim.Verdict == consensus.ClaimVerdictInsufficientEvidence || claim.Verdict == consensus.ClaimVerdictUndetermined {
				category := "undetermined"
				if claim.Verdict == consensus.ClaimVerdictInsufficientEvidence {
					category = "insufficient evidence"
				}
				out = append(out, RiskView{Category: category, TargetID: claim.ClaimID, Summary: claim.Statement})
			}
			if claim.Disposition == consensus.ClaimDispositionKeepWithCaveat {
				out = append(out, RiskView{Category: "keep_with_caveat", TargetID: claim.ClaimID, Summary: claim.Statement})
			}
		}
		for _, item := range extractVerifications(ledger) {
			if item.Status == consensus.VerificationStatusFailed || item.Status == consensus.VerificationStatusInconclusive {
				category := "verification failed"
				if item.Status == consensus.VerificationStatusInconclusive {
					category = "verification inconclusive"
				}
				out = append(out, RiskView{Category: category, TargetID: item.ClaimID, Summary: item.Summary})
			}
		}
		for _, item := range result.Observations {
			switch item.Outcome {
			case consensus.ObservationOutcomeContradicted:
				out = append(out, RiskView{Category: "observation_contradicted", TargetID: item.ObservationID, Summary: item.Summary})
			case consensus.ObservationOutcomeFollowUp:
				out = append(out, RiskView{Category: "follow_up_recommended", TargetID: item.ObservationID, Summary: item.Summary})
			}
		}
	}
	if result.FreeDebate != nil && result.FreeDebate.Outcome != consensus.FreeDebateOutcomeConsensus {
		out = append(out, RiskView{Category: "debate_outcome", TargetID: string(result.FreeDebate.Outcome), Summary: "自由辩论未形成完整共识"})
	}
	if result.Delphi != nil {
		for idx, item := range result.Delphi.DissentSummary {
			out = append(out, RiskView{Category: "delphi_dissent", TargetID: fmt.Sprintf("dissent-%d", idx+1), Summary: item})
		}
	}
	return out
}
