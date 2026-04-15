package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestSDKRunnerPassesEnvelopeToAdapter(t *testing.T) {
	runner := NewRunner(config.ProviderConfig{
		Type:    config.ProviderTypeSDK,
		Adapter: "sh",
		Args:    []string{"-c", "cat"},
	})
	out, err := runner.RunTask(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{RequestID: "req-1", SessionID: "session-1", AgentID: "agent-1"},
	}, "sdk-prompt", config.AgentConfig{
		ID:       "agent-1",
		Provider: "sdk",
		Model:    "default",
	}, "sdk-model", config.ProviderModelConfig{})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if !strings.Contains(out, "sdk-prompt") || !strings.Contains(out, `"providerModel": "sdk-model"`) {
		t.Fatalf("unexpected sdk output: %s", out)
	}
}
