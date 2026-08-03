package viewer

import (
	"slices"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/artifact"
)

func buildOverview(bundle Bundle) Overview {
	result := bundle.Result
	overview := Overview{
		RequestID:       result.RequestID,
		SessionID:       result.SessionID,
		Mode:            string(result.Mode),
		ParentRequestID: lineageParentRequestID(result.Lineage),
		ParentSessionID: lineageParentSessionID(result.Lineage),
		ParentCaseID:    lineageParentCaseID(result.Lineage),
		Trigger:         lineageTrigger(result.Lineage),
		PrimaryResult:   primaryResult(result),
		TerminalState:   string(result.TerminalState),
		Elapsed:         artifact.FormatDuration(time.Duration(result.Metrics.ElapsedMs) * time.Millisecond),
		RunDir:          displayRunDir(bundle.Files.RunDir),
		Goal:            result.TaskSpec.Goal,
		SuccessCriteria: slices.Clone(result.TaskSpec.SuccessCriteria),
		ArtifactCount:   len(extractArtifacts(bundle.Manifest, bundle.Files.RunDir)),
		WorkspacePaths:  slices.Clone(workspacePaths(result.TaskSpec.WorkspaceSnapshot)),
	}
	if result.CaseManifest != nil {
		overview.TaskType = string(result.CaseManifest.TaskType)
		overview.RiskLevel = string(result.CaseManifest.RiskLevel)
		overview.RequiredEvidence = string(result.CaseManifest.RequiredEvidenceLevel)
	}
	if snapshot := result.TaskSpec.WorkspaceSnapshot; snapshot != nil {
		overview.WorkspaceRoot = snapshot.Root
		overview.WorkspaceRev = snapshot.Revision
	}
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.Adjudication != nil {
			overview.ClaimCounts = countClaimViews(result.Adjudication.ClaimGraph)
			overview.ChallengeCount = len(result.Adjudication.ChallengeTickets)
			overview.VerificationCount = len(extractVerifications(bundle.Ledger))
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			overview.RoundCount = len(result.FreeDebate.Rounds)
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi != nil {
			overview.RoundCount = len(result.Delphi.Rounds)
		}
	}
	return overview
}

func buildObservations(result consensus.RunResult, runDir string, limit int) []ObservationView {
	out := make([]ObservationView, 0, len(result.Observations))
	for _, item := range result.Observations[:min(limit, len(result.Observations))] {
		out = append(out, ObservationView{
			ObservationID:     item.ObservationID,
			Outcome:           string(item.Outcome),
			Summary:           item.Summary,
			Reopen:            item.Reopen,
			FollowUpCaseID:    item.FollowUpCaseID,
			FollowUpRequestID: item.FollowUpRequestID,
			FollowUpArtifact:  displayCompanionPath(runDir, artifactRefPath(item.FollowUpArtifact)),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildFollowUps(result consensus.RunResult, runDir string, limit int) []FollowUpView {
	out := make([]FollowUpView, 0)
	for _, item := range result.Observations {
		if item.FollowUpCaseID == "" && item.FollowUpRequestID == "" && item.FollowUpArtifact == nil {
			continue
		}
		out = append(out, FollowUpView{
			ObservationID:     item.ObservationID,
			ParentRequestID:   result.RequestID,
			ParentSessionID:   result.SessionID,
			ParentCaseID:      lineageCaseID(result),
			Trigger:           "observe_contradiction",
			FollowUpCaseID:    item.FollowUpCaseID,
			FollowUpRequestID: item.FollowUpRequestID,
			ArtifactPath:      displayCompanionPath(runDir, artifactRefPath(item.FollowUpArtifact)),
		})
	}
	if result.Lineage != nil {
		out = append(out, FollowUpView{
			ParentRequestID:   result.Lineage.ParentRequestID,
			ParentSessionID:   result.Lineage.ParentSessionID,
			ParentCaseID:      result.Lineage.ParentCaseID,
			Trigger:           result.Lineage.Trigger,
			FollowUpRequestID: result.RequestID,
			FollowUpCaseID:    lineageCaseID(result),
		})
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}
