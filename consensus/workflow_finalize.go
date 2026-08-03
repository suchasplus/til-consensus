package consensus

import (
	"context"
	"errors"
	"strings"
	"time"
)

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

func determineWorkflowTerminalState(request StartRequest, run *workflowRun, result *RunResult) WorkflowTerminalState {
	if result.Action != nil && result.Action.Status == string(TerminalStateActionBlockedByRisk) {
		return TerminalStateActionBlockedByRisk
	}
	switch request.Mode {
	case WorkflowModeAdjudication:
		return DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, result.Action)
	case WorkflowModeFreeDebate:
		if result.FreeDebate == nil || len(result.FreeDebate.Claims) == 0 || result.FreeDebate.BallotSize == 0 {
			return TerminalStateInsufficientEvidence
		}
		switch result.FreeDebate.Outcome {
		case FreeDebateOutcomeConsensus:
			return TerminalStateCompleted
		case FreeDebateOutcomeQuorumNotMet:
			return TerminalStateInsufficientEvidence
		default:
			return TerminalStateUnresolvedConflict
		}
	case WorkflowModeDelphi:
		if result.Delphi == nil || len(result.Delphi.Statements) == 0 || strings.TrimSpace(result.Delphi.Recommendation) == "" {
			return TerminalStateInsufficientEvidence
		}
		if result.Delphi.Converged {
			return TerminalStateCompleted
		}
		if !result.Delphi.ResponseQuorumMet || !result.Delphi.MinResponsesMet {
			return TerminalStateInsufficientEvidence
		}
		return TerminalStateUnresolvedConflict
	default:
		return TerminalStateRequiresHumanReview
	}
}

func syncResultTerminalState(result *RunResult, terminalState WorkflowTerminalState) {
	result.TerminalState = terminalState
	if result.Adjudication != nil {
		result.Adjudication.TerminalState = terminalState
	}
}

func (e *Engine) appendActionEvidence(ctx context.Context, request StartRequest, run *workflowRun, action *ActionOutput) error {
	if action == nil {
		return nil
	}
	_, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindActionGenerated,
		Source:       EvidenceSourceActor,
		ProducerID:   action.ActorID,
		ProducerRole: "actor",
		Summary:      firstNonEmpty(action.Summary, action.Error),
		Metadata: map[string]any{
			"status":   action.Status,
			"executed": action.Executed,
		},
	})
	return err
}

func (e *Engine) finalizeWorkflow(ctx context.Context, request StartRequest, run *workflowRun, result *RunResult) (*RunResult, error) {
	result.CaseManifest = ptr(run.manifest)
	result.Degradations = append([]Degradation(nil), run.degradations...)
	syncResultTerminalState(result, determineWorkflowTerminalState(request, run, result))

	needsActionPhase := request.ActionPolicy != nil || result.Action != nil
	if needsActionPhase && run.state.Current() != SessionPhaseAction && run.state.Current() != SessionPhaseObserve {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAction, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	if request.ActionPolicy != nil && result.Action == nil && run.state.Current() != SessionPhaseObserve {
		result.Action = e.gatedAction(ctx, request, run, result)
		if err := e.appendActionEvidence(ctx, request, run, result.Action); err != nil {
			return nil, err
		}
	}
	if result.Action != nil {
		result.Action.Executed = result.Action.Status == "completed"
	}
	syncResultTerminalState(result, determineWorkflowTerminalState(request, run, result))
	if run.state.Current() == SessionPhaseAction {
		if err := e.saveWorkflowCheckpoint(ctx, request, run, result, SessionPhaseAction); err != nil {
			return nil, err
		}
	}

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
	result.Metrics = finalizeMetrics(run.metrics, run.startedAt, e.clock)
	syncResultTerminalState(result, result.TerminalState)
	if err := e.saveWorkflowCheckpoint(ctx, request, run, result, SessionPhaseObserve); err != nil {
		return nil, err
	}
	if err := e.finishSession(ctx, run.sessionID, run.state, result, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
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
