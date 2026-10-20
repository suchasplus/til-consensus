package runmanager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/storage"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func manager(t *testing.T, dir string, delay time.Duration) *Manager {
	t.Helper()
	m, err := New(Options{ProviderMode: "mock", DataDir: dir, Delay: delay})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m
}
func await(t *testing.T, m *Manager, id string, predicate func(model.RunSnapshot) bool) model.RunSnapshot {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		r, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(r) {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	r, _ := m.Get(id)
	t.Fatalf("讨论未达到预期状态: %#v", r)
	return r
}
func terminal(s model.RunSnapshot) bool {
	return s.Status == "completed" || s.Status == "failed" || s.Status == "stopped" || s.Status == "interrupted"
}

func TestThreeRealEngineModesAndDurableEvents(t *testing.T) {
	for _, mode := range []string{"free_debate", "adjudication", "delphi"} {
		t.Run(mode, func(t *testing.T) {
			m := manager(t, t.TempDir(), time.Millisecond)
			run, err := m.Start(model.StartRunInput{Goal: "设计一个可停止和恢复的桌面聊天应用", Mode: mode, IdempotencyKey: "mode-" + mode})
			if err != nil {
				t.Fatal(err)
			}
			finished := await(t, m, run.ID, terminal)
			if finished.Status != "completed" {
				t.Fatalf("status=%s error=%s", finished.Status, finished.Error)
			}
			if finished.Decision == nil || !finished.Decision.Mock || finished.Decision.CoreOutcome == "" {
				t.Fatalf("缺少显式mock与内核结果: %#v", finished.Decision)
			}
			if len(finished.Messages) < 5 || len(finished.Claims) == 0 || finished.Proposal == nil {
				t.Fatalf("缺少消息或结构化结果: %#v", finished)
			}
			record, err := m.stored(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if record.Request == nil || record.Request.TaskSpec.Materials[0].ID != run.ID+"-p0" {
				t.Fatal("P0没有作为冻结材料持久化")
			}
			session, err := m.loadSession(record)
			if err != nil || session == nil || session.Phase != consensus.SessionPhaseFinished || session.Result == nil {
				t.Fatalf("没有内核完成快照: %v", err)
			}
			if session.Result.Mode != consensus.WorkflowMode(mode) {
				t.Fatal("没有执行对应的内核模式")
			}
			for _, issue := range finished.Reviews {
				source := record.ReviewSources[issue.ID]
				if source.ClaimDraftIndex != nil && issue.ClaimID == "" {
					t.Fatalf("draft没有映射到内核claim: %#v %#v", issue, source)
				}
				if issue.Severity != "blocking" && issue.Severity != "warning" && issue.Severity != "info" {
					t.Fatal("severity不符合DTO", issue.Severity)
				}
			}
			for _, msg := range finished.Messages {
				switch msg.Kind {
				case "proposal", "review", "system", "decision":
				default:
					t.Fatal("message kind不符合DTO", msg.Kind)
				}
			}
			events, err := m.Events(run.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) < 8 {
				t.Fatalf("事件太少: %d", len(events))
			}
			var last int64
			for _, e := range events {
				if e.EventID <= last {
					t.Fatal("游标不递增")
				}
				last = e.EventID
			}
			if finished.LastEventID != last {
				t.Fatalf("快照游标=%d，实际=%d", finished.LastEventID, last)
			}
			none, err := m.Events(run.ID, last)
			if err != nil || none == nil || len(none) != 0 {
				t.Fatalf("空数组不可为null: %#v %v", none, err)
			}
			body, _ := json.Marshal(finished)
			if strings.Contains(string(body), `"messages":null`) || strings.Contains(string(body), `"reviews":null`) || strings.Contains(string(body), `"claims":null`) || strings.Contains(string(body), `"votes":null`) {
				t.Fatalf("JSON数组为null: %s", body)
			}
			if _, err = os.Stat(filepath.Join(m.runDir(run.ID), "ledger.root.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShutdownAndCheckpointResumeUsesFrozenConfig(t *testing.T) {
	dir := t.TempDir()
	m, err := New(Options{ProviderMode: "mock", DataDir: dir, Delay: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(model.StartRunInput{Goal: "关闭后台后继续", Mode: "free_debate", IdempotencyKey: "shutdown"})
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, r.ID, func(s model.RunSnapshot) bool { return s.Phase == "debate" && s.Checkpoint != "" })
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	second := manager(t, dir, 2*time.Second)
	interrupted, err := second.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != "interrupted" {
		t.Fatalf("退出后应该可恢复: %#v", interrupted)
	}
	resumed, err := second.Resume(model.ResumeRunInput{RunID: r.ID, IdempotencyKey: "resume-shutdown"})
	if err != nil {
		t.Fatal(err)
	}
	done := await(t, second, r.ID, terminal)
	if done.Status != "completed" {
		t.Fatalf("关闭后恢复失败: %s", done.Error)
	}
	record, err := second.stored(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Frozen.DelayMS != 30 || done.DeadlineAt != r.DeadlineAt || resumed.AttemptID == r.AttemptID {
		t.Fatal("恢复没有沿用冻结配置和原期限")
	}
}

func TestIdempotencyLeaseAndStopResume(t *testing.T) {
	m := manager(t, t.TempDir(), 25*time.Millisecond)
	input := model.StartRunInput{Goal: "验证停止与完整检查点恢复", Mode: "free_debate", IdempotencyKey: "start-once"}
	first, err := m.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.Start(input)
	if err != nil || again.ID != first.ID {
		t.Fatalf("重复开始创建新run: %v", err)
	}
	input.Goal = "另一请求"
	if _, err = m.Start(input); err == nil {
		t.Fatal("同key不同输入应拒绝")
	}
	if _, err = m.Resume(model.ResumeRunInput{RunID: first.ID, IdempotencyKey: "bad-parallel"}); err == nil {
		t.Fatal("活跃lease应拒绝重复恢复")
	}
	await(t, m, first.ID, func(s model.RunSnapshot) bool {
		return strings.Contains(s.Checkpoint, "initial") && s.Phase == "debate"
	})
	if _, err = m.Stop(first.ID); err != nil {
		t.Fatal(err)
	}
	stopped := await(t, m, first.ID, func(s model.RunSnapshot) bool { return s.Status == "stopped" })
	if stopped.Proposal == nil {
		t.Fatal("停止前P0未保存")
	}
	resumed, err := m.Resume(model.ResumeRunInput{RunID: first.ID, IdempotencyKey: "resume-once"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.AttemptID == stopped.AttemptID || resumed.DeadlineAt != first.DeadlineAt || resumed.SessionID != stopped.SessionID {
		t.Fatal("恢复更改期限/session或复用attempt")
	}
	duplicate, err := m.Resume(model.ResumeRunInput{RunID: first.ID, IdempotencyKey: "resume-once"})
	if err != nil || duplicate.AttemptID != resumed.AttemptID {
		t.Fatalf("恢复未幂等: %v", err)
	}
	done := await(t, m, first.ID, terminal)
	if done.Status != "completed" {
		t.Fatalf("恢复失败: %s %s", done.Status, done.Error)
	}
	count := 0
	for _, msg := range done.Messages {
		if msg.Phase == "preparing" && msg.AgentID == "codex" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("P0重复生成%d次", count)
	}
	for _, attempt := range []string{first.AttemptID, resumed.AttemptID} {
		if _, err = os.Stat(filepath.Join(m.runDir(first.ID), "artifacts", attempt)); err != nil {
			t.Fatal("attempt没有隔离", err)
		}
	}
}

func TestRestartPreservesResultsAndFinalizesFinishedSnapshot(t *testing.T) {
	dir := t.TempDir()
	m, err := New(Options{ProviderMode: "mock", DataDir: dir, Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(model.StartRunInput{Goal: "验证最终判定崩溃恢复", Mode: "delphi", IdempotencyKey: "restart"})
	if err != nil {
		t.Fatal(err)
	}
	done := await(t, m, r.ID, terminal)
	if done.Status != "completed" {
		t.Fatal(done.Error)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after the engine committed finished but before desktop finalization.
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.Load(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Snapshot.Status = "running"
	stored.Snapshot.HostPhase = "finalizing"
	stored.Snapshot.Decision = nil
	messages := []model.ChatMessage{}
	for _, msg := range stored.Snapshot.Messages {
		if msg.Kind != "decision" {
			messages = append(messages, msg)
		}
	}
	stored.Snapshot.Messages = messages
	if err = s.Save(context.Background(), stored, nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	s.Close()
	second := manager(t, dir, time.Second)
	recovered, err := second.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "completed" || recovered.Decision == nil || recovered.AttemptID != done.AttemptID || recovered.SessionID != done.SessionID {
		t.Fatalf("恢复应仅补最终判定: %#v", recovered)
	}
	storedAgain, err := second.stored(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := second.loadSession(storedAgain)
	if err != nil {
		t.Fatal(err)
	}
	if err = second.finalize(r.ID, session.Result); err != nil {
		t.Fatal(err)
	}
	twice, err := second.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	decisionMessages := 0
	for _, msg := range twice.Messages {
		if msg.ID == r.ID+"-decision-"+model.PolicyVersion {
			decisionMessages++
		}
	}
	if decisionMessages != 1 {
		t.Fatal("最终判定应按run与policy幂等", decisionMessages)
	}
	if _, err = New(Options{ProviderMode: "mock", DataDir: dir}); err == nil {
		t.Fatal("第二个host应被文件lease拒绝")
	}
}

func TestExpiredRunDoesNotResetDeadline(t *testing.T) {
	m, err := New(Options{ProviderMode: "mock", DataDir: t.TempDir(), Delay: time.Second, Deadline: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r, err := m.Start(model.StartRunInput{Goal: "短期限测试", Mode: "free_debate", IdempotencyKey: "expired"})
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, r.ID, terminal)
	if _, err = m.Resume(model.ResumeRunInput{RunID: r.ID, IdempotencyKey: "expired-resume"}); err == nil || !strings.Contains(err.Error(), "期限") {
		t.Fatalf("不应刷新过期期限: %v", err)
	}
}

func TestFrozenConfigTamperRejected(t *testing.T) {
	m := manager(t, t.TempDir(), time.Second)
	r, err := m.Start(model.StartRunInput{Goal: "冻结配置测试", Mode: "free_debate", IdempotencyKey: "frozen"})
	if err != nil {
		t.Fatal(err)
	}
	m.Stop(r.ID)
	await(t, m, r.ID, terminal)
	if err = m.update(r.ID, "test", func(r *model.StoredRun) error { r.Frozen.ProviderMode = "real"; return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Resume(model.ResumeRunInput{RunID: r.ID, IdempotencyKey: "tamper"}); err == nil {
		t.Fatal("不兼容快照应拒绝")
	}
	stored, err := m.stored(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(stored.Frozen)
	if stored.ConfigHash == workflow.Hash(string(body)) {
		t.Fatal("测试未改变配置")
	}
}

func TestNotificationFollowsDurableCommit(t *testing.T) {
	var m *Manager
	var err error
	m, err = New(Options{ProviderMode: "mock", DataDir: t.TempDir(), Delay: time.Millisecond, Notify: func(e model.DesktopEvent) {
		record, loadErr := m.store.Load(context.Background(), e.RunID)
		if loadErr != nil {
			t.Error(loadErr)
			return
		}
		if record.Snapshot.LastEventID < e.EventID {
			t.Error("通知早于快照游标提交")
		}
		events, eventErr := m.store.Events(context.Background(), e.RunID, e.EventID-1)
		if eventErr != nil {
			t.Error(eventErr)
			return
		}
		if len(events) == 0 || events[0].EventID != e.EventID {
			t.Error("通知早于事件落盘")
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r, err := m.Start(model.StartRunInput{Goal: "持久化后再推送", Mode: "delphi", IdempotencyKey: "durable-notify"})
	if err != nil {
		t.Fatal(err)
	}
	done := await(t, m, r.ID, terminal)
	if done.Status != "completed" {
		t.Fatal(done.Error)
	}
}
