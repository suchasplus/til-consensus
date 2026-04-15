package cli

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
	agentID string,
	role string,
	providerModel string,
	reasoning string,
	temperature *float64,
) (string, error) {
	cliType := r.provider.CLIType
	if cliType == "" {
		cliType = config.CLITypeGeneric
	}
	finalPrompt := prompt
	if cliType == config.CLITypeGeneric {
		envelope, err := json.MarshalIndent(map[string]any{
			"version": 1,
			"agent": map[string]any{
				"id":            agentID,
				"role":          role,
				"providerModel": providerModel,
				"reasoning":     reasoning,
				"temperature":   temperature,
			},
			"task":   task,
			"prompt": prompt,
		}, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal generic cli envelope: %w", err)
		}
		finalPrompt = string(envelope)
	}
	commandName := r.provider.Command
	if commandName == "" {
		commandName = cliType
	}
	args, stdin := buildBaseArgs(cliType, providerModel, finalPrompt, reasoning)
	args = append(args, renderArgs(r.provider.Args, task, agentID, role, providerModel)...)
	env := append(os.Environ(), renderEnv(r.provider.Env, task, agentID, role, providerModel)...)
	return runCommand(ctx, commandName, args, env, stdin)
}

func buildBaseArgs(cliType string, providerModel string, prompt string, reasoning string) ([]string, string) {
	switch cliType {
	case "claude":
		return []string{"--print", "--model", providerModel}, prompt
	case "codex":
		args := []string{"exec", "-m", providerModel, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		if reasoning != "" {
			args = []string{"exec", "-m", providerModel, "-c", "model_reasoning_effort=" + reasoning, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		}
		return args, prompt
	case "gemini":
		return []string{"--approval-mode", "yolo", "-m", providerModel}, prompt
	case "opencode":
		return []string{"run", prompt, "--dangerously-skip-permissions", "-m", providerModel}, ""
	default:
		return nil, prompt
	}
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

func runCommand(ctx context.Context, command string, args []string, env []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli provider failed: %w stderr=%s stdout=%s", err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}
	return stdout.String(), nil
}
