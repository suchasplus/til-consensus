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
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.OK {
		t.Fatal("expected malformed output to fail")
	}
	if awaited.Artifact == nil {
		t.Fatal("expected parse error artifact")
	}
	if _, err := os.Stat(awaited.Artifact.Path); err != nil {
		t.Fatalf("artifact missing: %v", err)
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
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("expected input artifact: %v", err)
	}
	body, err := os.ReadFile(awaited.Artifact.Path)
	if err != nil {
		t.Fatalf("read failure artifact: %v", err)
	}
	if !strings.Contains(string(body), `"class": "command_exit"`) {
		t.Fatalf("expected command_exit classification, got %s", string(body))
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
