package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/observer"
	tilruntime "github.com/suchasplus/til-consensus/runtime"
	filestore "github.com/suchasplus/til-consensus/store/file"
)

type Executor struct {
	Loaded       config.LoadedConfig
	SessionStore consensus.SessionStore
	Observer     consensus.Observer
	Ledger       consensus.Ledger
	Clock        consensus.Clock
	IDFactory    consensus.IDFactory
}

type Result struct {
	Plan   config.ResolvedRunPlan
	Output *consensus.RunResult
}

type ActionInput struct {
	Result        consensus.RunResult
	Prompt        string
	ActorID       string
	ArtifactsDir  string
	Timeout       time.Duration
	RetryAttempts int
}

type ActionResult struct {
	ActorID  string
	Output   consensus.ActionExecution
	Receipt  consensus.DispatchReceipt
	Attempts int
}

func LoadConfig(path string, profile string) (config.LoadedConfig, error) {
	resolvedPath, err := config.ResolveConfigPath(path)
	if err != nil {
		return config.LoadedConfig{}, err
	}
	return config.LoadWithProfile(resolvedPath, profile)
}

func NewExecutor(loaded config.LoadedConfig) *Executor {
	return &Executor{Loaded: loaded}
}

func (e *Executor) Resolve(input config.RunInput, overrides config.RunOverrides, now time.Time) (config.ResolvedRunPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return config.ResolveRunPlan(e.Loaded, input, overrides, now)
}

func (e *Executor) ResolveRequest(request consensus.StartRequest, verbose bool, debug bool) (config.ResolvedRunPlan, error) {
	return config.ResolveRunPlanForRequest(e.Loaded, request, verbose, debug)
}

func (e *Executor) Run(ctx context.Context, input config.RunInput, overrides config.RunOverrides, now time.Time) (Result, error) {
	plan, err := e.Resolve(input, overrides, now)
	if err != nil {
		return Result{}, err
	}
	result, err := e.RunPlan(ctx, plan)
	if err != nil {
		return Result{Plan: plan}, err
	}
	return Result{Plan: plan, Output: result}, nil
}

func (e *Executor) RunRequest(ctx context.Context, request consensus.StartRequest, verbose bool, debug bool) (Result, error) {
	plan, err := e.ResolveRequest(request, verbose, debug)
	if err != nil {
		return Result{}, err
	}
	result, err := e.RunPlan(ctx, plan)
	if err != nil {
		return Result{Plan: plan}, err
	}
	return Result{Plan: plan, Output: result}, nil
}

func (e *Executor) Resume(ctx context.Context, snapshot consensus.SessionSnapshot, verbose bool, debug bool) (Result, error) {
	if snapshot.Request == nil {
		return Result{}, fmt.Errorf("session %s has no resumable request", snapshot.SessionID)
	}
	plan, err := e.ResolveRequest(*snapshot.Request, verbose, debug)
	if err != nil {
		return Result{}, err
	}
	result, err := e.ResumePlan(ctx, plan, snapshot)
	if err != nil {
		return Result{Plan: plan}, err
	}
	return Result{Plan: plan, Output: result}, nil
}

func (e *Executor) Replay(ctx context.Context, snapshot consensus.SessionSnapshot, now time.Time, verbose bool, debug bool) (Result, error) {
	request, err := ReplayRequest(snapshot, now)
	if err != nil {
		return Result{}, err
	}
	return e.RunRequest(ctx, request, verbose, debug)
}

