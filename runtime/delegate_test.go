package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TestDelegatePersistsParseErrorArtifact(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:     config.ProviderTypeMock,
				Behavior: "malformed",
				Models:   map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers:   []string{"proposer-a"},
			Challengers: []string{"proposer-a"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// 此测试验证解析失败材料的完整落盘。一次失败还会执行修复、合规
	// 记录与多次 fsync；不要把慢 CI 的 1 秒调度/磁盘延迟误判为解析失败。
	// 超时语义由 TestDelegateTimeoutReturnsTimeoutMarker 独立验证。
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.Error == "__timeout__" {
		t.Fatalf("parse-error artifact task did not complete within its test allowance: %#v", awaited)
	}
	if awaited.OK {
		t.Fatal("expected malformed output to fail")
	}
	if awaited.Artifact == nil {
		t.Fatalf("expected parse error artifact, got %#v", awaited)
	}
	body, err := os.ReadFile(awaited.Artifact.Path)
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(awaited.Artifact.Path), "raw-error-") || !strings.Contains(string(body), "# --- raw text ---\nnot json") {
		t.Fatalf("expected persisted raw parse-error details, got %s: %s", awaited.Artifact.Path, body)
	}
}

func TestDelegateTimeoutReturnsTimeoutMarker(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:     config.ProviderTypeMock,
				Behavior: "timeout",
				Models:   map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers:   []string{"proposer-a"},
			Challengers: []string{"proposer-a"},
		},
	}), filepath.Join(tmp, "artifacts"))
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.Error != "__timeout__" {
		t.Fatalf("expected timeout marker, got %#v", awaited)
	}
}

func TestDelegateCancelDoesNotHoldMutexWhileCanceling(t *testing.T) {
	cancelStarted := make(chan struct{})
	releaseCancel := make(chan struct{})
	delegate := &Delegate{
		tasks: map[string]*taskEntry{
			"task-1": {
				cancel: func() {
					close(cancelStarted)
					<-releaseCancel
				},
				done: make(chan taskOutcome),
			},
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- delegate.Cancel(context.Background(), "task-1")
	}()
	<-cancelStarted

	lockAcquired := make(chan struct{})
	go func() {
		delegate.mu.Lock()
		_ = len(delegate.tasks)
		delegate.mu.Unlock()
		close(lockAcquired)
	}()
	select {
	case <-lockAcquired:
	case <-time.After(time.Second):
		close(releaseCancel)
		t.Fatal("Delegate.Cancel held mutex while invoking cancel")
	}
	close(releaseCancel)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel did not finish")
	}
	delegate.mu.Lock()
	_, exists := delegate.tasks["task-1"]
	delegate.mu.Unlock()
	if exists {
		t.Fatal("expected canceled task to be deleted")
	}
}

func TestDelegatePersistsInputAndFailureArtifacts(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypeGeneric,
				Command: "sh",
				Args:    []string{"-c", "echo boom >&2; exit 7"},
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "cli", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers:   []string{"proposer-a"},
			Challengers: []string{"proposer-a"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.OK || awaited.Artifact == nil {
		t.Fatalf("expected failed task with artifact, got %#v", awaited)
	}
	inputPath := filepath.Join(tmp, "input-proposer-a-propose-"+receipt.TaskID+".json")
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		t.Fatalf("expected input artifact: %v", err)
	}
	if got := inputInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("input artifact mode: got=%#o want=0600", got)
	}
	failureInfo, err := os.Stat(awaited.Artifact.Path)
	if err != nil {
		t.Fatalf("stat failure artifact: %v", err)
	}
	if got := failureInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("provider artifact mode: got=%#o want=0600", got)
	}
	body, err := os.ReadFile(awaited.Artifact.Path)
	if err != nil {
		t.Fatalf("read failure artifact: %v", err)
	}
	if !strings.Contains(string(body), `"class": "command_exit"`) {
		t.Fatalf("expected command_exit classification, got %s", string(body))
	}
	if !strings.Contains(string(body), `"exitCode": 7`) {
		t.Fatalf("expected exit code in failure artifact, got %s", string(body))
	}
}

func TestDelegateRejectsMissingCLIEnvironmentPassthroughWithoutLeakingValue(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "top-secret-value")
	delegate, err := NewDelegate(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"pi-cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypePi,
				Command: "pi",
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "gemini-3.6-flash"}},
			},
		},
		Agents: []config.AgentConfig{{ID: "proposer-pi", Provider: "pi-cli", Model: "default"}},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	_, err = delegate.Dispatch(context.Background(), consensus.ProposalTask{TaskMeta: consensus.TaskMeta{AgentID: "proposer-pi"}})
	if err == nil || !strings.Contains(err.Error(), "pi-cli") || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("expected migration error, got %v", err)
	}
	if strings.Contains(err.Error(), "top-secret-value") {
		t.Fatalf("migration error leaked credential: %v", err)
	}
}

