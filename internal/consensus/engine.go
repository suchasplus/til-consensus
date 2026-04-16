package consensus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const SchemaVersion = 2

var ErrGlobalDeadlineExceeded = errors.New("global deadline exceeded")

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

func (e *Engine) Start(ctx context.Context, input StartRequest) (_ *RunResult, err error) {
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
	switch request.Mode {
	case WorkflowModeAdjudication:
		return e.startAdjudication(ctx, request)
	case WorkflowModeFreeDebate:
		return e.startFreeDebate(ctx, request)
	case WorkflowModeDelphi:
		return e.startDelphi(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported mode: %s", request.Mode)
	}
}

func (e *Engine) executeTask(ctx context.Context, request StartRequest, sessionID string, task Task, startedAt time.Time, timeout time.Duration) (DispatchReceipt, AwaitedTask, error) {
	taskCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, startedAt)
	if deadlineErr != nil {
		return DispatchReceipt{}, AwaitedTask{}, ErrGlobalDeadlineExceeded
	}
	defer cancel()
	receipt, awaited, _, err := ExecuteTaskWithRetry(taskCtx, e.deps.TaskDelegate, task, e.effectivePerTaskTimeout(request, startedAt, timeout), request.WaitingPolicy.RetryAttempts, TaskRetryHooks{
		BeforeDispatch: func(attempt int, maxAttempts int) error {
			return e.emit(ctx, request, sessionID, RunEventTaskDispatched, SessionPhase(""), map[string]any{
				"taskKind":    task.Kind(),
				"agentId":     task.Meta().AgentID,
				"attempt":     attempt,
				"maxAttempts": maxAttempts,
			})
		},
		OnFailure: func(attempt int, maxAttempts int, reason string) error {
			return e.emit(ctx, request, sessionID, RunEventTaskFailed, SessionPhase(""), map[string]any{
				"taskKind":    task.Kind(),
				"agentId":     task.Meta().AgentID,
				"error":       reason,
				"attempt":     attempt,
				"maxAttempts": maxAttempts,
			})
		},
		OnRetry: func(nextAttempt int, maxAttempts int, reason string) error {
			return e.emit(ctx, request, sessionID, RunEventTaskRetrying, SessionPhase(""), map[string]any{
				"taskKind":    task.Kind(),
				"agentId":     task.Meta().AgentID,
				"attempt":     nextAttempt,
				"maxAttempts": maxAttempts,
				"error":       reason,
			})
		},
		OnSuccess: func(attempt int, maxAttempts int) error {
			return e.emit(ctx, request, sessionID, RunEventTaskCompleted, SessionPhase(""), map[string]any{
				"taskKind":    task.Kind(),
				"agentId":     task.Meta().AgentID,
				"attempt":     attempt,
				"maxAttempts": maxAttempts,
			})
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return receipt, AwaitedTask{}, ErrGlobalDeadlineExceeded
		}
		return receipt, awaited, err
	}
	return receipt, awaited, nil
}

func (e *Engine) executeAction(ctx context.Context, request StartRequest, input RunResult, startedAt time.Time) (*ActionOutput, error) {
	if request.ActionPolicy == nil {
		return nil, nil
	}
	actorID := firstNonEmpty(request.ActionPolicy.ActorID, request.Roles.Actor)
	if actorID == "" {
		return nil, fmt.Errorf("action requested but no actor configured")
	}
	actionCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, startedAt)
	if deadlineErr != nil {
		return nil, ErrGlobalDeadlineExceeded
	}
	defer cancel()
	_, awaited, err := e.executeTask(actionCtx, request, input.SessionID, ActionTask{
		TaskMeta: TaskMeta{
			SessionID: input.SessionID,
			RequestID: request.RequestID,
			AgentID:   actorID,
			Role:      "actor",
		},
		Prompt: request.ActionPolicy.Prompt,
		Input:  input,
	}, startedAt, request.WaitingPolicy.PerTaskTimeout)
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

func (e *Engine) buildAdjudicationResult(
	request StartRequest,
	sessionID string,
	manifest CaseManifest,
	claims []ClaimNode,
	tickets []ChallengeTicket,
	verificationResults []VerificationResult,
	revisionRecords []ClaimRevisionRecord,
	adjudicationRecords []AdjudicationRecord,
	arbiter ArbiterReport,
	report AdjudicationReport,
	action *ActionOutput,
	terminalState WorkflowTerminalState,
	observations []ObservationRecord,
	metrics Metrics,
	startedAt time.Time,
	failure *FailureInfo,
) *RunResult {
	if len(adjudicationRecords) == 0 && len(arbiter.Records) > 0 {
		adjudicationRecords = append([]AdjudicationRecord(nil), arbiter.Records...)
	}
	return &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeAdjudication,
		RequestID:     request.RequestID,
		SessionID:     sessionID,
		TaskSpec:      request.TaskSpec,
		CaseManifest:  &manifest,
		TerminalState: terminalState,
		Report:        report,
		Action:        action,
		Observations:  append([]ObservationRecord(nil), observations...),
		Metrics:       finalizeMetrics(metrics, startedAt, e.clock),
		Error: failure,
		Adjudication: &AdjudicationResultSection{
			TaskVerdict:         arbiter.TaskVerdict,
			TerminalState:       terminalState,
			ClaimGraph:          claims,
			ChallengeTickets:    tickets,
			VerificationResults: append([]VerificationResult(nil), verificationResults...),
			RevisionRecords:     append([]ClaimRevisionRecord(nil), revisionRecords...),
			AdjudicationRecords: append([]AdjudicationRecord(nil), adjudicationRecords...),
			ArbiterReport:       arbiter,
		},
	}
}

