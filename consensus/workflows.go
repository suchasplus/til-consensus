package consensus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"
)

type workflowRun struct {
	request             StartRequest
	sessionID           string
	startedAt           time.Time
	state               *StateMachine
	manifest            CaseManifest
	ledgerEntries       []EvidenceRecord
	ledgerCursor        int
	claimGraph          []ClaimNode
	challengeTickets    []ChallengeTicket
	verificationResults []VerificationResult
	revisionRecords     []ClaimRevisionRecord
	adjudicationRecords []AdjudicationRecord
	observations        []ObservationRecord
	metrics             Metrics
	degradations        []Degradation
}

// addDegradation collects a result-level record of a non-fatal failure. It is
// only called from the sequential batch-result loops after executeTaskBatch
// returns, so no locking is needed.
func (run *workflowRun) addDegradation(d Degradation) {
	const maxReasonLen = 300
	if len(d.Reason) > maxReasonLen {
		d.Reason = d.Reason[:maxReasonLen] + "..."
	}
	run.degradations = append(run.degradations, d)
}

func participantAbsentDegradation(result taskBatchResult, phase string, round int, impact string) Degradation {
	reason := ""
	if result.Err != nil {
		reason = result.Err.Error()
	}
	return Degradation{
		Kind:    DegradationParticipantAbsent,
		Phase:   phase,
		Round:   round,
		AgentID: result.Task.Meta().AgentID,
		Reason:  reason,
		Impact:  impact,
	}
}

func (e *Engine) beginWorkflow(ctx context.Context, request StartRequest) (*workflowRun, error) {
	run := &workflowRun{
		request:          request,
		sessionID:        e.ids.NewSessionID(),
		startedAt:        e.clock.Now(),
		state:            NewStateMachine(),
		manifest:         BuildCaseManifest(request),
		ledgerEntries:    make([]EvidenceRecord, 0, 32),
		claimGraph:       make([]ClaimNode, 0),
		challengeTickets: make([]ChallengeTicket, 0),
	}
	if err := e.deps.SessionStore.Save(ctx, SessionSnapshot{
		SessionID:     run.sessionID,
		RequestID:     request.RequestID,
		Request:       &request,
		Phase:         run.state.Current(),
		Checkpoint:    &SessionCheckpoint{Mode: request.Mode, LastCompletedPhase: SessionPhaseCreated},
		CaseManifest:  &run.manifest,
		StartedAt:     run.startedAt.Format(time.RFC3339Nano),
		ClaimGraph:    run.claimGraph,
		LedgerEntries: run.ledgerEntries,
		LedgerCursor:  run.ledgerCursor,
	}); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"goal": request.TaskSpec.Goal,
		"mode": request.Mode,
	}
	if len(request.Roles.Proposers) > 0 {
		payload["proposers"] = request.Roles.Proposers
	}
	if len(request.Roles.Challengers) > 0 {
		payload["challengers"] = request.Roles.Challengers
	}
	if len(request.Roles.Participants) > 0 {
		payload["participants"] = request.Roles.Participants
	}
	if request.Roles.Arbiter != "" {
		payload["arbiter"] = request.Roles.Arbiter
	}
	if request.Roles.Facilitator != "" {
		payload["facilitator"] = request.Roles.Facilitator
	}
	if err := e.emit(ctx, request, run.sessionID, RunEventSessionStarted, run.state.Current(), payload); err != nil {
		return nil, err
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseFrame, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:                  EvidenceKindCaseFramed,
		Source:                EvidenceSourceCoordinator,
		ProducerRole:          "coordinator",
		Summary:               run.manifest.CanonicalProblemStatement,
		SourceType:            "case_manifest",
		ProvenanceQuality:     ProvenanceQualityHigh,
		FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
		Metadata: map[string]any{
			"caseId":                run.manifest.CaseID,
			"taskType":              run.manifest.TaskType,
			"riskLevel":             run.manifest.RiskLevel,
			"requiredEvidenceLevel": run.manifest.RequiredEvidenceLevel,
			"outOfScope":            run.manifest.OutOfScope,
			"unresolvedQuestions":   run.manifest.UnresolvedQuestions,
		},
	}); err != nil {
		return nil, err
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseIngest, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindTaskIngested,
		Source:       EvidenceSourceCoordinator,
		ProducerRole: "coordinator",
		Summary:      "task ingested",
		Metadata: map[string]any{
			"goal":              request.TaskSpec.Goal,
			"mode":              request.Mode,
			"successCriteria":   request.TaskSpec.SuccessCriteria,
			"allowedTools":      request.TaskSpec.AllowedTools,
			"workspaceSnapshot": request.TaskSpec.WorkspaceSnapshot,
		},
	}); err != nil {
		return nil, err
	}
	for _, material := range request.TaskSpec.Materials {
		summary := firstNonEmpty(material.Title, material.ID, material.Path, "source material")
		excerpt := strings.TrimSpace(material.Content)
		if len(excerpt) > 240 {
			excerpt = excerpt[:240] + "..."
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindSourceMaterial,
			Source:                EvidenceSourceCoordinator,
			ProducerRole:          "coordinator",
			SourceType:            firstNonEmpty(material.Kind, "material"),
			SourceLocator:         firstNonEmpty(material.Path, material.ID),
			Summary:               summary,
			ContentExcerpt:        excerpt,
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveSecondHand,
			Metadata: map[string]any{
				"id":    material.ID,
				"title": material.Title,
				"hash":  material.Hash,
			},
		}); err != nil {
			return nil, err
		}
	}
	if _, err := e.runIngestSources(ctx, request, run, "initial"); err != nil {
		return nil, err
	}
	if err := e.patchRunState(ctx, run, SessionPatch{
		Phase: ptr(SessionPhaseIngest),
		Checkpoint: &SessionCheckpoint{
			Mode:               request.Mode,
			LastCompletedPhase: SessionPhaseIngest,
		},
	}); err != nil {
		return nil, err
	}
	return run, nil
}

func (e *Engine) startAdjudication(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
	run, err := e.beginWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, run.sessionID, run.state, run.claimGraph, run.challengeTickets, &run.ledgerCursor, run.startedAt, err)
	}()

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
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhasePropose, claimIDs(run.claimGraph), 0, 0, 0, true, nil, nil, nil); err != nil {
		return nil, err
	}
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
	activeClaimIDs := claimIDs(run.claimGraph)
	var (
		arbiterReport     ArbiterReport
		terminalState     WorkflowTerminalState
		fallbacksUsed     int
		lastRevisionRound int
		lastVerifyRounds  int
	)
