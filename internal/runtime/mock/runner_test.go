package mock

import (
	"context"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestRunTaskDeterministicProposal(t *testing.T) {
	raw, err := RunTask(context.Background(), consensus.ProposalTask{
		TaskMeta:  consensus.TaskMeta{AgentID: "proposer-a"},
		MaxClaims: 2,
	}, config.AgentConfig{ID: "proposer-a"}, config.ProviderConfig{
		Type:     config.ProviderTypeMock,
		Behavior: "deterministic",
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected raw type: %T", raw)
	}
	if value["summary"] == "" {
		t.Fatalf("unexpected deterministic output: %#v", value)
	}
}

func TestRunTaskMalformedAndError(t *testing.T) {
	_, err := RunTask(context.Background(), consensus.ProposalTask{}, config.AgentConfig{ID: "a"}, config.ProviderConfig{
		Type:     config.ProviderTypeMock,
		Behavior: "error",
		Error:    "boom",
	})
	if err == nil {
		t.Fatal("expected error behavior")
	}
	raw, err := RunTask(context.Background(), consensus.ProposalTask{}, config.AgentConfig{ID: "a"}, config.ProviderConfig{
		Type:     config.ProviderTypeMock,
		Behavior: "malformed",
	})
	if err != nil {
		t.Fatalf("malformed should still return raw text: %v", err)
	}
	if raw.(string) != "not json" {
		t.Fatalf("unexpected malformed output: %#v", raw)
	}
}

func TestRunTaskTimeoutHonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := RunTask(ctx, consensus.ProposalTask{}, config.AgentConfig{ID: "a"}, config.ProviderConfig{
		Type:     config.ProviderTypeMock,
		Behavior: "timeout",
	})
	if err == nil {
		t.Fatal("expected timeout behavior to fail")
	}
}
