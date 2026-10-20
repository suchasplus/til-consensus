package runmanager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestSelectorAndReportedModelBindSeparatelyAfterSuccessfulOutput(t *testing.T) {
	m, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "model-bind", ProviderMode: "cli", Status: "running"}, AttemptID: "attempt-bind", Messages: []model.ChatMessage{}}, Frozen: model.FrozenConfig{ProviderMode: "cli", Providers: []model.ProviderConfig{{ID: "grok", Model: "grok-4.7", Version: "1.0.62"}}}}
	frozen, _ := json.Marshal(r.Frozen)
	r.ConfigHash = workflow.Hash(string(frozen))
	if err := m.saveLocked(r, "test", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.active[r.Snapshot.ID] = cancel
	defer delete(m.active, r.Snapshot.ID)
	event := delegate.ProgressEvent{InvocationID: "inv-model-1", AgentID: "grok", Kind: "started", CLIVersion: "1.0.62", Model: "grok-4.7", ModelSource: "explicit-request"}
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal("selector必须被接受", err)
	}
	event.Kind = "usage"
	event.Model = "grok-4.7-build"
	event.ModelSource = "cli-report"
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal("合法canonical不应与selector做相等比较", err)
	}
	before, _ := m.stored(r.Snapshot.ID)
	if len(before.ModelBindings) != 0 {
		t.Fatal("输出未验证完成前不应绑定实际模型")
	}
	event.Kind = "completed"
	event.Complete = true
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal(err)
	}
	bound, _ := m.stored(r.Snapshot.ID)
	if bound.ModelBindings["grok"].Model != "grok-4.7-build" || bound.ModelBindings["grok"].RequestedModel != "grok-4.7" {
		t.Fatal("没有区分selector和reportedModel")
	}
	if bound.ConfigHash != r.ConfigHash || bound.Frozen.Providers[0].Model != "grok-4.7" {
		t.Fatal("运行事实修改了冻结配置")
	}
	metadata := bound.ActualMetadata[event.InvocationID]
	if metadata["requestedModel"] != "grok-4.7" || metadata["reportedModel"] != "grok-4.7-build" {
		t.Fatal("原始元数据未分别保存selector/canonical", metadata)
	}
	// The next task still launches with the original selector. Its reported
	// identity must agree with the prior successful canonical model.
	event.InvocationID = "inv-model-2"
	event.Kind = "started"
	event.Complete = false
	event.Model = "grok-4.7"
	event.ModelSource = "explicit-request"
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal("新调用selector被错误当作canonical漂移", err)
	}
	event.Kind = "usage"
	event.Model = "grok-4.8-build"
	event.ModelSource = "cli-report"
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err == nil || !strings.Contains(err.Error(), "新建关联讨论") {
		t.Fatal("后续canonical变更未中断", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("模型变更未取消整个讨论")
	}
	changed, _ := m.stored(r.Snapshot.ID)
	if changed.StopReason != "model_changed" || changed.Snapshot.Status != "failed" || changed.ModelBindings["grok"].Model != "grok-4.7-build" {
		t.Fatal("旧canonical绑定被静默改写")
	}
}
func TestIndependentIdentityUsesBoundCanonicalInsteadOfSelector(t *testing.T) {
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ProviderMode: "cli"}, Decision: &model.Decision{Outcome: "supported"}}, Frozen: model.FrozenConfig{Providers: []model.ProviderConfig{{ID: "claude", Model: "alias-a"}, {ID: "grok", Model: "alias-b"}, {ID: "agy", Model: "different"}}}, ModelBindings: map[string]model.ModelBinding{"claude": {Model: "same-canonical"}, "grok": {Model: "same-canonical"}, "agy": {Model: "different-canonical"}}}
	applyIdentityGate(r)
	if r.Snapshot.Decision.Outcome != "needs_human" {
		t.Fatal("不同selector指向同一canonical不能重复计票")
	}
}

func TestRestartDoesNotFinalizeOverRuntimeModelFailure(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Options{DataDir: dir, ProviderMode: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := first.Start(model.StartRunInput{Goal: "验证运行模型错误不会被完成快照覆盖", Mode: "delphi", IdempotencyKey: "runtime-error-recovery"})
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	done := await(t, first, run.ID, terminal)
	if done.Status != "completed" {
		first.Close()
		t.Fatal(done.Error)
	}
	if err := first.update(run.ID, "test.model_changed", func(r *model.StoredRun) error {
		r.StopReason = "model_changed"
		r.Snapshot.Status = "failed"
		r.Snapshot.Error = "报告模型变更，请新建关联讨论"
		return nil
	}, nil); err != nil {
		first.Close()
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := manager(t, dir, 0)
	restored, err := restarted.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != "failed" || !strings.Contains(restored.Error, "模型变更") {
		t.Fatal("重启错误地从完成内核快照重新判定", restored)
	}
}
