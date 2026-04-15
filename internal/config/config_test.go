package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestResolveConfigPathPriority(t *testing.T) {
	t.Run("project config wins", func(t *testing.T) {
		tmp := t.TempDir()
		projectConfig := filepath.Join(tmp, "til-consensus.yaml")
		if err := os.WriteFile(projectConfig, []byte("schema_version: 1\nproviders:\n  mock:\n    type: mock\nagents:\n  - id: a\n    provider: mock\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		globalRoot := filepath.Join(tmp, "xdg")
		globalConfig := filepath.Join(globalRoot, "til-consensus", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(globalConfig), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(globalConfig, []byte("schema_version: 1\nproviders:\n  mock:\n    type: mock\nagents:\n  - id: a\n    provider: mock\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		oldWD, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(oldWD) })
		oldXDG := os.Getenv("XDG_CONFIG_HOME")
		t.Cleanup(func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG) })
		if err := os.Chdir(tmp); err != nil {
			t.Fatal(err)
		}
		if err := os.Setenv("XDG_CONFIG_HOME", globalRoot); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveConfigPath("")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Clean(got) != filepath.Clean(projectConfig) && filepath.Clean(got) != filepath.Clean("/private"+projectConfig) {
			t.Fatalf("expected %s, got %s", projectConfig, got)
		}
	})

	t.Run("global config used when project missing", func(t *testing.T) {
		tmp := t.TempDir()
		globalRoot := filepath.Join(tmp, "xdg")
		globalConfig := filepath.Join(globalRoot, "til-consensus", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(globalConfig), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(globalConfig, []byte("schema_version: 1\nproviders:\n  mock:\n    type: mock\nagents:\n  - id: a\n    provider: mock\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		oldWD, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(oldWD) })
		oldXDG := os.Getenv("XDG_CONFIG_HOME")
		t.Cleanup(func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG) })
		if err := os.Chdir(tmp); err != nil {
			t.Fatal(err)
		}
		if err := os.Setenv("XDG_CONFIG_HOME", globalRoot); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveConfigPath("")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Clean(got) != filepath.Clean(globalConfig) && filepath.Clean(got) != filepath.Clean("/private"+globalConfig) {
			t.Fatalf("expected %s, got %s", globalConfig, got)
		}
	})
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "til-consensus.yaml")
	body := []byte(`
schema_version: 1
unknown_field: true
providers:
  mock:
    type: mock
agents:
  - id: a
    provider: mock
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestValidateCrossReferences(t *testing.T) {
	cfg := Config{
		SchemaVersion: 1,
		Providers: map[string]ProviderConfig{
			"mock": {Type: "mock"},
		},
		Agents: []AgentConfig{
			{ID: "a", Provider: "missing"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected provider reference validation error")
	}
}

func TestResolveRunPlanPrecedence(t *testing.T) {
	loaded := LoadedConfig{
		Path:      "/tmp/til-consensus.yaml",
		ConfigDir: "/tmp",
		Config: Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				DefaultAgents:   []string{"a", "b"},
				MinRounds:       2,
				MaxRounds:       3,
				Threshold:       0.8,
				PerTaskTimeout:  Duration{Duration: 15 * time.Minute},
				PerRoundTimeout: Duration{Duration: 15 * time.Minute},
				Composer:        "builtin",
				TraceLevel:      "compact",
			},
			Output: OutputConfig{
				Directory: "./out/{requestId}",
			},
			Providers: map[string]ProviderConfig{
				"mock": {Type: "mock"},
			},
			Agents: []AgentConfig{
				{ID: "a", Provider: "mock"},
				{ID: "b", Provider: "mock"},
				{ID: "c", Provider: "mock"},
			},
		},
	}
	input := RunInput{
		RequestID:       "req_from_input",
		Task:            "task from input",
		Agents:          []string{"b", "c"},
		MinRounds:       1,
		MaxRounds:       4,
		Threshold:       0.7,
		Timeout:         Duration{Duration: 10 * time.Minute},
		GlobalDeadline:  Duration{Duration: 30 * time.Minute},
		Action:          "act",
		Language:        "zh-CN",
		TokenBudgetHint: 1024,
	}
	overrides := RunOverrides{
		Task:      "task from flag",
		Agents:    []string{"a", "c"},
		MinRounds: 3,
		MaxRounds: 5,
		Threshold: 0.9,
		Timeout:   5 * time.Minute,
		Verbose:   true,
	}
	plan, err := ResolveRunPlan(loaded, input, overrides, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequestID != "req_from_input" {
		t.Fatalf("unexpected request id: %s", plan.RequestID)
	}
	if plan.Task != "task from flag" {
		t.Fatalf("unexpected task: %s", plan.Task)
	}
	if !reflect.DeepEqual(plan.ParticipantIDs, []string{"a", "c"}) {
		t.Fatalf("unexpected participants: %#v", plan.ParticipantIDs)
	}
	if got := plan.StartRequest.RoundPolicy; got.MinRounds != 3 || got.MaxRounds != 5 {
		t.Fatalf("unexpected round policy: %#v", got)
	}
	if got := plan.StartRequest.ConsensusPolicy.Threshold; got != 0.9 {
		t.Fatalf("unexpected threshold: %v", got)
	}
	if got := plan.StartRequest.WaitingPolicy.PerTaskTimeout; got != 5*time.Minute {
		t.Fatalf("unexpected per task timeout: %v", got)
	}
	if got := plan.StartRequest.WaitingPolicy.GlobalDeadline; got != 30*time.Minute {
		t.Fatalf("unexpected global deadline: %v", got)
	}
	if plan.StartRequest.ActionPolicy == nil || plan.StartRequest.ActionPolicy.Prompt != "act" {
		t.Fatalf("unexpected action policy: %#v", plan.StartRequest.ActionPolicy)
	}
	if plan.StartRequest.Constraints == nil || plan.StartRequest.Constraints.Language != "zh-CN" {
		t.Fatalf("unexpected constraints: %#v", plan.StartRequest.Constraints)
	}
}
