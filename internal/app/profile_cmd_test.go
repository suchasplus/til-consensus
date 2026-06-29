package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/telemetry"
	"github.com/suchasplus/til-consensus/internal/viewer"
)

func TestProfilePreflightCommandWritesReadinessArtifacts(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
defaults:
  success_criteria: [ok]
  per_task_timeout: 1m
  verification_policy:
    allow_semantic_verifier: true
output:
  directory: %s
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--all", "--verbose"}); err != nil {
		t.Fatalf("profile preflight failed: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{"profile preflight completed ready=1/1", "mock/mock", "readiness:"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("preflight output missing %q:\n%s", needle, output)
		}
	}
	entryIndex := strings.Index(output, "  - mock/mock")
	finalIndex := strings.Index(output, "[til-consensus] profile preflight completed ready=1/1")
	if entryIndex < 0 || finalIndex < 0 || entryIndex > finalIndex {
		t.Fatalf("expected provider block before final summary:\n%s", output)
	}
	resultPath := regexp.MustCompile(`result: ([^\n]+)`).FindStringSubmatch(output)
	if len(resultPath) != 2 {
		t.Fatalf("could not find result path in output:\n%s", output)
	}
	bundle, err := viewer.LoadBundle(viewer.InferRunFiles(strings.TrimSpace(resultPath[1])))
	if err != nil {
		t.Fatalf("load preflight bundle: %v", err)
	}
	if len(bundle.ProviderReadiness.Providers) != 1 || !bundle.ProviderReadiness.Providers[0].Ready {
		t.Fatalf("unexpected readiness: %#v", bundle.ProviderReadiness)
	}
}

func TestProfilePreflightAPIReportsMissingEnv(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
defaults:
  success_criteria: [ok]
output:
  directory: %s
providers:
  api:
    type: api
    protocol: openai-compatible
    base_url: http://127.0.0.1:1
    api_key_env: TIL_CONSENSUS_TEST_MISSING_KEY
    models:
      default:
        provider_model: test-model
agents:
  - id: proposer-a
    provider: api
    model: default
    role: proposer
  - id: challenger-a
    provider: api
    model: default
    role: challenger
  - id: arbiter-a
    provider: api
    model: default
    role: arbiter
roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("TIL_CONSENSUS_TEST_MISSING_KEY", "")

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--provider", "api"}); err != nil {
		t.Fatalf("profile preflight failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "env TIL_CONSENSUS_TEST_MISSING_KEY is not set") {
		t.Fatalf("expected missing env in output:\n%s", stdout.String())
	}
	resultPath := regexp.MustCompile(`readiness: ([^\n]+)`).FindStringSubmatch(stdout.String())
	if len(resultPath) != 2 {
		t.Fatalf("could not find readiness path in output:\n%s", stdout.String())
	}
	readiness, err := telemetry.ReadProviderReadinessFile(strings.TrimSpace(resultPath[1]))
	if err != nil {
		t.Fatalf("read readiness: %v", err)
	}
	if len(readiness.Providers) != 1 || readiness.Providers[0].Ready || readiness.Providers[0].APIKeyEnv != "TIL_CONSENSUS_TEST_MISSING_KEY" {
		t.Fatalf("unexpected readiness: %#v", readiness)
	}
}

func TestProfilePreflightCommandOutputOverride(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config", "til-consensus.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`schema_version: 1
defaults:
  success_criteria: [ok]
output:
  directory: ./config-relative-out/{requestId}
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	outputTemplate := filepath.Join(tmp, "override-out", "{requestId}")

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--output", outputTemplate, "--all"}); err != nil {
		t.Fatalf("profile preflight failed: %v", err)
	}
	resultPath := regexp.MustCompile(`result: ([^\n]+)`).FindStringSubmatch(stdout.String())
	if len(resultPath) != 2 {
		t.Fatalf("could not find result path in output:\n%s", stdout.String())
	}
	if !strings.HasPrefix(strings.TrimSpace(resultPath[1]), filepath.Join(tmp, "override-out")) {
		t.Fatalf("expected output override path, got %s", resultPath[1])
	}
	if _, err := os.Stat(strings.TrimSpace(resultPath[1])); err != nil {
		t.Fatalf("expected result under output override: %v", err)
	}
}

func TestProfilePreflightCommandRelativeOutputUsesWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	configDir := filepath.Join(tmp, "config")
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

	configPath := filepath.Join(configDir, "til-consensus.yaml")
	if err := os.WriteFile(configPath, []byte(`schema_version: 1
output:
  directory: ./out/{requestId}
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--all"}); err != nil {
		t.Fatalf("profile preflight failed: %v", err)
	}
	resultPath := regexp.MustCompile(`result: ([^\n]+)`).FindStringSubmatch(stdout.String())
	if len(resultPath) != 2 {
		t.Fatalf("could not find result path in output:\n%s", stdout.String())
	}
	got := strings.TrimSpace(resultPath[1])
	wantPrefix := filepath.Join(resolvedCWD, "out")
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected relative output under cwd %s, got %s", wantPrefix, got)
	}
	if strings.HasPrefix(got, configDir) {
		t.Fatalf("relative output should not be under config dir: %s", got)
	}
}