func (e *Engine) finishSession(ctx context.Context, sessionID string, state *StateMachine, result *RunResult, claims []ClaimNode, tickets []ChallengeTicket, ledgerCursor int) error {
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
	taskVerdict := ""
	if result.Adjudication != nil {
		taskVerdict = string(result.Adjudication.TaskVerdict)
	}
	return e.emit(ctx, StartRequest{RequestID: result.RequestID}, sessionID, RunEventSessionFinalized, state.Current(), map[string]any{
		"mode":        result.Mode,
		"taskVerdict": taskVerdict,
	})
}

func (e *Engine) failSession(ctx context.Context, request StartRequest, sessionID string, state *StateMachine, claims []ClaimNode, tickets []ChallengeTicket, ledgerCursor *int, startedAt time.Time, cause error) error {
	_ = state.Transition(SessionPhaseFailed)
	failure := &FailureInfo{
		Code:    "engine_failed",
		Message: cause.Error(),
	}
	result := &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          request.Mode,
		RequestID:     request.RequestID,
		SessionID:     sessionID,
		TaskSpec:      request.TaskSpec,
		CaseManifest:  ptr(BuildCaseManifest(request)),
		TerminalState: TerminalStateRequiresHumanReview,
		Report: AdjudicationReport{
			Summary: cause.Error(),
		},
		Metrics: finalizeMetrics(Metrics{}, startedAt, e.clock),
		Error: failure,
	}
	if request.Mode == WorkflowModeAdjudication {
		result.Adjudication = &AdjudicationResultSection{
			TaskVerdict:      TaskVerdictFailed,
			TerminalState:    TerminalStateRequiresHumanReview,
			ClaimGraph:       claims,
			ChallengeTickets: tickets,
			ArbiterReport: ArbiterReport{
				TaskVerdict: TaskVerdictFailed,
				Summary:     cause.Error(),
			},
		}
	}
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

func (e *Engine) effectivePerTaskTimeout(request StartRequest, startedAt time.Time, base time.Duration) time.Duration {
	remaining, limited := e.remainingGlobalDeadline(request, startedAt)
	if !limited {
		return base
	}
	if remaining <= 0 {
		return 0
	}
	if base <= 0 || remaining < base {
		return remaining
	}
	return base
}

func (e *Engine) remainingGlobalDeadline(request StartRequest, startedAt time.Time) (time.Duration, bool) {
	if request.WaitingPolicy.GlobalDeadline <= 0 {
		return 0, false
	}
	return request.WaitingPolicy.GlobalDeadline - e.clock.Now().Sub(startedAt), true
}

func (e *Engine) withGlobalDeadline(ctx context.Context, request StartRequest, startedAt time.Time) (context.Context, context.CancelFunc, error) {
	remaining, limited := e.remainingGlobalDeadline(request, startedAt)
	if !limited {
		return ctx, func() {}, nil
	}
	if remaining <= 0 {
		return nil, func() {}, ErrGlobalDeadlineExceeded
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, remaining)
	return deadlineCtx, cancel, nil
}

func (e *Engine) decideArbiter(ctx context.Context, request StartRequest, startedAt time.Time, input ArbiterInput, metrics *Metrics) (ArbiterReport, error) {
	deadlineCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, startedAt)
	if deadlineErr != nil {
		metrics.GlobalDeadlineHit = true
		return NewDefaultArbiter(DefaultArbiterDeps{}).ruleBasedDecision(input), nil
	}
	defer cancel()

	arbiter := e.deps.Arbiter
	if arbiter == nil {
		arbiter = NewDefaultArbiter(DefaultArbiterDeps{
			TaskDelegate:   e.deps.TaskDelegate,
			Clock:          e.clock,
			IDFactory:      e.ids,
			PerTaskTimeout: e.effectivePerTaskTimeout(request, startedAt, request.WaitingPolicy.PerTaskTimeout),
			RetryAttempts:  request.WaitingPolicy.RetryAttempts,
		})
	}
	report, err := arbiter.Decide(deadlineCtx, input)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, ErrGlobalDeadlineExceeded) {
			metrics.GlobalDeadlineHit = true
			return NewDefaultArbiter(DefaultArbiterDeps{}).ruleBasedDecision(input), nil
		}
		return ArbiterReport{}, err
	}
	return report, nil
}

func (e *Engine) composeReport(ctx context.Context, request StartRequest, sessionID string, startedAt time.Time, arbiter ArbiterReport, claims []ClaimNode, tickets []ChallengeTicket, metrics *Metrics) (AdjudicationReport, *ArtifactRef, error) {
	reportCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, startedAt)
	if deadlineErr != nil {
		metrics.GlobalDeadlineHit = true
		return BuildBuiltinReport(request, arbiter, claims, tickets), nil, nil
	}
	defer cancel()

	report, artifact, err := ComposeReport(reportCtx, e.deps.TaskDelegate, request, sessionID, arbiter, claims, tickets, WaitingPolicy{
		PerTaskTimeout: e.effectivePerTaskTimeout(request, startedAt, request.WaitingPolicy.PerTaskTimeout),
		GlobalDeadline: request.WaitingPolicy.GlobalDeadline,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, ErrGlobalDeadlineExceeded) {
			metrics.GlobalDeadlineHit = true
			return BuildBuiltinReport(request, arbiter, claims, tickets), nil, nil
		}
		return AdjudicationReport{}, nil, err
	}
	return report, artifact, nil
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
