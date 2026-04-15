package consensus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const ResultVersion = 1

type EngineDeps struct {
	TaskDelegate    TaskDelegate
	Observer        Observer
	WaitCoordinator WaitCoordinator
	SessionStore    SessionStore
	Clock           Clock
	IDFactory       IDFactory
}

type Engine struct {
	deps  EngineDeps
	wait  WaitCoordinator
	store SessionStore
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
	engine := &Engine{
		deps:  deps,
		clock: deps.Clock,
		ids:   deps.IDFactory,
	}
	if deps.WaitCoordinator != nil {
		engine.wait = deps.WaitCoordinator
	}
	if deps.SessionStore != nil {
		engine.store = deps.SessionStore
	}
	return engine
}

func (e *Engine) Start(ctx context.Context, input StartRequest) (_ *ConsensusResult, err error) {
	if e.deps.TaskDelegate == nil {
		return nil, fmt.Errorf("task delegate is required")
	}
	if e.wait == nil {
		e.wait = NewDefaultWaitCoordinator(e.deps.TaskDelegate)
	}
	if e.store == nil {
		return nil, fmt.Errorf("session store is required")
	}
	req, err := NormalizeStartRequest(input)
	if err != nil {
		return nil, err
	}
	startedAt := e.clock.Now()
	sessionID := e.ids.NewSessionID()
	state := NewStateMachine()
	allParticipants := make([]string, 0, len(req.Participants))
	activeParticipants := map[string]struct{}{}
	participantSessionMap := map[string]string{}
	for _, participant := range req.Participants {
		allParticipants = append(allParticipants, participant.ID)
		activeParticipants[participant.ID] = struct{}{}
		participantSessionMap[participant.ID] = req.SessionPolicy.SessionKeyPrefix + ":" + sessionID + ":" + participant.ID
	}

	rounds := make([]RoundRecord, 0)
	claimCatalog := make([]Claim, 0)
	eliminations := make([]EliminationRecord, 0)
	metrics := Metrics{}

	if err := e.store.Save(ctx, SessionSnapshot{
		SessionID:          sessionID,
		RequestID:          req.RequestID,
		Participants:       slices.Clone(allParticipants),
		ActiveParticipants: activeParticipantList(activeParticipants),
		Eliminations:       slices.Clone(eliminations),
		State:              state.Current(),
		StartedAt:          startedAt.Format(time.RFC3339Nano),
	}); err != nil {
		return nil, err
	}

	defer func() {
		if err == nil {
			return
		}
		_ = e.handleFailure(ctx, err, req, sessionID, state, activeParticipants, eliminations)
	}()

	if err := state.Transition(SessionStateRunning); err != nil {
		return nil, err
	}
	runningAt := e.clock.Now().Format(time.RFC3339Nano)
	if err := e.store.Patch(ctx, sessionID, SessionPatch{
		State:     ptr(state.Current()),
		RunningAt: &runningAt,
	}); err != nil {
		return nil, err
	}
	if err := e.emit(ctx, req, sessionID, EventSessionStarted, map[string]any{
		"participants":    slices.Clone(allParticipants),
		"minParticipants": req.ParticipantsPolicy.MinParticipants,
	}); err != nil {
		return nil, err
	}

	initial, err := e.runRound(ctx, runRoundArgs{
		Request:               req,
		SessionID:             sessionID,
		ParticipantSessionMap: participantSessionMap,
		ActiveParticipants:    activeParticipants,
		Phase:                 PhaseInitial,
		Round:                 0,
		ClaimCatalog:          claimCatalog,
		PreviousOutputs:       nil,
		StartedAt:             startedAt,
		Metrics:               &metrics,
		Eliminations:          &eliminations,
	})
	if err != nil {
		return nil, err
	}
	if err := ensureMinParticipants(req, len(activeParticipants)); err != nil {
		return nil, err
	}
	rounds = append(rounds, RoundRecord{Round: 0, Outputs: initial.Outputs, AppliedMerges: initial.AppliedMerges})
	claimCatalog = initial.Claims
	previousOutputs := initial.Outputs

	for round := 1; round <= req.RoundPolicy.MaxRounds; round++ {
		if e.isGlobalDeadlineHit(req, startedAt) {
			metrics.GlobalDeadlineHit = true
			if err := e.emit(ctx, req, sessionID, EventGlobalDeadlineHit, map[string]any{
				"round":            round,
				"globalDeadlineMs": req.WaitingPolicy.GlobalDeadline.Milliseconds(),
			}); err != nil {
				return nil, err
			}
			break
		}
		debate, err := e.runRound(ctx, runRoundArgs{
			Request:               req,
			SessionID:             sessionID,
			ParticipantSessionMap: participantSessionMap,
			ActiveParticipants:    activeParticipants,
			Phase:                 PhaseDebate,
			Round:                 round,
			ClaimCatalog:          claimCatalog,
			PreviousOutputs:       previousOutputs,
			StartedAt:             startedAt,
			Metrics:               &metrics,
			Eliminations:          &eliminations,
		})
		if err != nil {
			return nil, err
		}
		if err := ensureMinParticipants(req, len(activeParticipants)); err != nil {
			return nil, err
		}
		rounds = append(rounds, RoundRecord{Round: round, Outputs: debate.Outputs, AppliedMerges: debate.AppliedMerges})
		claimCatalog = debate.Claims
		previousOutputs = debate.Outputs
		if round >= req.RoundPolicy.MinRounds && ShouldEarlyStop(debate.Outputs, debate.NewClaimCount) {
			metrics.EarlyStopTriggered = true
			if err := e.emit(ctx, req, sessionID, EventEarlyStopTriggered, map[string]any{
				"round":  round,
				"reason": "all_agree_no_new_claims",
			}); err != nil {
				return nil, err
			}
			break
		}
	}

	finalVoteRound := 0
	if len(rounds) > 0 {
		finalVoteRound = rounds[len(rounds)-1].Round + 1
	}
	finalVoteOutputs := make([]ParticipantRoundOutput, 0)
	if !metrics.GlobalDeadlineHit {
		if e.isGlobalDeadlineHit(req, startedAt) {
			metrics.GlobalDeadlineHit = true
			if err := e.emit(ctx, req, sessionID, EventGlobalDeadlineHit, map[string]any{
				"round":            finalVoteRound,
				"globalDeadlineMs": req.WaitingPolicy.GlobalDeadline.Milliseconds(),
			}); err != nil {
				return nil, err
			}
		} else {
			finalVote, err := e.runRound(ctx, runRoundArgs{
				Request:               req,
				SessionID:             sessionID,
				ParticipantSessionMap: participantSessionMap,
				ActiveParticipants:    activeParticipants,
				Phase:                 PhaseFinalVote,
				Round:                 finalVoteRound,
				ClaimCatalog:          claimCatalog,
				PreviousOutputs:       previousOutputs,
				StartedAt:             startedAt,
				Metrics:               &metrics,
				Eliminations:          &eliminations,
			})
			if err != nil {
				return nil, err
			}
			if err := ensureMinParticipants(req, len(activeParticipants)); err != nil {
				return nil, err
			}
			finalVoteOutputs = finalVote.Outputs
			rounds = append(rounds, RoundRecord{
				Round:         finalVoteRound,
				Outputs:       finalVoteOutputs,
				AppliedMerges: finalVote.AppliedMerges,
			})
			claimCatalog = finalVote.Claims
		}
	}

	claimResolutions := BuildClaimResolutions(claimCatalog, finalVoteOutputs, req.ConsensusPolicy.Threshold, metrics.GlobalDeadlineHit)
	if err := e.emit(ctx, req, sessionID, EventConsensusDrafted, map[string]any{
		"claimCount":    countActiveClaims(claimCatalog),
		"resolvedCount": countResolvedClaims(claimResolutions),
	}); err != nil {
		return nil, err
	}

	scoreboard := ComputeParticipantScores(allParticipants, rounds, claimCatalog, req.ScoringPolicy)
	activeScoreboard := make([]ParticipantScore, 0)
	for _, score := range scoreboard {
		if _, ok := activeParticipants[score.ParticipantID]; ok {
			activeScoreboard = append(activeScoreboard, score)
		}
	}
	if err := ensureMinParticipants(req, len(activeScoreboard)); err != nil {
		return nil, err
	}
	var representative Representative
	if req.ReportPolicy.RepresentativeID != "" {
		if _, ok := activeParticipants[req.ReportPolicy.RepresentativeID]; ok {
			representative = Representative{
				ParticipantID: req.ReportPolicy.RepresentativeID,
				Reason:        RepresentativeReasonHostDesignated,
				Score:         scoreFor(scoreboard, req.ReportPolicy.RepresentativeID),
			}
		}
	}
	if representative.ParticipantID == "" {
		chosen, err := ChooseRepresentative(activeScoreboard, rounds, req.ScoringPolicy.TieBreaker)
		if err != nil {
			return nil, err
		}
		representative = chosen
	}
	representative.Speech = pickRepresentativeSpeech(representative.ParticipantID, rounds)
	status := AggregateSessionStatus(claimResolutions, metrics.GlobalDeadlineHit)

	report, err := e.composeReport(ctx, composeReportArgs{
		Request:            req,
		RequestID:          req.RequestID,
		SessionID:          sessionID,
		Status:             status,
		Representative:     representative,
		FinalClaims:        claimCatalog,
		ClaimResolutions:   claimResolutions,
		Rounds:             rounds,
		Scoreboard:         scoreboard,
		ActiveParticipants: activeParticipants,
	})
	if err != nil {
		return nil, err
	}

	if err := state.Transition(SessionStateFinalizing); err != nil {
		return nil, err
	}
	finalizingAt := e.clock.Now().Format(time.RFC3339Nano)
	if err := e.store.Patch(ctx, sessionID, SessionPatch{
		State:        ptr(state.Current()),
		FinalizingAt: &finalizingAt,
	}); err != nil {
		return nil, err
	}

	result := &ConsensusResult{
		ResultVersion: ResultVersion,
		RequestID:     req.RequestID,
		SessionID:     sessionID,
		Task: ConsensusTask{
			Prompt: req.Task,
			Title:  SelectTaskTitle(rounds, representative.ParticipantID, scoreboard, req.Task),
		},
		Status:           status,
		FinalClaims:      claimCatalog,
		ClaimResolutions: claimResolutions,
		Representative:   representative,
		Scoreboard:       scoreboard,
		Eliminations:     eliminations,
		Report:           report,
		Disagreements:    CollectDisagreements(rounds),
		Rounds:           rounds,
		Metrics: Metrics{
			ElapsedMs:          e.clock.Now().Sub(startedAt).Milliseconds(),
			TotalRounds:        len(rounds),
			TotalTurns:         metrics.TotalTurns,
			Retries:            metrics.Retries,
			WaitTimeouts:       metrics.WaitTimeouts,
			EarlyStopTriggered: metrics.EarlyStopTriggered,
			GlobalDeadlineHit:  metrics.GlobalDeadlineHit,
		},
	}
	actionOutput, err := e.executeAction(ctx, executeActionArgs{
		Request:            req,
		RequestID:          req.RequestID,
		SessionID:          sessionID,
		Result:             result,
		ActiveParticipants: activeParticipants,
	})
	if err != nil {
		return nil, err
	}
	if actionOutput != nil {
		result.Action = actionOutput
	}

	if err := state.Transition(SessionStateFinished); err != nil {
		return nil, err
	}
	finishedAt := e.clock.Now().Format(time.RFC3339Nano)
	if err := e.store.Patch(ctx, sessionID, SessionPatch{
		State:              ptr(state.Current()),
		Result:             result,
		ActiveParticipants: activeParticipantList(activeParticipants),
		Eliminations:       slices.Clone(eliminations),
		FinishedAt:         &finishedAt,
	}); err != nil {
		return nil, err
	}
	if err := e.emit(ctx, req, sessionID, EventFinalized, map[string]any{
		"status":         status,
		"representative": representative.ParticipantID,
	}); err != nil {
		return nil, err
	}
	return result, nil
}

