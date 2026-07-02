package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func TestBuildRunTelemetryAdjudication(t *testing.T) {
	result := consensus.RunResult{
		RequestID:     "tc_123",
		SessionID:     "session-1",
		Mode:          consensus.WorkflowModeAdjudication,
		TerminalState: consensus.TerminalStateCompleted,
		Metrics:       consensus.Metrics{ElapsedMs: 9100},
		Adjudication: &consensus.AdjudicationResultSection{
			TaskVerdict: consensus.TaskVerdictPartiallySupported,
			ClaimGraph: []consensus.ClaimNode{
				{ClaimID: "c1", Verdict: consensus.ClaimVerdictSupported, Disposition: consensus.ClaimDispositionKeepWithCaveat},
				{ClaimID: "c2", Verdict: consensus.ClaimVerdictUndetermined, Disposition: consensus.ClaimDispositionUnresolved},
			},
			ChallengeTickets: []consensus.ChallengeTicket{{TicketID: "t1"}},
			VerificationResults: []consensus.VerificationResult{
				{Status: consensus.VerificationStatusPassed},
				{Status: consensus.VerificationStatusInconclusive},
			},
		},
	}
	summary := ComplianceSummaryFile{
		Entries: []ComplianceSummaryEntry{
			{Provider: "claude", ProviderModel: "claude-opus-4-6", TaskKind: consensus.TaskKindPropose, Total: 1, Strict: 1},
			{Provider: "codex", ProviderModel: "gpt-5.4", TaskKind: consensus.TaskKindSemanticVerify, Total: 2, Repaired: 1, Failed: 1},
		},
	}
	got := BuildRunTelemetry(result, summary, "/tmp/run/artifacts", time.Date(2026, 4, 20, 1, 2, 3, 0, time.UTC))
	if got.RequestID != "tc_123" || got.Mode != consensus.WorkflowModeAdjudication {
		t.Fatalf("unexpected run telemetry header: %#v", got)
	}
	if got.WorkflowSummary.KeepWithCaveatClaims != 1 || got.WorkflowSummary.UnresolvedClaims != 1 {
		t.Fatalf("unexpected workflow summary: %#v", got.WorkflowSummary)
	}
	if got.VerificationSummary.Passed != 1 || got.VerificationSummary.Inconclusive != 1 {
		t.Fatalf("unexpected verification summary: %#v", got.VerificationSummary)
	}
	if len(got.TaskSummary) != 2 {
		t.Fatalf("unexpected task summary: %#v", got.TaskSummary)
	}
}

func TestBuildDailyReport(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "tc_123")
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := WriteProviderReadinessFile(filepath.Join(artifactsDir, "provider-readiness.json"), []ProviderReadinessEntry{
		{Provider: "claude", Ready: true, StrictJSON: true, RecoverableJSON: true, DurationMs: 1000},
		{Provider: "gemini", Ready: false, StrictJSON: false, RecoverableJSON: false, DurationMs: 3000, Error: "timeout"},
	}, time.Now().UTC()); err != nil {
		t.Fatalf("write readiness: %v", err)
	}
	summary := ComplianceSummaryFile{
		Version: 1,
		Entries: []ComplianceSummaryEntry{
			{Provider: "claude", ProviderModel: "claude-opus-4-6", TaskKind: consensus.TaskKindPropose, Total: 1, Strict: 1},
			{Provider: "codex", ProviderModel: "gpt-5.4", TaskKind: consensus.TaskKindSemanticVerify, Total: 1, Repaired: 1},
		},
	}
	body, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "strict-compliance-summary.json"), append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	runTelemetry := RunTelemetryFile{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RequestID:   "tc_123",
		SessionID:   "session-1",
		Mode:        consensus.WorkflowModeAdjudication,
		Result: RunTelemetryResult{
			PrimaryResult: "partially_supported",
			TaskVerdict:   "partially_supported",
			TerminalState: consensus.TerminalStateCompleted,
		},
		WorkflowSummary: WorkflowSummary{
			KeepWithCaveatClaims: 2,
			UnresolvedClaims:     0,
		},
		Timing: RunTelemetryTiming{ElapsedMs: 8800},
	}
	if err := WriteRunTelemetryFile(filepath.Join(artifactsDir, "run-telemetry.json"), runTelemetry); err != nil {
		t.Fatalf("write run telemetry: %v", err)
	}

	report, err := BuildDailyReport(root, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("BuildDailyReport failed: %v", err)
	}
	if len(report.Readiness) != 2 || len(report.TaskCompliance) != 2 || len(report.Workflow) != 1 {
		t.Fatalf("unexpected daily report: %#v", report)
	}
	bodyText := RenderDailyMarkdown(report)
	for _, needle := range []string{"每日 Telemetry 汇总", "Provider Readiness", "Task Compliance", "Workflow Quality", "tc_123"} {
		if !strings.Contains(bodyText, needle) {
			t.Fatalf("expected %q in markdown\n%s", needle, bodyText)
		}
	}
}