adjudicationCycle:
	for {
		verifyRounds := 0
		for revisionRound := 0; ; revisionRound++ {
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
			lastRevisionRound = revisionRound
			lastVerifyRounds = verifyRounds
			if verifyRounds >= request.LoopPolicy.MaxVerificationRounds {
				break
			}

			if revisionRound >= request.LoopPolicy.MaxRevisionRounds {
				break
			}
			revisionCandidates := selectClaimsForRevision(run.claimGraph, run.challengeTickets, run.verificationResults, activeClaimIDs)
			if len(revisionCandidates) == 0 {
				break
			}
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseRevise, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			revisedClaimIDs, materialChange, err := e.runRevisionPhase(ctx, request, run, revisionRound+1, revisionCandidates)
			if err != nil {
				return nil, err
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseRevise, revisedClaimIDs, revisionRound+1, verifyRounds, fallbacksUsed, materialChange, nil, nil, nil); err != nil {
				return nil, err
			}
			lastRevisionRound = revisionRound + 1
			lastVerifyRounds = verifyRounds
			if len(revisedClaimIDs) == 0 || !materialChange {
				break
			}
			activeClaimIDs = revisedClaimIDs
		}

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
		arbiterReport, err = e.decideArbiter(ctx, request, run.startedAt, arbiterInput, &run.metrics)
		if err != nil {
			return nil, err
		}
		run.adjudicationRecords = append([]AdjudicationRecord(nil), arbiterReport.Records...)
		for idx, decision := range arbiterReport.Decisions {
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:      decision.ClaimID,
				Kind:         EvidenceKindArbiterDecision,
				Source:       EvidenceSourceArbiter,
				ProducerID:   request.Roles.Arbiter,
				ProducerRole: "arbiter",
				Summary:      decision.Rationale,
				Metadata: mergeAnyMaps(maps.Clone(decision.Metadata), map[string]any{
					"verdict":     decision.Verdict,
					"confidence":  decision.Confidence,
					"disposition": firstRecordDisposition(run.adjudicationRecords, decision.ClaimID),
				}),
			})
			if err != nil {
				return nil, err
			}
			arbiterReport.Decisions[idx].EvidenceRefs = appendUnique(decision.EvidenceRefs, entry.EntryID)
		}
		if len(arbiterReport.Records) > 0 {
			for idx := range arbiterReport.Records {
				arbiterReport.Records[idx].EvidenceRefs = appendUnique(arbiterReport.Records[idx].EvidenceRefs, matchingDecisionEvidence(arbiterReport.Decisions, arbiterReport.Records[idx].TargetClaimID)...)
			}
		}
		for _, record := range arbiterReport.Records {
			decision := findArbiterDecision(arbiterReport.Decisions, record.TargetClaimID)
			if err := e.emit(ctx, request, run.sessionID, RunEventClaimAdjudicated, SessionPhaseAdjudicate, map[string]any{
				"claimId":           record.TargetClaimID,
				"disposition":       record.Disposition,
				"finalConfidence":   record.FinalConfidence,
				"actionability":     record.Actionability,
				"blockingRiskCount": len(record.BlockingRisks),
				"verdict":           decision.Verdict,
				"confidence":        decision.Confidence,
				"reason":            firstNonEmpty(record.Rationale, decision.Rationale),
				"metadata":          mergeAnyMaps(maps.Clone(record.Metadata), maps.Clone(decision.Metadata)),
			}); err != nil {
				return nil, err
			}
		}
		run.claimGraph = ApplyDecisions(run.claimGraph, arbiterReport.Decisions)
		run.claimGraph = ApplyAdjudicationRecords(run.claimGraph, arbiterReport.Records)
		if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseAdjudicate, activeClaimIDs, lastRevisionRound, lastVerifyRounds, fallbacksUsed, true, &arbiterReport, nil, nil); err != nil {
			return nil, err
		}
		terminalState = DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, nil)

		target, fallbackClaimIDs, fallbackReason := decideAdjudicationFallback(request, run.claimGraph, run.challengeTickets, arbiterReport, terminalState, fallbacksUsed)
		if target == FallbackTargetStop {
			break adjudicationCycle
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindAdjudicationFallback,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      fallbackReason,
			Metadata: map[string]any{
				"target":        target,
				"terminalState": terminalState,
				"claims":        fallbackClaimIDs,
				"attempt":       fallbacksUsed + 1,
			},
		}); err != nil {
			return nil, err
		}
		fallbacksUsed++
		switch target {
		case FallbackTargetIngest:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseIngest, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			newEvidence, err := e.runIngestSources(ctx, request, run, fmt.Sprintf("fallback-%d", fallbacksUsed))
			if err != nil {
				return nil, err
			}
			nextClaims := fallbackClaimIDs
			if len(nextClaims) == 0 {
				nextClaims = claimIDs(run.claimGraph)
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseIngest, nextClaims, lastRevisionRound, 0, fallbacksUsed, newEvidence, &arbiterReport, nil, nil); err != nil {
				return nil, err
			}
			if !newEvidence {
				break adjudicationCycle
			}
			if len(fallbackClaimIDs) > 0 {
				activeClaimIDs = fallbackClaimIDs
			} else {
				activeClaimIDs = claimIDs(run.claimGraph)
			}
		case FallbackTargetRevise:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseRevise, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			revisedClaimIDs, materialChange, err := e.runRevisionPhase(ctx, request, run, request.LoopPolicy.MaxRevisionRounds+fallbacksUsed, fallbackClaimIDs)
			if err != nil {
				return nil, err
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseRevise, revisedClaimIDs, request.LoopPolicy.MaxRevisionRounds+fallbacksUsed, 0, fallbacksUsed, materialChange, &arbiterReport, nil, nil); err != nil {
				return nil, err
			}
			if !materialChange || len(revisedClaimIDs) == 0 {
				break adjudicationCycle
			}
			activeClaimIDs = revisedClaimIDs
		default:
			break adjudicationCycle
		}
	}

	if current := run.state.Current(); current != SessionPhaseAdjudicate {
		switch current {
		case SessionPhaseIngest, SessionPhaseRevise:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAdjudicate, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	report, reportArtifact, err := e.composeReport(ctx, request, run.sessionID, run.startedAt, arbiterReport, run.claimGraph, run.challengeTickets, &run.metrics)
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
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseReport, claimIDs(run.claimGraph), 0, 0, fallbacksUsed, true, &arbiterReport, &report, nil); err != nil {
		return nil, err
	}

	result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, run.adjudicationRecords, arbiterReport, report, nil, terminalState, nil, run.metrics, run.startedAt, nil)
	result.Degradations = run.degradations
	if request.ActionPolicy != nil {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAction, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		actionOutput := e.gatedAction(ctx, request, run, result)
		result.Action = actionOutput
		result.TerminalState = DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, actionOutput)
		if actionOutput != nil {
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindActionGenerated,
				Source:       EvidenceSourceActor,
				ProducerID:   actionOutput.ActorID,
				ProducerRole: "actor",
				Summary:      actionOutput.Summary,
				Metadata: map[string]any{
					"status":   actionOutput.Status,
					"executed": actionOutput.Executed,
				},
			}); err != nil {
				return nil, err
			}
		}
		if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseAction, claimIDs(run.claimGraph), 0, 0, fallbacksUsed, true, &arbiterReport, &report, actionOutput); err != nil {
			return nil, err
		}
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseObserve, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if err := e.runObservePhase(ctx, request, run, result); err != nil {
		return nil, err
	}
	result.Observations = append([]ObservationRecord(nil), run.observations...)
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseObserve, claimIDs(run.claimGraph), 0, 0, fallbacksUsed, true, &arbiterReport, &report, result.Action); err != nil {
		return nil, err
	}
	if result.TerminalState == "" {
		result.TerminalState = terminalState
	}
	if err := e.finishSession(ctx, run.sessionID, run.state, result, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) runProposalPhase(ctx context.Context, request StartRequest, run *workflowRun) error {
proposalPhase:
	for pass := 0; pass < request.ProposalPolicy.MaxPasses; pass++ {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			break proposalPhase
		}
		tasks := make([]Task, 0, len(request.Roles.Proposers))
		for _, proposerID := range request.Roles.Proposers {
			tasks = append(tasks, ProposalTask{
				TaskMeta: TaskMeta{
					SessionID: run.sessionID,
					RequestID: request.RequestID,
					AgentID:   proposerID,
					Role:      "proposer",
					Metadata:  map[string]any{"pass": pass, "blind": true},
				},
				TaskSpec:       request.TaskSpec,
				Scope:          fmt.Sprintf("proposal pass %d", pass+1),
				MaxClaims:      request.ProposalPolicy.MaxClaimsPerWorker,
				DedupeStrategy: request.ProposalPolicy.DedupeStrategy,
			})
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				run.addDegradation(participantAbsentDegradation(result, "proposal", pass+1, "该 proposer 的提案缺席，claim 图基于其余 proposer 的输出"))
				continue
			}
			proposerID := result.Task.Meta().AgentID
			output, ok := result.Awaited.Output.(ProposalTaskResult)
			if !ok {
				return fmt.Errorf("proposal task returned unexpected result type")
			}
			workerEntry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:                  EvidenceKindWorkerOutput,
				Source:                EvidenceSourceWorker,
				ProducerID:            proposerID,
				ProducerRole:          "proposer",
				SourceType:            "proposal",
				Summary:               output.Output.Summary,
				Artifact:              result.Awaited.Artifact,
				ProvenanceQuality:     ProvenanceQualityMedium,
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata: map[string]any{
					"taskId":   result.Receipt.TaskID,
					"taskKind": TaskKindPropose,
					"pass":     pass,
					"blind":    true,
				},
			})
			if err != nil {
				return err
			}
			for _, draft := range output.Output.Claims {
				if strings.TrimSpace(draft.Statement) == "" {
					continue
				}
				if draft.ClaimType == "" {
					draft.ClaimType = inferClaimType(draft.Statement)
				}
				var created bool
				run.claimGraph, _, created = UpsertClaim(run.claimGraph, draft, proposerID, workerEntry.EntryID, e.ids)
				if !created {
					continue
				}
				run.metrics.ClaimsProposed++
				claim := run.claimGraph[len(run.claimGraph)-1]
				entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
					ClaimID:               claim.ClaimID,
					Kind:                  EvidenceKindClaimProposed,
					Source:                EvidenceSourceCoordinator,
					ProducerID:            proposerID,
					ProducerRole:          "proposer",
					SourceType:            "claim",
					Summary:               claim.Statement,
					ProvenanceQuality:     ProvenanceQualityMedium,
					FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
					Metadata: map[string]any{
						"title":                 claim.Title,
						"claimType":             claim.ClaimType,
						"scope":                 claim.Scope,
						"dependencies":          claim.Dependencies,
						"parentClaimIds":        claim.ParentClaimIDs,
						"boundaryConditions":    claim.BoundaryConditions,
						"applicability":         claim.Applicability,
						"sourceProposalAgentId": proposerID,
					},
				})
				if err != nil {
					return err
				}
				run.claimGraph = AttachEvidenceToClaim(run.claimGraph, claim.ClaimID, entry.EntryID)
			}
		}
		if run.metrics.GlobalDeadlineHit {
			break proposalPhase
		}
	}
	return nil
}

func (e *Engine) runChallengePhase(ctx context.Context, request StartRequest, run *workflowRun, claimIDs []string, round int) error {
	claims := filterClaimsByID(run.claimGraph, claimIDs)
	tasks := make([]Task, 0, len(request.Roles.Challengers))
	for _, challengerID := range request.Roles.Challengers {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			return nil
		}
		tasks = append(tasks, ChallengeTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   challengerID,
				Role:      "challenger",
				Metadata:  map[string]any{"round": round},
			},
			TaskSpec: request.TaskSpec,
			Claims:   claims,
		})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "challenge", round, "该 challenger 的质询缺席，claim 未经过其攻击检验"))
			continue
		}
		challengerID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(ChallengeTaskResult)
		if !ok {
			return fmt.Errorf("challenge task returned unexpected result type")
		}
		workerEntry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindWorkerOutput,
			Source:                EvidenceSourceWorker,
			ProducerID:            challengerID,
			ProducerRole:          "challenger",
			SourceType:            "attack",
			Summary:               output.Output.Summary,
			Artifact:              result.Awaited.Artifact,
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata: map[string]any{
				"taskId":   result.Receipt.TaskID,
				"taskKind": TaskKindChallenge,
				"round":    round,
			},
		})
		if err != nil {
			return err
		}
		for _, draft := range output.Output.Tickets {
			claim, ok := ResolveClaimRef(run.claimGraph, draft.ClaimID, draft.Statement)
			if !ok {
				continue
			}
			var created bool
			run.challengeTickets, _, created = UpsertChallenge(run.challengeTickets, draft, claim.ClaimID, challengerID, workerEntry.EntryID, e.ids)
			if !created {
				continue
			}
			run.metrics.ChallengesOpened++
			ticket := run.challengeTickets[len(run.challengeTickets)-1]
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:               claim.ClaimID,
				ChallengeID:           ticket.TicketID,
				Kind:                  EvidenceKindChallengeOpened,
				Source:                EvidenceSourceCoordinator,
				ProducerID:            challengerID,
				ProducerRole:          "challenger",
				SourceType:            "attack_record",
				Summary:               ticket.Statement,
				ProvenanceQuality:     ProvenanceQualityMedium,
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata: map[string]any{
					"kind":                         ticket.Kind,
					"attackType":                   firstNonEmpty(ticket.AttackType, ticket.Kind),
					"severity":                     ticket.Severity,
					"requestedChecks":              ticket.RequestedChecks,
					"suggestedFalsificationMethod": ticket.SuggestedFalsificationMethod,
					"round":                        round,
				},
			})
			if err != nil {
				return err
			}
			run.challengeTickets[len(run.challengeTickets)-1].EvidenceRefs = appendUnique(run.challengeTickets[len(run.challengeTickets)-1].EvidenceRefs, entry.EntryID)
			run.claimGraph = AttachChallengeToClaim(run.claimGraph, claim.ClaimID, ticket.TicketID)
		}
	}
	return nil
}

func (e *Engine) runVerifyPhase(ctx context.Context, request StartRequest, run *workflowRun, claimIDs []string) error {
	verifyCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, run.startedAt)
	if deadlineErr != nil {
		run.metrics.GlobalDeadlineHit = true
		return nil
	}
	defer cancel()
	verifier := e.deps.Verifier
	if verifier == nil {
		verifier = NewCompositeVerifier(CompositeVerifierDeps{
			TaskDelegate:   e.deps.TaskDelegate,
			Clock:          e.clock,
			IDFactory:      newLockedIDFactory(e.ids),
			ArtifactDir:    e.deps.ArtifactDir,
			PerTaskTimeout: e.effectivePerTaskTimeout(request, run.startedAt, request.WaitingPolicy.PerTaskTimeout),
			RetryAttempts:  request.WaitingPolicy.RetryAttempts,
		})
	}
	claims := make([]ClaimNode, 0)
	for _, claim := range filterClaimsByID(run.claimGraph, claimIDs) {
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		claims = append(claims, claim)
	}
	for _, verifyResult := range runVerificationBatch(verifyCtx, verifier, request, run.sessionID, claims, run.challengeTickets, request.VerificationPolicy.MaxParallelChecks) {
		claim := verifyResult.Claim
		findings := verifyResult.Findings
		verifyErr := verifyResult.Err
		if verifyErr != nil {
			if errors.Is(verifyErr, context.DeadlineExceeded) || errors.Is(verifyErr, context.Canceled) {
				run.metrics.GlobalDeadlineHit = true
				return nil
			}
			findings = []VerificationResult{{
				VerificationID: e.ids.NewEntityID("verify"),
				ClaimID:        claim.ClaimID,
				CheckName:      "verifier",
				Kind:           "verifier_error",
				Method:         "verifier",
				Status:         VerificationStatusInconclusive,
				Result:         VerificationStatusInconclusive,
				Summary:        verifyErr.Error(),
			}}
		}
		claimFindings := make([]VerificationResult, 0, len(findings))
		for _, finding := range findings {
			kind := EvidenceKindDeterministicCheck
			if finding.Kind == "semantic" {
				kind = EvidenceKindSemanticVerification
			}
			finding.Method = firstNonEmpty(finding.Method, finding.CheckName, finding.Kind)
			finding.Result = finding.Status
			if finding.Artifact != nil {
				finding.RawOutputReference = finding.Artifact.Path
			}
			metadata := map[string]any{
				"checkName":         finding.CheckName,
				"kind":              finding.Kind,
				"method":            finding.Method,
				"status":            finding.Status,
				"failureCode":       finding.FailureCode,
				"verdictSuggestion": finding.VerdictSuggestion,
				"confidence":        finding.Confidence,
				"confidenceDelta":   finding.ConfidenceDelta,
			}
			for key, value := range finding.Metadata {
				metadata[key] = value
			}
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:               claim.ClaimID,
				ChallengeID:           finding.ChallengeID,
				VerificationID:        finding.VerificationID,
				Kind:                  kind,
				Source:                EvidenceSourceVerifier,
				ProducerRole:          "verifier",
				SourceType:            finding.Method,
				Summary:               finding.Summary,
				Artifact:              finding.Artifact,
				SourceLocator:         finding.RawOutputReference,
				ProvenanceQuality:     evidenceQualityFromFinding(finding),
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata:              metadata,
			})
			if err != nil {
				return err
			}
			finding.EvidenceRef = entry.EntryID
			run.verificationResults = append(run.verificationResults, finding)
			claimFindings = append(claimFindings, finding)
			run.metrics.VerificationsRun++
		}
		for _, finding := range claimFindings {
			run.claimGraph = AttachVerificationOutcome(run.claimGraph, claim.ClaimID, finding)
		}
		run.challengeTickets = CloseChallenges(run.challengeTickets, claim.ClaimID, claimFindings, "verification completed")
	}
	return nil
}

