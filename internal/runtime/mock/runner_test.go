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

func TestResolveActionUsesParticipantSpecificScenarios(t *testing.T) {
	provider := config.ProviderConfig{
		Type: config.ProviderTypeMock,
		Participants: map[string]config.MockParticipantScenario{
			"agent-1": {
				Propose:        config.MockAction{Behavior: "error", Error: "propose"},
				Challenge:      config.MockAction{Behavior: "malformed"},
				SemanticVerify: config.MockAction{Behavior: "timeout"},
				Arbiter:        config.MockAction{Behavior: "error", Error: "arbiter"},
				Report:         config.MockAction{Behavior: "error", Error: "report"},
				Action:         config.MockAction{Behavior: "error", Error: "action"},
			},
		},
	}
	tests := []struct {
		task     consensus.Task
		behavior string
	}{
		{task: consensus.ProposalTask{}, behavior: "error"},
		{task: consensus.ChallengeTask{}, behavior: "malformed"},
		{task: consensus.SemanticVerificationTask{}, behavior: "timeout"},
		{task: consensus.ArbiterTask{}, behavior: "error"},
		{task: consensus.ReportTask{}, behavior: "error"},
		{task: consensus.ActionTask{}, behavior: "error"},
	}
	for _, tc := range tests {
		action := resolveAction(provider, tc.task, "agent-1")
		if action.Behavior != tc.behavior {
			t.Fatalf("unexpected behavior for %T: %#v", tc.task, action)
		}
	}
}

func TestFallbackBehaviorAndDeterministicBuilders(t *testing.T) {
	if got := fallbackBehavior(""); got != "deterministic" {
		t.Fatalf("unexpected fallback behavior: %s", got)
	}
	if got := fallbackBehavior("error"); got != "error" {
		t.Fatalf("unexpected explicit behavior: %s", got)
	}

	agent := config.AgentConfig{ID: "agent-1"}
	tests := []struct {
		name string
		task consensus.Task
	}{
		{
			name: "challenge",
			task: consensus.ChallengeTask{Claims: []consensus.ClaimNode{{ClaimID: "claim-1"}}},
		},
		{
			name: "semantic",
			task: consensus.SemanticVerificationTask{Claim: consensus.ClaimNode{ClaimID: "claim-1"}},
		},
		{
			name: "arbiter",
			task: consensus.ArbiterTask{Claims: []consensus.ClaimNode{{ClaimID: "claim-1"}}},
		},
		{
			name: "report",
			task: consensus.ReportTask{},
		},
		{
			name: "action",
			task: consensus.ActionTask{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := buildDeterministic(tc.task, agent)
			if value == nil {
				t.Fatalf("expected deterministic value for %s", tc.name)
			}
		})
	}
}
