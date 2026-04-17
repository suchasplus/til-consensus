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

func TestBuildDeterministicDelphiFacilitatorSummaryUsesStatementRecommendation(t *testing.T) {
	value := buildDeterministic(consensus.DelphiFacilitatorSummaryTask{
		StatementSummaries: []consensus.DelphiStatement{
			{StatementID: "s1", Statement: "Use monorepo"},
		},
	}, config.AgentConfig{ID: "facilitator-a"})
	raw, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected deterministic value type: %T", value)
	}
	if raw["recommendation"] != "Use monorepo" {
		t.Fatalf("unexpected recommendation: %#v", raw)
	}
}

func TestBuildDeterministicChallengeUsesSemanticChecksForStrategy(t *testing.T) {
	value := buildDeterministic(consensus.ChallengeTask{
		TaskSpec: consensus.TaskSpec{
			Goal: "Should we use a monorepo or polyrepo for our microservices?",
		},
		Claims: []consensus.ClaimNode{{ClaimID: "claim-1"}},
	}, config.AgentConfig{ID: "challenger-a"})
	raw, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected deterministic value type: %T", value)
	}
	tickets, ok := raw["tickets"].([]map[string]any)
	if !ok || len(tickets) != 1 {
		t.Fatalf("unexpected tickets: %#v", raw["tickets"])
	}
	checks, ok := tickets[0]["requestedChecks"].([]string)
	if !ok {
		t.Fatalf("unexpected requestedChecks type: %#v", tickets[0]["requestedChecks"])
	}
	if len(checks) != 1 || checks[0] != "semantic" {
		t.Fatalf("unexpected requested checks: %#v", checks)
	}
}

func TestBuildDeterministicArbiterDowngradesUnresolvedClaims(t *testing.T) {
	value := buildDeterministic(consensus.ArbiterTask{
		Claims: []consensus.ClaimNode{{ClaimID: "claim-1"}},
		Challenges: []consensus.ChallengeTicket{
			{ClaimID: "claim-1", Status: consensus.ChallengeStatusOpen},
		},
	}, config.AgentConfig{ID: "arbiter-a"})
	raw, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected deterministic value type: %T", value)
	}
	if raw["taskVerdict"] != consensus.TaskVerdictUndetermined {
		t.Fatalf("unexpected taskVerdict: %#v", raw["taskVerdict"])
	}
	records, ok := raw["records"].([]map[string]any)
	if !ok || len(records) != 1 {
		t.Fatalf("unexpected records: %#v", raw["records"])
	}
	if records[0]["disposition"] != consensus.ClaimDispositionUnresolved {
		t.Fatalf("unexpected disposition: %#v", records[0])
	}
}