func selectClaimsForRevision(claims []ClaimNode, tickets []ChallengeTicket, findings []VerificationResult, activeClaimIDs []string) []string {
	active := make(map[string]struct{}, len(activeClaimIDs))
	for _, id := range activeClaimIDs {
		active[id] = struct{}{}
	}
	out := make([]string, 0)
	for _, claim := range claims {
		if len(active) > 0 {
			if _, ok := active[claim.ClaimID]; !ok {
				continue
			}
		}
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		for _, ticket := range tickets {
			if ticket.ClaimID == claim.ClaimID && ticket.Status == ChallengeStatusOpen {
				out = appendUnique(out, claim.ClaimID)
				break
			}
		}
		for _, finding := range findings {
			if finding.ClaimID != claim.ClaimID {
				continue
			}
			if finding.Status != VerificationStatusPassed {
				out = appendUnique(out, claim.ClaimID)
			}
		}
	}
	return out
}

func (e *Engine) runRevisionPhase(ctx context.Context, request StartRequest, run *workflowRun, round int, targetClaimIDs []string) ([]string, bool, error) {
	revisionsByProposer := map[string][]ClaimNode{}
	for _, claim := range filterClaimsByID(run.claimGraph, targetClaimIDs) {
		proposerID := firstNonEmpty(claim.SourceProposalAgentID)
		if proposerID == "" && len(claim.ProposedBy) > 0 {
			proposerID = claim.ProposedBy[0]
		}
		if proposerID == "" && len(request.Roles.Proposers) > 0 {
			proposerID = request.Roles.Proposers[0]
		}
		revisionsByProposer[proposerID] = append(revisionsByProposer[proposerID], claim)
	}
	applied := make([]ClaimRevisionRecord, 0)
	proposerIDs := make([]string, 0, len(revisionsByProposer))
	for proposerID := range revisionsByProposer {
		proposerIDs = append(proposerIDs, proposerID)
	}
	slices.Sort(proposerIDs)
	type revisionTaskItem struct {
		proposerID string
		builtin    []ClaimRevisionRecord
	}
	taskItems := make([]revisionTaskItem, 0, len(proposerIDs))
	tasks := make([]Task, 0, len(proposerIDs))
	for _, proposerID := range proposerIDs {
		claims := revisionsByProposer[proposerID]
		groupClaimIDs := claimIDs(claims)
		findings := selectFindingsForClaims(run.verificationResults, groupClaimIDs)
		challenges := selectChallengesForClaims(run.challengeTickets, groupClaimIDs)
		builtin := builtinRevisionRecords(claims, challenges, findings, round, proposerID, e.ids)
		if proposerID == "" {
			applied = append(applied, builtin...)
			continue
		}
		taskItems = append(taskItems, revisionTaskItem{proposerID: proposerID, builtin: builtin})
		tasks = append(tasks, ReviseTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   proposerID,
				Role:      "proposer",
				Metadata:  map[string]any{"round": round},
			},
			TaskSpec:   request.TaskSpec,
			Manifest:   run.manifest,
			Round:      round,
			Claims:     claims,
			Challenges: challenges,
			Findings:   findings,
		})
	}
	for idx, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		item := taskItems[idx]
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "revision", round, "该 proposer 的修订缺席，改用内置降级修订"))
			applied = append(applied, item.builtin...)
			continue
		}
		output, ok := result.Awaited.Output.(ReviseTaskResult)
		if !ok {
			applied = append(applied, item.builtin...)
			continue
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindWorkerOutput,
			Source:                EvidenceSourceWorker,
			ProducerID:            item.proposerID,
			ProducerRole:          "proposer",
			SourceType:            "revision",
			Summary:               output.Output.Summary,
			Artifact:              result.Awaited.Artifact,
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata:              map[string]any{"taskId": result.Receipt.TaskID, "taskKind": TaskKindRevise, "round": round},
		})
		if err != nil {
			return nil, false, err
		}
		if len(output.Output.Revisions) == 0 {
			applied = append(applied, item.builtin...)
			continue
		}
		for _, draft := range output.Output.Revisions {
			applied = append(applied, ClaimRevisionRecord{
				RevisionID:         e.ids.NewEntityID("revision"),
				TargetClaimID:      draft.TargetClaimID,
				ProposerID:         item.proposerID,
				Action:             draft.Action,
				RevisedText:        draft.RevisedText,
				ConfidenceDelta:    draft.ConfidenceDelta,
				Caveats:            dedupeStrings(draft.Caveats),
				BoundaryConditions: dedupeStrings(draft.BoundaryConditions),
				Unresolved:         draft.Unresolved,
				Reason:             draft.Reason,
				EvidenceRefs:       filterEmpty([]string{entry.EntryID}),
				Round:              round,
				Metadata:           maps.Clone(draft.Metadata),
			})
		}
	}
	run.revisionRecords = append(run.revisionRecords, applied...)
	for _, record := range applied {
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			ClaimID:               record.TargetClaimID,
			Kind:                  EvidenceKindClaimRevised,
			Source:                EvidenceSourceCoordinator,
			ProducerID:            record.ProposerID,
			ProducerRole:          "proposer",
			SourceType:            "revision_record",
			Summary:               firstNonEmpty(record.Reason, string(record.Action)),
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata: map[string]any{
				"action":             record.Action,
				"confidenceDelta":    record.ConfidenceDelta,
				"caveats":            record.Caveats,
				"boundaryConditions": record.BoundaryConditions,
				"round":              record.Round,
				"metadata":           maps.Clone(record.Metadata),
			},
		})
		if err != nil {
			return nil, false, err
		}
		record.EvidenceRefs = appendUnique(record.EvidenceRefs, entry.EntryID)
		if err := e.emit(ctx, request, run.sessionID, RunEventClaimRevised, SessionPhaseRevise, map[string]any{
			"claimId":         record.TargetClaimID,
			"proposerId":      record.ProposerID,
			"action":          record.Action,
			"confidenceDelta": record.ConfidenceDelta,
			"round":           record.Round,
			"unresolved":      record.Unresolved,
			"reason":          record.Reason,
			"caveatCount":     len(record.Caveats),
			"boundaryCount":   len(record.BoundaryConditions),
			"revisedText":     record.RevisedText,
			"metadata":        maps.Clone(record.Metadata),
		}); err != nil {
			return nil, false, err
		}
	}
	var materialChange bool
	var revisedClaimIDs []string
	run.claimGraph, revisedClaimIDs, materialChange = ApplyRevisionRecords(run.claimGraph, applied, request.LoopPolicy.MaterialConfidenceDelta)
	return revisedClaimIDs, materialChange, nil
}

func builtinRevisionRecords(claims []ClaimNode, challenges []ChallengeTicket, findings []VerificationResult, round int, proposerID string, ids IDFactory) []ClaimRevisionRecord {
	out := make([]ClaimRevisionRecord, 0, len(claims))
	for _, claim := range claims {
		action := RevisionActionUnchanged
		reason := "未检测到材料性问题"
		confidenceDelta := 0.0
		caveats := make([]string, 0)
		for _, finding := range findings {
			if finding.ClaimID != claim.ClaimID {
				continue
			}
			switch finding.Status {
			case VerificationStatusFailed:
				action = RevisionActionWithdraw
				reason = firstNonEmpty(finding.Summary, "验证失败，暂时撤回 claim")
				confidenceDelta = -0.5
			case VerificationStatusInconclusive:
				if action != RevisionActionWithdraw {
					action = RevisionActionUnresolved
					reason = firstNonEmpty(finding.Summary, "验证结论不足，标记 unresolved")
					confidenceDelta = -0.2
				}
			}
		}
		for _, challenge := range challenges {
			if challenge.ClaimID != claim.ClaimID || challenge.Status != ChallengeStatusOpen {
				continue
			}
			if action == RevisionActionUnchanged {
				action = RevisionActionDowngrade
				reason = firstNonEmpty(challenge.AttackText, challenge.Statement, "存在未解决 challenge，降低信心")
				confidenceDelta = -0.15
			}
			caveats = append(caveats, firstNonEmpty(challenge.AttackText, challenge.Statement))
		}
		out = append(out, ClaimRevisionRecord{
			RevisionID:      ids.NewEntityID("revision"),
			TargetClaimID:   claim.ClaimID,
			ProposerID:      proposerID,
			Action:          action,
			ConfidenceDelta: confidenceDelta,
			Caveats:         dedupeStrings(caveats),
			Unresolved:      action == RevisionActionUnresolved,
			Reason:          reason,
			Round:           round,
		})
	}
	return out
}

func activeAdjudicationClaims(claims []ClaimNode) []ClaimNode {
	out := make([]ClaimNode, 0, len(claims))
	for _, claim := range claims {
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		out = append(out, claim)
	}
	return out
}

func claimIDs(claims []ClaimNode) []string {
	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		out = append(out, claim.ClaimID)
	}
	return out
}

func filterClaimsByID(claims []ClaimNode, ids []string) []ClaimNode {
	if len(ids) == 0 {
		return append([]ClaimNode(nil), claims...)
	}
	index := map[string]struct{}{}
	for _, id := range ids {
		index[id] = struct{}{}
	}
	out := make([]ClaimNode, 0, len(claims))
	for _, claim := range claims {
		if _, ok := index[claim.ClaimID]; ok {
			out = append(out, claim)
		}
	}
	return out
}

func selectFindingsForClaims(findings []VerificationResult, claimIDs []string) []VerificationResult {
	if len(claimIDs) == 0 {
		return append([]VerificationResult(nil), findings...)
	}
	index := map[string]struct{}{}
	for _, id := range claimIDs {
		index[id] = struct{}{}
	}
	out := make([]VerificationResult, 0)
	for _, finding := range findings {
		if _, ok := index[finding.ClaimID]; ok {
			out = append(out, finding)
		}
	}
	return out
}

