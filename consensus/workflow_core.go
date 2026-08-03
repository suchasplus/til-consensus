package consensus

import (
	"context"
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

func finalizeMetrics(metrics Metrics, startedAt time.Time, clock Clock) Metrics {
	metrics.ElapsedMs = clock.Now().Sub(startedAt).Milliseconds()
	return metrics
}
