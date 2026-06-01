package app

import (
	"bytes"
	"context"
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
	if err := os.WriteFile(configPath, []byte(`schema_version: 1
defaults:
  success_criteria: [ok]
  per_task_timeout: 1m
  verification_policy:
    allow_semantic_verifier: true
output:
  directory: ./out/{requestId}
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
	if err := os.WriteFile(configPath, []byte(`schema_version: 1
defaults:
  success_criteria: [ok]
output:
  directory: ./out/{requestId}
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
`), 0o644); err != nil {
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
