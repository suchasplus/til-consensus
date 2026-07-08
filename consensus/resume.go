package consensus

import (
	"context"
	"fmt"
	"strings"
)

func (e *Engine) resumeAdjudication(ctx context.Context, request StartRequest, snapshot SessionSnapshot) (_ *RunResult, err error) {
	run, err := e.restoreAdjudicationRun(snapshot, request)
	if err != nil {
		return nil, err
	}
	checkpoint := snapshot.Checkpoint
	if checkpoint == nil {
		checkpoint = &SessionCheckpoint{Mode: WorkflowModeAdjudication, LastCompletedPhase: SessionPhaseCreated}
	}
	activeClaimIDs := dedupeStrings(checkpoint.ActiveClaimIDs)
	if len(activeClaimIDs) == 0 {
		activeClaimIDs = claimIDs(run.claimGraph)
	}
	revisionRound := checkpoint.RevisionRound
	verifyRounds := checkpoint.VerifyRounds
	fallbacksUsed := checkpoint.FallbacksUsed
	arbiterReport := derefArbiter(snapshot.ArbiterReport)
	report := derefReport(snapshot.Report)
	action := cloneAction(snapshot.Action)

	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, run.sessionID, run.state, run.claimGraph, run.challengeTickets, &run.ledgerCursor, run.startedAt, err)
	}()

	switch checkpoint.LastCompletedPhase {
	case SessionPhaseCreated, SessionPhaseFrame, SessionPhaseIngest:
		if len(run.claimGraph) == 0 {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhasePropose, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			if err := e.runProposalPhase(ctx, request, run); err != nil {
				return nil, err
			}
			SortClaims(run.claimGraph)
			if err := ValidateClaimDependencies(run.claimGraph); err != nil {
				return nil, err
			}
			activeClaimIDs = claimIDs(run.claimGraph)
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhasePropose, activeClaimIDs, 0, 0, 0, true, nil, nil, nil); err != nil {
				return nil, err
			}
		}
		fallthrough
	case SessionPhasePropose:
		return e.runAdjudicationFromCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	case SessionPhaseChallenge:
		return e.afterChallengeCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	case SessionPhaseVerify:
		return e.afterVerifyCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	case SessionPhaseRevise:
		return e.afterRevisionCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	case SessionPhaseAdjudicate:
		return e.resumeAdjudicationDecision(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	case SessionPhaseReport:
		return e.resumeReportPhase(ctx, request, run, arbiterReport, report, action)
	case SessionPhaseAction:
		return e.resumeActionPhase(ctx, request, run, arbiterReport, report, action)
	case SessionPhaseObserve:
		return e.resumeObservePhase(ctx, request, run, arbiterReport, report, action)
	case SessionPhaseFinished:
		if snapshot.Result == nil {
			return nil, fmt.Errorf("session %s is finished but result is missing", snapshot.SessionID)
		}
		return snapshot.Result, nil
	default:
		return nil, fmt.Errorf("session %s phase %s cannot be resumed", snapshot.SessionID, checkpoint.LastCompletedPhase)
	}
}

func (e *Engine) runAdjudicationFromCheckpoint(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if len(run.claimGraph) == 0 {
		result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, nil, ArbiterReport{
			TaskVerdict: TaskVerdictFailed,
			Summary:     "未产生任何可裁决 claim",
		}, AdjudicationReport{
			Summary:             "未产生任何可裁决 claim",
			UnresolvedQuestions: append([]string(nil), run.manifest.UnresolvedQuestions...),
		}, nil, TerminalStateInsufficientEvidence, run.observations, run.metrics, run.startedAt, nil)
		result.Degradations = run.degradations
		if err := e.finishSession(ctx, run.sessionID, run.state, result, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		return result, nil
	}
	return e.afterProposalCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
}

func (e *Engine) afterProposalCheckpoint(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseChallenge, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if err := e.runChallengePhase(ctx, request, run, activeClaimIDs, revisionRound); err != nil {
		return nil, err
	}
	SortChallenges(run.challengeTickets)
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseChallenge, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, true, nil, nil, nil); err != nil {
		return nil, err
	}
	return e.afterChallengeCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
}

func (e *Engine) afterChallengeCheckpoint(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseVerify, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if err := e.runVerifyPhase(ctx, request, run, activeClaimIDs); err != nil {
		return nil, err
	}
	verifyRounds++
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseVerify, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, true, nil, nil, nil); err != nil {
		return nil, err
	}
	return e.afterVerifyCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
}

