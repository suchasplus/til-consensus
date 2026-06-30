package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathPrecedence(t *testing.T) {
	tmp := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})

	projectPath := filepath.Join(tmp, "til-consensus.yaml")
	globalHome := filepath.Join(tmp, "xdg")
	defaultGlobalPath := filepath.Join(globalHome, "til-consensus", "default.yaml")
	legacyGlobalPath := filepath.Join(globalHome, "til-consensus", "config.yaml")
	t.Setenv("XDG_CONFIG_HOME", globalHome)

	if err := os.MkdirAll(filepath.Dir(defaultGlobalPath), 0o755); err != nil {
		t.Fatalf("mkdir global dir: %v", err)
	}
	if err := os.WriteFile(defaultGlobalPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	resolved, err := ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath global failed: %v", err)
	}
	if samePath(t, resolved, defaultGlobalPath) == false {
		t.Fatalf("expected global path, got %s", resolved)
	}

	if err := os.Remove(defaultGlobalPath); err != nil {
		t.Fatalf("remove default global config: %v", err)
	}
	if err := os.WriteFile(legacyGlobalPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write legacy global config: %v", err)
	}
	resolved, err = ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath legacy global failed: %v", err)
	}
	if samePath(t, resolved, legacyGlobalPath) == false {
		t.Fatalf("expected legacy global path, got %s", resolved)
	}

	if err := os.WriteFile(projectPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	resolved, err = ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath project failed: %v", err)
	}
	if samePath(t, resolved, projectPath) == false {
		t.Fatalf("expected project path, got %s", resolved)
	}

	explicit := filepath.Join(tmp, "custom.yaml")
	if err := os.WriteFile(explicit, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write explicit config: %v", err)
	}
	resolved, err = ResolveConfigPath(explicit)
	if err != nil {
		t.Fatalf("ResolveConfigPath explicit failed: %v", err)
	}
	if samePath(t, resolved, explicit) == false {
		t.Fatalf("expected explicit path, got %s", resolved)
	}
}