func selectChallengesForClaims(tickets []ChallengeTicket, claimIDs []string) []ChallengeTicket {
	if len(claimIDs) == 0 {
		return append([]ChallengeTicket(nil), tickets...)
	}
	index := map[string]struct{}{}
	for _, id := range claimIDs {
		index[id] = struct{}{}
	}
	out := make([]ChallengeTicket, 0)
	for _, ticket := range tickets {
		if _, ok := index[ticket.ClaimID]; ok {
			out = append(out, ticket)
		}
	}
	return out
}

func inferClaimType(statement string) ClaimType {
	text := strings.ToLower(strings.TrimSpace(statement))
	switch {
	case strings.Contains(text, "should") || strings.Contains(text, "建议") || strings.Contains(text, "推荐"):
		return ClaimTypeRecommendation
	case strings.Contains(text, "because") || strings.Contains(text, "因此") || strings.Contains(text, "所以"):
		return ClaimTypeInference
	case strings.Contains(text, "assume") || strings.Contains(text, "假设"):
		return ClaimTypeAssumption
	default:
		return ClaimTypeFact
	}
}

func evidenceQualityFromFinding(finding VerificationResult) ProvenanceQuality {
	switch finding.Status {
	case VerificationStatusPassed:
		return ProvenanceQualityHigh
	case VerificationStatusFailed:
		return ProvenanceQualityHigh
	default:
		return ProvenanceQualityMedium
	}
}

func firstRecordDisposition(records []AdjudicationRecord, claimID string) ClaimDisposition {
	for _, record := range records {
		if record.TargetClaimID == claimID {
			return record.Disposition
		}
	}
	return ""
}

func matchingDecisionEvidence(decisions []ArbiterDecision, claimID string) []string {
	for _, decision := range decisions {
		if decision.ClaimID == claimID {
			return decision.EvidenceRefs
		}
	}
	return nil
}

func findArbiterDecision(decisions []ArbiterDecision, claimID string) ArbiterDecision {
	for _, decision := range decisions {
		if decision.ClaimID == claimID {
			return decision
		}
	}
	return ArbiterDecision{}
}

func (e *Engine) gatedAction(ctx context.Context, request StartRequest, run *workflowRun, result *RunResult) *ActionOutput {
	if request.ActionPolicy == nil {
		return nil
	}
	gate := request.ActionPolicy.RiskGate
	if gate == "" {
		gate = ActionRiskGateLowOnly
	}
	if !RiskGateAllows(gate, run.manifest.RiskLevel) {
		return &ActionOutput{
			ActorID:  firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor),
			Status:   string(TerminalStateActionBlockedByRisk),
			Summary:  "风险级别超过默认 action gate，保留为执行计划",
			Error:    "action blocked by risk gate",
			Executed: false,
		}
	}
	actionOutput, actionErr := e.executeAction(ctx, request, *result, run.startedAt)
	if actionErr != nil {
		return &ActionOutput{
			ActorID:  firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor),
			Status:   "failed",
			Error:    actionErr.Error(),
			Executed: false,
		}
	}
	if actionOutput != nil {
		actionOutput.Executed = actionOutput.Status == "completed"
	}
	return actionOutput
}