func (e *Engine) afterVerifyCheckpoint(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if verifyRounds >= request.LoopPolicy.MaxVerificationRounds || revisionRound >= request.LoopPolicy.MaxRevisionRounds {
		return e.resumeAdjudicationDecision(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	}
	revisionCandidates := selectClaimsForRevision(run.claimGraph, run.challengeTickets, run.verificationResults, activeClaimIDs)
	if len(revisionCandidates) == 0 {
		return e.resumeAdjudicationDecision(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseRevise, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	revisedClaimIDs, materialChange, err := e.runRevisionPhase(ctx, request, run, revisionRound+1, revisionCandidates)
	if err != nil {
		return nil, err
	}
	if len(revisedClaimIDs) == 0 {
		revisedClaimIDs = revisionCandidates
	}
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseRevise, revisedClaimIDs, revisionRound+1, verifyRounds, fallbacksUsed, materialChange, nil, nil, nil); err != nil {
		return nil, err
	}
	nextCheckpoint := &SessionCheckpoint{MaterialChange: materialChange}
	return e.afterRevisionCheckpoint(ctx, request, run, revisedClaimIDs, revisionRound+1, verifyRounds, fallbacksUsed, nextCheckpoint, arbiterReport, report, action)
}

func (e *Engine) afterRevisionCheckpoint(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if checkpoint != nil && !checkpoint.MaterialChange {
		return e.resumeAdjudicationDecision(ctx, request, run, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, checkpoint, arbiterReport, report, action)
	}
	return e.afterProposalCheckpoint(ctx, request, run, activeClaimIDs, revisionRound, 0, fallbacksUsed, checkpoint, arbiterReport, report, action)
}

func (e *Engine) resumeAdjudicationDecision(
	ctx context.Context,
	request StartRequest,
	run *workflowRun,
	activeClaimIDs []string,
	revisionRound int,
	verifyRounds int,
	fallbacksUsed int,
	checkpoint *SessionCheckpoint,
	arbiterReport ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
) (*RunResult, error) {
	if run.state.Current() != SessionPhaseAdjudicate || len(run.adjudicationRecords) == 0 || checkpoint == nil || checkpoint.LastCompletedPhase != SessionPhaseAdjudicate {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAdjudicate, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		arbiterInput := ArbiterInput{
			Request:    request,
			SessionID:  run.sessionID,
			Claims:     activeAdjudicationClaims(run.claimGraph),
			Challenges: run.challengeTickets,
			Ledger:     run.ledgerEntries,
			Findings:   run.verificationResults,
		}
		var err error
		arbiterReport, err = e.decideArbiter(ctx, request, run.startedAt, arbiterInput, &run.metrics)
		if err != nil {
			return nil, err
		}
		run.adjudicationRecords = append([]AdjudicationRecord(nil), arbiterReport.Records...)
		for idx, decision := range arbiterReport.Decisions {
			entry, appendErr := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:      decision.ClaimID,
				Kind:         EvidenceKindArbiterDecision,
				Source:       EvidenceSourceArbiter,
				ProducerID:   request.Roles.Arbiter,
				ProducerRole: "arbiter",
				Summary:      decision.Rationale,
				Metadata: map[string]any{
					"verdict":     decision.Verdict,
					"confidence":  decision.Confidence,
					"disposition": firstRecordDisposition(run.adjudicationRecords, decision.ClaimID),
				},
			})
			if appendErr != nil {
				return nil, appendErr
			}
			arbiterReport.Decisions[idx].EvidenceRefs = appendUnique(decision.EvidenceRefs, entry.EntryID)
		}
		if len(arbiterReport.Records) > 0 {
			for idx := range arbiterReport.Records {
				arbiterReport.Records[idx].EvidenceRefs = appendUnique(arbiterReport.Records[idx].EvidenceRefs, matchingDecisionEvidence(arbiterReport.Decisions, arbiterReport.Records[idx].TargetClaimID)...)
			}
		}
		run.claimGraph = ApplyDecisions(run.claimGraph, arbiterReport.Decisions)
		run.claimGraph = ApplyAdjudicationRecords(run.claimGraph, arbiterReport.Records)
		if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseAdjudicate, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, true, &arbiterReport, nil, nil); err != nil {
			return nil, err
		}
	}
	terminalState := DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, action)
	target, fallbackClaimIDs, _ := decideAdjudicationFallback(request, run.claimGraph, run.challengeTickets, arbiterReport, terminalState, fallbacksUsed)
	switch target {
	case FallbackTargetIngest:
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseIngest, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		newEvidence, err := e.runIngestSources(ctx, request, run, fmt.Sprintf("resume-fallback-%d", fallbacksUsed+1))
		if err != nil {
			return nil, err
		}
		nextClaims := fallbackClaimIDs
		if len(nextClaims) == 0 {
			nextClaims = claimIDs(run.claimGraph)
		}
		if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseIngest, nextClaims, revisionRound, 0, fallbacksUsed+1, newEvidence, &arbiterReport, nil, nil); err != nil {
			return nil, err
		}
		if !newEvidence {
			return e.resumeReportPhase(ctx, request, run, arbiterReport, report, action)
		}
		return e.afterProposalCheckpoint(ctx, request, run, nextClaims, 0, 0, fallbacksUsed+1, nil, arbiterReport, report, action)
	case FallbackTargetRevise:
		nextClaims := fallbackClaimIDs
		if len(nextClaims) == 0 {
			nextClaims = activeClaimIDs
		}
		checkpoint = &SessionCheckpoint{MaterialChange: true}
		return e.afterRevisionCheckpoint(ctx, request, run, nextClaims, request.LoopPolicy.MaxRevisionRounds+fallbacksUsed+1, 0, fallbacksUsed+1, checkpoint, arbiterReport, report, action)
	default:
		return e.resumeReportPhase(ctx, request, run, arbiterReport, report, action)
	}
}