func TestLoadRunInputJSONAndHelpers(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "run.json")
	if err := os.WriteFile(inputPath, []byte(`{
  "request_id": "req-1",
  "task_spec": {
    "goal": "verify patch",
    "success_criteria": ["a", "b"]
  }
}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	input, err := LoadRunInput(inputPath)
	if err != nil {
		t.Fatalf("LoadRunInput failed: %v", err)
	}
	if input.RequestID != "req-1" || input.TaskSpec.Goal != "verify patch" {
		t.Fatalf("unexpected input: %#v", input)
	}

	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath failed: %v", err)
	}
	if filepath.Base(defaultPath) != "default.yaml" {
		t.Fatalf("unexpected default path: %s", defaultPath)
	}
	if got := toAbs("til-consensus.yaml", "/tmp/base"); got != filepath.Join("/tmp/base", "til-consensus.yaml") {
		t.Fatalf("unexpected toAbs result: %s", got)
	}
}

func TestLoadConfigIncludesAndOverlays(t *testing.T) {
	tmp := t.TempDir()
	partials := filepath.Join(tmp, "partials")
	if err := os.MkdirAll(partials, 0o755); err != nil {
		t.Fatalf("mkdir partials: %v", err)
	}
	writeConfigTestFile(t, filepath.Join(partials, "base.yaml"), `
defaults:
  success_criteria:
    - from-base
  per_task_timeout: 10s
  proposal_policy:
    max_claims_per_worker: 3
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
output:
  directory: ./base-out/{requestId}
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock-base
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
`)
	writeConfigTestFile(t, filepath.Join(partials, "override.yaml"), `
providers:
  mock:
    models:
      default:
        max_output_tokens: 123
agents:
  - id: proposer-a
    system_prompt: from-include-override
`)
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
include:
  - partials/base.yaml
  - partials/override.yaml
defaults:
  success_criteria:
    - from-main
  proposal_policy:
    max_claims_per_worker: 5
output:
  directory: ./main-out/{requestId}
providers:
  mock:
    models:
      default:
        provider_model: mock-main
agents:
  - id: proposer-a
    system_prompt: from-main
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
roles:
  adjudication:
    arbiter: arbiter-a
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cfg := loaded.Config
	if cfg.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", cfg.SchemaVersion)
	}
	if got := cfg.Defaults.SuccessCriteria; len(got) != 1 || got[0] != "from-main" {
		t.Fatalf("expected main list replacement, got %#v", got)
	}
	if cfg.Defaults.ProposalPolicy.MaxClaimsPerWorker != 5 {
		t.Fatalf("expected main scalar override, got %#v", cfg.Defaults.ProposalPolicy)
	}
	if !cfg.Defaults.VerificationPolicy.AllowSemanticVerifier || cfg.Defaults.VerificationPolicy.MaxParallelChecks != 2 {
		t.Fatalf("expected inherited verification policy, got %#v", cfg.Defaults.VerificationPolicy)
	}
	if cfg.Output.Directory != "./main-out/{requestId}" {
		t.Fatalf("expected main output override, got %s", cfg.Output.Directory)
	}
	model := cfg.Providers["mock"].Models["default"]
	if model.ProviderModel != "mock-main" || model.MaxOutputTokens != 123 {
		t.Fatalf("expected provider model deep merge, got %#v", model)
	}
	var proposer AgentConfig
	for _, agent := range cfg.Agents {
		if agent.ID == "proposer-a" {
			proposer = agent
			break
		}
	}
	if proposer.Provider != "mock" || proposer.SystemPrompt != "from-main" {
		t.Fatalf("expected agent merge by id, got %#v", proposer)
	}
	if len(cfg.Roles.Proposers) != 1 || cfg.Roles.Proposers[0] != "proposer-a" || cfg.Roles.Arbiter != "arbiter-a" {
		t.Fatalf("expected roles from include plus main, got %#v", cfg.Roles)
	}
}

func TestLoadConfigIncludeCycleFails(t *testing.T) {
	tmp := t.TempDir()
	first := filepath.Join(tmp, "first.yaml")
	second := filepath.Join(tmp, "second.yaml")
	writeConfigTestFile(t, first, `
schema_version: 1
include:
  - second.yaml
providers:
  mock:
    type: mock
agents:
  - id: proposer-a
    provider: mock
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [proposer-a]
`)
	writeConfigTestFile(t, second, `
include:
  - first.yaml
`)

	_, err := Load(first)
	if err == nil {
		t.Fatal("expected include cycle to fail")
	}
	if !strings.Contains(err.Error(), "config include cycle detected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadProfilesDoesNotRequireWorkflowRoles(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "providers-only.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: openai-compatible
    base_url: http://127.0.0.1:1
    models:
      default:
        provider_model: test-model
`)

	loaded, err := LoadProfiles(configPath)
	if err != nil {
		t.Fatalf("LoadProfiles should accept provider-only config: %v", err)
	}
	if _, ok := loaded.Config.Providers["api"]; !ok {
		t.Fatalf("expected api provider in loaded config")
	}

	if _, err := Load(configPath); err == nil {
		t.Fatalf("Load should still require full workflow config")
	}
}

func TestLoadProfilesValidatesProviderProfilesOnly(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "bad-provider.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: unsupported
    models:
      default:
        provider_model: test-model
`)
	if _, err := LoadProfiles(configPath); err == nil {
		t.Fatalf("expected LoadProfiles to reject invalid provider protocol")
	}

	configPath = filepath.Join(tmp, "bad-agent.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: openai-compatible
    models:
      default:
        provider_model: test-model
agents:
  - id: verifier-a
    provider: missing
    model: default
`)
	if _, err := LoadProfiles(configPath); err != nil {
		t.Fatalf("LoadProfiles should ignore unrelated agent references: %v", err)
	}
}

