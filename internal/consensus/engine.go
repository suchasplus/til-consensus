package consensus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = 1

type EngineDeps struct {
	TaskDelegate TaskDelegate
	Verifier     Verifier
	Arbiter      Arbiter
	Observer     Observer
	Ledger       Ledger
	SessionStore SessionStore
	Clock        Clock
	IDFactory    IDFactory
	ArtifactDir  string
}

type Engine struct {
	deps  EngineDeps
	clock Clock
	ids   IDFactory
}

func NewEngine(deps EngineDeps) *Engine {
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	if deps.IDFactory == nil {
		deps.IDFactory = randomIDFactory{}
	}
	return &Engine{
		deps:  deps,
		clock: deps.Clock,
		ids:   deps.IDFactory,
	}
}

func (e *Engine) Start(ctx context.Context, input StartRequest) (_ *AdjudicationResult, err error) {
	if e.deps.TaskDelegate == nil {
		return nil, fmt.Errorf("task delegate is required")
	}
	if e.deps.SessionStore == nil {
		return nil, fmt.Errorf("session store is required")
	}
	request, err := NormalizeStartRequest(input)
	if err != nil {
		return nil, err
	}

	startedAt := e.clock.Now()
	sessionID := e.ids.NewSessionID()
	state := NewStateMachine()
	ledgerEntries := make([]EvidenceRecord, 0, 32)
	ledgerCursor := 0
	claimGraph := make([]ClaimNode, 0)
	challengeTickets := make([]ChallengeTicket, 0)
	verificationResults := make([]VerificationResult, 0)
	metrics := Metrics{}

	if err := e.deps.SessionStore.Save(ctx, SessionSnapshot{
		SessionID:    sessionID,
		RequestID:    request.RequestID,
		Phase:        state.Current(),
		StartedAt:    startedAt.Format(time.RFC3339Nano),
		ClaimGraph:   claimGraph,
		LedgerCursor: ledgerCursor,
	}); err != nil {
		return nil, err
	}

	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, sessionID, state, claimGraph, challengeTickets, &ledgerCursor, startedAt, err)
	}()

	if err := e.emit(ctx, request, sessionID, RunEventSessionStarted, state.Current(), map[string]any{
		"goal":        request.TaskSpec.Goal,
		"proposers":   request.Roles.Proposers,
		"challengers": request.Roles.Challengers,
	}); err != nil {
		return nil, err
	}

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseIngest, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindTaskIngested,
		Source:       EvidenceSourceCoordinator,
		ProducerRole: "coordinator",
		Summary:      "task ingested",
		Metadata: map[string]any{
			"goal":              request.TaskSpec.Goal,
			"successCriteria":   request.TaskSpec.SuccessCriteria,
			"allowedTools":      request.TaskSpec.AllowedTools,
			"workspaceSnapshot": request.TaskSpec.WorkspaceSnapshot,
		},
	}); err != nil {
		return nil, err
	}

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhasePropose, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	for pass := 0; pass < request.ProposalPolicy.MaxPasses; pass++ {
		if e.isGlobalDeadlineHit(request, startedAt) {
			metrics.GlobalDeadlineHit = true
			break
		}
		for _, proposerID := range request.Roles.Proposers {
			receipt, awaited, taskErr := e.executeTask(ctx, request, sessionID, ProposalTask{
				TaskMeta: TaskMeta{
					SessionID: sessionID,
					RequestID: request.RequestID,
					AgentID:   proposerID,
					Role:      "proposer",
					Metadata: map[string]any{
						"pass": pass,
					},
				},
				TaskSpec:       request.TaskSpec,
				Scope:          fmt.Sprintf("proposal pass %d", pass+1),
				MaxClaims:      request.ProposalPolicy.MaxClaimsPerWorker,
				DedupeStrategy: request.ProposalPolicy.DedupeStrategy,
			}, request.WaitingPolicy.PerTaskTimeout)
			metrics.TasksDispatched++
			if taskErr != nil {
				if strings.Contains(taskErr.Error(), "__timeout__") {
					metrics.WaitTimeouts++
				}
				continue
			}
			output, ok := awaited.Output.(ProposalTaskResult)
			if !ok {
				return nil, fmt.Errorf("proposal task returned unexpected result type")
			}
			workerEntry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindWorkerOutput,
				Source:       EvidenceSourceWorker,
				ProducerID:   proposerID,
				ProducerRole: "proposer",
				Summary:      output.Output.Summary,
				Artifact:     awaited.Artifact,
				Metadata: map[string]any{
					"taskId":   receipt.TaskID,
					"taskKind": TaskKindPropose,
				},
			})
			if err != nil {
				return nil, err
			}
			for _, draft := range output.Output.Claims {
				if strings.TrimSpace(draft.Statement) == "" {
					continue
				}
				var created bool
				claimGraph, _, created = UpsertClaim(claimGraph, draft, proposerID, workerEntry.EntryID, e.ids)
				if !created {
					continue
				}
				metrics.ClaimsProposed++
				claim := claimGraph[len(claimGraph)-1]
				entry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
					ClaimID:      claim.ClaimID,
					Kind:         EvidenceKindClaimProposed,
					Source:       EvidenceSourceCoordinator,
					ProducerID:   proposerID,
					ProducerRole: "proposer",
					Summary:      claim.Statement,
					Metadata: map[string]any{
						"title":         claim.Title,
						"scope":         claim.Scope,
						"dependencies":  claim.Dependencies,
						"applicability": claim.Applicability,
					},
				})
				if err != nil {
					return nil, err
				}
				claimGraph = AttachEvidenceToClaim(claimGraph, claim.ClaimID, entry.EntryID)
			}
		}
	}
	SortClaims(claimGraph)
	if err := ValidateClaimDependencies(claimGraph); err != nil {
		return nil, err
	}
	if len(claimGraph) == 0 {
		result := e.buildResult(request, sessionID, claimGraph, challengeTickets, ArbiterReport{
			TaskVerdict: TaskVerdictFailed,
			Summary:     "未产生任何可裁决 claim",
		}, AdjudicationReport{
			Summary: "未产生任何可裁决 claim",
		}, nil, metrics, startedAt, nil)
		if err := e.finishSession(ctx, sessionID, state, result, claimGraph, challengeTickets, ledgerCursor); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseChallenge, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	for _, challengerID := range request.Roles.Challengers {
		if e.isGlobalDeadlineHit(request, startedAt) {
			metrics.GlobalDeadlineHit = true
			break
		}
		receipt, awaited, taskErr := e.executeTask(ctx, request, sessionID, ChallengeTask{
			TaskMeta: TaskMeta{
				SessionID: sessionID,
				RequestID: request.RequestID,
				AgentID:   challengerID,
				Role:      "challenger",
			},
			TaskSpec: request.TaskSpec,
			Claims:   claimGraph,
		}, request.WaitingPolicy.PerTaskTimeout)
		metrics.TasksDispatched++
		if taskErr != nil {
			if strings.Contains(taskErr.Error(), "__timeout__") {
				metrics.WaitTimeouts++
			}
			continue
		}
		output, ok := awaited.Output.(ChallengeTaskResult)
		if !ok {
			return nil, fmt.Errorf("challenge task returned unexpected result type")
		}
		workerEntry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindWorkerOutput,
			Source:       EvidenceSourceWorker,
			ProducerID:   challengerID,
			ProducerRole: "challenger",
			Summary:      output.Output.Summary,
			Artifact:     awaited.Artifact,
			Metadata: map[string]any{
				"taskId":   receipt.TaskID,
				"taskKind": TaskKindChallenge,
			},
		})
		if err != nil {
			return nil, err
		}
		for _, draft := range output.Output.Tickets {
			claim, ok := ResolveClaimRef(claimGraph, draft.ClaimID, draft.Statement)
			if !ok {
				continue
			}
			var created bool
			challengeTickets, _, created = UpsertChallenge(challengeTickets, draft, claim.ClaimID, challengerID, workerEntry.EntryID, e.ids)
			if !created {
				continue
			}
			metrics.ChallengesOpened++
			ticket := challengeTickets[len(challengeTickets)-1]
			entry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
				ClaimID:      claim.ClaimID,
				ChallengeID:  ticket.TicketID,
				Kind:         EvidenceKindChallengeOpened,
				Source:       EvidenceSourceCoordinator,
				ProducerID:   challengerID,
				ProducerRole: "challenger",
				Summary:      ticket.Statement,
				Metadata: map[string]any{
					"kind":            ticket.Kind,
					"requestedChecks": ticket.RequestedChecks,
				},
			})
			if err != nil {
				return nil, err
			}
			challengeTickets[len(challengeTickets)-1].EvidenceRefs = appendUnique(challengeTickets[len(challengeTickets)-1].EvidenceRefs, entry.EntryID)
			claimGraph = AttachChallengeToClaim(claimGraph, claim.ClaimID, ticket.TicketID)
		}
	}
	SortChallenges(challengeTickets)

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseVerify, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	verifier := e.deps.Verifier
	if verifier == nil {
		verifier = NewCompositeVerifier(CompositeVerifierDeps{
			TaskDelegate:   e.deps.TaskDelegate,
			Clock:          e.clock,
			IDFactory:      e.ids,
			ArtifactDir:    e.deps.ArtifactDir,
			PerTaskTimeout: request.WaitingPolicy.PerTaskTimeout,
		})
	}
	for _, claim := range claimGraph {
		if e.isGlobalDeadlineHit(request, startedAt) {
			metrics.GlobalDeadlineHit = true
			break
		}
		findings, verifyErr := verifier.Run(ctx, VerificationRequest{
			Request:    request,
			SessionID:  sessionID,
			Claim:      claim,
			Challenges: selectChallenges(challengeTickets, claim.ClaimID),
		})
		if verifyErr != nil {
			findings = []VerificationResult{{
				VerificationID: e.ids.NewEntityID("verify"),
				ClaimID:        claim.ClaimID,
				Kind:           "verifier_error",
				Status:         VerificationStatusInconclusive,
				Summary:        verifyErr.Error(),
			}}
		}
		claimVerificationRefs := make([]string, 0, len(findings))
		for _, finding := range findings {
			kind := EvidenceKindDeterministicCheck
			if finding.Kind == "semantic" {
				kind = EvidenceKindSemanticVerification
			}
			metadata := map[string]any{
				"kind":              finding.Kind,
				"status":            finding.Status,
				"failureCode":       finding.FailureCode,
				"verdictSuggestion": finding.VerdictSuggestion,
				"confidence":        finding.Confidence,
			}
			for key, value := range finding.Metadata {
				metadata[key] = value
			}
			entry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
				ClaimID:        claim.ClaimID,
				ChallengeID:    finding.ChallengeID,
				VerificationID: finding.VerificationID,
				Kind:           kind,
				Source:         EvidenceSourceVerifier,
				ProducerRole:   "verifier",
				Summary:        finding.Summary,
				Artifact:       finding.Artifact,
				Metadata:       metadata,
			})
			if err != nil {
				return nil, err
			}
			finding.EvidenceRef = entry.EntryID
			verificationResults = append(verificationResults, finding)
			claimVerificationRefs = append(claimVerificationRefs, entry.EntryID)
			metrics.VerificationsRun++
		}
		for _, verificationRef := range claimVerificationRefs {
			claimGraph = AttachVerificationToClaim(claimGraph, claim.ClaimID, verificationRef)
		}
		challengeTickets = CloseChallenges(challengeTickets, claim.ClaimID, claimVerificationRefs, "verification completed")
	}

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseAdjudicate, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	arbiter := e.deps.Arbiter
	if arbiter == nil {
		arbiter = NewDefaultArbiter(DefaultArbiterDeps{
			TaskDelegate:   e.deps.TaskDelegate,
			Clock:          e.clock,
			IDFactory:      e.ids,
			PerTaskTimeout: request.WaitingPolicy.PerTaskTimeout,
		})
	}
	arbiterReport, err := arbiter.Decide(ctx, ArbiterInput{
		Request:    request,
		SessionID:  sessionID,
		Claims:     claimGraph,
		Challenges: challengeTickets,
		Ledger:     ledgerEntries,
		Findings:   verificationResults,
	})
	if err != nil {
		return nil, err
	}
	for idx, decision := range arbiterReport.Decisions {
		entry, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
			ClaimID:      decision.ClaimID,
			Kind:         EvidenceKindArbiterDecision,
			Source:       EvidenceSourceArbiter,
			ProducerID:   request.Roles.Arbiter,
			ProducerRole: "arbiter",
			Summary:      decision.Rationale,
			Metadata: map[string]any{
				"verdict":    decision.Verdict,
				"confidence": decision.Confidence,
			},
		})
		if err != nil {
			return nil, err
		}
		arbiterReport.Decisions[idx].EvidenceRefs = appendUnique(decision.EvidenceRefs, entry.EntryID)
	}
	claimGraph = ApplyDecisions(claimGraph, arbiterReport.Decisions)

	if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseReport, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	report, reportArtifact, err := ComposeReport(ctx, e.deps.TaskDelegate, request, sessionID, arbiterReport, claimGraph, challengeTickets, request.WaitingPolicy)
	if err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindReportGenerated,
		Source:       EvidenceSourceReporter,
		ProducerID:   request.Roles.Reporter,
		ProducerRole: "reporter",
		Summary:      report.Summary,
		Artifact:     reportArtifact,
	}); err != nil {
		return nil, err
	}

	var actionOutput *ActionOutput
	if request.ActionPolicy != nil {
		if err := e.advancePhase(ctx, request, sessionID, state, SessionPhaseAction, claimGraph, challengeTickets, ledgerCursor); err != nil {
			return nil, err
		}
		actionResult, actionErr := e.executeAction(ctx, request, sessionID, claimGraph, challengeTickets, arbiterReport, report, startedAt)
		if actionErr != nil {
			actionOutput = &ActionOutput{
				ActorID: firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor),
				Status:  "failed",
				Error:   actionErr.Error(),
			}
		} else {
			actionOutput = actionResult
		}
		if actionOutput != nil {
			if _, err := e.appendEvidence(ctx, request, sessionID, &ledgerEntries, &ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindActionGenerated,
				Source:       EvidenceSourceActor,
				ProducerID:   actionOutput.ActorID,
				ProducerRole: "actor",
				Summary:      actionOutput.Summary,
				Metadata: map[string]any{
					"status": actionOutput.Status,
				},
			}); err != nil {
				return nil, err
			}
		}
	}

	result := e.buildResult(request, sessionID, claimGraph, challengeTickets, arbiterReport, report, actionOutput, metrics, startedAt, nil)
	if err := e.finishSession(ctx, sessionID, state, result, claimGraph, challengeTickets, ledgerCursor); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) executeTask(ctx context.Context, request StartRequest, sessionID string, task Task, timeout time.Duration) (DispatchReceipt, AwaitedTask, error) {
	if err := e.emit(ctx, request, sessionID, RunEventTaskDispatched, SessionPhase(""), map[string]any{
		"taskKind": task.Kind(),
		"agentId":  task.Meta().AgentID,
	}); err != nil {
		return DispatchReceipt{}, AwaitedTask{}, err
	}
	receipt, err := e.deps.TaskDelegate.Dispatch(ctx, task)
	if err != nil {
		_ = e.emit(ctx, request, sessionID, RunEventTaskFailed, SessionPhase(""), map[string]any{
			"taskKind": task.Kind(),
			"agentId":  task.Meta().AgentID,
			"error":    err.Error(),
		})
		return DispatchReceipt{}, AwaitedTask{}, err
	}
	awaited, err := e.deps.TaskDelegate.Await(ctx, receipt.TaskID, timeout)
	if err != nil {
		_ = e.emit(ctx, request, sessionID, RunEventTaskFailed, SessionPhase(""), map[string]any{
			"taskKind": task.Kind(),
			"agentId":  task.Meta().AgentID,
			"error":    err.Error(),
		})
		return receipt, AwaitedTask{}, err
	}
	if !awaited.OK {
		_ = e.emit(ctx, request, sessionID, RunEventTaskFailed, SessionPhase(""), map[string]any{
			"taskKind": task.Kind(),
			"agentId":  task.Meta().AgentID,
			"error":    awaited.Error,
		})
		return receipt, awaited, errors.New(awaited.Error)
	}
	if err := e.emit(ctx, request, sessionID, RunEventTaskCompleted, SessionPhase(""), map[string]any{
		"taskKind": task.Kind(),
		"agentId":  task.Meta().AgentID,
	}); err != nil {
		return receipt, awaited, err
	}
	return receipt, awaited, nil
}

