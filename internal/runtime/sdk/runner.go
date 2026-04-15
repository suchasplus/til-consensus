package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Runner struct {
	provider config.ProviderConfig
}

func NewRunner(provider config.ProviderConfig) *Runner {
	return &Runner{provider: provider}
}

func (r *Runner) RunTask(
	ctx context.Context,
	task consensus.Task,
	prompt string,
	agent config.AgentConfig,
	providerModel string,
	modelConfig config.ProviderModelConfig,
) (string, error) {
	envelope := map[string]any{
		"version": 1,
		"agent": map[string]any{
			"id":            agent.ID,
			"provider":      agent.Provider,
			"model":         agent.Model,
			"providerModel": providerModel,
			"role":          agent.Role,
			"systemPrompt":  agent.SystemPrompt,
			"reasoning":     firstNonEmpty(agent.Reasoning, modelConfig.Reasoning),
		},
		"provider": map[string]any{
			"type":    r.provider.Type,
			"adapter": r.provider.Adapter,
			"options": r.provider.Options,
		},
		"prompt": prompt,
		"task":   task,
	}
	if agent.Temperature != nil {
		envelope["agent"].(map[string]any)["temperature"] = *agent.Temperature
	} else if modelConfig.Temperature != nil {
		envelope["agent"].(map[string]any)["temperature"] = *modelConfig.Temperature
	}
	body, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sdk envelope: %w", err)
	}
	cmd := exec.CommandContext(ctx, r.provider.Adapter, renderArgs(r.provider.Args, task, agent.ID, agent.Role, providerModel)...)
	cmd.Env = append(os.Environ(), renderEnv(r.provider.Env, task, agent.ID, agent.Role, providerModel)...)
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sdk provider failed: %w stderr=%s stdout=%s", err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}
	return stdout.String(), nil
}

func renderArgs(args []string, task consensus.Task, agentID string, role string, providerModel string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, renderTemplate(arg, task, agentID, role, providerModel))
	}
	return out
}

func renderEnv(env map[string]string, task consensus.Task, agentID string, role string, providerModel string) []string {
	out := make([]string, 0, len(env)+6)
	for key, value := range env {
		out = append(out, key+"="+renderTemplate(value, task, agentID, role, providerModel))
	}
	meta := task.Meta()
	out = append(out,
		"TIL_CONSENSUS_TASK_KIND="+string(task.Kind()),
		"TIL_CONSENSUS_REQUEST_ID="+meta.RequestID,
		"TIL_CONSENSUS_SESSION_ID="+meta.SessionID,
		"TIL_CONSENSUS_AGENT_ID="+meta.AgentID,
		"TIL_CONSENSUS_PROVIDER_MODEL="+providerModel,
	)
	if role != "" {
		out = append(out, "TIL_CONSENSUS_ROLE="+role)
	}
	return out
}

func renderTemplate(value string, task consensus.Task, agentID string, role string, providerModel string) string {
	meta := task.Meta()
	value = strings.ReplaceAll(value, "{requestId}", meta.RequestID)
	value = strings.ReplaceAll(value, "{sessionId}", meta.SessionID)
	value = strings.ReplaceAll(value, "{agentId}", meta.AgentID)
	value = strings.ReplaceAll(value, "{taskKind}", string(task.Kind()))
	value = strings.ReplaceAll(value, "{providerModel}", providerModel)
	value = strings.ReplaceAll(value, "{role}", role)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