type runRoundArgs struct {
	Request               StartRequest
	SessionID             string
	ParticipantSessionMap map[string]string
	ActiveParticipants    map[string]struct{}
	Phase                 Phase
	Round                 int
	ClaimCatalog          []Claim
	PreviousOutputs       []ParticipantRoundOutput
	StartedAt             time.Time
	Metrics               *Metrics
	Eliminations          *[]EliminationRecord
}

type runRoundResult struct {
	Outputs       []ParticipantRoundOutput
	Claims        []Claim
	NewClaimCount int
	AppliedMerges []RoundAppliedMerge
}

func (e *Engine) runRound(ctx context.Context, args runRoundArgs) (runRoundResult, error) {
	participants := activeParticipantList(args.ActiveParticipants)
	dispatches := make([]WaitTask, 0, len(participants))
	for _, participantID := range participants {
		participant, ok := findParticipant(args.Request.Participants, participantID)
		if !ok {
			return runRoundResult{}, fmt.Errorf("missing participant config for %s", participantID)
		}
		peerInputs := make([]PeerRoundInput, 0)
		for _, output := range args.PreviousOutputs {
			if output.ParticipantID == participantID {
				continue
			}
			text, truncated := ApplyTextBudget(output.FullResponse, args.Request.PeerContextPolicy.MaxCharsPerPeerResponse, args.Request.PeerContextPolicy.OverflowStrategy)
			peerInputs = append(peerInputs, PeerRoundInput{
				ParticipantID: output.ParticipantID,
				Round:         output.Round,
				FullResponse:  text,
				Truncated:     truncated,
			})
			if len(peerInputs) >= args.Request.PeerContextPolicy.MaxPeersPerRound {
				break
			}
		}
		visibleClaims := slices.Clone(args.ClaimCatalog)
		if args.Phase != PhaseInitial {
			visibleClaims = filterActiveClaims(args.ClaimCatalog)
		}
		task := RoundTask{
			TaskMeta: TaskMeta{
				SessionID:     args.SessionID,
				RequestID:     args.Request.RequestID,
				ParticipantID: participantID,
				Metadata: map[string]any{
					"participantSessionKey": args.ParticipantSessionMap[participantID],
					"role":                  participant.Role,
					"constraints":           args.Request.Constraints,
					"context":               args.Request.Context,
				},
			},
			Phase:           args.Phase,
			Round:           args.Round,
			Prompt:          e.buildRoundPrompt(args.Request, args.Phase, args.Round),
			SelfHistoryRef:  &SelfHistoryRef{StickySession: true},
			PeerRoundInputs: peerInputs,
			ClaimCatalog:    visibleClaims,
		}
		dispatched, err := e.deps.TaskDelegate.Dispatch(ctx, task)
		if err != nil {
			return runRoundResult{}, fmt.Errorf("dispatch round task for %s: %w", participantID, err)
		}
		dispatches = append(dispatches, WaitTask{
			TaskID:        dispatched.TaskID,
			ParticipantID: participantID,
		})
	}
	taskIDs := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
		taskIDs = append(taskIDs, dispatch.TaskID)
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventRoundDispatched, map[string]any{
		"phase":        args.Phase,
		"round":        args.Round,
		"participants": participants,
		"taskIds":      taskIDs,
	}); err != nil {
		return runRoundResult{}, err
	}

	waited, err := e.wait.WaitRound(ctx, WaitRoundRequest{
		Round:  args.Round,
		Tasks:  dispatches,
		Policy: e.resolveRoundWaitingPolicy(args.Request, args.StartedAt),
		OnTaskSettled: func(ctx context.Context, settled TaskSettled) error {
			if settled.Status == "completed" {
				args.Metrics.TotalTurns++
				payload := map[string]any{
					"phase":           args.Phase,
					"round":           args.Round,
					"participantId":   settled.ParticipantID,
					"summary":         settled.Output.Summary,
					"extractedClaims": len(settled.Output.ExtractedClaims),
					"judgements":      len(settled.Output.Judgements),
					"fullResponse":    settled.Output.FullResponse,
				}
				return e.emit(ctx, args.Request, args.SessionID, EventParticipantResponded, payload)
			}
			reason := "error"
			if settled.Status == "timeout" {
				reason = "timeout"
				args.Metrics.WaitTimeouts++
			}
			if _, ok := args.ActiveParticipants[settled.ParticipantID]; !ok {
				return nil
			}
			delete(args.ActiveParticipants, settled.ParticipantID)
			*args.Eliminations = append(*args.Eliminations, EliminationRecord{
				ParticipantID: settled.ParticipantID,
				Round:         args.Round,
				Reason:        reason,
				At:            e.clock.Now().Format(time.RFC3339Nano),
			})
			return e.emit(ctx, args.Request, args.SessionID, EventParticipantEliminated, map[string]any{
				"phase":         args.Phase,
				"round":         args.Round,
				"participantId": settled.ParticipantID,
				"reason":        reason,
				"error":         settled.Error,
			})
		},
	})
	if err != nil {
		return runRoundResult{}, err
	}

	outputs := make([]ParticipantRoundOutput, 0, len(waited.Completed))
	for _, output := range waited.Completed {
		if output.Phase == args.Phase && output.Round == args.Round {
			if _, ok := args.ActiveParticipants[output.ParticipantID]; ok {
				outputs = append(outputs, output)
			}
		}
	}
	claims := slices.Clone(args.ClaimCatalog)
	newClaimCount := 0
	mergeEvents := make([]RoundAppliedMerge, 0)
	if args.Phase != PhaseFinalVote {
		claims, newClaimCount, mergeEvents = UpdateClaims(args.ClaimCatalog, outputs)
	}
	for _, merge := range mergeEvents {
		if err := e.emit(ctx, args.Request, args.SessionID, EventClaimsMerged, map[string]any{
			"phase":         args.Phase,
			"round":         args.Round,
			"sourceClaimId": merge.SourceClaimID,
			"mergedInto":    merge.TargetClaimID,
		}); err != nil {
			return runRoundResult{}, err
		}
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventRoundCompleted, map[string]any{
		"phase":              args.Phase,
		"round":              args.Round,
		"completed":          len(outputs),
		"timedOut":           len(waited.TimedOut),
		"failed":             len(waited.Failed),
		"activeParticipants": activeParticipantList(args.ActiveParticipants),
		"claimCatalogSize":   len(claims),
		"newClaims":          newClaimCount,
		"mergeCount":         len(mergeEvents),
	}); err != nil {
		return runRoundResult{}, err
	}
	if err := e.store.Patch(ctx, args.SessionID, SessionPatch{
		ActiveParticipants: activeParticipantList(args.ActiveParticipants),
		Eliminations:       slices.Clone(*args.Eliminations),
	}); err != nil {
		return runRoundResult{}, err
	}
	return runRoundResult{
		Outputs:       outputs,
		Claims:        claims,
		NewClaimCount: newClaimCount,
		AppliedMerges: mergeEvents,
	}, nil
}

