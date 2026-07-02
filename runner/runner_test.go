package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	memorystore "github.com/suchasplus/til-consensus/store/memory"
)

func TestExecutorRunAndActWithMockProvider(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.InitTemplate()
	cfg.Output.Directory = filepath.Join(tmp, "out", "{requestId}")
	cfg = config.Normalize(cfg)
	loaded := config.LoadedConfig{
		ConfigDir: tmp,
		Config:    cfg,
	}
	executor := NewExecutor(loaded)
	executor.SessionStore = memorystore.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := executor.Run(ctx, config.RunInput{
		Mode: consensus.WorkflowModeAdjudication,
		TaskSpec: config.TaskSpecInput{
			Goal: "判断这个 patch 是否真正修复了竞态问题",
		},
	}, config.RunOverrides{}, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Output == nil {
		t.Fatal("expected run output")
	}
	if result.Output.RequestID != result.Plan.RequestID {
		t.Fatalf("request id mismatch: output=%s plan=%s", result.Output.RequestID, result.Plan.RequestID)
	}
	if result.Output.SessionID == "" {
		t.Fatal("expected session id")
	}

	action, err := executor.Act(ctx, ActionInput{
		Result:       *result.Output,
		Prompt:       "执行后续 action",
		ArtifactsDir: result.Plan.ArtifactsDir,
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("Act failed: %v", err)
	}
	if action.ActorID != "actor-a" {
		t.Fatalf("unexpected actor id: %s", action.ActorID)
	}
	if action.Output.FullResponse == "" {
		t.Fatal("expected action response")
	}
}

func TestReplayRequestPreservesLineage(t *testing.T) {
	parent := consensus.StartRequest{
		RequestID: "parent-request",
		TaskSpec:  consensus.TaskSpec{Goal: "parent goal"},
	}
	snapshot := consensus.SessionSnapshot{
		SessionID: "parent-session",
		RequestID: "parent-request",
		Request:   &parent,
		Result: &consensus.RunResult{
			CaseManifest: &consensus.CaseManifest{CaseID: "case-1"},
		},
	}
	replayed, err := ReplayRequest(snapshot, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("ReplayRequest failed: %v", err)
	}
	if replayed.RequestID == "" || replayed.RequestID == parent.RequestID {
		t.Fatalf("expected new request id, got %q", replayed.RequestID)
	}
	if replayed.Lineage == nil {
		t.Fatal("expected lineage")
	}
	if replayed.Lineage.ParentRequestID != "parent-request" ||
		replayed.Lineage.ParentSessionID != "parent-session" ||
		replayed.Lineage.ParentCaseID != "case-1" ||
		replayed.Lineage.Trigger != "session_replay" {
		t.Fatalf("unexpected lineage: %#v", replayed.Lineage)
	}
}