func TestProfilePreflightCommandDoesNotRequireWorkflowRoles(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "providers-only.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--all"}); err != nil {
		t.Fatalf("profile preflight should not require workflow roles: %v", err)
	}
	if !strings.Contains(stdout.String(), "profile preflight completed ready=1/1") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestProfilePreflightVerbosePrintsExpandedCLIArgs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "providers-only.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  antigravity-cli:
    type: cli
    cli_type: antigravity
    command: til-consensus-missing-agy-for-test
    models:
      default:
        provider_model: Gemini 3.5 Flash (High)
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--provider", "antigravity-cli", "--verbose"}); err != nil {
		t.Fatalf("profile preflight command should complete with readiness failure: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{
		`command: til-consensus-missing-agy-for-test --model "Gemini 3.5 Flash (High)" -p`,
		"只返回一个 JSON 对象",
		"provider: type=cli protocol=antigravity",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("preflight verbose output missing %q:\n%s", needle, output)
		}
	}
}

func TestProfilePreflightVerbosePrintsRealCodexTempPaths(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "providers-only.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  codex-cli:
    type: cli
    cli_type: codex
    command: til-consensus-missing-codex-for-test
    models:
      default:
        provider_model: gpt-5.5
        reasoning: xhigh
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--provider", "codex-cli", "--verbose"}); err != nil {
		t.Fatalf("profile preflight command should complete with readiness failure: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{
		"command: til-consensus-missing-codex-for-test exec -m gpt-5.5 -c model_reasoning_effort=xhigh",
		"--output-schema ",
		"til-consensus-codex-schema-",
		"--output-last-message ",
		"til-consensus-codex-last-message-",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("preflight verbose output missing %q:\n%s", needle, output)
		}
	}
	for _, forbidden := range []string{"<schema-file>", "<last-message-file>"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("preflight verbose output should not contain %q:\n%s", forbidden, output)
		}
	}
}

func TestProfilePreflightAgentFilterValidatesSelectedAgent(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "bad-agent.yaml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: verifier-a
    provider: missing
    model: default
`, filepath.ToSlash(filepath.Join(tmp, "out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := newProfilePreflightCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	err := cmd.Run(context.Background(), []string{"preflight", "--config", configPath, "--agent", "verifier-a"})
	if err == nil {
		t.Fatalf("expected selected invalid agent to fail")
	}
	if !strings.Contains(err.Error(), "agent verifier-a references unknown provider missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfilePreflightCommandColorizesFinalSummaryWhenForced(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	tmp := t.TempDir()
	successConfig := filepath.Join(tmp, "success.yaml")
	if err := os.WriteFile(successConfig, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
`, filepath.ToSlash(filepath.Join(tmp, "success-out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write success config: %v", err)
	}
	successCmd := newProfilePreflightCommand()
	var successOut bytes.Buffer
	successCmd.Writer = &successOut
	if err := successCmd.Run(context.Background(), []string{"preflight", "--config", successConfig, "--all"}); err != nil {
		t.Fatalf("success preflight failed: %v", err)
	}
	if !strings.Contains(successOut.String(), ansi(32, "[til-consensus] profile preflight completed ready=1/1")) {
		t.Fatalf("expected green final summary, got:\n%q", successOut.String())
	}

	failureConfig := filepath.Join(tmp, "failure.yaml")
	if err := os.WriteFile(failureConfig, []byte(fmt.Sprintf(`schema_version: 1
output:
  directory: %s
providers:
  api:
    type: api
    protocol: openai-compatible
    base_url: http://127.0.0.1:1
    api_key_env: TIL_CONSENSUS_TEST_MISSING_KEY
    models:
      default:
        provider_model: test-model
`, filepath.ToSlash(filepath.Join(tmp, "failure-out", "{requestId}")))), 0o644); err != nil {
		t.Fatalf("write failure config: %v", err)
	}
	t.Setenv("TIL_CONSENSUS_TEST_MISSING_KEY", "")
	failureCmd := newProfilePreflightCommand()
	var failureOut bytes.Buffer
	failureCmd.Writer = &failureOut
	if err := failureCmd.Run(context.Background(), []string{"preflight", "--config", failureConfig, "--provider", "api"}); err != nil {
		t.Fatalf("failure preflight command should complete with readiness failure: %v", err)
	}
	if !strings.Contains(failureOut.String(), ansi(31, "[til-consensus] profile preflight completed ready=0/1")) {
		t.Fatalf("expected red final summary, got:\n%q", failureOut.String())
	}
}