type composeReportArgs struct {
	Request            StartRequest
	RequestID          string
	SessionID          string
	Status             ConsensusStatus
	Representative     Representative
	FinalClaims        []Claim
	ClaimResolutions   []ClaimResolution
	Rounds             []RoundRecord
	Scoreboard         []ParticipantScore
	ActiveParticipants map[string]struct{}
}

func (e *Engine) composeReport(ctx context.Context, args composeReportArgs) (FinalReport, error) {
	fallback := func() FinalReport {
		return BuildBuiltinReport(
			args.Request.ReportPolicy.IncludeDeliberationTrace,
			args.Request.ReportPolicy.TraceLevel,
			args.Status,
			args.Representative.Speech,
			args.Rounds,
			args.Representative.ParticipantID,
			args.FinalClaims,
			args.ClaimResolutions,
		)
	}
	if args.Request.ReportPolicy.Composer != ReportComposerRepresentative {
		return fallback(), nil
	}
	reporterID := args.Request.ReportPolicy.RepresentativeID
	if reporterID == "" {
		reporterID = args.Representative.ParticipantID
	}
	task := ReportTask{
		TaskMeta: TaskMeta{
			SessionID:     args.Request.SessionPolicy.SessionKeyPrefix + ":" + args.SessionID + ":report:" + reporterID,
			RequestID:     args.RequestID,
			ParticipantID: reporterID,
			Metadata: map[string]any{
				"constraints": args.Request.Constraints,
				"context":     args.Request.Context,
			},
		},
		Prompt: e.buildReportPrompt(args.Request),
		Input: ReportInput{
			Status:           args.Status,
			Representative:   args.Representative,
			FinalClaims:      args.FinalClaims,
			ClaimResolutions: args.ClaimResolutions,
			Scoreboard:       args.Scoreboard,
			Rounds:           args.Rounds,
		},
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventReportDispatched, map[string]any{
		"reporterId": reporterID,
		"composer":   "representative",
	}); err != nil {
		return FinalReport{}, err
	}
	dispatched, err := e.deps.TaskDelegate.Dispatch(ctx, task)
	if err != nil {
		_ = e.emit(ctx, args.Request, args.SessionID, EventReportCompleted, map[string]any{
			"reporterId": reporterID,
			"mode":       "builtin",
			"reason":     "dispatch_failed",
		})
		return fallback(), nil
	}
	awaited, err := e.deps.TaskDelegate.Await(ctx, dispatched.TaskID, args.Request.WaitingPolicy.PerTaskTimeout)
	if err != nil || !awaited.OK {
		reason := "await_failed"
		if err == nil {
			reason = "task_failed"
		}
		_ = e.emit(ctx, args.Request, args.SessionID, EventReportCompleted, map[string]any{
			"reporterId": reporterID,
			"mode":       "builtin",
			"reason":     reason,
		})
		return fallback(), nil
	}
	typed, ok := awaited.Output.(ReportTaskResult)
	if !ok || typed.Output.FinalSummary == "" || typed.Output.RepresentativeSpeech == "" {
		_ = e.emit(ctx, args.Request, args.SessionID, EventReportCompleted, map[string]any{
			"reporterId": reporterID,
			"mode":       "builtin",
			"reason":     "parse_failed",
		})
		return fallback(), nil
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventReportCompleted, map[string]any{
		"reporterId": reporterID,
		"mode":       "representative",
	}); err != nil {
		return FinalReport{}, err
	}
	report := typed.Output
	report.Mode = "representative"
	return report, nil
}