func (e *Engine) runObservePhase(ctx context.Context, request StartRequest, run *workflowRun, result *RunResult) error {
	observation := ObservationRecord{
		ObservationID: e.ids.NewEntityID("observe"),
		Outcome:       ObservationOutcomePending,
		Summary:       "等待后续证据验证本次裁决是否成立",
	}
	switch {
	case result.Action == nil:
		observation.Outcome = ObservationOutcomeNoAction
		observation.Summary = "未执行 action，保留为后续观察项"
	case result.Action.Status == string(TerminalStateActionBlockedByRisk):
		observation.Outcome = ObservationOutcomeFollowUp
		observation.Summary = "action 因风险门禁被阻止，建议人工复核后续执行"
		observation.Reopen = true
		observation.FollowUpCaseID = run.manifest.CaseID + "_followup"
	case result.Action.Executed:
		observation.Outcome = ObservationOutcomePending
		observation.Summary = "action 已执行，等待观测是否与 retained claims 一致"
	default:
		observation.Outcome = ObservationOutcomePending
		observation.Summary = firstNonEmpty(result.Action.Error, "action 未完成，保留后续观察")
	}
	entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:                  EvidenceKindObservationRecorded,
		Source:                EvidenceSourceCoordinator,
		ProducerRole:          "coordinator",
		SourceType:            "observation",
		Summary:               observation.Summary,
		ProvenanceQuality:     ProvenanceQualityMedium,
		FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
		ObservedAt:            e.clock.Now().Format(time.RFC3339Nano),
		Metadata: map[string]any{
			"outcome":        observation.Outcome,
			"reopen":         observation.Reopen,
			"followUpCaseId": observation.FollowUpCaseID,
		},
	})
	if err != nil {
		return err
	}
	observation.EvidenceRefs = append(observation.EvidenceRefs, entry.EntryID)
	run.observations = append(run.observations, observation)
	if err := e.emit(ctx, request, run.sessionID, RunEventObservationAdded, SessionPhaseObserve, map[string]any{
		"observationId":  observation.ObservationID,
		"outcome":        observation.Outcome,
		"reopen":         observation.Reopen,
		"followUpCaseId": observation.FollowUpCaseID,
		"summary":        observation.Summary,
	}); err != nil {
		return err
	}

	for _, source := range request.ObservePolicy.Sources {
		metadata := map[string]string{
			"TIL_CONSENSUS_CASE_ID":        run.manifest.CaseID,
			"TIL_CONSENSUS_TERMINAL_STATE": string(result.TerminalState),
		}
		if result.Action != nil {
			metadata["TIL_CONSENSUS_ACTION_STATUS"] = result.Action.Status
		}
		sourceResult, err := runExternalCommandSource(ctx, e.deps, e.clock, e.ids, request, run.sessionID, source, "observe", metadata)
		if err != nil {
			return err
		}
		item := ObservationRecord{
			ObservationID: e.ids.NewEntityID("observe"),
			Outcome:       ObservationOutcomePending,
			Summary:       sourceResult.Summary,
		}
		switch {
		case sourceResult.ExecFailed || sourceResult.Contradicted:
			item.Outcome = ObservationOutcomeContradicted
		case sourceResult.MatchedOK:
			item.Outcome = ObservationOutcomeHeldUp
		default:
			item.Outcome = ObservationOutcomePending
		}
		if item.Outcome == ObservationOutcomeContradicted && request.ObservePolicy.OnContradiction == ObserveContradictionReopen {
			item.Reopen = true
			followUpCaseID, followUpRequestID, followUpArtifact, err := e.createFollowUpCaseArtifact(request, run, source, sourceResult, "observe_contradiction")
			if err != nil {
				return err
			}
			item.FollowUpCaseID = followUpCaseID
			item.FollowUpRequestID = followUpRequestID
			item.FollowUpArtifact = followUpArtifact
			result.TerminalState = TerminalStateRequiresHumanReview
			if result.Adjudication != nil {
				result.Adjudication.TerminalState = TerminalStateRequiresHumanReview
			}
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindObservationRecorded,
			Source:                EvidenceSourceCoordinator,
			ProducerRole:          "coordinator",
			SourceType:            firstNonEmpty(source.SourceType, "external_observe"),
			SourceLocator:         firstNonEmpty(source.Reference, source.Command),
			Summary:               item.Summary,
			ContentExcerpt:        sourceResult.Excerpt,
			Artifact:              sourceResult.Artifact,
			ProvenanceQuality:     ProvenanceQualityHigh,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			ObservedAt:            e.clock.Now().Format(time.RFC3339Nano),
			Notes:                 append([]string(nil), sourceResult.Notes...),
			Metadata: mergeAnyMaps(sourceResult.Metadata, map[string]any{
				"sourceName":        source.Name,
				"outcome":           item.Outcome,
				"reopen":            item.Reopen,
				"followUpCaseId":    item.FollowUpCaseID,
				"followUpRequestId": item.FollowUpRequestID,
				"matchedOK":         sourceResult.MatchedOK,
				"contradicted":      sourceResult.Contradicted,
				"execFailed":        sourceResult.ExecFailed,
				"failureClass":      sourceResult.FailureClass,
				"terminalState":     result.TerminalState,
				"actionExecuted":    result.Action != nil && result.Action.Executed,
			}),
		})
		if err != nil {
			return err
		}
		item.EvidenceRefs = append(item.EvidenceRefs, entry.EntryID)
		if item.FollowUpArtifact != nil {
			followUpEntry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:                  EvidenceKindFollowUpCaseCreated,
				Source:                EvidenceSourceCoordinator,
				ProducerRole:          "coordinator",
				SourceType:            "follow_up_case",
				SourceLocator:         item.FollowUpCaseID,
				Summary:               "已生成 follow-up case 请求",
				Artifact:              item.FollowUpArtifact,
				ProvenanceQuality:     ProvenanceQualityHigh,
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata: map[string]any{
					"followUpCaseId":    item.FollowUpCaseID,
					"followUpRequestId": item.FollowUpRequestID,
					"sourceName":        source.Name,
					"trigger":           "observe_contradiction",
				},
			})
			if err != nil {
				return err
			}
			item.EvidenceRefs = append(item.EvidenceRefs, followUpEntry.EntryID)
		}
		run.observations = append(run.observations, item)
		if err := e.emit(ctx, request, run.sessionID, RunEventObservationAdded, SessionPhaseObserve, map[string]any{
			"observationId":  item.ObservationID,
			"outcome":        item.Outcome,
			"reopen":         item.Reopen,
			"followUpCaseId": item.FollowUpCaseID,
			"summary":        item.Summary,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runIngestSources(ctx context.Context, request StartRequest, run *workflowRun, reason string) (bool, error) {
	if len(request.IngestPolicy.Sources) == 0 {
		return false, nil
	}
	newEvidence := false
	for _, source := range request.IngestPolicy.Sources {
		sourceResult, err := runExternalCommandSource(ctx, e.deps, e.clock, e.ids, request, run.sessionID, source, "ingest", map[string]string{
			"TIL_CONSENSUS_CASE_ID": run.manifest.CaseID,
			"TIL_CONSENSUS_REASON":  reason,
		})
		if err != nil {
			return newEvidence, err
		}
		if sourceResult.Artifact != nil || strings.TrimSpace(sourceResult.Excerpt) != "" || sourceResult.MatchedOK || sourceResult.Contradicted || sourceResult.ExecFailed {
			newEvidence = true
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindSourceMaterial,
			Source:                EvidenceSourceCoordinator,
			ProducerRole:          "coordinator",
			SourceType:            firstNonEmpty(source.SourceType, "external_ingest"),
			SourceLocator:         firstNonEmpty(source.Reference, source.Command),
			Summary:               sourceResult.Summary,
			ContentExcerpt:        sourceResult.Excerpt,
			Artifact:              sourceResult.Artifact,
			ProvenanceQuality:     ProvenanceQualityHigh,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			ObservedAt:            e.clock.Now().Format(time.RFC3339Nano),
			Notes:                 append([]string(nil), sourceResult.Notes...),
			Metadata: mergeAnyMaps(sourceResult.Metadata, map[string]any{
				"reason":       reason,
				"collector":    "external_command",
				"sourceName":   source.Name,
				"matchedOK":    sourceResult.MatchedOK,
				"contradicted": sourceResult.Contradicted,
				"execFailed":   sourceResult.ExecFailed,
				"failureClass": sourceResult.FailureClass,
				"command":      source.Command,
				"args":         append([]string(nil), source.Args...),
			}),
		}); err != nil {
			return newEvidence, err
		}
	}
	return newEvidence, nil
}

func decideAdjudicationFallback(request StartRequest, claims []ClaimNode, tickets []ChallengeTicket, arbiter ArbiterReport, terminalState WorkflowTerminalState, fallbacksUsed int) (FallbackTarget, []string, string) {
	if fallbacksUsed >= request.FallbackPolicy.MaxFallbackRounds {
		return FallbackTargetStop, nil, ""
	}
	records := arbiter.Records
	if len(records) == 0 {
		records = deriveRecordsFromDecisions(request, activeAdjudicationClaims(claims), arbiter.Decisions)
	}
	unresolvedClaimIDs := claimIDsByDisposition(records, ClaimDispositionUnresolved)
	caveatedClaimIDs := claimIDsByDisposition(records, ClaimDispositionKeepWithCaveat)
	openChallengeClaimIDs := claimIDsForOpenChallenges(tickets)
	switch terminalState {
	case TerminalStateInsufficientEvidence:
		return request.FallbackPolicy.OnInsufficientEvidence, fallbackClaimIDs(unresolvedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "裁决结果显示证据不足，自动回退到补充证据/修订流程"
	case TerminalStateUnresolvedConflict:
		return request.FallbackPolicy.OnUnresolvedConflict, fallbackClaimIDs(openChallengeClaimIDs, unresolvedClaimIDs, claimIDs(claims)), "裁决结果存在未解决冲突，自动回退到补充证据/修订流程"
	}
	if len(unresolvedClaimIDs) > 0 {
		return request.FallbackPolicy.OnUnresolvedClaims, fallbackClaimIDs(unresolvedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "存在 unresolved claim，自动回退继续修订或补充证据"
	}
	if len(caveatedClaimIDs) > 0 {
		return request.FallbackPolicy.OnKeepWithCaveat, fallbackClaimIDs(caveatedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "存在 keep_with_caveat claim，自动回退继续修订或补充证据"
	}
	return FallbackTargetStop, nil, ""
}

func claimIDsByDisposition(records []AdjudicationRecord, dispositions ...ClaimDisposition) []string {
	allowed := make(map[ClaimDisposition]struct{}, len(dispositions))
	for _, disposition := range dispositions {
		allowed[disposition] = struct{}{}
	}
	out := make([]string, 0)
	for _, record := range records {
		if _, ok := allowed[record.Disposition]; ok {
			out = appendUnique(out, record.TargetClaimID)
		}
	}
	return out
}

func claimIDsForOpenChallenges(tickets []ChallengeTicket) []string {
	out := make([]string, 0)
	for _, ticket := range tickets {
		if ticket.Status == ChallengeStatusOpen {
			out = appendUnique(out, ticket.ClaimID)
		}
	}
	return out
}

func fallbackClaimIDs(groups ...[]string) []string {
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		return dedupeStrings(group)
	}
	return nil
}

func mergeAnyMaps(base map[string]any, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneAnyMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func (e *Engine) startFreeDebate(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
	run, err := e.beginWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, run.sessionID, run.state, nil, nil, &run.ledgerCursor, run.startedAt, err)
	}()

	claims := make([]DebateClaim, 0)
	rounds := make([]DebateRoundRecord, 0, request.DebatePolicy.MaxRounds+2)
	votes := make([]DebateVoteRecord, 0)

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseInitial, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindDebateRoundOpened,
		Source:       EvidenceSourceCoordinator,
		ProducerRole: "coordinator",
		Summary:      "initial round opened",
		Metadata: map[string]any{
			"round": 0,
			"phase": "initial",
		},
	}); err != nil {
		return nil, err
	}
	initialRound := DebateRoundRecord{Round: 0, Phase: "initial", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
	initialTasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		initialTasks = append(initialTasks, InitialProposalTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   participantID,
				Role:      "participant",
			},
			TaskSpec:  request.TaskSpec,
			Round:     0,
			MaxClaims: request.ProposalPolicy.MaxClaimsPerWorker,
		})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, initialTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(initialTasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "initial", 0, "该参与者未提交初始提案，辩论基于其余参与者的 claim"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(InitialProposalTaskResult)
		if !ok {
			return nil, fmt.Errorf("initial proposal task returned unexpected result type")
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateRoundOutput,
			Source:       EvidenceSourceWorker,
			ProducerID:   participantID,
			ProducerRole: "participant",
			Summary:      output.Output.Summary,
			Artifact:     result.Awaited.Artifact,
			Metadata:     map[string]any{"round": 0, "taskId": result.Receipt.TaskID},
		})
		if err != nil {
			return nil, err
		}
		participant := DebateParticipantOutput{AgentID: participantID, Summary: output.Output.Summary}
		for _, draft := range output.Output.Claims {
			draft = canonicalizeDebateClaimDraft(draft)
			if strings.TrimSpace(draft.Statement) == "" {
				continue
			}
			if isDebateProcessClaim(draft) {
				participant.ProcessNotes = append(participant.ProcessNotes, debateProcessNote(draft))
				continue
			}
			var claimID string
			claims, claimID = upsertDebateClaim(claims, draft, participantID, 0, entry.EntryID, e.ids)
			if claimID == "" {
				continue
			}
			participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
			run.metrics.ClaimsProposed++
		}
		initialRound.ParticipantOutputs = append(initialRound.ParticipantOutputs, participant)
	}
	rounds = append(rounds, initialRound)
	if len(claims) == 0 {
		section := &FreeDebateResultSection{Outcome: FreeDebateOutcomeNoConsensus, Rounds: rounds}
		report := buildFreeDebateReport(request, section)
		result := &RunResult{
			SchemaVersion: SchemaVersion,
			Mode:          WorkflowModeFreeDebate,
			RequestID:     request.RequestID,
			SessionID:     run.sessionID,
			Lineage:       request.Lineage,
			TaskSpec:      request.TaskSpec,
			Report:        report,
			Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
			Degradations:  run.degradations,
			FreeDebate:    section,
		}
		if err := e.finishSession(ctx, run.sessionID, run.state, result, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDebate, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	for round := 1; round <= request.DebatePolicy.MaxRounds; round++ {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			break
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateRoundOpened,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      "debate round opened",
			Metadata:     map[string]any{"round": round, "phase": "debate"},
		}); err != nil {
			return nil, err
		}
		roundRecord := DebateRoundRecord{Round: round, Phase: "debate", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
		roundNewClaims := 0
		onlyAgree := true
		maxNewClaims := request.DebatePolicy.MaxNewClaimsPerRound
		if ceiling := request.DebatePolicy.MaxActiveClaims; ceiling > 0 && len(activeDebateClaims(claims)) >= ceiling {
			// Active-claim ceiling reached: this round may only judge, revise,
			// and merge; MaxNewClaims < 0 marks "no new claims allowed".
			maxNewClaims = -1
		}
		roundTasks := make([]Task, 0, len(request.Roles.Participants))
		for _, participantID := range request.Roles.Participants {
			selfClaims, peerClaims := splitDebateClaims(claims, participantID)
			roundTasks = append(roundTasks, DebateRoundTask{
				TaskMeta: TaskMeta{
					SessionID: run.sessionID,
					RequestID: request.RequestID,
					AgentID:   participantID,
					Role:      "participant",
				},
				TaskSpec:        request.TaskSpec,
				Round:           round,
				SelfClaims:      selfClaims,
				PeerClaims:      peerClaims,
				RoundSummary:    summarizeDebateClaims(peerClaims),
				MaxNewClaims:    maxNewClaims,
				PeerContextMode: request.DebatePolicy.PeerContextMode,
			})
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, roundTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(roundTasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				onlyAgree = false
				run.addDegradation(participantAbsentDegradation(result, "debate", round, "该参与者缺席本轮辩论，未对 peer claims 给出评判"))
				continue
			}
			participantID := result.Task.Meta().AgentID
			output, ok := result.Awaited.Output.(DebateRoundTaskResult)
			if !ok {
				return nil, fmt.Errorf("debate round task returned unexpected result type")
			}
			newClaims, truncatedNewClaims := truncateDebateNewClaims(output.Output.NewClaims, maxNewClaims)
			metadata := map[string]any{"round": round, "taskId": result.Receipt.TaskID}
			if truncatedNewClaims > 0 {
				metadata["truncatedNewClaims"] = truncatedNewClaims
				metadata["maxNewClaims"] = maxNewClaims
			}
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDebateRoundOutput,
				Source:       EvidenceSourceWorker,
				ProducerID:   participantID,
				ProducerRole: "participant",
				Summary:      output.Output.Summary,
				Artifact:     result.Awaited.Artifact,
				Metadata:     metadata,
			})
			if err != nil {
				return nil, err
			}
			participant := DebateParticipantOutput{AgentID: participantID, Summary: output.Output.Summary}
			for _, draft := range newClaims {
				draft = canonicalizeDebateClaimDraft(draft)
				if strings.TrimSpace(draft.Statement) == "" {
					continue
				}
				if isDebateProcessClaim(draft) {
					participant.ProcessNotes = append(participant.ProcessNotes, debateProcessNote(draft))
					continue
				}
				var claimID string
				claims, claimID = upsertDebateClaim(claims, draft, participantID, round, entry.EntryID, e.ids)
				if claimID == "" {
					continue
				}
				participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
				roundNewClaims++
				run.metrics.ClaimsProposed++
			}
			for _, judgement := range output.Output.Judgements {
				record := DebateJudgementRecord{
					ClaimID:         strings.TrimSpace(judgement.ClaimID),
					Judgement:       judgement.Judgement,
					Rationale:       judgement.Rationale,
					MergeWithClaims: dedupeStrings(judgement.MergeWithClaims),
				}
				if judgement.Judgement != DebateJudgementAgree && judgement.Judgement != DebateJudgementNoChange {
					onlyAgree = false
				}
				if judgement.Judgement == DebateJudgementRevise && strings.TrimSpace(judgement.RevisedStatement) != "" {
					claims, record.RevisedClaimID = upsertDebateClaim(claims, ClaimDraft{
						Title:     "Revision by " + participantID,
						Statement: judgement.RevisedStatement,
					}, participantID, round, entry.EntryID, e.ids)
					if record.RevisedClaimID != "" {
						roundNewClaims++
						run.metrics.ClaimsProposed++
					}
				}
				if len(record.MergeWithClaims) > 0 {
					claims = markDebateClaimMerged(claims, record.ClaimID, record.MergeWithClaims[0])
				}
				participant.Judgements = append(participant.Judgements, record)
			}
			roundRecord.ParticipantOutputs = append(roundRecord.ParticipantOutputs, participant)
		}
		roundRecord.Summary = summarizeDebateRound(roundRecord)
		rounds = append(rounds, roundRecord)
		if request.DebatePolicy.SemanticDedup.Cadence != DebateSemanticDedupCadenceFinal {
			// per_round cadence: consolidate right away so the next round's
			// peer context (and eventually the ballot) stays canonical, and a
			// single failed dedup call only loses one round of consolidation.
			claims = e.runDebateSemanticDedup(ctx, request, run, claims, round)
		}
		if request.DebatePolicy.EnableEarlyStop && round >= request.DebatePolicy.MinRounds && roundNewClaims == 0 && onlyAgree {
			break
		}
	}
	if request.DebatePolicy.SemanticDedup.Cadence == DebateSemanticDedupCadenceFinal || len(rounds) <= 1 {
		// final cadence keeps the single pre-vote pass; per_round already
		// deduped after the last executed round unless no debate round ran.
		claims = e.runDebateSemanticDedup(ctx, request, run, claims, len(rounds))
	}

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseFinalVote, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	activeClaims := activeDebateClaims(claims)
	voteTasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		voteTasks = append(voteTasks, FinalVoteTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   participantID,
				Role:      "participant",
			},
			TaskSpec: request.TaskSpec,
			Round:    len(rounds),
			Claims:   activeClaims,
		})
	}
	voters := make([]string, 0, len(request.Roles.Participants))
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, voteTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(voteTasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "final_vote", len(rounds), "该参与者未投票，claim 接受判定仅基于成功投票的参与者"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(FinalVoteTaskResult)
		if !ok {
			return nil, fmt.Errorf("final vote task returned unexpected result type")
		}
		voters = appendUnique(voters, participantID)
		participantVotes := make([]DebateVoteRecord, 0, len(output.Output.Votes))
		for _, draft := range output.Output.Votes {
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:      draft.ClaimID,
				Kind:         EvidenceKindDebateVoteCast,
				Source:       EvidenceSourceWorker,
				ProducerID:   participantID,
				ProducerRole: "participant",
				Summary:      output.Output.Summary,
				Artifact:     result.Awaited.Artifact,
				Metadata: map[string]any{
					"taskId":     result.Receipt.TaskID,
					"vote":       draft.Vote,
					"confidence": *draft.Confidence,
					"rationale":  draft.Rationale,
				},
			})
			if err != nil {
				return nil, err
			}
			participantVotes = append(participantVotes, DebateVoteRecord{
				ClaimID:     draft.ClaimID,
				AgentID:     participantID,
				Vote:        draft.Vote,
				Confidence:  *draft.Confidence,
				Rationale:   draft.Rationale,
				EvidenceRef: entry.EntryID,
			})
		}
		votes = append(votes, participantVotes...)
	}

	resolutions, outcome := resolveDebateClaims(activeClaims, claims, votes, request.DebatePolicy)
	absentVoters := make([]string, 0)
	for _, participantID := range request.Roles.Participants {
		if !slices.Contains(voters, participantID) {
			absentVoters = append(absentVoters, participantID)
		}
	}
	if quorum := request.DebatePolicy.VoteQuorum; quorum > 0 && len(request.Roles.Participants) > 0 {
		required := quorum * float64(len(request.Roles.Participants))
		if float64(len(voters)) < required {
			impact := fmt.Sprintf("仅 %d/%d 参与者完成投票，低于 quorum %.2f，outcome 由 %s 降级为 %s", len(voters), len(request.Roles.Participants), quorum, outcome, FreeDebateOutcomeQuorumNotMet)
			outcome = FreeDebateOutcomeQuorumNotMet
			run.addDegradation(Degradation{
				Kind:   DegradationQuorumNotMet,
				Phase:  "final_vote",
				Round:  len(rounds),
				Impact: impact,
			})
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDebateVoteQuorum,
				Source:       EvidenceSourceCoordinator,
				ProducerRole: "coordinator",
				Summary:      impact,
				Metadata: map[string]any{
					"voteQuorum":   quorum,
					"voters":       voters,
					"absentVoters": absentVoters,
					"participants": len(request.Roles.Participants),
				},
			}); err != nil {
				return nil, err
			}
		}
	}
	section := &FreeDebateResultSection{
		Outcome:          outcome,
		Rounds:           rounds,
		Claims:           claims,
		ClaimResolutions: resolutions,
		Votes:            votes,
		Voters:           voters,
		AbsentVoters:     absentVoters,
		BallotSize:       len(activeClaims),
	}
	report := buildFreeDebateReport(request, section)
	report, reportArtifact, err := e.composeModeReport(ctx, request, run, WorkflowModeFreeDebate, report, TaskVerdictFromDebateOutcome(outcome), map[string]any{
		"freeDebate": section,
	})
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

	result := &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeFreeDebate,
		RequestID:     request.RequestID,
		SessionID:     run.sessionID,
		Lineage:       request.Lineage,
		TaskSpec:      request.TaskSpec,
		Report:        report,
		Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
		Degradations:  run.degradations,
		FreeDebate:    section,
	}
	if request.ActionPolicy != nil {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAction, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
		actionOutput, actionErr := e.executeAction(ctx, request, *result, run.startedAt)
		if actionErr != nil {
			actionOutput = &ActionOutput{
				ActorID: firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor),
				Status:  "failed",
				Error:   actionErr.Error(),
			}
		}
		result.Action = actionOutput
	}
	if err := e.finishSession(ctx, run.sessionID, run.state, result, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) startDelphi(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
	run, err := e.beginWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, run.sessionID, run.state, nil, nil, &run.ledgerCursor, run.startedAt, err)
	}()

	statements := make([]DelphiStatement, 0)
	distributions := map[string][]float64{}
	rounds := make([]DelphiRoundRecord, 0, request.DelphiPolicy.MaxRounds*2)
	lastSummary := ""
	for round := 1; round <= request.DelphiPolicy.MaxRounds; round++ {
		var phase SessionPhase
		if round == 1 {
			phase = SessionPhaseDelphiQuestionnaire
		} else {
			phase = SessionPhaseDelphiRevision
		}
		if run.state.Current() != phase {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, phase, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDelphiRoundOpened,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      "delphi round opened",
			Metadata: map[string]any{
				"round": round,
				"phase": phase,
			},
		}); err != nil {
			return nil, err
		}
		responses := make([]DelphiResponseRecord, 0, len(request.Roles.Participants))
		tasks := make([]Task, 0, len(request.Roles.Participants))
		for _, participantID := range request.Roles.Participants {
			if round == 1 {
				tasks = append(tasks, DelphiQuestionnaireTask{
					TaskMeta: TaskMeta{
						SessionID: run.sessionID,
						RequestID: request.RequestID,
						AgentID:   participantID,
						Role:      "participant",
					},
					TaskSpec:           request.TaskSpec,
					Round:              round,
					Questionnaire:      request.TaskSpec.Goal,
					PreviousStatements: statements,
					PreviousSummary:    lastSummary,
				})
			} else {
				tasks = append(tasks, DelphiRevisionTask{
					TaskMeta: TaskMeta{
						SessionID: run.sessionID,
						RequestID: request.RequestID,
						AgentID:   participantID,
						Role:      "participant",
					},
					TaskSpec:           request.TaskSpec,
					Round:              round,
					StatementSummaries: statements,
					PreviousSummary:    lastSummary,
				})
			}
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				run.addDegradation(participantAbsentDegradation(result, "delphi_round", round, "该参与者本轮未提交评分/修订，共识统计缺少其意见"))
				continue
			}
			var outputResponses []DelphiResponseDraft
			var summary string
			switch typed := result.Awaited.Output.(type) {
			case DelphiQuestionnaireTaskResult:
				outputResponses = typed.Output.Responses
				summary = typed.Output.Summary
			case DelphiRevisionTaskResult:
				outputResponses = typed.Output.Responses
				summary = typed.Output.Summary
			default:
				return nil, fmt.Errorf("delphi task returned unexpected result type")
			}
			for _, draft := range outputResponses {
				record := DelphiResponseRecord{
					StatementID: strings.TrimSpace(draft.StatementID),
					Statement:   strings.TrimSpace(draft.Statement),
					Rating:      draft.Rating,
					Rationale:   draft.Rationale,
				}
				responses = append(responses, record)
				if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
					Kind:         EvidenceKindDelphiResponse,
					Source:       EvidenceSourceWorker,
					ProducerRole: "participant",
					Summary:      summary,
					Artifact:     result.Awaited.Artifact,
					Metadata: map[string]any{
						"round":     round,
						"taskId":    result.Receipt.TaskID,
						"statement": record.Statement,
						"rating":    record.Rating,
					},
				}); err != nil {
					return nil, err
				}
			}
		}
		statements, distributions = aggregateDelphiResponses(statements, responses, round, request.DelphiPolicy)
		lastSummary, recommendation, dissent, facilitatorArtifact, err := e.buildDelphiSummary(ctx, request, run, round, statements)
		if err != nil {
			return nil, err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDelphiRoundSummary,
			Source:       EvidenceSourceCoordinator,
			ProducerID:   request.Roles.Facilitator,
			ProducerRole: "facilitator",
			Summary:      lastSummary,
			Artifact:     facilitatorArtifact,
			Metadata: map[string]any{
				"round":          round,
				"recommendation": recommendation,
			},
		}); err != nil {
			return nil, err
		}
		rounds = append(rounds, DelphiRoundRecord{
			Round:      round,
			Phase:      string(phase),
			Summary:    lastSummary,
			Responses:  responses,
			Statements: cloneDelphiStatements(statements),
		})
		bestConsensus, _ := bestDelphiStatement(statements)
		if round >= request.DelphiPolicy.MinRounds && bestConsensus >= request.DelphiPolicy.ConvergenceThreshold {
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDelphiConvergence,
				Source:       EvidenceSourceCoordinator,
				ProducerRole: "coordinator",
				Summary:      "delphi convergence threshold reached",
				Metadata:     map[string]any{"round": round, "consensusLevel": bestConsensus},
			}); err != nil {
				return nil, err
			}
			section := &DelphiResultSection{
				Rounds:              rounds,
				Statements:          statements,
				RatingDistributions: distributions,
				ConsensusLevel:      bestConsensus,
				Recommendation:      recommendation,
				DissentSummary:      dissent,
			}
			return e.finishDelphi(ctx, request, run, section)
		}
		if round < request.DelphiPolicy.MaxRounds {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDelphiSummary, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
	}
	bestConsensus, recommendation := bestDelphiStatement(statements)
	section := &DelphiResultSection{
		Rounds:              rounds,
		Statements:          statements,
		RatingDistributions: distributions,
		ConsensusLevel:      bestConsensus,
		Recommendation:      recommendation,
		DissentSummary:      buildDelphiDissentSummary(statements),
	}
	return e.finishDelphi(ctx, request, run, section)
}

