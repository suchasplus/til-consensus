package runmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestHistoryDeleteRestorePersistsWithoutRemovingEvidence(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Options{DataDir: dir, ProviderMode: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if first != nil {
			_ = first.Close()
		}
	}()
	r := budgetRecord(t, first, "history-completed", "", "history-completed", "completed", 7)
	originalHash := r.ConfigHash
	artifact := filepath.Join(first.runDir(r.Snapshot.ID), "artifacts", "retained.output.json")
	if err := os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte(`{"evidence":"keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	deleted, err := first.DeleteRun(r.Snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339Nano, deleted.DeletedAt); err != nil {
		t.Fatal("删除时间未持久化", err)
	}
	if deleted.Status != "completed" || deleted.Usage.Calls != 7 || deleted.ChainCalls != 7 {
		t.Fatal("删除改变了运行结果或预算", deleted)
	}
	visible, err := first.List()
	if err != nil || len(visible) != 0 {
		t.Fatal("删除项仍显示在最近讨论", visible, err)
	}
	trash, err := first.ListDeleted()
	if err != nil || len(trash) != 1 || trash[0].ID != deleted.ID || trash[0].DeletedAt != deleted.DeletedAt {
		t.Fatal("最近删除未保留快照", trash, err)
	}
	all, err := first.store.All(context.Background())
	if err != nil || len(all) != 1 {
		t.Fatal("软删除移除了底层记录", all, err)
	}
	exported, err := first.Export(deleted.ID, "json")
	if err != nil || !strings.Contains(exported.Content, `"deletedAt"`) {
		t.Fatal("已删除记录不能继续审计导出", err)
	}
	events, err := first.Events(deleted.ID, r.Snapshot.LastEventID)
	if err != nil || len(events) != 1 || events[0].Type != "run.deleted" || events[0].Payload["deletedAt"] != deleted.DeletedAt {
		t.Fatal("删除事件未与快照原子保存", events, err)
	}
	again, err := first.DeleteRun(deleted.ID)
	if err != nil || again.DeletedAt != deleted.DeletedAt || again.LastEventID != deleted.LastEventID {
		t.Fatal("重复删除不幂等", again, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	first = nil

	second := manager(t, dir, 0)
	loaded, err := second.Get(deleted.ID)
	if err != nil || loaded.DeletedAt != deleted.DeletedAt || loaded.LastEventID != deleted.LastEventID {
		t.Fatal("重启改变了删除状态或审计事件", loaded, err)
	}
	visible, err = second.List()
	if err != nil || len(visible) != 0 {
		t.Fatal("重启重新显示了删除项", visible, err)
	}
	trash, err = second.ListDeleted()
	if err != nil || len(trash) != 1 || trash[0].DeletedAt != deleted.DeletedAt {
		t.Fatal("重启丢失了最近删除", trash, err)
	}
	restored, err := second.RestoreRun(deleted.ID)
	if err != nil || restored.DeletedAt != "" || restored.Status != deleted.Status || restored.Usage.Calls != deleted.Usage.Calls {
		t.Fatal("恢复错误地启动讨论或改变历史结果", restored, err)
	}
	visible, err = second.List()
	if err != nil || len(visible) != 1 || visible[0].ID != deleted.ID {
		t.Fatal("恢复后最近讨论未出现", visible, err)
	}
	trash, err = second.ListDeleted()
	if err != nil || len(trash) != 0 {
		t.Fatal("恢复后仍留在最近删除", trash, err)
	}
	again, err = second.RestoreRun(deleted.ID)
	if err != nil || again.LastEventID != restored.LastEventID {
		t.Fatal("重复恢复不幂等", again, err)
	}
	events, err = second.Events(deleted.ID, deleted.LastEventID)
	if err != nil || len(events) != 1 || events[0].Type != "run.restored" || events[0].Payload["previousDeletedAt"] != deleted.DeletedAt {
		t.Fatal("恢复事件错误", events, err)
	}
	record, err := second.stored(deleted.ID)
	if err != nil || record.ConfigHash != originalHash || len(record.CallReservations) != 7 {
		t.Fatal("删除恢复改动了冻结配置或请求证据", err)
	}
	if body, err := os.ReadFile(artifact); err != nil || string(body) != `{"evidence":"keep"}` {
		t.Fatal("删除恢复清除了调用证据", err)
	}
}

func TestHistoryDeleteRejectsRunningAndCleanupLease(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	r := budgetRecord(t, m, "history-active", "", "history-active", "running", 0)
	if _, err := m.DeleteRun(r.Snapshot.ID); err == nil || !strings.Contains(err.Error(), "等待 CLI 完全退出") {
		t.Fatal("运行状态允许删除", err)
	}
	if err := m.update(r.Snapshot.ID, "test.failed-before-cleanup", func(current *model.StoredRun) error {
		current.Snapshot.Status = "failed"
		current.Snapshot.Phase = "stopping"
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	// This explicit lease test double represents a process whose failure has
	// been saved while subprocess cleanup is still pending; no CLI is invoked.
	m.mu.Lock()
	m.active[r.Snapshot.ID] = func() {}
	m.mu.Unlock()
	if _, err := m.DeleteRun(r.Snapshot.ID); err == nil || !strings.Contains(err.Error(), "等待 CLI 完全退出") {
		t.Fatal("清理尚未完成的lease允许删除", err)
	}
	m.mu.Lock()
	delete(m.active, r.Snapshot.ID)
	m.mu.Unlock()
	if snapshot, err := m.DeleteRun(r.Snapshot.ID); err != nil || snapshot.DeletedAt == "" {
		t.Fatal("已结束的讨论不能删除", err)
	}
}

func TestHistoryDeletedRecordsStillCountTowardChainBudget(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	root := budgetRecord(t, m, "history-budget-root", "", "history-budget-root", "completed", 30)
	sibling := budgetRecord(t, m, "history-budget-sibling", root.Snapshot.ID, root.Snapshot.ID, "completed", 30)
	child := budgetRecord(t, m, "history-budget-child", root.Snapshot.ID, root.Snapshot.ID, "stopped", 29)
	for _, id := range []string{root.Snapshot.ID, sibling.Snapshot.ID} {
		if _, err := m.DeleteRun(id); err != nil {
			t.Fatal(err)
		}
	}
	visible, err := m.List()
	if err != nil || len(visible) != 1 || visible[0].ID != child.Snapshot.ID {
		t.Fatal("最近讨论过滤错误", visible, err)
	}
	snapshot, err := m.Get(child.Snapshot.ID)
	if err != nil || snapshot.ChainCalls != 89 || snapshot.ChainLimit != 90 || snapshot.LineageRootID != root.Snapshot.ID {
		t.Fatal("删除父项或兄弟项重置了链预算", snapshot, err)
	}
	if _, err := m.Resume(model.ResumeRunInput{RunID: child.Snapshot.ID, IdempotencyKey: "history-child-resume"}); err != nil {
		t.Fatal("父项已删除阻断已有孩子恢复", err)
	}
	finished := await(t, m, child.Snapshot.ID, terminal)
	if finished.ChainCalls != 90 || finished.Usage.Calls != 30 {
		t.Fatal("孩子恢复绕过了已删除项的链预算", finished)
	}
	if _, err := m.RestoreRun(root.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Branch(model.BranchRunInput{RunID: root.Snapshot.ID, Goal: "仍不能绕过已删除兄弟项的预算", IdempotencyKey: "history-full-chain"}); err == nil || !strings.Contains(err.Error(), "关联讨论链") {
		t.Fatal("最近删除绕过关联链预算", err)
	}
}

func TestHistoryDeletedSourceCannotStartBranchOrReplayIdempotency(t *testing.T) {
	m := manager(t, t.TempDir(), time.Second)
	input := model.StartRunInput{Goal: "显式 Mock 删除与幂等回归", Mode: "free_debate", IdempotencyKey: "history-start"}
	run, err := m.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	await(t, m, run.ID, func(s model.RunSnapshot) bool { return s.Status == "stopped" })
	// Persist a prior resume identity directly as a test fixture. The snapshot
	// stays idle and the test does not issue an additional model request.
	m.mu.Lock()
	r, err := m.store.Load(context.Background(), run.ID)
	if err == nil {
		err = m.saveLocked(r, "test.resume-identity", nil, "history-resume", "resume", workflow.Hash(run.ID))
	}
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := m.DeleteRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		call func() (model.RunSnapshot, error)
	}{
		{"start-replay", func() (model.RunSnapshot, error) { return m.Start(input) }},
		{"resume-replay", func() (model.RunSnapshot, error) {
			return m.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "history-resume"})
		}},
		{"resume-new", func() (model.RunSnapshot, error) {
			return m.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "history-new-resume"})
		}},
		{"branch", func() (model.RunSnapshot, error) {
			return m.Branch(model.BranchRunInput{RunID: run.ID, Goal: "不能从删除项新建", IdempotencyKey: "history-branch"})
		}},
		{"locked-branch-entry", func() (model.RunSnapshot, error) {
			return m.start(model.StartRunInput{Goal: "模拟来源读取后发生删除", Mode: "free_debate", IdempotencyKey: "history-racing-branch"}, run.ID, "{}")
		}},
	}
	for _, item := range checks {
		t.Run(item.name, func(t *testing.T) {
			if _, err := item.call(); err == nil || !strings.Contains(err.Error(), "最近删除，请先恢复") {
				t.Fatal("已删除讨论被重复使用", err)
			}
		})
	}
	after, err := m.Get(run.ID)
	if err != nil || after.LastEventID != deleted.LastEventID || after.Usage.Calls != deleted.Usage.Calls {
		t.Fatal("被拒绝的重复请求改变了快照或额度", after, err)
	}
	if _, err := m.RestoreRun(run.ID); err != nil {
		t.Fatal(err)
	}
	if replayed, err := m.Start(input); err != nil || replayed.ID != run.ID || replayed.Status != "stopped" {
		t.Fatal("恢复后原幂等标识丢失", replayed, err)
	}
}

func TestHistoryExistingChildKeepsFrozenContextAfterParentDeletion(t *testing.T) {
	m := manager(t, t.TempDir(), time.Millisecond)
	parent := budgetRecord(t, m, "history-context-parent", "", "history-context-parent", "completed", 0)
	const content = "父讨论已确认的限制需要完整保留"
	if err := m.update(parent.Snapshot.ID, "test.parent-proposal", func(current *model.StoredRun) error {
		current.Snapshot.Proposal = &model.Proposal{ID: "parent-proposal", Title: "原始方案", Content: content, Hash: workflow.Hash(content)}
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	child, err := m.Branch(model.BranchRunInput{RunID: parent.Snapshot.ID, Goal: "验证已有分支恢复", IdempotencyKey: "history-context-child"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(child.ID); err != nil {
		t.Fatal(err)
	}
	await(t, m, child.ID, func(s model.RunSnapshot) bool { return s.Status == "stopped" })
	before, err := m.stored(child.ID)
	if err != nil || !strings.Contains(before.ParentMaterial, content) {
		t.Fatal("已有孩子没有冻结父材料", err)
	}
	if _, err := m.DeleteRun(parent.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Resume(model.ResumeRunInput{RunID: child.ID, IdempotencyKey: "history-context-resume"}); err != nil {
		t.Fatal("父项移至最近删除后不能恢复孩子", err)
	}
	finished := await(t, m, child.ID, terminal)
	if finished.Status != "completed" {
		t.Fatal("孩子的冻结上下文无法继续", finished.Error)
	}
	after, err := m.stored(child.ID)
	if err != nil || after.ParentMaterial != before.ParentMaterial || after.Frozen.ParentMaterialHash != before.Frozen.ParentMaterialHash {
		t.Fatal("恢复改变或丢失了删除父项的冻结材料", err)
	}
}