type executeActionArgs struct {
	Request            StartRequest
	RequestID          string
	SessionID          string
	Result             *ConsensusResult
	ActiveParticipants map[string]struct{}
}

func (e *Engine) executeAction(ctx context.Context, args executeActionArgs) (*ActionOutput, error) {
	if args.Request.ActionPolicy == nil || strings.TrimSpace(args.Request.ActionPolicy.Prompt) == "" {
		return nil, nil
	}
	actorID := args.Request.ActionPolicy.ActorID
	if actorID == "" {
		actorID = args.Result.Representative.ParticipantID
	}
	if _, ok := args.ActiveParticipants[actorID]; !ok {
		_ = e.emit(ctx, args.Request, args.SessionID, EventActionFailed, map[string]any{
			"actorId": actorID,
			"reason":  "inactive_actor",
		})
		return &ActionOutput{
			ActorID: actorID,
			Status:  "failed",
			Error:   "actor is not an active participant",
		}, nil
	}
	task := ActionTask{
		TaskMeta: TaskMeta{
			SessionID:     args.Request.SessionPolicy.SessionKeyPrefix + ":" + args.SessionID + ":action:" + actorID,
			RequestID:     args.RequestID,
			ParticipantID: actorID,
		},
		Prompt: args.Request.ActionPolicy.Prompt,
		Input: ActionInput{
			Status:               args.Result.Status,
			FinalSummary:         args.Result.Report.FinalSummary,
			RepresentativeSpeech: args.Result.Report.RepresentativeSpeech,
			Claims:               args.Result.FinalClaims,
			ClaimResolutions:     args.Result.ClaimResolutions,
			Scoreboard:           args.Result.Scoreboard,
			Disagreements:        args.Result.Disagreements,
		},
	}
	if args.Request.ActionPolicy.IncludeFullResult {
		cloned := *args.Result
		task.FullResult = &cloned
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventActionDispatched, map[string]any{
		"actorId": actorID,
		"prompt":  task.Prompt,
	}); err != nil {
		return nil, err
	}
	dispatched, err := e.deps.TaskDelegate.Dispatch(ctx, task)
	if err != nil {
		_ = e.emit(ctx, args.Request, args.SessionID, EventActionFailed, map[string]any{
			"actorId": actorID,
			"reason":  "dispatch_failed",
		})
		return &ActionOutput{ActorID: actorID, Status: "failed", Error: err.Error()}, nil
	}
	awaited, err := e.deps.TaskDelegate.Await(ctx, dispatched.TaskID, args.Request.WaitingPolicy.PerTaskTimeout)
	if err != nil || !awaited.OK {
		reason := "await_failed"
		msg := ""
		if err != nil {
			msg = err.Error()
		} else {
			reason = "task_failed"
			msg = awaited.Error
		}
		_ = e.emit(ctx, args.Request, args.SessionID, EventActionFailed, map[string]any{
			"actorId": actorID,
			"reason":  reason,
		})
		return &ActionOutput{ActorID: actorID, Status: "failed", Error: msg}, nil
	}
	typed, ok := awaited.Output.(ActionTaskResult)
	if !ok {
		_ = e.emit(ctx, args.Request, args.SessionID, EventActionFailed, map[string]any{
			"actorId": actorID,
			"reason":  "parse_failed",
		})
		return &ActionOutput{ActorID: actorID, Status: "failed", Error: "action parse failed"}, nil
	}
	if err := e.emit(ctx, args.Request, args.SessionID, EventActionCompleted, map[string]any{
		"actorId": actorID,
		"summary": typed.Output.Summary,
	}); err != nil {
		return nil, err
	}
	return &ActionOutput{
		ActorID:      actorID,
		Status:       "completed",
		FullResponse: typed.Output.FullResponse,
		Summary:      typed.Output.Summary,
	}, nil
}

