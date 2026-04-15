package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	commandrunner "github.com/suchasplus/til-consensus/internal/runtime/command"
	mockrunner "github.com/suchasplus/til-consensus/internal/runtime/mock"
	openairunner "github.com/suchasplus/til-consensus/internal/runtime/openai"
)

type Delegate struct {
	mu     sync.Mutex
	agents map[string]ResolvedAgentRuntime
	tasks  map[string]*taskEntry
	runDir string
	seq    int
}

type taskEntry struct {
	cancel context.CancelFunc
	done   chan taskOutcome
	task   consensus.Task
}

type taskOutcome struct {
	result consensus.TaskResult
	err    error
}

func NewDelegate(cfg config.Config, runDir string) (*Delegate, error) {
	agents := map[string]ResolvedAgentRuntime{}
	for _, agent := range cfg.Agents {
		provider, ok := cfg.Providers[agent.Provider]
		if !ok {
			return nil, fmt.Errorf("unknown provider %s", agent.Provider)
		}
		model := agent.Model
		if model == "" {
			model = provider.Model
		}
		agents[agent.ID] = ResolvedAgentRuntime{
			AgentConfig:   agent,
			ProviderName:  agent.Provider,
			Provider:      provider,
			ProviderModel: model,
		}
	}
	return &Delegate{
		agents: agents,
		tasks:  map[string]*taskEntry{},
		runDir: runDir,
	}, nil
}

func (d *Delegate) Dispatch(ctx context.Context, task consensus.Task) (consensus.DispatchReceipt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	agent, ok := d.agents[task.Meta().ParticipantID]
	if !ok {
		return consensus.DispatchReceipt{}, fmt.Errorf("unknown agent id: %s", task.Meta().ParticipantID)
	}
	taskID := fmt.Sprintf("task-%d", d.seq)
	d.seq++
	taskCtx, cancel := context.WithCancel(ctx)
	done := make(chan taskOutcome, 1)
	d.tasks[taskID] = &taskEntry{cancel: cancel, done: done, task: task}
	go func() {
		raw, err := d.getRunner(agent.Provider).RunTask(taskCtx, ProviderTaskRequest{Task: task, Agent: agent})
		if err != nil {
			done <- taskOutcome{err: err}
			return
		}
		result, err := NormalizeTaskOutput(task, raw)
		if err != nil {
			if parseErr, ok := err.(*JSONParseError); ok {
				_ = d.persistRawParseError(task, parseErr)
			}
			done <- taskOutcome{err: err}
			return
		}
		done <- taskOutcome{result: result}
	}()
	return consensus.DispatchReceipt{
		TaskID:        taskID,
		ParticipantID: task.Meta().ParticipantID,
		Kind:          task.Kind(),
	}, nil
}

func (d *Delegate) Await(ctx context.Context, taskID string, timeout time.Duration) (consensus.AwaitedTask, error) {
	d.mu.Lock()
	entry, ok := d.tasks[taskID]
	d.mu.Unlock()
	if !ok {
		return consensus.AwaitedTask{OK: false, Error: "unknown_task_id:" + taskID}, nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return consensus.AwaitedTask{}, ctx.Err()
	case <-timer.C:
		entry.cancel()
		d.mu.Lock()
		delete(d.tasks, taskID)
		d.mu.Unlock()
		return consensus.AwaitedTask{OK: false, Error: "__timeout__"}, nil
	case outcome := <-entry.done:
		d.mu.Lock()
		delete(d.tasks, taskID)
		d.mu.Unlock()
		if outcome.err != nil {
			return consensus.AwaitedTask{OK: false, Error: outcome.err.Error()}, nil
		}
		return consensus.AwaitedTask{OK: true, Output: outcome.result}, nil
	}
}

func (d *Delegate) Cancel(_ context.Context, taskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.tasks[taskID]; ok {
		entry.cancel()
		delete(d.tasks, taskID)
	}
	return nil
}

func (d *Delegate) getRunner(provider config.ProviderConfig) ProviderRunner {
	switch provider.Type {
	case "mock":
		return providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return mockrunner.RunTask(ctx, req.Task, req.Agent.AgentConfig, req.Agent.Provider)
		})
	case "openai":
		return providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return openairunner.RunTask(
				ctx,
				BuildTaskPrompt(req.Task, req.Agent, true),
				req.Agent.SystemPrompt,
				req.Agent.Provider,
				req.Agent.ProviderModel,
				TaskOutputJSONSchema(req.Task),
			)
		})
	default:
		return providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return commandrunner.RunTask(
				ctx,
				req.Task,
				BuildTaskPrompt(req.Task, req.Agent, true),
				req.Agent.Provider,
				req.Agent.ID,
				req.Agent.ProviderModel,
			)
		})
	}
}

func (d *Delegate) persistRawParseError(task consensus.Task, parseErr *JSONParseError) error {
	if err := os.MkdirAll(d.runDir, 0o755); err != nil {
		return err
	}
	filename := buildRawErrorFilename(task)
	body := strings.Join([]string{
		"# til-consensus raw agent output",
		"# error: " + parseErr.Message,
		"# participant_id: " + task.Meta().ParticipantID,
		"# request_id: " + task.Meta().RequestID,
		"# session_id: " + task.Meta().SessionID,
		"",
		"# --- raw text ---",
		parseErr.RawText,
		"",
		"# --- extracted candidate ---",
		parseErr.ExtractedCandidate,
		"",
	}, "\n")
	return os.WriteFile(filepath.Join(d.runDir, filename), []byte(body), 0o644)
}

func buildRawErrorFilename(task consensus.Task) string {
	safeParticipant := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(task.Meta().ParticipantID)
	if round, ok := task.(consensus.RoundTask); ok {
		return fmt.Sprintf("raw-error-%s-%s-%d.txt", safeParticipant, round.Phase, round.Round)
	}
	return fmt.Sprintf("raw-error-%s-%s.txt", safeParticipant, task.Kind())
}
