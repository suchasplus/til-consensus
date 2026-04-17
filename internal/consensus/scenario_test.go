package consensus_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	memorystore "github.com/suchasplus/til-consensus/internal/store/memory"
)

type scenarioLedger struct {
	entries []consensus.EvidenceRecord
}

func (l *scenarioLedger) Append(_ context.Context, entry consensus.EvidenceRecord) (consensus.EvidenceRecord, error) {
	entry.Seq = len(l.entries)
	l.entries = append(l.entries, entry)
	return entry, nil
}

type scenarioDelegate struct {
	tasks map[string]consensus.Task
	next  int
}

func (d *scenarioDelegate) Dispatch(_ context.Context, task consensus.Task) (consensus.DispatchReceipt, error) {
	if d.tasks == nil {
		d.tasks = map[string]consensus.Task{}
	}
	taskID := fmt.Sprintf("task-%d", d.next)
	d.next++
	d.tasks[taskID] = task
	return consensus.DispatchReceipt{TaskID: taskID, AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *scenarioDelegate) Await(_ context.Context, taskID string, _ time.Duration) (consensus.AwaitedTask, error) {
	task := d.tasks[taskID]
	switch value := task.(type) {
	case consensus.ProposalTask:
		return consensus.AwaitedTask{OK: true, Output: consensus.ProposalTaskResult{Output: consensus.ProposalOutput{
			Summary: "scenario proposal",
			Claims: []consensus.ClaimDraft{{
				Title:      "Scenario claim",
				Statement:  value.TaskSpec.Goal + " 成立",
				ClaimType:  consensus.ClaimTypeInference,
				Confidence: 0.6,
			}},
		}}}, nil
	case consensus.ChallengeTask:
		return consensus.AwaitedTask{OK: true, Output: consensus.ChallengeTaskResult{Output: consensus.ChallengeOutput{
			Summary: "no extra challenge",
			Tickets: nil,
		}}}, nil
	case consensus.ReviseTask:
		return consensus.AwaitedTask{OK: true, Output: consensus.ReviseTaskResult{Output: consensus.ReviseOutput{
			Summary: "scenario revise",
			Revisions: []consensus.ClaimRevisionDraft{{
				TargetClaimID:   firstClaimID(value.Claims),
				Action:          consensus.RevisionActionRevise,
				RevisedText:     value.TaskSpec.Goal + " 在新增证据下成立",
				ConfidenceDelta: 0.25,
				Reason:          "根据新增证据修订 claim",
			}},
		}}}, nil
	case consensus.ReportTask:
		return consensus.AwaitedTask{OK: true, Output: consensus.ReportTaskResult{Output: consensus.AdjudicationReport{
			Summary: "scenario report",
		}}}, nil
	case consensus.ActionTask:
		return consensus.AwaitedTask{OK: true, Output: consensus.ActionTaskResult{Output: consensus.ActionExecution{
			FullResponse: "executed",
			Summary:      "executed",
		}}}, nil
	default:
		return consensus.AwaitedTask{OK: false, Error: "unexpected task"}, nil
	}
}

func (d *scenarioDelegate) Cancel(_ context.Context, _ string) error { return nil }

type scenarioArbiter struct {
	reports []consensus.ArbiterReport
	calls   int
}

func (a *scenarioArbiter) Decide(_ context.Context, input consensus.ArbiterInput) (consensus.ArbiterReport, error) {
	idx := a.calls
	a.calls++
	if idx >= len(a.reports) {
		idx = len(a.reports) - 1
	}
	report := a.reports[idx]
	claimID := ""
	if len(input.Claims) > 0 {
		claimID = input.Claims[0].ClaimID
	}
	for i := range report.Decisions {
		if strings.TrimSpace(report.Decisions[i].ClaimID) == "" {
			report.Decisions[i].ClaimID = claimID
		}
	}
	for i := range report.Records {
		if strings.TrimSpace(report.Records[i].TargetClaimID) == "" {
			report.Records[i].TargetClaimID = claimID
		}
	}
	return report, nil
}

func TestScenarioFallbackEvidenceReversal(t *testing.T) {
	request, artifactsDir, expected := loadScenarioRequest(t, "fallback-reversal")
	ledger := &scenarioLedger{}
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: &scenarioDelegate{},
		Arbiter: &scenarioArbiter{reports: []consensus.ArbiterReport{
			{
				TaskVerdict: consensus.TaskVerdictUndetermined,
				Summary:     "initial insufficient evidence",
				Decisions: []consensus.ArbiterDecision{{
					Verdict:    consensus.ClaimVerdictUndetermined,
					Confidence: 0.35,
					Rationale:  "initial insufficient evidence",
				}},
				Records: []consensus.AdjudicationRecord{{
					Disposition:     consensus.ClaimDispositionUnresolved,
					Rationale:       "initial insufficient evidence",
					FinalConfidence: 0.35,
					Actionability:   consensus.ActionabilityBlocked,
				}},
			},
			{
				TaskVerdict: consensus.TaskVerdictSupported,
				Summary:     "fresh evidence retained the claim",
				Decisions: []consensus.ArbiterDecision{{
					Verdict:    consensus.ClaimVerdictSupported,
					Confidence: 0.88,
					Rationale:  "fresh evidence retained the claim",
				}},
				Records: []consensus.AdjudicationRecord{{
					Disposition:     consensus.ClaimDispositionKeep,
					Rationale:       "fresh evidence retained the claim",
					FinalConfidence: 0.88,
					Actionability:   consensus.ActionabilityReady,
				}},
			},
		}},
		Ledger:       ledger,
		SessionStore: memorystore.New(),
		ArtifactDir:  artifactsDir,
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Adjudication == nil || result.Adjudication.TaskVerdict != consensus.TaskVerdictSupported {
		t.Fatalf("expected supported adjudication result, got %#v", result)
	}
	foundStructuredMetadata := false
	for _, entry := range ledger.entries {
		if entry.Kind != consensus.EvidenceKindSourceMaterial {
			continue
		}
		if score, ok := entry.Metadata["score"].(float64); ok && score == 0.92 {
			foundStructuredMetadata = true
			break
		}
	}
	if !foundStructuredMetadata {
		t.Fatalf("expected structured ingest metadata in ledger, got %#v", ledger.entries)
	}
	assertSummaryFragments(t, artifact.BuildSummary(result), expected)
}

func TestScenarioObserveNegatesAction(t *testing.T) {
	request, artifactsDir, expected := loadScenarioRequest(t, "observe-negates-action")
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: &scenarioDelegate{},
		Arbiter: &scenarioArbiter{reports: []consensus.ArbiterReport{{
			TaskVerdict: consensus.TaskVerdictSupported,
			Summary:     "claim retained before action",
			Decisions: []consensus.ArbiterDecision{{
				Verdict:    consensus.ClaimVerdictSupported,
				Confidence: 0.81,
				Rationale:  "claim retained before action",
			}},
			Records: []consensus.AdjudicationRecord{{
				Disposition:     consensus.ClaimDispositionKeep,
				Rationale:       "claim retained before action",
				FinalConfidence: 0.81,
				Actionability:   consensus.ActionabilityReady,
			}},
		}}},
		SessionStore: memorystore.New(),
		ArtifactDir:  artifactsDir,
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.TerminalState != consensus.TerminalStateRequiresHumanReview {
		t.Fatalf("expected requires_human_review, got %#v", result)
	}
	foundFollowUp := false
	for _, item := range result.Observations {
		if item.Outcome == consensus.ObservationOutcomeContradicted {
			if item.FollowUpArtifact == nil || item.FollowUpArtifact.Path == "" {
				t.Fatalf("expected follow-up artifact, got %#v", item)
			}
			if _, err := os.Stat(item.FollowUpArtifact.Path); err != nil {
				t.Fatalf("expected follow-up artifact path to exist: %v", err)
			}
			foundFollowUp = true
		}
	}
	if !foundFollowUp {
		t.Fatalf("expected contradictory observation with follow-up artifact, got %#v", result.Observations)
	}
	assertSummaryFragments(t, artifact.BuildSummary(result), expected)
}

func TestScenarioCodingCompositeChecks(t *testing.T) {
	request, artifactsDir, expected := loadScenarioRequest(t, "coding-composite")
	ledger := &scenarioLedger{}
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: &scenarioDelegate{},
		Arbiter: &scenarioArbiter{reports: []consensus.ArbiterReport{{
			TaskVerdict: consensus.TaskVerdictSupported,
			Summary:     "coding composite checks passed",
			Decisions: []consensus.ArbiterDecision{{
				Verdict:    consensus.ClaimVerdictSupported,
				Confidence: 0.86,
				Rationale:  "git diff、tests 和 benchmark 都通过",
			}},
			Records: []consensus.AdjudicationRecord{{
				Disposition:     consensus.ClaimDispositionKeep,
				Rationale:       "git diff、tests 和 benchmark 都通过",
				FinalConfidence: 0.86,
				Actionability:   consensus.ActionabilityReady,
			}},
		}}},
		Ledger:       ledger,
		SessionStore: memorystore.New(),
		ArtifactDir:  artifactsDir,
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Adjudication == nil || result.Adjudication.TaskVerdict != consensus.TaskVerdictSupported {
		t.Fatalf("expected supported result, got %#v", result)
	}
	var sawDiff, sawTest, sawBench bool
	for _, item := range result.Adjudication.VerificationResults {
		switch item.CheckName {
		case "diff":
			sawDiff = item.Status == consensus.VerificationStatusPassed
		case "unit":
			sawTest = item.Status == consensus.VerificationStatusPassed
		case "perf":
			sawBench = item.Status == consensus.VerificationStatusPassed
		}
	}
	if !sawDiff || !sawTest || !sawBench {
		t.Fatalf("expected diff/test/bench checks to pass, got %#v", result.Adjudication.VerificationResults)
	}
	assertSummaryFragments(t, artifact.BuildSummary(result), expected)
}

func TestScenarioFactualConflictAndFreshness(t *testing.T) {
	request, artifactsDir, expected := loadScenarioRequest(t, "factual-conflict")
	ledger := &scenarioLedger{}
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: &scenarioDelegate{},
		Arbiter: &scenarioArbiter{reports: []consensus.ArbiterReport{{
			TaskVerdict: consensus.TaskVerdictUndetermined,
			Summary:     "fresh source conflicts with older source",
			Decisions: []consensus.ArbiterDecision{{
				Verdict:    consensus.ClaimVerdictUndetermined,
				Confidence: 0.42,
				Rationale:  "来源相互冲突，且需要人工确认 freshness",
			}},
			Records: []consensus.AdjudicationRecord{{
				Disposition:     consensus.ClaimDispositionUnresolved,
				Rationale:       "来源相互冲突，且需要人工确认 freshness",
				FinalConfidence: 0.42,
				Actionability:   consensus.ActionabilityBlocked,
			}},
		}}},
		Ledger:       ledger,
		SessionStore: memorystore.New(),
		ArtifactDir:  artifactsDir,
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Adjudication == nil || result.Adjudication.TaskVerdict != consensus.TaskVerdictUndetermined {
		t.Fatalf("expected undetermined adjudication, got %#v", result)
	}
	foundFreshMetadata := false
	foundConflictFailure := false
	for _, entry := range ledger.entries {
		if entry.Kind != consensus.EvidenceKindSourceMaterial {
			continue
		}
		switch publishedAt := entry.Metadata["publishedAt"].(type) {
		case string:
			if publishedAt == "2026-04-15T09:00:00Z" {
				foundFreshMetadata = true
			}
		case time.Time:
			if publishedAt.UTC().Format(time.RFC3339) == "2026-04-15T09:00:00Z" {
				foundFreshMetadata = true
			}
		}
		if failureClass, ok := entry.Metadata["failureClass"].(string); ok && failureClass == "structured_failure" {
			foundConflictFailure = true
		}
	}
	if !foundFreshMetadata || !foundConflictFailure {
		t.Fatalf("expected freshness/conflict metadata in ledger, got %#v", ledger.entries)
	}
	assertSummaryFragments(t, artifact.BuildSummary(result), expected)
}

func loadScenarioRequest(t *testing.T, name string) (consensus.StartRequest, string, []string) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "scenarios", name)
	tmp := t.TempDir()
	if err := copyDir(root, tmp); err != nil {
		t.Fatalf("copy scenario fixture: %v", err)
	}
	if err := prepareScenarioWorkspace(tmp, name); err != nil {
		t.Fatalf("prepare scenario workspace: %v", err)
	}
	input, err := config.LoadRunInput(filepath.Join(tmp, "run.yaml"))
	if err != nil {
		t.Fatalf("LoadRunInput failed: %v", err)
	}
	loaded := config.LoadedConfig{
		ConfigDir: tmp,
		Config: config.Normalize(config.Config{
			SchemaVersion: 1,
			Output: config.OutputConfig{
				Directory: "./out/{requestId}",
			},
		}),
	}
	plan, err := config.ResolveRunPlan(loaded, input, config.RunOverrides{}, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("ResolveRunPlan failed: %v", err)
	}
	expectedBody, err := os.ReadFile(filepath.Join(tmp, "expected-run-summary.txt"))
	if err != nil {
		t.Fatalf("read expected summary: %v", err)
	}
	return plan.StartRequest, plan.ArtifactsDir, splitExpectedFragments(string(expectedBody))
}

func prepareScenarioWorkspace(root string, name string) error {
	switch name {
	case "coding-composite":
		if err := runCmd(root, "git", "init"); err != nil {
			return err
		}
		if err := runCmd(root, "git", "config", "user.email", "test@example.com"); err != nil {
			return err
		}
		if err := runCmd(root, "git", "config", "user.name", "Test User"); err != nil {
			return err
		}
		if err := runCmd(root, "git", "add", "."); err != nil {
			return err
		}
		if err := runCmd(root, "git", "commit", "-m", "base"); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "internal", "service.go"), []byte("package internal\n\nconst Enabled = true\n"), 0o644); err != nil {
			return err
		}
		return nil
	default:
		return nil
	}
}

func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}

func splitExpectedFragments(body string) []string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if item := strings.TrimSpace(line); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func assertSummaryFragments(t *testing.T, summary string, expected []string) {
	t.Helper()
	for _, fragment := range expected {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary missing fragment %q\n%s", fragment, summary)
		}
	}
}

func firstClaimID(claims []consensus.ClaimNode) string {
	if len(claims) == 0 {
		return ""
	}
	return claims[0].ClaimID
}