func (e *Engine) resolveRoundWaitingPolicy(req StartRequest, startedAt time.Time) WaitingPolicy {
	if req.WaitingPolicy.GlobalDeadline <= 0 {
		return req.WaitingPolicy
	}
	remaining := req.WaitingPolicy.GlobalDeadline - e.clock.Now().Sub(startedAt)
	if remaining < time.Millisecond {
		remaining = time.Millisecond
	}
	policy := req.WaitingPolicy
	if remaining < policy.PerRoundTimeout {
		policy.PerRoundTimeout = remaining
	}
	return policy
}

func (e *Engine) isGlobalDeadlineHit(req StartRequest, startedAt time.Time) bool {
	if req.WaitingPolicy.GlobalDeadline <= 0 {
		return false
	}
	return e.clock.Now().Sub(startedAt) >= req.WaitingPolicy.GlobalDeadline
}

func (e *Engine) buildRoundPrompt(req StartRequest, phase Phase, round int) string {
	parts := []string{
		"You are a participant in a structured multi-agent consensus session.",
		"Return one valid JSON object only. Do not wrap with markdown code fences.",
		"phase=" + string(phase),
		fmt.Sprintf("round=%d", round),
		"task=" + req.Task,
	}
	if req.Constraints != nil && req.Constraints.Language != "" {
		parts = append(parts, "language="+req.Constraints.Language)
	}
	if req.Constraints != nil && req.Constraints.TokenBudgetHint > 0 {
		parts = append(parts, fmt.Sprintf("token_budget_hint=%d", req.Constraints.TokenBudgetHint))
	}
	switch phase {
	case PhaseInitial:
		parts = append(parts,
			"",
			"Phase goal:",
			"- Produce an independent analysis.",
			"- Propose concrete claims for later debate.",
			"",
			"Initial phase JSON:",
			`{"fullResponse":"...","summary":"...","taskTitle":"...","extractedClaims":[{"title":"...","statement":"...","category":"pro"}],"judgements":[]}`,
		)
	case PhaseDebate:
		parts = append(parts,
			"",
			"Phase goal:",
			"- Critique and refine active claims from claimCatalog and peerRoundInputs.",
			"- Use mergesWith when two claims are duplicates.",
			"- Add extractedClaims only for genuinely new claims.",
			"",
			"Debate phase JSON:",
			`{"fullResponse":"...","summary":"...","judgements":[{"claimId":"c1","stance":"revise","confidence":0.82,"rationale":"...","revisedStatement":"...","mergesWith":"c0"}],"extractedClaims":[]}`,
		)
	default:
		parts = append(parts,
			"",
			"Phase goal:",
			"- Vote each active claim independently.",
			"- Every active claim should appear exactly once in claimVotes.",
			"",
			"Final vote JSON:",
			`{"fullResponse":"...","summary":"...","judgements":[{"claimId":"c1","stance":"agree","confidence":0.9,"rationale":"..."}],"claimVotes":[{"claimId":"c1","vote":"accept","reason":"..."}]}`,
		)
	}
	return strings.Join(parts, "\n")
}

