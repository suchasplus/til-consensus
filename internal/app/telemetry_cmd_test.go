package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/telemetry"
)

func TestTelemetryDailyCommandWritesMarkdown(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "tc_123")
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := telemetry.WriteProviderReadinessFile(filepath.Join(artifactsDir, "provider-readiness.json"), []telemetry.ProviderReadinessEntry{
		{Provider: "claude", Ready: true, StrictJSON: true, RecoverableJSON: true, DurationMs: 1200},
	}, time.Now().UTC()); err != nil {
		t.Fatalf("write provider readiness: %v", err)
	}
	summary := telemetry.ComplianceSummaryFile{
		Version: 1,
		Entries: []telemetry.ComplianceSummaryEntry{
			{Provider: "claude", ProviderType: "cli", ProviderModel: "claude-opus-4-6", TaskKind: "propose", Total: 1, Strict: 1},
		},
	}
	body, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "strict-compliance-summary.json"), append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	runTelemetry := telemetry.RunTelemetryFile{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RequestID:   "tc_123",
		SessionID:   "session-1",
		Mode:        "adjudication",
		Providers:   []string{"claude/claude-opus-4-6"},
		TaskSummary: []telemetry.RunTaskSummary{{TaskKind: "propose", Total: 1, Strict: 1}},
		WorkflowSummary: telemetry.WorkflowSummary{
			Claims:               2,
			KeepWithCaveatClaims: 1,
			UnresolvedClaims:     0,
		},
		Result: telemetry.RunTelemetryResult{
			PrimaryResult: "partially_supported",
			TaskVerdict:   "partially_supported",
			TerminalState: "completed",
		},
		Timing: telemetry.RunTelemetryTiming{ElapsedMs: 5500},
	}
	if err := telemetry.WriteRunTelemetryFile(filepath.Join(artifactsDir, "run-telemetry.json"), runTelemetry); err != nil {
		t.Fatalf("write run telemetry: %v", err)
	}

	cmd := newTelemetryDailyCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"daily", "--root", root, "--since", "48h"}); err != nil {
		t.Fatalf("telemetry daily failed: %v", err)
	}
	got := stdout.String()
	for _, needle := range []string{"每日 Telemetry 汇总", "Provider Readiness", "Task Compliance", "Workflow Quality", "claude", "tc_123"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected %q in telemetry report\n%s", needle, got)
		}
	}
}

func TestResolveTelemetryRootUsesConfigOutputRoot(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	writeFile(t, configPath, `schema_version: 1
output:
  directory: "./logs/out/{requestId}"
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
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
    arbiter: arbiter-a
`)
	root, err := resolveTelemetryRoot(configPath, "")
	if err != nil {
		t.Fatalf("resolveTelemetryRoot failed: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(root), "/logs/out") {
		t.Fatalf("unexpected telemetry root: %s", root)
	}
}