func TestDelegateInputArtifactAuditsCLIEnvironmentWithoutSecrets(t *testing.T) {
	t.Setenv("CUSTOM_TOKEN", "host-secret")
	passthrough := []string{"CUSTOM_TOKEN"}
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:           config.ProviderTypeCLI,
				CLIType:        config.CLITypeGeneric,
				Command:        "sh",
				Args:           []string{"-c", `printf '{"summary":"ok","claims":[{"statement":"claim"}]}'`},
				EnvPassthrough: &passthrough,
				Env: map[string]string{
					"CUSTOM_ENDPOINT": "https://user:password@example.com/v1?token=secret#fragment",
				},
				Models: map[string]config.ProviderModelConfig{"default": {ProviderModel: "local"}},
			},
		},
		Agents: []config.AgentConfig{{ID: "proposer-a", Provider: "cli", Model: "default"}},
	}, tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{TaskMeta: consensus.TaskMeta{
		RequestID: "req-1", SessionID: "session-1", AgentID: "proposer-a",
	}})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, time.Second)
	if err != nil || !awaited.OK {
		t.Fatalf("Await failed: awaited=%#v err=%v", awaited, err)
	}
	body, err := os.ReadFile(filepath.Join(tmp, "input-proposer-a-propose-"+receipt.TaskID+".json"))
	if err != nil {
		t.Fatalf("read input artifact: %v", err)
	}
	for _, want := range []string{`"cliEnvironment"`, `"CUSTOM_TOKEN"`, `"CUSTOM_ENDPOINT"`, `"https://example.com/v1"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("input artifact missing %q: %s", want, body)
		}
	}
	for _, secret := range []string{"host-secret", "password", "token=secret"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("input artifact leaked %q: %s", secret, body)
		}
	}
}

func TestDelegateUsesUniqueArtifactPathsPerTask(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:   config.ProviderTypeMock,
				Models: map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers: []string{"proposer-a"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	dispatchAndAwait := func(req string) (*consensus.ArtifactRef, string) {
		t.Helper()
		receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
			TaskMeta: consensus.TaskMeta{
				RequestID: req,
				SessionID: "session-" + req,
				AgentID:   "proposer-a",
			},
		})
		if err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}
		awaited, err := delegate.Await(context.Background(), receipt.TaskID, time.Second)
		if err != nil {
			t.Fatalf("Await failed: %v", err)
		}
		if !awaited.OK || awaited.Artifact == nil {
			t.Fatalf("expected artifact, got %#v", awaited)
		}
		return awaited.Artifact, receipt.TaskID
	}

	firstArtifact, firstTaskID := dispatchAndAwait("req-1")
	secondArtifact, secondTaskID := dispatchAndAwait("req-2")
	if firstArtifact.Path == secondArtifact.Path {
		t.Fatalf("expected unique artifact paths, got %s", firstArtifact.Path)
	}
	if !strings.Contains(firstArtifact.Path, firstTaskID) || !strings.Contains(secondArtifact.Path, secondTaskID) {
		t.Fatalf("expected task ids in artifact paths: %s / %s", firstArtifact.Path, secondArtifact.Path)
	}
	if _, err := os.Stat(firstArtifact.Path); err != nil {
		t.Fatalf("first artifact missing: %v", err)
	}
	if _, err := os.Stat(secondArtifact.Path); err != nil {
		t.Fatalf("second artifact missing: %v", err)
	}
}

func TestDelegatePersistsStrictComplianceTelemetryForCanonicalOutput(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:   config.ProviderTypeMock,
				Models: map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if !awaited.OK {
		t.Fatalf("expected strict path to succeed, got %#v", awaited)
	}

	reportPath := filepath.Join(tmp, buildComplianceReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	body, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read compliance report: %v", readErr)
	}
	for _, fragment := range []string{`"strictCompliant": true`, `"finalStatus": "strict"`} {
		if !strings.Contains(string(body), fragment) {
			t.Fatalf("expected strict compliance report to contain %q, got %s", fragment, string(body))
		}
	}

	summaryPath := filepath.Join(tmp, "strict-compliance-summary.json")
	summaryBody, readSummaryErr := os.ReadFile(summaryPath)
	if readSummaryErr != nil {
		t.Fatalf("read compliance summary: %v", readSummaryErr)
	}
	if !strings.Contains(string(summaryBody), `"strict": 1`) {
		t.Fatalf("expected strict summary count, got %s", string(summaryBody))
	}
}

func TestDelegatePersistsNormalizedComplianceTelemetryForNumericStringFix(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "runner.sh")
	script := `#!/bin/sh
printf '{"summary":"proposal","claims":[{"statement":"fixed claim","confidence":"0.8"}]}'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypeGeneric,
				Command: "sh",
				Args:    []string{scriptPath},
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "cli", Model: "default"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if !awaited.OK {
		t.Fatalf("expected normalized path to succeed, got %#v", awaited)
	}

	reportPath := filepath.Join(tmp, buildComplianceReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	body, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read compliance report: %v", readErr)
	}
	for _, fragment := range []string{`"strictCompliant": false`, `"normalizedWithoutFix": true`, `"finalStatus": "normalized"`} {
		if !strings.Contains(string(body), fragment) {
			t.Fatalf("expected normalized compliance report to contain %q, got %s", fragment, string(body))
		}
	}

	summaryPath := filepath.Join(tmp, "strict-compliance-summary.json")
	summaryBody, readSummaryErr := os.ReadFile(summaryPath)
	if readSummaryErr != nil {
		t.Fatalf("read compliance summary: %v", readSummaryErr)
	}
	if !strings.Contains(string(summaryBody), `"normalized": 1`) {
		t.Fatalf("expected normalized summary count, got %s", string(summaryBody))
	}
}

