package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func budgetRecord(t *testing.T, m *Manager, id, parent, root, status string, calls int) *model.StoredRun {
	t.Helper()
	deadline := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: id, Goal: id, Mode: "free_debate", ProviderMode: "mock", Status: status}, AttemptID: "attempt-" + id, ParentRunID: parent, LineageRootID: root, DeadlineAt: deadline, Messages: []model.ChatMessage{}}, Frozen: model.FrozenConfig{ChainRootID: root, ChainLimit: DefaultChainLimit, EngineVersion: model.EngineVersion, PolicyVersion: model.PolicyVersion, ProviderMode: "mock", GoalHash: workflow.Hash(id), Mode: "free_debate", DeadlineAt: deadline}, CallReservations: map[string]model.CallReservation{}}
	for i := 0; i < calls; i++ {
		r.CallReservations[fmt.Sprintf("%s-prior-%02d", id, i)] = model.CallReservation{AgentID: "claude", AttemptID: r.Snapshot.AttemptID, At: now()}
	}
	r.Snapshot.Usage.Calls = calls
	if status == "stopped" {
		r.StopReason = "user"
	}
	body, _ := json.Marshal(r.Frozen)
	r.ConfigHash = workflow.Hash(string(body))
	m.mu.Lock()
	err := m.saveLocked(r, "test.seed", nil, "", "", "")
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestRunBudgetAllowsThirtyRejectsThirtyOneBeforeMockCall(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	r := budgetRecord(t, m, "run-boundary", "", "run-boundary", "running", 29)
	var executed atomic.Int32
	d := delegate.New(delegate.Options{Mode: "mock", AttemptID: r.Snapshot.AttemptID, ArtifactDir: t.TempDir(), OnBeforeCall: func(invocation, agent string) error {
		if err := m.reserveCall(r.Snapshot.ID, r.Snapshot.AttemptID, invocation, agent); err != nil {
			return err
		}
		executed.Add(1)
		return nil
	}})
	defer d.Close()
	task := consensus.InitialProposalTask{TaskMeta: consensus.TaskMeta{AgentID: "claude"}, TaskSpec: consensus.TaskSpec{Goal: "预算边界测试"}, MaxClaims: 2}
	var acceptedInvocation string
	for i := 0; i < 2; i++ {
		receipt, err := d.Dispatch(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		result, err := d.Await(context.Background(), receipt.TaskID, time.Second*5)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if !result.OK {
				t.Fatal("第30个请求被拒绝", result.Error)
			}
			acceptedInvocation = receipt.TaskID
		} else if result.OK {
			t.Fatal("第31个请求仍执行Mock")
		}
	}
	if executed.Load() != 1 {
		t.Fatalf("调用前hook实际通过%d次，应仅一次", executed.Load())
	}
	snapshot, _ := m.Get(r.Snapshot.ID)
	if snapshot.Usage.Calls != 30 || snapshot.ChainCalls != 30 {
		t.Fatal("拒绝31后预算增长", snapshot.Usage, snapshot.ChainCalls)
	}
	// A duplicate started notification for the same invocation is idempotent.
	for i := 0; i < 2; i++ {
		if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, delegate.ProgressEvent{InvocationID: acceptedInvocation, AgentID: "claude", Kind: "started"}); err != nil {
			t.Fatal("重复started被重复收费", err)
		}
	}
	repeated, _ := m.Get(r.Snapshot.ID)
	if repeated.Usage.Calls != 30 || repeated.ChainCalls != 30 {
		t.Fatal("started和OnBeforeCall重复记账")
	}
}
func TestConcurrentSiblingReservationsStopAtChainNinety(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	root := budgetRecord(t, m, "root-chain", "", "root-chain", "completed", 30)
	a := budgetRecord(t, m, "sibling-a", root.Snapshot.ID, root.Snapshot.ID, "running", 29)
	b := budgetRecord(t, m, "sibling-b", root.Snapshot.ID, root.Snapshot.ID, "running", 29)
	c := budgetRecord(t, m, "sibling-c", root.Snapshot.ID, root.Snapshot.ID, "running", 0)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := []*model.StoredRun{a, b, c}[i%3]
			if m.reserveCall(target.Snapshot.ID, target.Snapshot.AttemptID, fmt.Sprintf("concurrent-%d", i), "grok") == nil {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 2 {
		t.Fatalf("并发siblings从88起应只允许2次，实际%d", allowed.Load())
	}
	for _, record := range []*model.StoredRun{root, a, b, c} {
		snapshot, err := m.Get(record.Snapshot.ID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ChainCalls != 90 || snapshot.ChainLimit != 90 || snapshot.Usage.Calls > 30 {
			t.Fatalf("链或单run预算错误: %#v", snapshot)
		}
	}
	if err := m.reserveCall(c.Snapshot.ID, c.Snapshot.AttemptID, "chain-91", "grok"); err == nil {
		t.Fatal("第91个请求被允许")
	}
	countBefore, _ := m.List()
	if _, err := m.Branch(model.BranchRunInput{RunID: root.Snapshot.ID, Goal: "不能用分叉绕过预算", IdempotencyKey: "blocked-chain-branch"}); err == nil || !strings.Contains(err.Error(), "关联讨论链") {
		t.Fatal("分支绕过满链预算", err)
	}
	countAfter, _ := m.List()
	if len(countAfter) != len(countBefore) {
		t.Fatal("满链分支留下了新run")
	}
	other := budgetRecord(t, m, "other-root", "", "other-root", "running", 0)
	if err := m.reserveCall(other.Snapshot.ID, other.Snapshot.AttemptID, "other-root-first", "agy"); err != nil {
		t.Fatal("独立root被另一条满链阻止", err)
	}
	separate, _ := m.Get(other.Snapshot.ID)
	if separate.ChainCalls != 1 || separate.ChainLimit != 90 {
		t.Fatal("独立root的预算串线")
	}
}
func TestChainBudgetSurvivesRestartAndResume(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Options{DataDir: dir, ProviderMode: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	root := budgetRecord(t, first, "restart-root", "", "restart-root", "completed", 30)
	_ = budgetRecord(t, first, "restart-sibling", root.Snapshot.ID, root.Snapshot.ID, "completed", 30)
	target := budgetRecord(t, first, "restart-target", root.Snapshot.ID, root.Snapshot.ID, "stopped", 29)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := manager(t, dir, 0)
	restored, _ := second.Get(target.Snapshot.ID)
	if restored.ChainCalls != 89 || restored.Usage.Calls != 29 || restored.ChainLimit != 90 {
		t.Fatal("重启重置了预算", restored.Usage, restored.ChainCalls)
	}
	if _, err := second.Resume(model.ResumeRunInput{RunID: target.Snapshot.ID, IdempotencyKey: "budget-resume"}); err != nil {
		t.Fatal(err)
	}
	done := await(t, second, target.Snapshot.ID, terminal)
	if done.ChainCalls != 90 || done.Usage.Calls != 30 {
		t.Fatal("恢复超过剩余1次预算或计数清零", done.Usage, done.ChainCalls)
	}
	record, _ := second.stored(target.Snapshot.ID)
	if record.Frozen.ChainLimit != 90 || record.Frozen.ChainRootID != root.Snapshot.ID {
		t.Fatal("恢复没有沿用冻结链配置")
	}
}
func TestLegacyJSONBudgetBaselineAndFrozenLimitRemainCompatible(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Options{DataDir: dir, ProviderMode: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	r := budgetRecord(t, first, "legacy-budget", "", "legacy-budget", "stopped", 0)
	// A pre-chain-policy snapshot may preserve only total usage, not reservations.
	first.mu.Lock()
	r.Frozen.PolicyVersion = "m3-cli-v1"
	r.Frozen.ChainRootID = ""
	r.Frozen.ChainLimit = 0
	r.CallReservations = nil
	r.Snapshot.Usage.Calls = 7
	body, _ := json.Marshal(r.Frozen)
	r.ConfigHash = workflow.Hash(string(body))
	err = first.saveLocked(r, "test.legacy", nil, "", "", "")
	first.mu.Unlock()
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	originalHash := r.ConfigHash
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := manager(t, dir, 0)
	restored, _ := second.Get(r.Snapshot.ID)
	if restored.ChainCalls != 7 || restored.ChainLimit != 90 {
		t.Fatal("旧JSON总数或缺省链限额丢失")
	}
	if err := second.update(r.Snapshot.ID, "test.resume", func(current *model.StoredRun) error {
		current.Snapshot.Status = "running"
		current.StopReason = ""
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := second.reserveCall(r.Snapshot.ID, r.Snapshot.AttemptID, "legacy-next", "claude"); err != nil {
		t.Fatal(err)
	}
	migrated, _ := second.stored(r.Snapshot.ID)
	if migrated.CallCountBase != 7 || migrated.Snapshot.Usage.Calls != 8 || migrated.ConfigHash != originalHash || migrated.Frozen.ChainLimit != 0 {
		t.Fatal("旧预算未迁入基数或冻结快照被改写")
	}
	if err := validateFrozen(migrated); err != nil {
		t.Fatal("兼容策略无法继续读取旧冻结配置", err)
	}
	// The root's frozen limit wins over the current default on every request.
	custom := budgetRecord(t, second, "frozen-limit-root", "", "frozen-limit-root", "completed", 30)
	limited := budgetRecord(t, second, "frozen-limit-child", custom.Snapshot.ID, custom.Snapshot.ID, "running", 29)
	if err := second.update(custom.Snapshot.ID, "test.freeze-limit", func(current *model.StoredRun) error {
		current.Frozen.ChainLimit = 60
		body, _ := json.Marshal(current.Frozen)
		current.ConfigHash = workflow.Hash(string(body))
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := second.reserveCall(limited.Snapshot.ID, limited.Snapshot.AttemptID, "frozen-60", "grok"); err != nil {
		t.Fatal(err)
	}
	extra := budgetRecord(t, second, "frozen-limit-extra", custom.Snapshot.ID, custom.Snapshot.ID, "running", 0)
	if err := second.reserveCall(extra.Snapshot.ID, extra.Snapshot.AttemptID, "frozen-61", "grok"); err == nil {
		t.Fatal("当前默认90覆盖了冻结的60次上限")
	}
	snapshot, _ := second.Get(extra.Snapshot.ID)
	if snapshot.ChainCalls != 60 || snapshot.ChainLimit != 60 {
		t.Fatal("快照没有显示冻结root限额")
	}
}

func TestLegacyBranchWithoutRootFieldCannotEscapeAncestorBudget(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	root := budgetRecord(t, m, "legacy-tree-root", "", "legacy-tree-root", "completed", 30)
	_ = budgetRecord(t, m, "legacy-tree-a", root.Snapshot.ID, root.Snapshot.ID, "completed", 30)
	_ = budgetRecord(t, m, "legacy-tree-b", root.Snapshot.ID, root.Snapshot.ID, "completed", 30)
	legacy := budgetRecord(t, m, "legacy-tree-child", root.Snapshot.ID, "", "completed", 0)
	if err := m.update(legacy.Snapshot.ID, "test.legacy-root", func(r *model.StoredRun) error {
		r.Frozen.PolicyVersion = "m3-cli-v1"
		r.Frozen.ChainRootID = ""
		r.Frozen.ChainLimit = 0
		body, _ := json.Marshal(r.Frozen)
		r.ConfigHash = workflow.Hash(string(body))
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	inherited, _ := m.Get(legacy.Snapshot.ID)
	if inherited.ChainCalls != 90 {
		t.Fatal("旧分支没有沿parent读取祖先累计")
	}
	if _, err := m.Branch(model.BranchRunInput{RunID: legacy.Snapshot.ID, Goal: "旧JSON再分支不能创建新预算root", IdempotencyKey: "legacy-branch-budget"}); err == nil {
		t.Fatal("缺lineageRootId的旧JSON绕过关联链额度")
	}
}