func (e *Engine) executeAction(ctx context.Context, request StartRequest, sessionID string, claims []ClaimNode, tickets []ChallengeTicket, arbiter ArbiterReport, report AdjudicationReport, startedAt time.Time) (*ActionOutput, error) {
	actorID := firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor)
	if actorID == "" {
		return nil, fmt.Errorf("action requested but no actor configured")
	}
	input := AdjudicationResult{
		SchemaVersion:    SchemaVersion,
		RequestID:        request.RequestID,
		SessionID:        sessionID,
		TaskSpec:         request.TaskSpec,
		TaskVerdict:      arbiter.TaskVerdict,
		ClaimGraph:       claims,
		ChallengeTickets: tickets,
		ArbiterReport:    arbiter,
		Report:           report,
		Metrics: Metrics{
			ElapsedMs: e.clock.Now().Sub(startedAt).Milliseconds(),
		},
	}
	_, awaited, err := e.executeTask(ctx, request, sessionID, ActionTask{
		TaskMeta: TaskMeta{
			SessionID: sessionID,
			RequestID: request.RequestID,
			AgentID:   actorID,
			Role:      "actor",
		},
		Prompt: request.ActionPolicy.Prompt,
		Input:  input,
	}, request.WaitingPolicy.PerTaskTimeout)
	if err != nil {
		return nil, err
	}
	typed, ok := awaited.Output.(ActionTaskResult)
	if !ok {
		return nil, fmt.Errorf("action task returned unexpected result type")
	}
	return &ActionOutput{
		ActorID:      actorID,
		Status:       "completed",
		FullResponse: typed.Output.FullResponse,
		Summary:      typed.Output.Summary,
	}, nil
}

