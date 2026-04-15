package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestGenericCLIRunnerPassesPromptViaStdin(t *testing.T) {
	runner := NewRunner(config.ProviderConfig{
		Type:    config.ProviderTypeCLI,
		CLIType: config.CLITypeGeneric,
		Command: "sh",
		Args:    []string{"-c", "cat"},
	})
	out, err := runner.RunTask(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{RequestID: "req-1", SessionID: "session-1", AgentID: "agent-1"},
	}, "hello-cli", "agent-1", "role", "model-x", "", nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if !strings.Contains(out, "hello-cli") || !strings.Contains(out, `"task"`) {
		t.Fatalf("unexpected cli output: %s", out)
	}
}

func TestBuildBaseArgsCodexIsOneShot(t *testing.T) {
	args, stdin := buildBaseArgs("codex", "gpt-5", "prompt", "high")
	if len(args) == 0 || stdin != "prompt" {
		t.Fatalf("unexpected codex args/stdin: %#v %q", args, stdin)
	}
	for _, arg := range args {
		if strings.Contains(arg, "resume") || strings.Contains(arg, "session") {
			t.Fatalf("expected one-shot args, got %#v", args)
		}
	}
}