func (e *Engine) resumeReportPhase(ctx context.Context, request StartRequest, run *workflowRun, arbiterReport ArbiterReport, report AdjudicationReport, action *ActionOutput) (*RunResult, error) {
	switch run.state.Current() {
	case SessionPhaseIngest, SessionPhaseRevise:
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAdjudicate, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	if run.state.Current() != SessionPhaseReport {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(report.Summary) == "" {
		var reportArtifact *ArtifactRef
		var err error
		report, reportArtifact, err = e.composeReport(ctx, request, run.sessionID, run.startedAt, arbiterReport, run.claimGraph, run.challengeTickets, &run.metrics)
		if err != nil {
			return nil, err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindReportGenerated,
			Source:       EvidenceSourceReporter,
			ProducerID:   request.Roles.Reporter,
			ProducerRole: "reporter",
			Summary:      report.Summary,
			Artifact:     reportArtifact,
		}); err != nil {
			return nil, err
		}
	}
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseReport, claimIDs(run.claimGraph), 0, 0, 0, true, &arbiterReport, &report, action); err != nil {
		return nil, err
	}
	if request.ActionPolicy == nil {
		return e.resumeObservePhase(ctx, request, run, arbiterReport, report, action)
	}
	return e.resumeActionPhase(ctx, request, run, arbiterReport, report, action)
}

func (e *Engine) resumeActionPhase(ctx context.Context, request StartRequest, run *workflowRun, arbiterReport ArbiterReport, report AdjudicationReport, action *ActionOutput) (*RunResult, error) {
	result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, run.adjudicationRecords, arbiterReport, report, action, DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, action), run.observations, run.metrics, run.startedAt, nil)
	result.Degradations = run.degradations
	if run.state.Current() != SessionPhaseAction {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAction, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	if action == nil && request.ActionPolicy != nil {
		action = e.gatedAction(ctx, request, run, result)
		result.Action = action
		result.TerminalState = DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, action)
		if action != nil {
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindActionGenerated,
				Source:       EvidenceSourceActor,
				ProducerID:   action.ActorID,
				ProducerRole: "actor",
				Summary:      action.Summary,
				Metadata: map[string]any{
					"status":   action.Status,
					"executed": action.Executed,
				},
			}); err != nil {
				return nil, err
			}
		}
	}
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseAction, claimIDs(run.claimGraph), 0, 0, 0, true, &arbiterReport, &report, action); err != nil {
		return nil, err
	}
	return e.resumeObservePhase(ctx, request, run, arbiterReport, report, action)
}

func (e *Engine) resumeObservePhase(ctx context.Context, request StartRequest, run *workflowRun, arbiterReport ArbiterReport, report AdjudicationReport, action *ActionOutput) (*RunResult, error) {
	result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, run.adjudicationRecords, arbiterReport, report, action, DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, action), run.observations, run.metrics, run.startedAt, nil)
	result.Degradations = run.degradations
	if run.state.Current() != SessionPhaseObserve {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseObserve, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	if len(run.observations) == 0 {
		if err := e.runObservePhase(ctx, request, run, result); err != nil {
			return nil, err
		}
	}
	result.Observations = append([]ObservationRecord(nil), run.observations...)
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseObserve, claimIDs(run.claimGraph), 0, 0, 0, true, &arbiterReport, &report, action); err != nil {
		return nil, err
	}
	if err := e.finishSession(ctx, run.sessionID, run.state, result, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
}

func derefArbiter(in *ArbiterReport) ArbiterReport {
	if in == nil {
		return ArbiterReport{}
	}
	return *in
}

func derefReport(in *AdjudicationReport) AdjudicationReport {
	if in == nil {
		return AdjudicationReport{}
	}
	return *in
}

func cloneAction(in *ActionOutput) *ActionOutput {
	if in == nil {
		return nil
	}
	clone := *in
	return &clone
}