func (e *Engine) finishDelphi(ctx context.Context, request StartRequest, run *workflowRun, section *DelphiResultSection) (*RunResult, error) {
	if run.state.Current() != SessionPhaseReport {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	report := buildDelphiReport(request, section)
	report, artifact, err := e.composeModeReport(ctx, request, run, WorkflowModeDelphi, report, TaskVerdictFromDelphi(section), map[string]any{
		"delphi": section,
	})
	if err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindReportGenerated,
		Source:       EvidenceSourceReporter,
		ProducerID:   request.Roles.Reporter,
		ProducerRole: "reporter",
		Summary:      report.Summary,
		Artifact:     artifact,
	}); err != nil {
		return nil, err
	}
	result := &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeDelphi,
		RequestID:     request.RequestID,
		SessionID:     run.sessionID,
		Lineage:       request.Lineage,
		TaskSpec:      request.TaskSpec,
		Report:        report,
		Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
		Degradations:  run.degradations,
		Delphi:        section,
	}
	if request.ActionPolicy != nil {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAction, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
		actionOutput, actionErr := e.executeAction(ctx, request, *result, run.startedAt)
		if actionErr != nil {
			actionOutput = &ActionOutput{
				ActorID: firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor),
				Status:  "failed",
				Error:   actionErr.Error(),
			}
		}
		result.Action = actionOutput
	}
	if err := e.finishSession(ctx, run.sessionID, run.state, result, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) composeModeReport(ctx context.Context, request StartRequest, run *workflowRun, mode WorkflowMode, builtin AdjudicationReport, verdict TaskVerdict, payload map[string]any) (AdjudicationReport, *ArtifactRef, error) {
	if request.Roles.Reporter == "" || e.deps.TaskDelegate == nil {
		return builtin, nil, nil
	}
	reporterFallback := func(reason string) {
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "report",
			AgentID: request.Roles.Reporter,
			Reason:  reason,
			Impact:  "reporter 任务未完成，报告回退为内置模板摘要",
		})
	}
	reportCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, run.startedAt)
	if deadlineErr != nil {
		run.metrics.GlobalDeadlineHit = true
		reporterFallback("global deadline exceeded")
		return builtin, nil, nil
	}
	defer cancel()
	_, awaited, err := e.executeTask(reportCtx, request, run.sessionID, ReportTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.Reporter,
			Role:      "reporter",
		},
		TaskSpec:    request.TaskSpec,
		TaskVerdict: verdict,
		Mode:        mode,
		Payload:     payload,
	}, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if errors.Is(err, ErrGlobalDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			run.metrics.GlobalDeadlineHit = true
		}
		reporterFallback(err.Error())
		return builtin, awaited.Artifact, nil
	}
	typed, ok := awaited.Output.(ReportTaskResult)
	if !ok {
		reporterFallback("report task returned unexpected result type")
		return builtin, awaited.Artifact, nil
	}
	return typed.Output, awaited.Artifact, nil
}

