package consensus

import (
	"context"
	"time"
)

const (
	checkpointStepDebateInitialComplete   = "debate_initial_complete"
	checkpointStepDebateRoundComplete     = "debate_round_complete"
	checkpointStepDebateRoundsComplete    = "debate_rounds_complete"
	checkpointStepDebateSynthesisComplete = "debate_synthesis_complete"
	checkpointStepDebateVoteComplete      = "debate_vote_complete"
	checkpointStepDelphiRoundComplete     = "delphi_round_complete"
	checkpointStepDelphiRoundsComplete    = "delphi_rounds_complete"
	checkpointStepReportComplete          = "report_complete"
	checkpointStepActionComplete          = "action_complete"
	checkpointStepObserveComplete         = "observe_complete"
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
	patch.Degradations = append([]Degradation{}, run.degradations...)
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

func (e *Engine) saveWorkflowCheckpoint(ctx context.Context, request StartRequest, run *workflowRun, result *RunResult, completedPhase SessionPhase) error {
	checkpoint := &SessionCheckpoint{
		Mode:               request.Mode,
		LastCompletedPhase: completedPhase,
		Step:               finalizationCheckpointStep(completedPhase),
		MaterialChange:     true,
	}
	patch := SessionPatch{
		Phase:      ptr(completedPhase),
		Checkpoint: checkpoint,
		Report:     &result.Report,
		Action:     result.Action,
	}
	switch {
	case result.Adjudication != nil:
		checkpoint.ActiveClaimIDs = claimIDs(run.claimGraph)
		patch.ArbiterReport = &result.Adjudication.ArbiterReport
	case result.FreeDebate != nil:
		patch.DebateClaims = result.FreeDebate.Claims
		patch.DebateRounds = result.FreeDebate.Rounds
		patch.DebateVotes = result.FreeDebate.Votes
		if count := len(result.FreeDebate.Rounds); count > 0 {
			checkpoint.DebateRound = result.FreeDebate.Rounds[count-1].Round
		}
	case result.Delphi != nil:
		patch.DelphiRounds = result.Delphi.Rounds
		patch.DelphiStatements = result.Delphi.Statements
		patch.DelphiRatingDistributions = result.Delphi.RatingDistributions
		patch.DelphiConsensusLevel = ptr(result.Delphi.ConsensusLevel)
		patch.DelphiConverged = ptr(result.Delphi.Converged)
		patch.DelphiResponseQuorumMet = ptr(result.Delphi.ResponseQuorumMet)
		patch.DelphiMinResponsesMet = ptr(result.Delphi.MinResponsesMet)
		patch.DelphiStableRounds = ptr(result.Delphi.StableRounds)
		patch.DelphiRecommendation = ptr(result.Delphi.Recommendation)
		patch.DelphiDissentSummary = result.Delphi.DissentSummary
		if count := len(result.Delphi.Rounds); count > 0 {
			checkpoint.DelphiRound = result.Delphi.Rounds[count-1].Round
		}
	}
	return e.patchRunState(ctx, run, patch)
}

func finalizationCheckpointStep(phase SessionPhase) string {
	switch phase {
	case SessionPhaseReport:
		return checkpointStepReportComplete
	case SessionPhaseAction:
		return checkpointStepActionComplete
	case SessionPhaseObserve:
		return checkpointStepObserveComplete
	default:
		return ""
	}
}

func (e *Engine) saveFreeDebateCheckpoint(
	ctx context.Context,
	run *workflowRun,
	completedPhase SessionPhase,
	step string,
	debateRound int,
	claims []DebateClaim,
	rounds []DebateRoundRecord,
	votes []DebateVoteRecord,
	report *AdjudicationReport,
) error {
	run.state.state = completedPhase
	patch := SessionPatch{
		Phase:        ptr(completedPhase),
		Checkpoint:   &SessionCheckpoint{Mode: WorkflowModeFreeDebate, LastCompletedPhase: completedPhase, Step: step, DebateRound: debateRound, MaterialChange: true},
		DebateClaims: append([]DebateClaim{}, claims...),
		DebateRounds: append([]DebateRoundRecord{}, rounds...),
		DebateVotes:  append([]DebateVoteRecord{}, votes...),
	}
	if report != nil {
		patch.Report = report
	}
	return e.patchRunState(ctx, run, patch)
}

func (e *Engine) saveDelphiCheckpoint(
	ctx context.Context,
	run *workflowRun,
	completedPhase SessionPhase,
	step string,
	delphiRound int,
	stableStatementID string,
	section *DelphiResultSection,
	report *AdjudicationReport,
) error {
	run.state.state = completedPhase
	checkpoint := &SessionCheckpoint{
		Mode:                    WorkflowModeDelphi,
		LastCompletedPhase:      completedPhase,
		Step:                    step,
		DelphiRound:             delphiRound,
		DelphiStableStatementID: stableStatementID,
		MaterialChange:          true,
	}
	patch := SessionPatch{
		Phase:                       ptr(completedPhase),
		Checkpoint:                  checkpoint,
		DelphiRounds:                append([]DelphiRoundRecord{}, section.Rounds...),
		DelphiStatements:            cloneDelphiStatements(section.Statements),
		DelphiRatingDistributions:   cloneRatingDistributions(section.RatingDistributions),
		DelphiConsensusLevel:        ptr(section.ConsensusLevel),
		DelphiConverged:             ptr(section.Converged),
		DelphiResponseQuorumMet:     ptr(section.ResponseQuorumMet),
		DelphiMinResponsesMet:       ptr(section.MinResponsesMet),
		DelphiStableRounds:          ptr(section.StableRounds),
		DelphiLastStableStatementID: ptr(stableStatementID),
		DelphiRecommendation:        ptr(section.Recommendation),
		DelphiDissentSummary:        append([]string{}, section.DissentSummary...),
	}
	if report != nil {
		patch.Report = report
	}
	return e.patchRunState(ctx, run, patch)
}

func cloneRatingDistributions(in map[string][]float64) map[string][]float64 {
	if in == nil {
		return map[string][]float64{}
	}
	out := make(map[string][]float64, len(in))
	for key, values := range in {
		out[key] = append([]float64(nil), values...)
	}
	return out
}

func (e *Engine) restoreAdjudicationRun(snapshot SessionSnapshot, request StartRequest) (*workflowRun, error) {
	startedAt := e.restoredStartedAt(snapshot.StartedAt)
	run := &workflowRun{
		request:             request,
		sessionID:           snapshot.SessionID,
		startedAt:           startedAt,
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
		degradations:        append([]Degradation(nil), snapshot.Degradations...),
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

func (e *Engine) restoreModeRun(ctx context.Context, snapshot SessionSnapshot, request StartRequest) (*workflowRun, error) {
	run := &workflowRun{
		request:          request,
		sessionID:        snapshot.SessionID,
		startedAt:        e.restoredStartedAt(snapshot.StartedAt),
		state:            NewStateMachine(),
		manifest:         BuildCaseManifest(request),
		ledgerEntries:    append([]EvidenceRecord(nil), snapshot.LedgerEntries...),
		ledgerCursor:     snapshot.LedgerCursor,
		observations:     append([]ObservationRecord(nil), snapshot.Observations...),
		metrics:          snapshot.Metrics,
		degradations:     append([]Degradation(nil), snapshot.Degradations...),
		claimGraph:       append([]ClaimNode(nil), snapshot.ClaimGraph...),
		challengeTickets: append([]ChallengeTicket(nil), snapshot.ChallengeTickets...),
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
	if err := e.patchRunState(ctx, run, SessionPatch{ResumeCount: &resumeCount, LastResumedAt: &lastResumedAt}); err != nil {
		return nil, err
	}
	return run, nil
}

func (e *Engine) restoredStartedAt(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return e.clock.Now()
}
