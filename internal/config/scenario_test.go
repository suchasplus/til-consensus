package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScenarioInputsLoadAndResolve(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read scenarios: %v", err)
	}
	loaded := LoadedConfig{
		ConfigDir: ".",
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults:      InitTemplate().Defaults,
			Output:        InitTemplate().Output,
			Providers: map[string]ProviderConfig{
				"mock": {
					Type:   ProviderTypeMock,
					Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock-default"}},
				},
			},
			Agents: []AgentConfig{
				{ID: "proposer-a", Provider: "mock", Model: "default"},
				{ID: "challenger-a", Provider: "mock", Model: "default"},
				{ID: "arbiter-a", Provider: "mock", Model: "default"},
				{ID: "reporter-a", Provider: "mock", Model: "default"},
				{ID: "verifier-a", Provider: "mock", Model: "default"},
			},
			Roles: RolesConfig{
				Proposers:        []string{"proposer-a"},
				Challengers:      []string{"challenger-a"},
				Arbiter:          "arbiter-a",
				Reporter:         "reporter-a",
				SemanticVerifier: "verifier-a",
			},
		}),
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			input, err := LoadRunInput(filepath.Join(root, name, "run.yaml"))
			if err != nil {
				t.Fatalf("LoadRunInput failed: %v", err)
			}
			plan, err := ResolveRunPlan(loaded, input, RunOverrides{}, time.Unix(1700000000, 0).UTC())
			if err != nil {
				t.Fatalf("ResolveRunPlan failed: %v", err)
			}
			if strings.TrimSpace(plan.StartRequest.TaskSpec.Goal) == "" {
				t.Fatal("expected non-empty goal")
			}
			if plan.ResultPath == "" || plan.LedgerPath == "" || plan.ArtifactsDir == "" {
				t.Fatalf("expected output paths to be populated: %#v", plan)
			}
		})
	}
}