func (e *Engine) buildResult(request StartRequest, sessionID string, claims []ClaimNode, tickets []ChallengeTicket, arbiter ArbiterReport, report AdjudicationReport, action *ActionOutput, metrics Metrics, startedAt time.Time, failure *FailureInfo) *AdjudicationResult {
	return &AdjudicationResult{
		SchemaVersion:    SchemaVersion,
		RequestID:        request.RequestID,
		SessionID:        sessionID,
		TaskSpec:         request.TaskSpec,
		TaskVerdict:      arbiter.TaskVerdict,
		ClaimGraph:       claims,
		ChallengeTickets: tickets,
		ArbiterReport:    arbiter,
		Report:           report,
		Action:           action,
		Metrics: Metrics{
			ElapsedMs:         e.clock.Now().Sub(startedAt).Milliseconds(),
			ClaimsProposed:    metrics.ClaimsProposed,
			ChallengesOpened:  metrics.ChallengesOpened,
			VerificationsRun:  metrics.VerificationsRun,
			TasksDispatched:   metrics.TasksDispatched,
			WaitTimeouts:      metrics.WaitTimeouts,
			GlobalDeadlineHit: metrics.GlobalDeadlineHit,
		},
		Error: failure,
	}
}

func (e *Engine) finishSession(ctx context.Context, sessionID string, state *StateMachine, result *AdjudicationResult, claims []ClaimNode, tickets []ChallengeTicket, ledgerCursor int) error {
	if err := state.Transition(SessionPhaseFinished); err != nil {
		return err
	}
	finishedAt := e.clock.Now().Format(time.RFC3339Nano)
	if err := e.deps.SessionStore.Patch(ctx, sessionID, SessionPatch{
		Phase:            ptr(state.Current()),
		ClaimGraph:       claims,
		ChallengeTickets: tickets,
		LedgerCursor:     &ledgerCursor,
		FinishedAt:       &finishedAt,
		Result:           result,
	}); err != nil {
		return err
	}
	return e.emit(ctx, StartRequest{RequestID: result.RequestID}, sessionID, RunEventSessionFinalized, state.Current(), map[string]any{
		"taskVerdict": result.TaskVerdict,
	})
}