func TestProviderAndModelEnabledDefaultsAndValidation(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "enabled-defaults.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: openai-compatible
    models:
      default:
        provider_model: test-model
`)
	loaded, err := LoadProfiles(configPath)
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}
	provider := loaded.Config.Providers["api"]
	if !IsProviderEnabled(provider) {
		t.Fatalf("provider should default to enabled")
	}
	if !IsProviderModelEnabled(provider.Models["default"]) {
		t.Fatalf("provider model should default to enabled")
	}

	configPath = filepath.Join(tmp, "disabled-provider.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  disabled-api:
    enabled: false
    type: api
    protocol: openai-compatible
    models:
      default:
        provider_model: disabled-model
agents:
  - id: proposer-a
    provider: disabled-api
    model: default
    role: proposer
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [proposer-a]
`)
	_, err = Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "provider disabled-api is disabled") {
		t.Fatalf("expected disabled provider reference error, got %v", err)
	}
	if _, err := LoadProfiles(configPath); err != nil {
		t.Fatalf("LoadProfiles should allow disabled provider declarations: %v", err)
	}

	configPath = filepath.Join(tmp, "disabled-model.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: openai-compatible
    models:
      default:
        enabled: false
        provider_model: disabled-model
agents:
  - id: proposer-a
    provider: api
    model: default
    role: proposer
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [proposer-a]
`)
	_, err = Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "model default for provider api is disabled") {
		t.Fatalf("expected disabled model reference error, got %v", err)
	}
}

func TestLoadProfilesRejectsCLIMaxOutputTokens(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "bad-cli-token-budget.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  codex-cli:
    type: cli
    cli_type: codex
    command: codex
    models:
      default:
        provider_model: gpt-5.5
        max_output_tokens: 0
`)
	_, err := LoadProfiles(configPath)
	if err == nil {
		t.Fatalf("expected LoadProfiles to reject cli max_output_tokens")
	}
	if !strings.Contains(err.Error(), "max_output_tokens is API-only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadProfilesRejectsCLIMaxTokenOptionsAndArgs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "bad-cli-options.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  gemini-cli:
    type: cli
    cli_type: gemini
    command: gemini
    args:
      - --max-output-tokens=4096
    options:
      extra_body:
        generationConfig:
          maxOutputTokens: 4096
    models:
      default:
        provider_model: gemini-3.1-pro-preview
`)
	_, err := LoadProfiles(configPath)
	if err == nil {
		t.Fatalf("expected LoadProfiles to reject cli max token options")
	}
	if !strings.Contains(err.Error(), "options.extra_body.generationConfig.maxOutputTokens") {
		t.Fatalf("unexpected error: %v", err)
	}

	configPath = filepath.Join(tmp, "bad-cli-args.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  gemini-cli:
    type: cli
    cli_type: gemini
    command: gemini
    args:
      - --max-output-tokens=4096
    models:
      default:
        provider_model: gemini-3.1-pro-preview
`)
	_, err = LoadProfiles(configPath)
	if err == nil {
		t.Fatalf("expected LoadProfiles to reject cli max token args")
	}
	if !strings.Contains(err.Error(), "args[0]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadProfilesRejectsUnknownProviderModelFields(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "bad-model-field.yaml")
	writeConfigTestFile(t, configPath, `
schema_version: 1
providers:
  api:
    type: api
    protocol: openai-compatible
    base_url: http://127.0.0.1:1
    models:
      default:
        provider_model: test-model
        max_tokens: 4096
`)
	_, err := LoadProfiles(configPath)
	if err == nil {
		t.Fatalf("expected LoadProfiles to reject unknown provider model field")
	}
	if !strings.Contains(err.Error(), `unknown provider model field "max_tokens"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestModelIDsAndSingleModelID(t *testing.T) {
	provider := ProviderConfig{
		Models: map[string]ProviderModelConfig{
			"b": {ProviderModel: "b"},
			"a": {ProviderModel: "a"},
		},
	}
	ids := ModelIDs(provider)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected model ids: %#v", ids)
	}
	if _, ok := singleModelID(provider); ok {
		t.Fatalf("expected multiple models to fail inference")
	}
	if got, ok := singleModelID(ProviderConfig{
		Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}},
	}); !ok || got != "default" {
		t.Fatalf("unexpected single model inference: %s %t", got, ok)
	}
}

func samePath(t *testing.T, left string, right string) bool {
	t.Helper()
	leftEval, err := filepath.EvalSymlinks(left)
	if err != nil {
		leftEval = left
	}
	rightEval, err := filepath.EvalSymlinks(right)
	if err != nil {
		rightEval = right
	}
	return leftEval == rightEval
}

func writeConfigTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
