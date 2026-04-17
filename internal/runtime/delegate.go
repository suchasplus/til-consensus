package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	apirunner "github.com/suchasplus/til-consensus/internal/runtime/api"
	clirunner "github.com/suchasplus/til-consensus/internal/runtime/cli"
	mockrunner "github.com/suchasplus/til-consensus/internal/runtime/mock"
	sdkrunner "github.com/suchasplus/til-consensus/internal/runtime/sdk"
)

type Delegate struct {
	mu          sync.Mutex
	agents      map[string]ResolvedAgentRuntime
	runners     map[string]ProviderRunner
	tasks       map[string]*taskEntry
	artifactDir string
	seq         int
}

type taskEntry struct {
	cancel context.CancelFunc
	done   chan taskOutcome
	task   consensus.Task
}

type taskOutcome struct {
	result   consensus.TaskResult
	artifact *consensus.ArtifactRef
	err      error
}

func NewDelegate(cfg config.Config, artifactDir string) (*Delegate, error) {
	cfg = config.Normalize(cfg)
	agents := map[string]ResolvedAgentRuntime{}
	for _, agent := range cfg.Agents {
		provider, ok := cfg.Providers[agent.Provider]
		if !ok {
			return nil, fmt.Errorf("unknown agent id provider %s", agent.Provider)
		}
		resolved, err := resolveAgentRuntime(agent, provider)
		if err != nil {
			return nil, fmt.Errorf("resolve agent %s: %w", agent.ID, err)
		}
		agents[agent.ID] = resolved
	}
	return &Delegate{
		agents:      agents,
		runners:     map[string]ProviderRunner{},
		tasks:       map[string]*taskEntry{},
		artifactDir: artifactDir,
	}, nil
}

func (d *Delegate) Dispatch(ctx context.Context, task consensus.Task) (consensus.DispatchReceipt, error) {
	d.mu.Lock()
	agent, ok := d.agents[task.Meta().AgentID]
	if !ok {
		d.mu.Unlock()
		return consensus.DispatchReceipt{}, fmt.Errorf("unknown agent id: %s", task.Meta().AgentID)
	}
	runner, err := d.getRunnerLocked(agent)
	if err != nil {
		d.mu.Unlock()
		return consensus.DispatchReceipt{}, err
	}
	taskID := fmt.Sprintf("task-%d", d.seq)
	d.seq++
	taskCtx, cancel := context.WithCancel(ctx)
	done := make(chan taskOutcome, 1)
	d.tasks[taskID] = &taskEntry{cancel: cancel, done: done, task: task}
	d.mu.Unlock()

	go func(taskID string) {
		inputArtifact, inputErr := d.persistTaskInput(taskID, task, agent)
		if inputErr != nil {
			done <- taskOutcome{err: inputErr}
			return
		}
		raw, err := runner.RunTask(taskCtx, ProviderTaskRequest{Task: task, Agent: agent})
		if err != nil {
			artifact, _ := d.persistTaskFailure(taskID, task, agent, err, inputArtifact)
			done <- taskOutcome{artifact: artifact, err: err}
			return
		}
		artifact, persistErr := d.persistRawOutput(taskID, task, raw)
		if persistErr != nil {
			done <- taskOutcome{err: persistErr}
			return
		}
		result, err := NormalizeTaskOutput(task, raw)
		if err != nil {
			if parseErr, ok := err.(*JSONParseError); ok {
				artifact, _ = d.persistRawParseError(taskID, task, parseErr)
			}
			done <- taskOutcome{artifact: artifact, err: err}
			return
		}
		done <- taskOutcome{result: result, artifact: artifact}
	}(taskID)
	return consensus.DispatchReceipt{
		TaskID:  taskID,
		AgentID: task.Meta().AgentID,
		Kind:    task.Kind(),
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
		entry.cancel()
		drainTaskOutcome(entry.done, 100*time.Millisecond)
		d.mu.Lock()
		delete(d.tasks, taskID)
		d.mu.Unlock()
		return consensus.AwaitedTask{}, ctx.Err()
	case <-timer.C:
		entry.cancel()
		drainTaskOutcome(entry.done, 100*time.Millisecond)
		d.mu.Lock()
		delete(d.tasks, taskID)
		d.mu.Unlock()
		return consensus.AwaitedTask{OK: false, Error: "__timeout__"}, nil
	case outcome := <-entry.done:
		d.mu.Lock()
		delete(d.tasks, taskID)
		d.mu.Unlock()
		if outcome.err != nil {
			return consensus.AwaitedTask{OK: false, Error: outcome.err.Error(), Artifact: outcome.artifact}, nil
		}
		return consensus.AwaitedTask{OK: true, Output: outcome.result, Artifact: outcome.artifact}, nil
	}
}