func (e *Engine) runDebateSemanticDedup(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, round int) []DebateClaim {
	policy := request.DebatePolicy.SemanticDedup
	if !policy.Enabled || request.Roles.SemanticDeduper == "" {
		return claims
	}
	activeClaims := activeDebateClaims(claims)
	if len(activeClaims) < 2 {
		return claims
	}
	receipt, awaited, err := e.executeTask(ctx, request, run.sessionID, SemanticDedupTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.SemanticDeduper,
			Role:      "semantic-deduper",
		},
		TaskSpec:            request.TaskSpec,
		Round:               round,
		Claims:              activeClaims,
		SimilarityThreshold: policy.SimilarityThreshold,
	}, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if errors.Is(err, ErrGlobalDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			run.metrics.GlobalDeadlineHit = true
		}
		if strings.Contains(err.Error(), "__timeout__") {
			run.metrics.WaitTimeouts++
		}
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSemanticDedup,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.SemanticDeduper,
			ProducerRole: "semantic-deduper",
			Summary:      "semantic dedup failed: " + err.Error(),
			Artifact:     awaited.Artifact,
			Metadata: map[string]any{
				"taskId":              receipt.TaskID,
				"taskKind":            TaskKindSemanticDedup,
				"round":               round,
				"similarityThreshold": policy.SimilarityThreshold,
				"status":              "failed",
			},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "semantic_dedup",
			Round:   round,
			AgentID: request.Roles.SemanticDeduper,
			Reason:  err.Error(),
			Impact:  fmt.Sprintf("语义去重未执行，%d 条 active claim 未合并直接进入 final vote", len(activeClaims)),
		})
		return claims
	}
	output, ok := awaited.Output.(SemanticDedupTaskResult)
	if !ok {
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSemanticDedup,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.SemanticDeduper,
			ProducerRole: "semantic-deduper",
			Summary:      "semantic dedup returned unexpected result type",
			Artifact:     awaited.Artifact,
			Metadata: map[string]any{
				"taskId":   receipt.TaskID,
				"taskKind": TaskKindSemanticDedup,
				"round":    round,
				"status":   "failed",
			},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "semantic_dedup",
			Round:   round,
			AgentID: request.Roles.SemanticDeduper,
			Reason:  "semantic dedup returned unexpected result type",
			Impact:  fmt.Sprintf("语义去重未执行，%d 条 active claim 未合并直接进入 final vote", len(activeClaims)),
		})
		return claims
	}
	entry, appendErr := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindDebateSemanticDedup,
		Source:       EvidenceSourceWorker,
		ProducerID:   request.Roles.SemanticDeduper,
		ProducerRole: "semantic-deduper",
		Summary:      output.Output.Summary,
		Artifact:     awaited.Artifact,
		Metadata: map[string]any{
			"taskId":              receipt.TaskID,
			"taskKind":            TaskKindSemanticDedup,
			"round":               round,
			"mergeCount":          len(output.Output.Merges),
			"similarityThreshold": policy.SimilarityThreshold,
			"status":              "completed",
		},
	})
	evidenceRef := ""
	if appendErr == nil {
		evidenceRef = entry.EntryID
	}
	return applyDebateSemanticDedup(claims, output.Output.Merges, evidenceRef)
}

// truncateDebateNewClaims enforces the per-round new-claims budget: limit > 0
// keeps the first limit drafts, limit < 0 drops all drafts (active-claim
// ceiling reached), limit == 0 means no budget was set.
func truncateDebateNewClaims(drafts []ClaimDraft, limit int) ([]ClaimDraft, int) {
	if limit == 0 || len(drafts) == 0 {
		return drafts, 0
	}
	allowed := limit
	if allowed < 0 {
		allowed = 0
	}
	if len(drafts) <= allowed {
		return drafts, 0
	}
	return drafts[:allowed], len(drafts) - allowed
}

func upsertDebateClaim(claims []DebateClaim, draft ClaimDraft, ownerID string, round int, evidenceRef string, ids IDFactory) ([]DebateClaim, string) {
	draft = canonicalizeDebateClaimDraft(draft)
	if strings.TrimSpace(draft.Statement) == "" {
		return claims, ""
	}
	key := normalizedClaimKey(draft.Statement)
	for idx := range claims {
		if normalizedClaimKey(claims[idx].Statement) != key {
			continue
		}
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, evidenceRef)
		claims[idx].ProposedBy = appendUnique(claims[idx].ProposedBy, ownerID)
		claims[idx].Active = true
		return claims, claims[idx].ClaimID
	}
	claim := DebateClaim{
		ClaimID:      ids.NewEntityID("debate_claim"),
		Title:        strings.TrimSpace(draft.Title),
		Statement:    strings.TrimSpace(draft.Statement),
		OwnerID:      ownerID,
		ProposedBy:   filterEmpty([]string{ownerID}),
		Round:        round,
		Active:       true,
		EvidenceRefs: filterEmpty([]string{evidenceRef}),
	}
	return append(claims, claim), claim.ClaimID
}

func markDebateClaimMerged(claims []DebateClaim, claimID string, targetID string) []DebateClaim {
	return mergeDebateClaim(claims, claimID, targetID, "")
}

func applyDebateSemanticDedup(claims []DebateClaim, merges []DebateClaimMergeDraft, evidenceRef string) []DebateClaim {
	for _, merge := range merges {
		claims = mergeDebateClaim(claims, merge.SourceClaimID, merge.TargetClaimID, evidenceRef)
	}
	return claims
}

func mergeDebateClaim(claims []DebateClaim, sourceID string, targetID string, evidenceRef string) []DebateClaim {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return claims
	}
	sourceIdx := -1
	targetIdx := -1
	for idx := range claims {
		switch claims[idx].ClaimID {
		case sourceID:
			sourceIdx = idx
		case targetID:
			targetIdx = idx
		}
	}
	if sourceIdx < 0 || targetIdx < 0 || !claims[sourceIdx].Active || !claims[targetIdx].Active {
		return claims
	}
	source := claims[sourceIdx]
	claims[sourceIdx].Active = false
	claims[sourceIdx].MergedInto = targetID
	claims[sourceIdx].EvidenceRefs = appendUnique(claims[sourceIdx].EvidenceRefs, evidenceRef)
	sourceProposers := appendUniqueStrings(source.ProposedBy, source.OwnerID)
	sourceEvidenceRefs := appendUniqueStrings(source.EvidenceRefs, evidenceRef)
	claims[targetIdx].ProposedBy = appendUniqueStrings(claims[targetIdx].ProposedBy, sourceProposers...)
	claims[targetIdx].MergedClaimIDs = appendUnique(claims[targetIdx].MergedClaimIDs, sourceID)
	claims[targetIdx].EvidenceRefs = appendUniqueStrings(claims[targetIdx].EvidenceRefs, sourceEvidenceRefs...)
	return claims
}

func appendUniqueStrings(base []string, values ...string) []string {
	out := slices.Clone(base)
	for _, value := range values {
		out = appendUnique(out, value)
	}
	return out
}

func splitDebateClaims(claims []DebateClaim, ownerID string) ([]DebateClaim, []DebateClaim) {
	self := make([]DebateClaim, 0)
	peer := make([]DebateClaim, 0)
	for _, claim := range claims {
		if !claim.Active {
			continue
		}
		if claim.OwnerID == ownerID || slices.Contains(claim.ProposedBy, ownerID) {
			self = append(self, claim)
			continue
		}
		peer = append(peer, claim)
	}
	return self, peer
}

func summarizeDebateClaims(claims []DebateClaim) string {
	if len(claims) == 0 {
		return "no peer claims"
	}
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		parts = append(parts, firstNonEmpty(claim.Title, claim.Statement))
	}
	return strings.Join(parts, "; ")
}

func summarizeDebateRound(round DebateRoundRecord) string {
	if len(round.ParticipantOutputs) == 0 {
		return "no participant output"
	}
	parts := make([]string, 0, len(round.ParticipantOutputs))
	for _, item := range round.ParticipantOutputs {
		parts = append(parts, firstNonEmpty(item.Summary, item.AgentID))
	}
	return strings.Join(parts, "; ")
}

func activeDebateClaims(claims []DebateClaim) []DebateClaim {
	out := make([]DebateClaim, 0)
	for _, claim := range claims {
		if claim.Active {
			out = append(out, claim)
		}
	}
	return out
}

// debateVoteCoherent reports whether the coarse vote label agrees with the
// continuous confidence support score defined by the vote prompt contract:
// accept requires confidence >= 0.5, reject requires confidence <= 0.5,
// abstain is unconstrained. Votes that fail this (e.g. reject with 0.9,
// typically a "confidence in my judgment" misreading) are excluded from
// resolution math so they cannot flip an accept decision.
func debateVoteCoherent(vote DebateVoteRecord) bool {
	switch vote.Vote {
	case DebateVoteAccept:
		return vote.Confidence >= 0.5
	case DebateVoteReject:
		return vote.Confidence <= 0.5
	case DebateVoteAbstain:
		return true
	default:
		return false
	}
}

