package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

func TestE2EQuickstartCommandChainAndWeb(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")

	var initOut bytes.Buffer
	if err := runConfigInitCommand(&initOut, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("config init failed: %v", err)
	}

	validateCmd := newConfigValidateCommand()
	validateStdout, validateStderr, err := runCLICommand(context.Background(), validateCmd, []string{"validate", "--config", configPath})
	if err != nil {
		t.Fatalf("config validate failed: %v\nstderr=%s", err, validateStderr)
	}
	if !strings.Contains(validateStdout, "config is valid") {
		t.Fatalf("unexpected validate output: %s", validateStdout)
	}

	runCmd := newRunCommand()
	runStdout, runStderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--task", "Should we use a monorepo or polyrepo for our microservices?"})
	if err != nil {
		t.Fatalf("quickstart run failed: %v\nstderr=%s", err, runStderr)
	}
	resultPath := extractResultPath(t, runStdout)

	viewCmd := newViewCommand()
	viewStdout, viewStderr, err := runCLICommand(context.Background(), viewCmd, []string{"view", "--result", resultPath, "--verbose"})
	if err != nil {
		t.Fatalf("quickstart view failed: %v\nstderr=%s", err, viewStderr)
	}
	for _, fragment := range []string{"运行头部", "mode: adjudication", "关键 Claims", "验证明细"} {
		if !strings.Contains(viewStdout, fragment) {
			t.Fatalf("expected %q in view output\n%s", fragment, viewStdout)
		}
	}
	if strings.Contains(viewStdout, "git diff 执行失败") || strings.Contains(viewStdout, "读取 git revision 失败") {
		t.Fatalf("quickstart view should not contain coding-only verification noise\n%s", viewStdout)
	}

	webCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	webCmd := newViewCommand()
	webStdout := &syncBuffer{}
	webStderr := &syncBuffer{}
	webCmd.Writer = webStdout
	webCmd.ErrWriter = webStderr
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- webCmd.Run(webCtx, []string{"view", "--config", configPath, "--web", "--host", "127.0.0.1", "--port", "0"})
	}()
	urlValue, err := waitForWebURL(webStdout, doneCh)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitForHealthz(urlValue+"/api/healthz", doneCh); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(urlValue + "/api/document")
	if err != nil {
		t.Fatalf("fetch api/document failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	var doc viewer.Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode api/document failed: %v", err)
	}
	if doc.Overview.Mode != string(consensus.WorkflowModeAdjudication) {
		t.Fatalf("unexpected web mode: %#v", doc.Overview)
	}
	if doc.Overview.RequestID == "" || len(doc.Claims) == 0 {
		t.Fatalf("unexpected web document: %#v", doc)
	}
	cancel()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("web view command returned error: %v\nstderr=%s", err, webStderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for web view command to exit")
	}
}