func (e *Executor) Act(ctx context.Context, input ActionInput) (ActionResult, error) {
	if strings.TrimSpace(input.Prompt) == "" {
		return ActionResult{}, fmt.Errorf("action prompt is required")
	}
	if strings.TrimSpace(input.ArtifactsDir) == "" {
		return ActionResult{}, fmt.Errorf("artifacts dir is required")
	}
	actorID := strings.TrimSpace(input.ActorID)
	if actorID == "" {
		mode := e.Loaded.Config.Defaults.Mode
		if mode == "" {
			mode = consensus.WorkflowModeAdjudication
		}
		actorID = config.RoleAssignmentsForMode(e.Loaded.Config.Roles, mode).Actor
	}
	if actorID == "" {
		return ActionResult{}, fmt.Errorf("missing actor agent id")
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = consensus.DefaultPerTaskTimeout
	}
	retryAttempts := input.RetryAttempts
	if retryAttempts <= 0 {
		retryAttempts = consensus.DefaultTaskRetryAttempts
	}
	delegate, err := tilruntime.NewDelegate(e.Loaded.Config, input.ArtifactsDir)
	if err != nil {
		return ActionResult{}, err
	}
	task := consensus.ActionTask{
		TaskMeta: consensus.TaskMeta{
			SessionID: input.Result.SessionID,
			RequestID: input.Result.RequestID,
			AgentID:   actorID,
			Role:      "actor",
		},
		Prompt: input.Prompt,
		Input:  input.Result,
	}
	receipt, awaited, attempts, err := consensus.ExecuteTaskWithRetry(ctx, delegate, task, timeout, retryAttempts, consensus.TaskRetryHooks{})
	if err != nil {
		return ActionResult{}, err
	}
	if !awaited.OK {
		return ActionResult{}, fmt.Errorf("action failed: %s", awaited.Error)
	}
	typed, ok := awaited.Output.(consensus.ActionTaskResult)
	if !ok {
		return ActionResult{}, fmt.Errorf("unexpected action result type")
	}
	return ActionResult{
		ActorID:  actorID,
		Output:   typed.Output,
		Receipt:  receipt,
		Attempts: attempts,
	}, nil
}

func (e *Executor) RunPlan(ctx context.Context, plan config.ResolvedRunPlan) (*consensus.RunResult, error) {
	engine, err := e.newEngine(plan)
	if err != nil {
		return nil, err
	}
	return engine.Start(ctx, plan.StartRequest)
}

func (e *Executor) ResumePlan(ctx context.Context, plan config.ResolvedRunPlan, snapshot consensus.SessionSnapshot) (*consensus.RunResult, error) {
	engine, err := e.newEngine(plan)
	if err != nil {
		return nil, err
	}
	return engine.Resume(ctx, snapshot)
}

func (e *Executor) SessionStoreForLoaded() consensus.SessionStore {
	if e.SessionStore != nil {
		return e.SessionStore
	}
	return filestore.New(config.ResolveSessionStoreDir(e.Loaded))
}

func (e *Executor) newEngine(plan config.ResolvedRunPlan) (*consensus.Engine, error) {
	delegate, err := tilruntime.NewDelegate(e.Loaded.Config, plan.ArtifactsDir)
	if err != nil {
		return nil, err
	}
	sessionStore := e.SessionStore
	if sessionStore == nil {
		sessionStore = filestore.New(plan.SessionStoreDir)
	}
	runObserver := e.Observer
	if runObserver == nil {
		runObserver = observer.NewJSONL(plan.EventsPath)
	}
	ledger := e.Ledger
	if ledger == nil {
		ledger = observer.NewLedger(plan.LedgerPath, plan.ManifestPath)
	}
	return consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: delegate,
		Observer:     runObserver,
		Ledger:       ledger,
		SessionStore: sessionStore,
		Clock:        e.Clock,
		IDFactory:    e.IDFactory,
		ArtifactDir:  plan.ArtifactsDir,
	}), nil
}

func ReplayRequest(snapshot consensus.SessionSnapshot, now time.Time) (consensus.StartRequest, error) {
	if snapshot.Request == nil {
		return consensus.StartRequest{}, fmt.Errorf("session %s has no replayable request", snapshot.SessionID)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	request := *snapshot.Request
	request.RequestID = consensus.NewRequestID(now)
	request.Lineage = &consensus.RunLineage{
		ParentRequestID: snapshot.RequestID,
		ParentSessionID: snapshot.SessionID,
		Trigger:         "session_replay",
	}
	if snapshot.Result != nil && snapshot.Result.CaseManifest != nil {
		request.Lineage.ParentCaseID = snapshot.Result.CaseManifest.CaseID
	}
	return request, nil
}