func aggregateSupportScore(values []float64, method DebateVoteAggregation) float64 {
	if len(values) == 0 {
		return 0
	}
	if method == DebateVoteAggregationMean {
		mean, _ := meanAndVariance(values)
		return mean
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func resolveDebateClaims(activeClaims []DebateClaim, allClaims []DebateClaim, votes []DebateVoteRecord, policy DebatePolicy) ([]DebateClaimResolution, FreeDebateOutcome) {
	resolutions := make([]DebateClaimResolution, 0, len(allClaims))
	accepted := 0
	for _, claim := range allClaims {
		if !claim.Active {
			resolutions = append(resolutions, DebateClaimResolution{
				ClaimID:        claim.ClaimID,
				Accepted:       false,
				FinalStatement: claim.Statement,
				MergedInto:     claim.MergedInto,
				ProposedBy:     slices.Clone(claim.ProposedBy),
			})
			continue
		}
		supporters := make([]string, 0)
		opposers := make([]string, 0)
		abstainers := make([]string, 0)
		confidences := make([]float64, 0)
		incoherent := 0
		validVotes := 0
		for _, vote := range votes {
			if vote.ClaimID != claim.ClaimID {
				continue
			}
			if !debateVoteCoherent(vote) {
				incoherent++
				continue
			}
			confidences = append(confidences, vote.Confidence)
			switch vote.Vote {
			case DebateVoteAccept:
				supporters = append(supporters, vote.AgentID)
				validVotes++
			case DebateVoteReject:
				opposers = append(opposers, vote.AgentID)
				validVotes++
			case DebateVoteAbstain:
				abstainers = append(abstainers, vote.AgentID)
			}
		}
		ratio := 0.0
		if validVotes > 0 {
			ratio = float64(len(supporters)) / float64(validVotes)
		}
		supportScore := aggregateSupportScore(confidences, policy.VoteAggregation)
		confidenceMean, confidenceVariance := meanAndVariance(confidences)
		acceptedClaim := len(confidences) > 0 && supportScore >= policy.VoteThreshold
		if acceptedClaim {
			accepted++
		}
		resolutions = append(resolutions, DebateClaimResolution{
			ClaimID:            claim.ClaimID,
			Accepted:           acceptedClaim,
			SupportScore:       supportScore,
			SupportRatio:       ratio,
			ConfidenceMean:     confidenceMean,
			ConfidenceVariance: confidenceVariance,
			ConfidenceStdDev:   math.Sqrt(confidenceVariance),
			VoteCount:          len(confidences),
			IncoherentVotes:    incoherent,
			SupportingVoters:   supporters,
			OpposingVoters:     opposers,
			AbstainingVoters:   abstainers,
			FinalStatement:     claim.Statement,
			ProposedBy:         slices.Clone(claim.ProposedBy),
		})
	}
	switch {
	case len(activeClaims) > 0 && accepted == len(activeClaims):
		return resolutions, FreeDebateOutcomeConsensus
	case accepted > 0:
		return resolutions, FreeDebateOutcomePartialConsensus
	default:
		return resolutions, FreeDebateOutcomeNoConsensus
	}
}

func meanAndVariance(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	varianceSum := 0.0
	for _, value := range values {
		delta := value - mean
		varianceSum += delta * delta
	}
	return mean, varianceSum / float64(len(values))
}

func TaskVerdictFromDebateOutcome(outcome FreeDebateOutcome) TaskVerdict {
	switch outcome {
	case FreeDebateOutcomeConsensus:
		return TaskVerdictSupported
	case FreeDebateOutcomePartialConsensus:
		return TaskVerdictPartiallySupported
	case FreeDebateOutcomeQuorumNotMet:
		return TaskVerdictUndetermined
	default:
		return TaskVerdictUndetermined
	}
}

func buildFreeDebateReport(request StartRequest, section *FreeDebateResultSection) AdjudicationReport {
	highlights := make([]string, 0, len(section.ClaimResolutions))
	for _, resolution := range section.ClaimResolutions {
		if resolution.Accepted {
			highlights = append(highlights, fmt.Sprintf("accepted: %s", resolution.FinalStatement))
		}
	}
	nextActions := make([]string, 0)
	if section.Outcome != FreeDebateOutcomeConsensus {
		nextActions = append(nextActions, "补充更多独立论据，或缩小争议范围")
	}
	return AdjudicationReport{
		Summary:     fmt.Sprintf("自由辩论结果为 %s。任务：%s", section.Outcome, request.TaskSpec.Goal),
		Highlights:  highlights,
		NextActions: nextActions,
	}
}

func aggregateDelphiResponses(existing []DelphiStatement, responses []DelphiResponseRecord, round int, policy DelphiPolicy) ([]DelphiStatement, map[string][]float64) {
	type aggregate struct {
		statement string
		ratings   []float64
		reasons   []string
	}
	index := map[string]int{}
	for idx := range existing {
		index[existing[idx].StatementID] = idx
	}
	aggregates := map[string]*aggregate{}
	for _, response := range responses {
		statementID := strings.TrimSpace(response.StatementID)
		statementText := strings.TrimSpace(response.Statement)
		if statementID == "" {
			statementID = findDelphiStatementID(existing, statementText)
		}
		if statementID == "" {
			statementID = "statement_" + normalizedClaimKey(statementText)
		}
		if statementText == "" {
			if idx, ok := index[statementID]; ok {
				statementText = existing[idx].Statement
			}
		}
		agg, ok := aggregates[statementID]
		if !ok {
			agg = &aggregate{statement: statementText}
			aggregates[statementID] = agg
		}
		agg.ratings = append(agg.ratings, response.Rating)
		if reason := strings.TrimSpace(response.Rationale); reason != "" {
			agg.reasons = append(agg.reasons, reason)
		}
	}
	next := make([]DelphiStatement, 0, len(aggregates))
	distributions := map[string][]float64{}
	for statementID, agg := range aggregates {
		mean := average(agg.ratings)
		consensus := consensusFromRatings(agg.ratings, policy.RatingScaleMin, policy.RatingScaleMax)
		distributions[statementID] = append([]float64(nil), agg.ratings...)
		next = append(next, DelphiStatement{
			StatementID:           statementID,
			Statement:             agg.statement,
			MeanRating:            mean,
			ConsensusLevel:        consensus,
			ResponseCount:         len(agg.ratings),
			LastRound:             round,
			RepresentativeReasons: dedupeStrings(agg.reasons)[:min(3, len(dedupeStrings(agg.reasons)))],
		})
	}
	slices.SortFunc(next, func(left, right DelphiStatement) int {
		if left.MeanRating == right.MeanRating {
			if left.ConsensusLevel == right.ConsensusLevel {
				return strings.Compare(left.StatementID, right.StatementID)
			}
			if left.ConsensusLevel > right.ConsensusLevel {
				return -1
			}
			return 1
		}
		if left.MeanRating > right.MeanRating {
			return -1
		}
		return 1
	})
	return next, distributions
}

func findDelphiStatementID(existing []DelphiStatement, statement string) string {
	key := normalizedClaimKey(statement)
	for _, item := range existing {
		if normalizedClaimKey(item.Statement) == key {
			return item.StatementID
		}
	}
	return ""
}

func consensusFromRatings(ratings []float64, scaleMin int, scaleMax int) float64 {
	if len(ratings) == 0 {
		return 0
	}
	minRating := ratings[0]
	maxRating := ratings[0]
	for _, rating := range ratings[1:] {
		if rating < minRating {
			minRating = rating
		}
		if rating > maxRating {
			maxRating = rating
		}
	}
	span := float64(scaleMax - scaleMin)
	if span <= 0 {
		return 0
	}
	level := 1 - ((maxRating - minRating) / span)
	if level < 0 {
		return 0
	}
	if level > 1 {
		return 1
	}
	return math.Round(level*100) / 100
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return math.Round((total/float64(len(values)))*100) / 100
}

func (e *Engine) buildDelphiSummary(ctx context.Context, request StartRequest, run *workflowRun, round int, statements []DelphiStatement) (string, string, []string, *ArtifactRef, error) {
	builtinSummary := buildDelphiRoundSummary(statements)
	recommendation := ""
	if len(statements) > 0 {
		recommendation = statements[0].Statement
	}
	dissent := buildDelphiDissentSummary(statements)
	if request.Roles.Facilitator == "" || e.deps.TaskDelegate == nil {
		return builtinSummary, recommendation, dissent, nil, nil
	}
	_, awaited, err := e.executeTask(ctx, request, run.sessionID, DelphiFacilitatorSummaryTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.Facilitator,
			Role:      "facilitator",
		},
		TaskSpec:           request.TaskSpec,
		Round:              round,
		StatementSummaries: statements,
	}, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if strings.Contains(err.Error(), "__timeout__") {
			run.metrics.WaitTimeouts++
		}
		return builtinSummary, recommendation, dissent, nil, nil
	}
	typed, ok := awaited.Output.(DelphiFacilitatorSummaryTaskResult)
	if !ok {
		return builtinSummary, recommendation, dissent, awaited.Artifact, nil
	}
	if len(typed.Output.Statements) > 0 {
		statements = typed.Output.Statements
		builtinSummary = buildDelphiRoundSummary(statements)
		if len(statements) > 0 {
			recommendation = statements[0].Statement
		}
		dissent = buildDelphiDissentSummary(statements)
	}
	if typed.Output.Recommendation != "" {
		recommendation = typed.Output.Recommendation
	}
	if len(typed.Output.DissentSummary) > 0 {
		dissent = typed.Output.DissentSummary
	}
	return firstNonEmpty(typed.Output.Summary, builtinSummary), recommendation, dissent, awaited.Artifact, nil
}

func buildDelphiRoundSummary(statements []DelphiStatement) string {
	if len(statements) == 0 {
		return "本轮未形成有效候选结论"
	}
	parts := make([]string, 0, len(statements))
	for _, item := range statements {
		parts = append(parts, fmt.Sprintf("%s(%.2f/%.2f)", item.Statement, item.MeanRating, item.ConsensusLevel))
	}
	return strings.Join(parts, "; ")
}

func bestDelphiStatement(statements []DelphiStatement) (float64, string) {
	if len(statements) == 0 {
		return 0, ""
	}
	return statements[0].ConsensusLevel, statements[0].Statement
}

func buildDelphiDissentSummary(statements []DelphiStatement) []string {
	out := make([]string, 0)
	for _, statement := range statements {
		if statement.ConsensusLevel >= 0.7 {
			continue
		}
		out = append(out, fmt.Sprintf("%s 仍存在明显分歧", statement.Statement))
	}
	return dedupeStrings(out)
}

func buildDelphiReport(request StartRequest, section *DelphiResultSection) AdjudicationReport {
	highlights := make([]string, 0, min(3, len(section.Statements)))
	for _, item := range section.Statements[:min(3, len(section.Statements))] {
		highlights = append(highlights, fmt.Sprintf("%s: %.2f / %.2f", item.Statement, item.MeanRating, item.ConsensusLevel))
	}
	nextActions := make([]string, 0)
	if section.ConsensusLevel < request.DelphiPolicy.ConvergenceThreshold {
		nextActions = append(nextActions, "继续补充材料后再发起下一轮 Delphi")
	}
	return AdjudicationReport{
		Summary:     fmt.Sprintf("Delphi 推荐结论：%s", firstNonEmpty(section.Recommendation, "未形成明确推荐")),
		Highlights:  highlights,
		NextActions: nextActions,
	}
}

func TaskVerdictFromDelphi(section *DelphiResultSection) TaskVerdict {
	switch {
	case section == nil || len(section.Statements) == 0:
		return TaskVerdictFailed
	case section.ConsensusLevel >= 0.8:
		return TaskVerdictSupported
	case section.ConsensusLevel >= 0.5:
		return TaskVerdictPartiallySupported
	default:
		return TaskVerdictUndetermined
	}
}

func cloneDelphiStatements(values []DelphiStatement) []DelphiStatement {
	out := make([]DelphiStatement, len(values))
	copy(out, values)
	return out
}

func finalizeMetrics(metrics Metrics, startedAt time.Time, clock Clock) Metrics {
	metrics.ElapsedMs = clock.Now().Sub(startedAt).Milliseconds()
	return metrics
}