func drainTaskOutcome(done <-chan taskOutcome, grace time.Duration) {
	if grace <= 0 {
		grace = 100 * time.Millisecond
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
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

func (d *Delegate) getRunnerLocked(agent ResolvedAgentRuntime) (ProviderRunner, error) {
	if runner, ok := d.runners[agent.ID]; ok {
		return runner, nil
	}
	var runner ProviderRunner
	switch agent.Provider.Type {
	case config.ProviderTypeMock:
		runner = providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return mockrunner.RunTask(ctx, req.Task, req.Agent.AgentConfig, req.Agent.Provider)
		})
	case config.ProviderTypeAPI:
		apiRunner := apirunner.NewRunner(agent.Provider)
		runner = providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return apiRunner.RunTask(
				ctx,
				BuildTaskPrompt(req.Task, req.Agent, true),
				req.Agent.SystemPrompt,
				req.Agent.ProviderModel,
				req.Agent.EffectiveTemperature(),
				req.Agent.EffectiveReasoning(),
				req.Agent.ModelConfig.MaxOutputTokens,
				TaskOutputJSONSchema(req.Task),
			)
		})
	case config.ProviderTypeCLI:
		cliRunner := clirunner.NewRunner(agent.Provider)
		runner = providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return cliRunner.RunTask(
				ctx,
				req.Task,
				BuildTaskPrompt(req.Task, req.Agent, true),
				req.Agent.ID,
				req.Agent.Role,
				req.Agent.ProviderModel,
				req.Agent.EffectiveReasoning(),
				req.Agent.EffectiveTemperature(),
			)
		})
	case config.ProviderTypeSDK:
		sdkRunner := sdkrunner.NewRunner(agent.Provider)
		runner = providerRunnerFunc(func(ctx context.Context, req ProviderTaskRequest) (any, error) {
			return sdkRunner.RunTask(
				ctx,
				req.Task,
				BuildTaskPrompt(req.Task, req.Agent, true),
				req.Agent.AgentConfig,
				req.Agent.ProviderModel,
				req.Agent.ModelConfig,
			)
		})
	default:
		return nil, fmt.Errorf("unsupported provider type %q for agent %s", agent.Provider.Type, agent.ID)
	}
	d.runners[agent.ID] = runner
	return runner, nil
}

func (d *Delegate) persistRawOutput(taskID string, task consensus.Task, raw any) (*consensus.ArtifactRef, error) {
	if strings.TrimSpace(d.artifactDir) == "" {
		return nil, nil
	}
	var (
		body      []byte
		mediaType = "application/json"
	)
	switch value := raw.(type) {
	case string:
		body = []byte(value)
		mediaType = "text/plain"
	default:
		var err error
		body, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal raw output: %w", err)
		}
		body = append(body, '\n')
	}
	filename := filepath.Join(d.artifactDir, buildRawFilename(task, taskID))
	return writeArtifact(filename, body, mediaType)
}

