package config

import (
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func mismatchScaffoldConfig() LoadedConfig {
	return LoadedConfig{
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				// Simulates profile: adjudication already applied onto defaults.
				Mode: consensus.WorkflowModeAdjudication,
				SuccessCriteria: []string{
					"每条关键 claim 必须给出 claim 级裁决",
					"必须标注 keep、revise、reject 或 unresolved 的裁决状态",
				},
			},
			Profiles: map[string]ProfileConfig{
				"free-debate": {
					Defaults: DefaultsConfig{
						Mode:            consensus.WorkflowModeFreeDebate,
						SuccessCriteria: []string{"必须呈现主要支持方和反对方观点"},
					},
				},
			},
			Output: OutputConfig{Directory: "./out/{requestId}"},
			Providers: map[string]ProviderConfig{
				"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
			},
			Agents: []AgentConfig{
				{ID: "d1", Provider: "mock", Model: "default"},
				{ID: "d2", Provider: "mock", Model: "default"},
			},
			Roles: RolesConfig{
				FreeDebate: DebateRolesConfig{Participants: []string{"d1", "d2"}},
			},
		}),
	}
}

func TestResolveRunPlanSwapsMismatchedProfileCriteria(t *testing.T) {
	loaded := mismatchScaffoldConfig()
	plan, err := ResolveRunPlan(loaded, RunInput{TaskSpec: TaskSpecInput{Goal: "debate this"}}, RunOverrides{
		Mode: consensus.WorkflowModeFreeDebate,
	}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	got := plan.StartRequest.TaskSpec.SuccessCriteria
	if len(got) != 1 || got[0] != "必须呈现主要支持方和反对方观点" {
		t.Fatalf("expected criteria borrowed from free-debate profile, got %#v", got)
	}
	if len(plan.Notices) != 1 || !strings.Contains(plan.Notices[0], "free-debate") {
		t.Fatalf("expected a notice naming the borrowed profile, got %#v", plan.Notices)
	}
}

func TestResolveRunPlanFallsBackToBuiltinCriteria(t *testing.T) {
	loaded := mismatchScaffoldConfig()
	loaded.Config.Profiles = nil
	plan, err := ResolveRunPlan(loaded, RunInput{TaskSpec: TaskSpecInput{Goal: "debate this"}}, RunOverrides{
		Mode: consensus.WorkflowModeFreeDebate,
	}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	got := plan.StartRequest.TaskSpec.SuccessCriteria
	want := defaultSuccessCriteriaForMode(consensus.WorkflowModeFreeDebate)
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected built-in free-debate criteria, got %#v", got)
	}
	if len(plan.Notices) != 1 || !strings.Contains(plan.Notices[0], "built-in") {
		t.Fatalf("expected a built-in fallback notice, got %#v", plan.Notices)
	}
}

func TestResolveRunPlanKeepsExplicitCriteriaAndMatchingMode(t *testing.T) {
	loaded := mismatchScaffoldConfig()
	// Explicit input criteria are user intent: never swapped, no notice.
	plan, err := ResolveRunPlan(loaded, RunInput{TaskSpec: TaskSpecInput{
		Goal:            "debate this",
		SuccessCriteria: []string{"from-input"},
	}}, RunOverrides{Mode: consensus.WorkflowModeFreeDebate}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	if got := plan.StartRequest.TaskSpec.SuccessCriteria; len(got) != 1 || got[0] != "from-input" {
		t.Fatalf("expected explicit input criteria untouched, got %#v", got)
	}
	if len(plan.Notices) != 0 {
		t.Fatalf("expected no notices for explicit criteria, got %#v", plan.Notices)
	}

	// Matching mode keeps the defaults criteria without a notice.
	loaded.Config.Roles.Adjudication = AdjudicationRolesConfig{
		Proposers:   []string{"d1"},
		Challengers: []string{"d2"},
	}
	loaded.Config = Normalize(loaded.Config)
	plan, err = ResolveRunPlan(loaded, RunInput{TaskSpec: TaskSpecInput{Goal: "adjudicate this"}}, RunOverrides{
		Mode:        consensus.WorkflowModeAdjudication,
		Proposers:   []string{"d1"},
		Challengers: []string{"d2"},
	}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan (matching mode) failed: %v", err)
	}
	if got := plan.StartRequest.TaskSpec.SuccessCriteria; len(got) != 2 || !strings.Contains(got[1], "keep、revise、reject") {
		t.Fatalf("expected defaults criteria kept for matching mode, got %#v", got)
	}
	if len(plan.Notices) != 0 {
		t.Fatalf("expected no notices for matching mode, got %#v", plan.Notices)
	}
}