func (e *Engine) buildReportPrompt(req StartRequest) string {
	lines := []string{
		"You are the report composer for til-consensus.",
		"Return one valid JSON object only. Do not wrap with markdown code fences.",
		"Generate FinalReport with required fields: finalSummary, representativeSpeech, mode, traceIncluded, traceLevel.",
		fmt.Sprintf("trace=%t", req.ReportPolicy.IncludeDeliberationTrace),
		"traceLevel=" + string(req.ReportPolicy.TraceLevel),
		`{"mode":"representative","traceIncluded":false,"traceLevel":"compact","finalSummary":"...","representativeSpeech":"..."}`,
	}
	if req.Constraints != nil && req.Constraints.Language != "" {
		lines = append(lines, "language="+req.Constraints.Language)
	}
	return strings.Join(lines, "\n")
}

func (e *Engine) emit(ctx context.Context, req StartRequest, sessionID string, eventType EventType, payload map[string]any) error {
	if e.deps.Observer == nil {
		return nil
	}
	return e.deps.Observer.OnEvent(ctx, ConsensusEvent{
		SessionID: sessionID,
		RequestID: req.RequestID,
		Type:      eventType,
		At:        e.clock.Now().Format(time.RFC3339Nano),
		Payload:   maps.Clone(payload),
	})
}

