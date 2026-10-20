package runmanager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

// The build fingerprint tests cover which source edits preserve this identity.
// This test uses explicitly labelled Mock providers to exercise the persisted
// resume boundary when that identity is preserved or changed between binaries.
func TestExecutionSourceIdentityGuardsResumeWithoutRewritingHistory(t *testing.T) {
	cleanProviderEnv(t)
	originalVersion := model.EngineVersion
	t.Cleanup(func() { model.EngineVersion = originalVersion })
	model.EngineVersion = "source-sha256:" + strings.Repeat("a", 64)
	dir := t.TempDir()
	var active *Manager
	t.Cleanup(func() {
		if active != nil {
			_ = active.Close()
		}
	})
	open := func() *Manager {
		t.Helper()
		m, err := New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		active = m
		return m
	}
	closeManager := func() {
		t.Helper()
		if err := active.Close(); err != nil {
			t.Fatal(err)
		}
		active = nil
	}
	first := open()
	run, err := first.Start(model.StartRunInput{Goal: "显式 Mock：验证合仓后源码身份与冻结恢复", Mode: "free_debate", IdempotencyKey: "source-identity-start"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	closeManager()

	// A renderer, documentation or test-only commit produces the same execution
	// source identity. Restarting such a binary must keep the frozen run usable.
	uiOnlyBuild := open()
	before, err := uiOnlyBuild.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozenJSON, _ := json.Marshal(before.Frozen)
	configHash := before.ConfigHash
	resumed, err := uiOnlyBuild.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "source-identity-ui-resume"})
	if err != nil {
		t.Fatal("同一执行源码身份的 UI 更新阻止了恢复", err)
	}
	if resumed.AttemptID == before.Snapshot.AttemptID {
		t.Fatal("恢复没有创建新的 attempt")
	}
	if _, err := uiOnlyBuild.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	closeManager()

	// A real execution-source change must fail before spending budget, creating
	// an attempt, appending events or reserving the idempotency key.
	model.EngineVersion = "source-sha256:" + strings.Repeat("b", 64)
	changedBuild := open()
	storedBefore, err := changedBuild.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentFrozen, _ := json.Marshal(storedBefore.Frozen)
	if string(currentFrozen) != string(frozenJSON) || storedBefore.ConfigHash != configHash {
		t.Fatal("兼容恢复或重启篡改了冻结配置")
	}
	if _, err := changedBuild.Get(run.ID); err != nil {
		t.Fatal("新执行源码身份不应阻止读取历史", err)
	}
	if _, err := changedBuild.Export(run.ID, "json"); err != nil {
		t.Fatal("新执行源码身份不应阻止导出审计历史", err)
	}
	eventsBefore, err := changedBuild.Events(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeJSON, _ := json.Marshal(storedBefore)
	if _, err := changedBuild.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "source-identity-rejected-resume"}); err == nil || !strings.Contains(err.Error(), "冻结配置不兼容") {
		t.Fatal("不同执行源码身份被静默接受", err)
	}
	storedAfter, err := changedBuild.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, _ := json.Marshal(storedAfter)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("被拒绝的恢复改变了快照、预算或冻结证据")
	}
	eventsAfter, err := changedBuild.Events(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeEventsJSON, _ := json.Marshal(eventsBefore)
	afterEventsJSON, _ := json.Marshal(eventsAfter)
	if string(beforeEventsJSON) != string(afterEventsJSON) {
		t.Fatal("被拒绝的恢复写入了持久化事件")
	}
	id, err := changedBuild.store.LookupKey(context.Background(), "source-identity-rejected-resume", "resume", workflow.Hash(run.ID))
	if err != nil || id != "" {
		t.Fatal("被拒绝的恢复占用了幂等键", id, err)
	}
}