func TestE2EScenarioFixtureMatrix(t *testing.T) {
	tests := []struct {
		name   string
		assert func(t *testing.T, result consensus.RunResult, ledger []consensus.EvidenceRecord, summary string)
	}{
		{
			name: "coding-composite",
			assert: func(t *testing.T, result consensus.RunResult, _ []consensus.EvidenceRecord, summary string) {
				t.Helper()
				if result.CaseManifest == nil || result.CaseManifest.TaskType != consensus.CaseTaskTypeCoding {
					t.Fatalf("unexpected case manifest: %#v", result.CaseManifest)
				}
				if result.Metrics.VerificationsRun == 0 {
					t.Fatalf("expected verifications to run, got %#v", result.Metrics)
				}
				if !strings.Contains(summary, "判断这个 patch 是否既修复了问题又没有引入性能回退") {
					t.Fatalf("unexpected summary:\n%s", summary)
				}
			},
		},
		{
			name: "factual-conflict",
			assert: func(t *testing.T, result consensus.RunResult, ledger []consensus.EvidenceRecord, _ string) {
				t.Helper()
				if result.CaseManifest == nil || result.CaseManifest.TaskType != consensus.CaseTaskTypeFactual {
					t.Fatalf("unexpected case manifest: %#v", result.CaseManifest)
				}
				hasSource := false
				hasFailureClass := false
				for _, entry := range ledger {
					if entry.Kind != consensus.EvidenceKindSourceMaterial {
						continue
					}
					hasSource = true
					if failureClass, ok := entry.Metadata["failureClass"].(string); ok && failureClass == "structured_failure" {
						hasFailureClass = true
					}
				}
				if !hasSource || !hasFailureClass {
					t.Fatalf("expected structured source conflict evidence, got %#v", ledger)
				}
			},
		},
		{
			name: "fallback-reversal",
			assert: func(t *testing.T, result consensus.RunResult, ledger []consensus.EvidenceRecord, _ string) {
				t.Helper()
				if result.Adjudication == nil || result.Adjudication.TaskVerdict != consensus.TaskVerdictSupported {
					t.Fatalf("expected supported adjudication, got %#v", result.Adjudication)
				}
				if result.TerminalState != consensus.TerminalStateCompleted {
					t.Fatalf("expected completed terminal state, got %#v", result)
				}
				hasSource := false
				for _, entry := range ledger {
					if entry.Kind == consensus.EvidenceKindSourceMaterial {
						hasSource = true
						break
					}
				}
				if !hasSource {
					t.Fatalf("expected fallback ingest evidence, got %#v", ledger)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := stageScenarioFixture(t, tc.name)
			configPath := filepath.Join(root, "til-consensus.yaml")
			writeE2EMockConfig(t, configPath, filepath.Join(root, "out", "{requestId}"))

			runCmd := newRunCommand()
			runStdout, runStderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--input", filepath.Join(root, "run.yaml")})
			if err != nil {
				t.Fatalf("scenario run failed: %v\nstderr=%s", err, runStderr)
			}
			resultPath := extractResultPath(t, runStdout)
			summaryPath := filepath.Join(filepath.Dir(resultPath), "summary.md")
			summaryBody, err := os.ReadFile(summaryPath)
			if err != nil {
				t.Fatalf("read summary failed: %v", err)
			}
			result := loadRunResult(t, resultPath)
			ledger := loadLedgerEntries(t, filepath.Join(filepath.Dir(resultPath), "ledger.jsonl"))
			tc.assert(t, result, ledger, string(summaryBody))

			viewCmd := newViewCommand()
			viewStdout, viewStderr, err := runCLICommand(context.Background(), viewCmd, []string{"view", "--result", resultPath, "--verbose"})
			if err != nil {
				t.Fatalf("scenario view failed: %v\nstderr=%s", err, viewStderr)
			}
			if !strings.Contains(viewStdout, "运行头部") || !strings.Contains(viewStdout, "相关文件") {
				t.Fatalf("unexpected scenario view output:\n%s", viewStdout)
			}
		})
	}
}

func TestE2EObserveNegatesActionFixtureFollowupChain(t *testing.T) {
	root := stageScenarioFixture(t, "observe-negates-action")
	configPath := filepath.Join(root, "til-consensus.yaml")
	writeE2EMockConfig(t, configPath, filepath.Join(root, "out", "{requestId}"))

	runCmd := newRunCommand()
	runStdout, runStderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--input", filepath.Join(root, "run.yaml")})
	if err != nil {
		t.Fatalf("observe scenario run failed: %v\nstderr=%s", err, runStderr)
	}
	resultPath := extractResultPath(t, runStdout)
	result := loadRunResult(t, resultPath)
	if result.TerminalState != consensus.TerminalStateRequiresHumanReview {
		t.Fatalf("expected requires_human_review, got %#v", result)
	}
	hasReopenedWithArtifact := false
	for _, observation := range result.Observations {
		if observation.Reopen && observation.FollowUpArtifact != nil {
			hasReopenedWithArtifact = true
			break
		}
	}
	if !hasReopenedWithArtifact {
		t.Fatalf("expected reopened observation with followup artifact, got %#v", result.Observations)
	}

	artifacts, err := filepath.Glob(filepath.Join(filepath.Dir(resultPath), "artifacts", "followups", "*.json"))
	if err != nil {
		t.Fatalf("glob followup artifacts: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("expected followup artifact to be created")
	}

	followupCmd := newFollowUpCommand()
	followStdout, followStderr, err := runCLICommand(context.Background(), followupCmd, []string{"followup", "run", "--config", configPath, "--artifact", artifacts[0]})
	if err != nil {
		t.Fatalf("followup run failed: %v\nstderr=%s", err, followStderr)
	}
	childResultPath := extractResultPath(t, followStdout)

	viewCmd := newViewCommand()
	viewStdout, viewStderr, err := runCLICommand(context.Background(), viewCmd, []string{"view", "--result", childResultPath, "--section", "observations", "--section", "followups", "--verbose"})
	if err != nil {
		t.Fatalf("view child followup failed: %v\nstderr=%s", err, viewStderr)
	}
	for _, fragment := range []string{"Observations", "Follow-ups", "parent request=", "triggered by observation="} {
		if !strings.Contains(viewStdout, fragment) {
			t.Fatalf("expected %q in followup view output\n%s", fragment, viewStdout)
		}
	}
}

func runCLICommand(ctx context.Context, cmd *cli.Command, args []string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	setCommandWriters(cmd, &stdout, &stderr)
	err := cmd.Run(ctx, args)
	return stdout.String(), stderr.String(), err
}

func setCommandWriters(cmd *cli.Command, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	cmd.Writer = stdout
	cmd.ErrWriter = stderr
	for _, child := range cmd.Commands {
		setCommandWriters(child, stdout, stderr)
	}
}

func extractResultPath(t *testing.T, stdout string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*result:\s+(.+?/result\.json)\s*$`)
	match := re.FindStringSubmatch(stdout)
	if len(match) != 2 {
		t.Fatalf("result path not found in stdout:\n%s", stdout)
	}
	return strings.TrimSpace(match[1])
}

func stageScenarioFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "scenarios", name)
	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copy scenario fixture failed: %v", err)
	}
	if err := prepareScenarioWorkspace(dst, name); err != nil {
		t.Fatalf("prepare scenario workspace failed: %v", err)
	}
	return dst
}

func writeE2EMockConfig(t *testing.T, path string, outputDir string) {
	t.Helper()
	cfg := config.Normalize(config.Config{
		SchemaVersion: 1,
		Defaults:      config.InitTemplate().Defaults,
		Output: config.OutputConfig{
			Directory: outputDir,
		},
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:     config.ProviderTypeMock,
				Behavior: "deterministic",
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "mock-default"},
				},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default", Role: "proposer"},
			{ID: "challenger-a", Provider: "mock", Model: "default", Role: "challenger"},
			{ID: "arbiter-a", Provider: "mock", Model: "default", Role: "arbiter"},
			{ID: "verifier-a", Provider: "mock", Model: "default", Role: "semantic-verifier"},
			{ID: "reporter-a", Provider: "mock", Model: "default", Role: "reporter"},
			{ID: "actor-a", Provider: "mock", Model: "default", Role: "actor"},
			{ID: "participant-a", Provider: "mock", Model: "default", Role: "participant"},
			{ID: "participant-b", Provider: "mock", Model: "default", Role: "participant"},
			{ID: "participant-c", Provider: "mock", Model: "default", Role: "participant"},
			{ID: "facilitator-a", Provider: "mock", Model: "default", Role: "facilitator"},
		},
		Roles: config.RolesConfig{
			Proposers:        []string{"proposer-a"},
			Challengers:      []string{"challenger-a"},
			Participants:     []string{"participant-a", "participant-b", "participant-c"},
			Arbiter:          "arbiter-a",
			SemanticVerifier: "verifier-a",
			Facilitator:      "facilitator-a",
			Reporter:         "reporter-a",
			Actor:            "actor-a",
		},
	})
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("write e2e config failed: %v", err)
	}
}

func prepareScenarioWorkspace(root string, name string) error {
	switch name {
	case "coding-composite":
		if err := runExternalCmd(root, "git", "init"); err != nil {
			return err
		}
		if err := runExternalCmd(root, "git", "config", "user.email", "test@example.com"); err != nil {
			return err
		}
		if err := runExternalCmd(root, "git", "config", "user.name", "Test User"); err != nil {
			return err
		}
		if err := runExternalCmd(root, "git", "add", "."); err != nil {
			return err
		}
		if err := runExternalCmd(root, "git", "commit", "-m", "base"); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, "internal", "service.go"), []byte("package internal\n\nconst Enabled = true\n"), 0o644)
	default:
		return nil
	}
}

func runExternalCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyDir(src string, dst string) error {
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
			return os.MkdirAll(target, info.Mode())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode())
	})
}

func loadRunResult(t *testing.T, path string) consensus.RunResult {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result failed: %v", err)
	}
	result, err := consensus.DecodeRunResult(body)
	if err != nil {
		t.Fatalf("decode result failed: %v", err)
	}
	return result
}

func loadLedgerEntries(t *testing.T, path string) []consensus.EvidenceRecord {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open ledger failed: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()
	out := make([]consensus.EvidenceRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry consensus.EvidenceRecord
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode ledger entry failed: %v\n%s", err, line)
		}
		out = append(out, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ledger failed: %v", err)
	}
	return out
}