func (e *Engine) handleFailure(ctx context.Context, cause error, req StartRequest, sessionID string, state *StateMachine, active map[string]struct{}, eliminations []EliminationRecord) error {
	if state.Current() == SessionStateFinished || state.Current() == SessionStateFailed {
		return nil
	}
	if err := state.Transition(SessionStateFailed); err != nil {
		return err
	}
	failure := toFailureInfo(cause)
	failedAt := e.clock.Now().Format(time.RFC3339Nano)
	_ = e.store.Patch(ctx, sessionID, SessionPatch{
		State:              ptr(state.Current()),
		ActiveParticipants: activeParticipantList(active),
		Eliminations:       slices.Clone(eliminations),
		Error:              &failure,
		FailedAt:           &failedAt,
	})
	_ = e.emit(ctx, req, sessionID, EventFailed, map[string]any{
		"code":    failure.Code,
		"message": failure.Message,
	})
	return nil
}

func ensureMinParticipants(req StartRequest, count int) error {
	if count >= req.ParticipantsPolicy.MinParticipants {
		return nil
	}
	return fmt.Errorf("round failed minimum participant requirement: completed=%d, required=%d", count, req.ParticipantsPolicy.MinParticipants)
}

func findParticipant(participants []Participant, id string) (Participant, bool) {
	for _, participant := range participants {
		if participant.ID == id {
			return participant, true
		}
	}
	return Participant{}, false
}