func (d *Delegate) persistRawParseError(taskID string, task consensus.Task, parseErr *JSONParseError) (*consensus.ArtifactRef, error) {
	if strings.TrimSpace(d.artifactDir) == "" {
		return nil, nil
	}
	body := strings.Join([]string{
		"# til-consensus raw agent output",
		"# error: " + parseErr.Message,
		"# agent_id: " + task.Meta().AgentID,
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
	filename := filepath.Join(d.artifactDir, buildParseErrorFilename(task, taskID))
	return writeArtifact(filename, []byte(body), "text/plain")
}

func (d *Delegate) persistTaskInput(taskID string, task consensus.Task, agent ResolvedAgentRuntime) (*consensus.ArtifactRef, error) {
	if strings.TrimSpace(d.artifactDir) == "" {
		return nil, nil
	}
	payload := map[string]any{
		"version": 1,
		"agent": map[string]any{
			"id":            agent.ID,
			"role":          agent.Role,
			"provider":      agent.ProviderName,
			"providerType":  agent.Provider.Type,
			"providerModel": agent.ProviderModel,
		},
		"task": task,
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal task input: %w", err)
	}
	body = append(body, '\n')
	filename := filepath.Join(d.artifactDir, buildInputFilename(task, taskID))
	return writeArtifact(filename, body, "application/json")
}

func (d *Delegate) persistTaskFailure(taskID string, task consensus.Task, agent ResolvedAgentRuntime, err error, inputArtifact *consensus.ArtifactRef) (*consensus.ArtifactRef, error) {
	if strings.TrimSpace(d.artifactDir) == "" {
		return nil, nil
	}
	providerErr := classifyProviderError(err)
	payload := map[string]any{
		"version": 1,
		"agent": map[string]any{
			"id":            agent.ID,
			"role":          agent.Role,
			"provider":      agent.ProviderName,
			"providerType":  agent.Provider.Type,
			"providerModel": agent.ProviderModel,
		},
		"task": map[string]any{
			"taskId":    taskID,
			"kind":      task.Kind(),
			"requestId": task.Meta().RequestID,
			"sessionId": task.Meta().SessionID,
			"agentId":   task.Meta().AgentID,
		},
		"error": map[string]any{
			"class":      providerErr.Class,
			"operation":  providerErr.Operation,
			"statusCode": providerErr.StatusCode,
			"message":    providerErr.Error(),
		},
	}
	if inputArtifact != nil {
		payload["inputArtifact"] = inputArtifact
	}
	body, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal task failure: %w", marshalErr)
	}
	body = append(body, '\n')
	filename := filepath.Join(d.artifactDir, buildFailureFilename(task, taskID))
	return writeArtifact(filename, body, "application/json")
}

func buildRawFilename(task consensus.Task, taskID string) string {
	safeAgent := sanitizeFilename(task.Meta().AgentID)
	return fmt.Sprintf("raw-%s-%s-%s.json", safeAgent, task.Kind(), sanitizeFilename(taskID))
}

func buildInputFilename(task consensus.Task, taskID string) string {
	safeAgent := sanitizeFilename(task.Meta().AgentID)
	return fmt.Sprintf("input-%s-%s-%s.json", safeAgent, task.Kind(), sanitizeFilename(taskID))
}

func buildParseErrorFilename(task consensus.Task, taskID string) string {
	safeAgent := sanitizeFilename(task.Meta().AgentID)
	return fmt.Sprintf("raw-error-%s-%s-%s.txt", safeAgent, task.Kind(), sanitizeFilename(taskID))
}

func buildFailureFilename(task consensus.Task, taskID string) string {
	safeAgent := sanitizeFilename(task.Meta().AgentID)
	return fmt.Sprintf("failure-%s-%s-%s.json", safeAgent, task.Kind(), sanitizeFilename(taskID))
}

func writeArtifact(path string, body []byte, mediaType string) (*consensus.ArtifactRef, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(body)
	return &consensus.ArtifactRef{
		Path:      path,
		Hash:      hex.EncodeToString(hash[:]),
		MediaType: mediaType,
	}, nil
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(value)
}

func resolveAgentRuntime(agent config.AgentConfig, provider config.ProviderConfig) (ResolvedAgentRuntime, error) {
	var (
		modelID       = agent.Model
		modelConfig   config.ProviderModelConfig
		providerModel string
	)
	if len(provider.Models) > 0 {
		if modelID == "" {
			if inferred, ok := singleModelID(provider); ok {
				modelID = inferred
			}
		}
		if modelID == "" {
			return ResolvedAgentRuntime{}, fmt.Errorf("model is required")
		}
		resolved, ok := provider.Models[modelID]
		if !ok {
			return ResolvedAgentRuntime{}, fmt.Errorf("unknown model %s", modelID)
		}
		modelConfig = resolved
		providerModel = resolved.ProviderModel
		if providerModel == "" {
			providerModel = modelID
		}
	} else {
		providerModel = firstNonEmpty(agent.Model, provider.Model)
	}
	resolvedAgent := agent
	resolvedAgent.Model = modelID
	return ResolvedAgentRuntime{
		AgentConfig:   resolvedAgent,
		ProviderName:  agent.Provider,
		Provider:      provider,
		ModelConfig:   modelConfig,
		ProviderModel: providerModel,
	}, nil
}

func singleModelID(provider config.ProviderConfig) (string, bool) {
	if len(provider.Models) != 1 {
		return "", false
	}
	for modelID := range provider.Models {
		return modelID, true
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
