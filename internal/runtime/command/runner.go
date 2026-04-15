package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func RunTask(ctx context.Context, task consensus.Task, prompt string, provider config.ProviderConfig, agentID string, providerModel string) (string, error) {
	command := exec.CommandContext(ctx, provider.Command, renderArgs(provider.Args, task, agentID, providerModel)...)
	command.Env = append(command.Environ(), renderEnv(provider.Env, task, agentID, providerModel)...)
	command.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("command provider failed: %w stderr=%s stdout=%s", err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}
	return stdout.String(), nil
}

func renderArgs(args []string, task consensus.Task, agentID string, providerModel string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, renderTemplate(arg, task, agentID, providerModel))
	}
	return out
}

func renderEnv(env map[string]string, task consensus.Task, agentID string, providerModel string) []string {
	out := make([]string, 0, len(env)+6)
	for key, value := range env {
		out = append(out, key+"="+renderTemplate(value, task, agentID, providerModel))
	}
	meta := task.Meta()
	out = append(out,
		"TIL_CONSENSUS_REQUEST_ID="+meta.RequestID,
		"TIL_CONSENSUS_SESSION_ID="+meta.SessionID,
		"TIL_CONSENSUS_PARTICIPANT_ID="+meta.ParticipantID,
		"TIL_CONSENSUS_TASK_KIND="+string(task.Kind()),
	)
	if roundTask, ok := task.(consensus.RoundTask); ok {
		out = append(out,
			"TIL_CONSENSUS_PHASE="+string(roundTask.Phase),
			"TIL_CONSENSUS_ROUND="+strconv.Itoa(roundTask.Round),
		)
	}
	return out
}

func renderTemplate(value string, task consensus.Task, agentID string, providerModel string) string {
	meta := task.Meta()
	value = strings.ReplaceAll(value, "{requestId}", meta.RequestID)
	value = strings.ReplaceAll(value, "{sessionId}", meta.SessionID)
	value = strings.ReplaceAll(value, "{participantId}", meta.ParticipantID)
	value = strings.ReplaceAll(value, "{taskKind}", string(task.Kind()))
	value = strings.ReplaceAll(value, "{providerModel}", providerModel)
	value = strings.ReplaceAll(value, "{agentId}", agentID)
	if roundTask, ok := task.(consensus.RoundTask); ok {
		value = strings.ReplaceAll(value, "{phase}", string(roundTask.Phase))
		value = strings.ReplaceAll(value, "{round}", strconv.Itoa(roundTask.Round))
	}
	return value
}