func activeParticipantList(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for participantID := range set {
		out = append(out, participantID)
	}
	slices.Sort(out)
	return out
}

func filterActiveClaims(claims []Claim) []Claim {
	out := make([]Claim, 0)
	for _, claim := range claims {
		if claim.Status == ClaimStatusActive || claim.Status == "" {
			out = append(out, claim)
		}
	}
	return out
}

func countActiveClaims(claims []Claim) int {
	count := 0
	for _, claim := range claims {
		if claim.Status == ClaimStatusActive {
			count++
		}
	}
	return count
}

func countResolvedClaims(items []ClaimResolution) int {
	count := 0
	for _, item := range items {
		if item.Status == ClaimResolutionResolved {
			count++
		}
	}
	return count
}

func scoreFor(scoreboard []ParticipantScore, participantID string) float64 {
	for _, score := range scoreboard {
		if score.ParticipantID == participantID {
			return score.Total
		}
	}
	return 0
}

func pickRepresentativeSpeech(participantID string, rounds []RoundRecord) string {
	var latest *ParticipantRoundOutput
	for i := range rounds {
		for j := range rounds[i].Outputs {
			output := &rounds[i].Outputs[j]
			if output.ParticipantID != participantID {
				continue
			}
			if latest == nil || output.Round >= latest.Round {
				latest = output
			}
		}
	}
	if latest == nil {
		return participantID + " has no available output."
	}
	return latest.FullResponse
}

func toFailureInfo(err error) FailureInfo {
	if err == nil {
		return FailureInfo{Code: "UnknownError", Message: "unknown error"}
	}
	return FailureInfo{Code: "Error", Message: err.Error()}
}

type randomIDFactory struct{}

func (randomIDFactory) NewSessionID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "consensus_" + time.Now().UTC().Format("20060102150405")
	}
	return "consensus_" + hex.EncodeToString(buf)
}

func ptr[T any](v T) *T { return &v }
