package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func TestResolveRunPlanPrecedence(t *testing.T) {
	tmp := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd after chdir: %v", err)
	}

	loaded := LoadedConfig{
		ConfigDir: filepath.Join(tmp, "config"),
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
	if plan.StartRequest.WaitingPolicy.RetryAttempts != consensus.DefaultTaskRetryAttempts {
		t.Fatalf("unexpected retry attempts: %#v", plan.StartRequest.WaitingPolicy)
	}
	if plan.ResultPath != filepath.Join(cwd, "out", plan.RequestID, "result.json") {
		t.Fatalf("unexpected result path: %s", plan.ResultPath)
	}
}

func TestResolveRunArtifactsRelativeOutputUsesWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	configDir := filepath.Join(tmp, "configs")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	resolvedCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd after chdir: %v", err)
	}

	loaded := LoadedConfig{
		ConfigDir: configDir,
		Config: Normalize(Config{
			SchemaVersion: 1,
			Output:        OutputConfig{Directory: "./out/{requestId}"},
		}),
	}
	paths := ResolveRunArtifacts(loaded, "req-1")
	want := filepath.Join(resolvedCWD, "out", "req-1", "result.json")
	if paths.ResultPath != want {
		t.Fatalf("relative output should resolve from cwd: got=%s want=%s", paths.ResultPath, want)
	}
	if strings.HasPrefix(paths.ResultPath, configDir) {
		t.Fatalf("relative output should not resolve from config dir: %s", paths.ResultPath)
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

func TestResolveRunPlanSupportsDebateAndDelphiModes(t *testing.T) {
	tmp := t.TempDir()
	loaded := LoadedConfig{
		ConfigDir: tmp,
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				Mode: consensus.WorkflowModeFreeDebate,
			},
			Output: OutputConfig{Directory: "./out/{requestId}"},
			Providers: map[string]ProviderConfig{
				"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
			},
			Agents: []AgentConfig{
				{ID: "p1", Provider: "mock", Model: "default"},
				{ID: "p2", Provider: "mock", Model: "default"},
				{ID: "f1", Provider: "mock", Model: "default"},
			},
			Roles: RolesConfig{
				Participants: []string{"p1", "p2"},
				Facilitator:  "f1",
			},
		}),
	}

	debatePlan, err := ResolveRunPlan(loaded, RunInput{
		TaskSpec: TaskSpecInput{Goal: "debate goal"},
	}, RunOverrides{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan debate failed: %v", err)
	}
	if debatePlan.Mode != consensus.WorkflowModeFreeDebate || len(debatePlan.StartRequest.Roles.Participants) != 2 {
		t.Fatalf("unexpected debate plan: %#v", debatePlan)
	}

	delphiPlan, err := ResolveRunPlan(loaded, RunInput{
		Mode:     consensus.WorkflowModeDelphi,
		TaskSpec: TaskSpecInput{Goal: "delphi goal"},
	}, RunOverrides{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan delphi failed: %v", err)
	}
	if delphiPlan.Mode != consensus.WorkflowModeDelphi || delphiPlan.StartRequest.Roles.Facilitator != "f1" {
		t.Fatalf("unexpected delphi plan: %#v", delphiPlan)
	}
}

func TestResolveRunPlanRejectsUnknownOverrideParticipant(t *testing.T) {
	tmp := t.TempDir()
	loaded := LoadedConfig{
		ConfigDir: tmp,
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				Mode: consensus.WorkflowModeDelphi,
			},
			Output: OutputConfig{Directory: "./out/{requestId}"},
			Providers: map[string]ProviderConfig{
				"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
			},
			Agents: []AgentConfig{
				{ID: "participant-a", Provider: "mock", Model: "default"},
				{ID: "participant-b", Provider: "mock", Model: "default"},
				{ID: "facilitator-a", Provider: "mock", Model: "default"},
			},
			Roles: RolesConfig{
				Participants: []string{"participant-a", "participant-b"},
				Facilitator:  "facilitator-a",
			},
		}),
	}

	_, err := ResolveRunPlan(loaded, RunInput{
		TaskSpec: TaskSpecInput{Goal: "delphi goal"},
	}, RunOverrides{
		Mode:         consensus.WorkflowModeDelphi,
		Participants: []string{"participant-a", "participant-b", "participant-c"},
	}, time.Unix(1700000000, 0))
	if err == nil {
		t.Fatal("expected unknown participant override to fail")
	}
	if got := err.Error(); got != "roles.delphi.participants references unknown agent participant-c" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveRunPlanCarriesAdjudicationFallbackAndObservationPolicies(t *testing.T) {
	tmp := t.TempDir()
	loaded := LoadedConfig{
		ConfigDir: tmp,
		Config: Normalize(Config{
			SchemaVersion: 1,
			Defaults: DefaultsConfig{
				TaskType:   consensus.CaseTaskTypeStrategy,
				OutOfScope: []string{"不讨论组织架构重组"},
				IngestPolicy: consensus.IngestPolicy{
					Sources: []consensus.ExternalCommandSource{{
						Name:    "ingest-a",
						Command: "sh",
						Args:    []string{"-c", "printf ingest"},
					}},
				},
				FallbackPolicy: consensus.AdjudicationFallbackPolicy{
					MaxFallbackRounds:      2,
					OnInsufficientEvidence: consensus.FallbackTargetIngest,
					OnUnresolvedConflict:   consensus.FallbackTargetIngest,
					OnUnresolvedClaims:     consensus.FallbackTargetRevise,
					OnKeepWithCaveat:       consensus.FallbackTargetRevise,
				},
				ObservePolicy: consensus.ObservePolicy{
					OnContradiction: consensus.ObserveContradictionReopen,
					Sources: []consensus.ExternalCommandSource{{
						Name:    "observe-a",
						Command: "sh",
						Args:    []string{"-c", "printf observe"},
					}},
				},
			},
			Output: OutputConfig{Directory: "./out/{requestId}"},
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

	plan, err := ResolveRunPlan(loaded, RunInput{
		TaskSpec: TaskSpecInput{Goal: "评估 monorepo 还是 polyrepo"},
	}, RunOverrides{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	if plan.StartRequest.TaskSpec.TaskType != consensus.CaseTaskTypeStrategy {
		t.Fatalf("expected task type to propagate, got %#v", plan.StartRequest.TaskSpec.TaskType)
	}
	if len(plan.StartRequest.TaskSpec.OutOfScope) != 1 || plan.StartRequest.TaskSpec.OutOfScope[0] != "不讨论组织架构重组" {
		t.Fatalf("unexpected out_of_scope: %#v", plan.StartRequest.TaskSpec.OutOfScope)
	}
	if len(plan.StartRequest.IngestPolicy.Sources) != 1 || plan.StartRequest.IngestPolicy.Sources[0].Name != "ingest-a" {
		t.Fatalf("unexpected ingest policy: %#v", plan.StartRequest.IngestPolicy)
	}
	if plan.StartRequest.FallbackPolicy.MaxFallbackRounds != 2 || plan.StartRequest.FallbackPolicy.OnInsufficientEvidence != consensus.FallbackTargetIngest {
		t.Fatalf("unexpected fallback policy: %#v", plan.StartRequest.FallbackPolicy)
	}
	if len(plan.StartRequest.ObservePolicy.Sources) != 1 || plan.StartRequest.ObservePolicy.OnContradiction != consensus.ObserveContradictionReopen {
		t.Fatalf("unexpected observe policy: %#v", plan.StartRequest.ObservePolicy)
	}
}

func TestResolveRunPlanForRequestAndSessionStoreDir(t *testing.T) {
	tmp := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd after chdir: %v", err)
	}

	loaded := LoadedConfig{
		ConfigDir: filepath.Join(tmp, "config"),
		Config: Normalize(Config{
			SchemaVersion: 1,
			Output:        OutputConfig{Directory: "./out/{requestId}"},
			Providers: map[string]ProviderConfig{
				"mock": {Type: ProviderTypeMock, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "mock"}}},
			},
			Agents: []AgentConfig{
				{ID: "p1", Provider: "mock", Model: "default"},
				{ID: "c1", Provider: "mock", Model: "default"},
			},
		}),
	}
	request, err := consensus.NormalizeStartRequest(consensus.StartRequest{
		RequestID: "child-request-1",
		Lineage: &consensus.RunLineage{
			ParentRequestID: "parent-request-1",
			ParentSessionID: "parent-session-1",
			Trigger:         "observe_contradiction",
		},
		TaskSpec: consensus.TaskSpec{Goal: "follow-up goal"},
		Roles: consensus.RoleAssignments{
			Proposers:   []string{"p1"},
			Challengers: []string{"c1"},
		},
		ProposalPolicy: consensus.ProposalPolicy{MaxPasses: 1, MaxClaimsPerWorker: 1},
		VerificationPolicy: consensus.VerificationPolicy{
			MaxParallelChecks: 1,
		},
		ArbiterPolicy: consensus.ArbiterPolicy{AllowUndetermined: true, BlindReview: true},
		ReportPolicy:  consensus.ReportPolicy{Style: "builtin"},
		WaitingPolicy: consensus.WaitingPolicy{PerTaskTimeout: time.Second},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest failed: %v", err)
	}
	plan, err := ResolveRunPlanForRequest(loaded, request, true, false)
	if err != nil {
		t.Fatalf("ResolveRunPlanForRequest failed: %v", err)
	}
	if plan.RequestID != "child-request-1" || plan.StartRequest.Lineage == nil || plan.StartRequest.Lineage.ParentRequestID != "parent-request-1" {
		t.Fatalf("unexpected plan lineage: %#v", plan)
	}
	want := filepath.Join(cwd, "out", "_sessions")
	if plan.SessionStoreDir != want {
		t.Fatalf("unexpected session store dir: got=%s want=%s", plan.SessionStoreDir, want)
	}
	if got := ResolveSessionStoreDir(loaded); got != want {
		t.Fatalf("unexpected resolved session store dir: got=%s want=%s", got, want)
	}
}