func (e *Engine) failSession(ctx context.Context, request StartRequest, sessionID string, state *StateMachine, claims []ClaimNode, tickets []ChallengeTicket, ledgerCursor *int, startedAt time.Time, cause error) error {
	_ = state.Transition(SessionPhaseFailed)
	failure := &FailureInfo{
		Code:    "engine_failed",
		Message: cause.Error(),
	}
	result := e.buildResult(request, sessionID, claims, tickets, ArbiterReport{
		TaskVerdict: TaskVerdictFailed,
		Summary:     cause.Error(),
	}, AdjudicationReport{
		Summary: cause.Error(),
	}, nil, Metrics{}, startedAt, failure)
	failedAt := e.clock.Now().Format(time.RFC3339Nano)
	if err := e.deps.SessionStore.Patch(ctx, sessionID, SessionPatch{
		Phase:            ptr(state.Current()),
		ClaimGraph:       claims,
		ChallengeTickets: tickets,
		LedgerCursor:     ledgerCursor,
		FailedAt:         &failedAt,
		Result:           result,
		Error:            failure,
	}); err != nil {
		return err
	}
	return e.emit(ctx, request, sessionID, RunEventSessionFailed, state.Current(), map[string]any{
		"error": cause.Error(),
	})
}

func (e *Engine) appendEvidence(ctx context.Context, request StartRequest, sessionID string, entries *[]EvidenceRecord, cursor *int, entry EvidenceRecord) (EvidenceRecord, error) {
	entry.SchemaVersion = SchemaVersion
	entry.RequestID = request.RequestID
	entry.SessionID = sessionID
	entry.CreatedAt = e.clock.Now().Format(time.RFC3339Nano)
	if entry.EntryID == "" {
		entry.EntryID = e.ids.NewEntityID("ledger")
	}
	entry.Seq = *cursor
	if e.deps.Ledger != nil {
		record, err := e.deps.Ledger.Append(ctx, entry)
		if err != nil {
			return EvidenceRecord{}, err
		}
		entry = record
	}
	*entries = append(*entries, entry)
	*cursor = *cursor + 1
	if err := e.emit(ctx, request, sessionID, RunEventLedgerAppended, SessionPhase(""), map[string]any{
		"entryId": entry.EntryID,
		"kind":    entry.Kind,
		"claimId": entry.ClaimID,
	}); err != nil {
		return EvidenceRecord{}, err
	}
	return entry, nil
}

