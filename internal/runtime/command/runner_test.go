package command

import (
	"context"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestRunTask(t *testing.T) {
	task := consensus.RoundTask{
		TaskMeta: consensus.TaskMeta{
			RequestID:     "req-1",
			SessionID:     "sess-1",
			ParticipantID: "agent-a",
		},
		Phase: consensus.PhaseDebate,
		Round: 2,
	}
	got, err := RunTask(context.Background(), task, "ignored prompt", config.ProviderConfig{
		Type:    "command",
		Command: "sh",
		Args:    []string{"-c", `printf "%s" "$TIL_CONSENSUS_REQUEST_ID:$TIL_CONSENSUS_PHASE:$TIL_CONSENSUS_ROUND:$X_CUSTOM"`},
		Env: map[string]string{
			"X_CUSTOM": "{agentId}",
		},
	}, "agent-a", "model-x")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "req-1:debate:2:agent-a" {
		t.Fatalf("unexpected command output: %q", got)
	}
}