func TestDelegateRepairsMalformedOutputWithSameProvider(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "runner.sh")
	script := `#!/bin/sh
input="$(cat)"
case "$input" in
  *"repairing your own previous output"*)
    printf '{"summary":"proposal repaired","claims":[{"statement":"fixed claim"}]}'
    ;;
  *)
    printf 'not json'
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypeGeneric,
				Command: "sh",
				Args:    []string{scriptPath},
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "cli", Model: "default"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if !awaited.OK {
		t.Fatalf("expected repair retry to succeed, got %#v", awaited)
	}
	if awaited.Artifact == nil || !strings.Contains(awaited.Artifact.Path, buildRepairAttemptTaskID(receipt.TaskID, 1)) {
		t.Fatalf("expected repaired raw artifact, got %#v", awaited.Artifact)
	}
	reportPath := filepath.Join(tmp, buildRepairReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, buildRepairAttemptTaskID(receipt.TaskID, 1)))
	body, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read repair report: %v", readErr)
	}
	if !strings.Contains(string(body), `"succeeded": true`) {
		t.Fatalf("expected successful repair report, got %s", string(body))
	}
	compliancePath := filepath.Join(tmp, buildComplianceReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	complianceBody, complianceErr := os.ReadFile(compliancePath)
	if complianceErr != nil {
		t.Fatalf("read compliance report: %v", complianceErr)
	}
	for _, fragment := range []string{`"repairAttempted": true`, `"repairSucceeded": true`, `"finalStatus": "repaired"`} {
		if !strings.Contains(string(complianceBody), fragment) {
			t.Fatalf("expected repaired compliance report to contain %q, got %s", fragment, string(complianceBody))
		}
	}
}

func TestDelegateReturnsRepairFailureAfterSecondMalformedOutput(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "runner.sh")
	script := `#!/bin/sh
printf 'not json'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypeGeneric,
				Command: "sh",
				Args:    []string{scriptPath},
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "cli", Model: "default"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.OK {
		t.Fatalf("expected repair retry to fail, got %#v", awaited)
	}
	if !strings.Contains(awaited.Error, "repair attempt failed") {
		t.Fatalf("expected repair failure marker, got %#v", awaited)
	}
	if awaited.Artifact == nil || !strings.Contains(awaited.Artifact.Path, buildRepairAttemptTaskID(receipt.TaskID, 1)) {
		t.Fatalf("expected repair-stage artifact, got %#v", awaited.Artifact)
	}
	reportPath := filepath.Join(tmp, buildRepairReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, buildRepairAttemptTaskID(receipt.TaskID, 1)))
	body, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read repair report: %v", readErr)
	}
	if !strings.Contains(string(body), `"succeeded": false`) {
		t.Fatalf("expected failed repair report, got %s", string(body))
	}
	compliancePath := filepath.Join(tmp, buildComplianceReportFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	complianceBody, complianceErr := os.ReadFile(compliancePath)
	if complianceErr != nil {
		t.Fatalf("read compliance report: %v", complianceErr)
	}
	for _, fragment := range []string{`"repairAttempted": true`, `"repairSucceeded": false`, `"finalStatus": "failed"`} {
		if !strings.Contains(string(complianceBody), fragment) {
			t.Fatalf("expected failed compliance report to contain %q, got %s", fragment, string(complianceBody))
		}
	}
}

func TestDelegatePersistsDecodeErrorArtifactSeparatelyFromRawParseError(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "runner.sh")
	script := `#!/bin/sh
printf '{"summary":"proposal","claims":[{"claim":"alias field","confidence":"0.8"}]}'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: config.CLITypeGeneric,
				Command: "sh",
				Args:    []string{scriptPath},
				Models:  map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "cli", Model: "default"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}

	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 5*time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.OK {
		t.Fatalf("expected schema-invalid output to fail, got %#v", awaited)
	}
	if awaited.Artifact == nil {
		t.Fatalf("expected decode error artifact, got %#v", awaited)
	}
	if !strings.Contains(filepath.Base(awaited.Artifact.Path), "decode-error-") {
		t.Fatalf("expected decode-error artifact, got %s", awaited.Artifact.Path)
	}
	initialDecodePath := filepath.Join(tmp, buildDecodeErrorFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	if _, statErr := os.Stat(initialDecodePath); statErr != nil {
		t.Fatalf("expected initial decode error artifact: %v", statErr)
	}
	initialRawParsePath := filepath.Join(tmp, buildParseErrorFilename(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, receipt.TaskID))
	if _, statErr := os.Stat(initialRawParsePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no raw parse artifact for schema error, got err=%v", statErr)
	}
}