func (e *Engine) advancePhase(ctx context.Context, request StartRequest, sessionID string, state *StateMachine, next SessionPhase, claims []ClaimNode, tickets []ChallengeTicket, ledgerCursor int) error {
	if err := state.Transition(next); err != nil {
		return err
	}
	if err := e.deps.SessionStore.Patch(ctx, sessionID, SessionPatch{
		Phase:            ptr(state.Current()),
		ClaimGraph:       claims,
		ChallengeTickets: tickets,
		LedgerCursor:     &ledgerCursor,
	}); err != nil {
		return err
	}
	return e.emit(ctx, request, sessionID, RunEventPhaseChanged, state.Current(), nil)
}

func (e *Engine) emit(ctx context.Context, request StartRequest, sessionID string, eventType RunEventType, phase SessionPhase, payload map[string]any) error {
	if e.deps.Observer == nil {
		return nil
	}
	return e.deps.Observer.OnEvent(ctx, RunEvent{
		SessionID: sessionID,
		RequestID: request.RequestID,
		Type:      eventType,
		Phase:     phase,
		At:        e.clock.Now().Format(time.RFC3339Nano),
		Payload:   payload,
	})
}

func (e *Engine) isGlobalDeadlineHit(request StartRequest, startedAt time.Time) bool {
	if request.WaitingPolicy.GlobalDeadline <= 0 {
		return false
	}
	return e.clock.Now().Sub(startedAt) >= request.WaitingPolicy.GlobalDeadline
}

func selectChallenges(tickets []ChallengeTicket, claimID string) []ChallengeTicket {
	out := make([]ChallengeTicket, 0)
	for _, ticket := range tickets {
		if ticket.ClaimID == claimID {
			out = append(out, ticket)
		}
	}
	return out
}

func ptr[T any](value T) *T {
	return &value
}

type randomIDFactory struct{}

func (randomIDFactory) NewSessionID() string {
	return "session_" + randomHex(6)
}

func (randomIDFactory) NewEntityID(prefix string) string {
	return prefix + "_" + randomHex(6)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
