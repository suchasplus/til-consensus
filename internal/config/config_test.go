package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestResolveRunPlanPrecedence(t *testing.T) {
	tmp := t.TempDir()
	loaded := LoadedConfig{
		ConfigDir: tmp,
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				SuccessCriteria: []string{"from-default"},
				PerTaskTimeout:  Duration{Duration: 10 * time.Second},
			},
			Output: OutputConfig{
				Directory: "./out/{requestId}",
			},
			Providers: map[string]ProviderConfig{
				"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
			},
			Agents: []AgentConfig{
				{ID: "p1", Provider: "mock", Model: "default"},
				{ID: "c1", Provider: "mock", Model: "default"},
				{ID: "a1", Provider: "mock", Model: "default"},
			},
			Roles: RolesConfig{
				Proposers:   []string{"p1"},
				Challengers: []string{"c1"},
				Arbiter:     "a1",
			},
		}),
	}
	input := RunInput{
		TaskSpec: TaskSpecInput{
			Goal:            "from-input",
			SuccessCriteria: []string{"from-input"},
		},
	}
	plan, err := ResolveRunPlan(loaded, input, RunOverrides{
		Task:            "from-flag",
		SuccessCriteria: []string{"from-flag"},
		Proposers:       []string{"p1"},
		Challengers:     []string{"c1"},
	}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	if plan.Task != "from-flag" {
		t.Fatalf("expected flag task precedence, got %s", plan.Task)
	}
	if got := plan.StartRequest.TaskSpec.SuccessCriteria; len(got) != 1 || got[0] != "from-flag" {
		t.Fatalf("unexpected success criteria: %#v", got)
	}
	if plan.ResultPath != filepath.Join(tmp, "out", plan.RequestID, "result.json") {
		t.Fatalf("unexpected result path: %s", plan.ResultPath)
	}
}

func TestValidateRejectsUnknownRoleReference(t *testing.T) {
	cfg := Normalize(Config{
		SchemaVersion: 1,
		Providers: map[string]ProviderConfig{
			"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
		},
		Agents: []AgentConfig{
			{ID: "p1", Provider: "mock", Model: "default"},
			{ID: "c1", Provider: "mock", Model: "default"},
		},
		Roles: RolesConfig{
			Proposers:   []string{"p1"},
			Challengers: []string{"missing"},
		},
	})
	if err := Validate(cfg); err == nil {
		t.Fatal("expected unknown role reference to fail validation")
	}
}
