package consensus

import (
	"context"
	"time"
)

func (e *Engine) patchRunState(ctx context.Context, run *workflowRun, patch SessionPatch) error {
	patch.CaseManifest = ptr(run.manifest)
	patch.ClaimGraph = run.claimGraph
	patch.ChallengeTickets = run.challengeTickets
	patch.LedgerEntries = run.ledgerEntries
	patch.LedgerCursor = ptr(run.ledgerCursor)
	patch.VerificationResults = run.verificationResults
	patch.RevisionRecords = run.revisionRecords
	patch.AdjudicationRecords = run.adjudicationRecords
	patch.Observations = run.observations
	patch.Metrics = ptr(run.metrics)
	return e.deps.SessionStore.Patch(ctx, run.sessionID, patch)
}

func (e *Engine) saveAdjudicationCheckpoint(
	ctx context.Context,
	run *workflowRun,
	completedPhase SessionPhase,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	materialChange bool,
	arbiter *ArbiterReport,
	report *AdjudicationReport,
	action *ActionOutput,
) error {
	run.state.state = completedPhase
	checkpoint := &SessionCheckpoint{
		Mode:               WorkflowModeAdjudication,
		LastCompletedPhase: completedPhase,
		ActiveClaimIDs:     dedupeStrings(activeClaimIDs),
		RevisionRound:      revisionRound,
		VerifyRounds:       verifyRounds,
		FallbacksUsed:      fallbacksUsed,
		MaterialChange:     materialChange,
	}
	patch := SessionPatch{
		Phase:      ptr(completedPhase),
		Checkpoint: checkpoint,
	}
	if arbiter != nil {
		patch.ArbiterReport = arbiter
	}
	if report != nil {
		patch.Report = report
	}
	if action != nil {
		patch.Action = action
	}
	return e.patchRunState(ctx, run, patch)
}

func (e *Engine) restoreAdjudicationRun(snapshot SessionSnapshot, request StartRequest) (*workflowRun, error) {
	run := &workflowRun{
		request:             request,
		sessionID:           snapshot.SessionID,
		startedAt:           e.clock.Now(),
		state:               NewStateMachine(),
		manifest:            BuildCaseManifest(request),
		ledgerEntries:       append([]EvidenceRecord(nil), snapshot.LedgerEntries...),
		ledgerCursor:        snapshot.LedgerCursor,
		claimGraph:          append([]ClaimNode(nil), snapshot.ClaimGraph...),
		challengeTickets:    append([]ChallengeTicket(nil), snapshot.ChallengeTickets...),
		verificationResults: append([]VerificationResult(nil), snapshot.VerificationResults...),
		revisionRecords:     append([]ClaimRevisionRecord(nil), snapshot.RevisionRecords...),
		adjudicationRecords: append([]AdjudicationRecord(nil), snapshot.AdjudicationRecords...),
		observations:        append([]ObservationRecord(nil), snapshot.Observations...),
		metrics:             snapshot.Metrics,
	}
	if snapshot.CaseManifest != nil {
		run.manifest = *snapshot.CaseManifest
	}
	run.state.state = snapshot.Phase
	if snapshot.Checkpoint != nil && snapshot.Checkpoint.LastCompletedPhase != "" {
		run.state.state = snapshot.Checkpoint.LastCompletedPhase
	}
	lastResumedAt := e.clock.Now().Format(time.RFC3339Nano)
	resumeCount := snapshot.ResumeCount + 1
	if err := e.patchRunState(context.Background(), run, SessionPatch{
		ResumeCount:   &resumeCount,
		LastResumedAt: &lastResumedAt,
	}); err != nil {
		return nil, err
	}
	return run, nil
}
