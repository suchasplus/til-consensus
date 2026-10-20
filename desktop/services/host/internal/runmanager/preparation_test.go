package runmanager

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/storage"
	tilrunner "github.com/suchasplus/til-consensus/runner"
)

func preparedFixture(_ context.Context, r model.PreparationRecord, c providers.Config, _ string, _ time.Duration) (model.PreparationRecord, error) {
	r.Classification = &tilrunner.ClassifyResult{Recommendation: "free_debate", Confidence: 0.85, Summary: "适合讨论取舍", Why: []string{"存在多种可行方案"}, MissingInformation: []string{}}
	r.MaterialAnalysis = &model.MaterialAnalysis{Summary: "材料已按原文读取", Materials: []model.MaterialSummary{}, Gaps: []model.MaterialGap{}, Limitations: []string{"仅为测试fixture"}}
	r.Source = &model.PreparationSource{ProviderID: c.ID, CLIType: "mock", CLIVersion: "mock-v1", RequestedModel: "mock", ActualModel: "mock", ModelSource: "mock", Mock: true}
	r.Usage = model.Usage{Calls: 1, InputTokens: 100, OutputTokens: 20, Source: "mock"}
	return r, nil
}
func preparationManager(t *testing.T, dir string, execute func(context.Context, model.PreparationRecord, providers.Config, string, time.Duration) (model.PreparationRecord, error)) *Manager {
	t.Helper()
	m, err := New(Options{DataDir: dir, ProviderMode: "mock", ExecutePreparation: execute})
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
func prepInput(id string) model.PreparationInput {
	return model.PreparationInput{ID: id, Goal: "为团队选择一套可恢复的迁移方案", Materials: []model.MaterialInput{{ID: "source-a", Title: "用户原始约束", Source: "pasted", Content: "必须可回滚。\r\n 保留空白 "}}}
}
func waitPreparation(t *testing.T, m *Manager, id, status string) model.PreparationRecord {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, err := m.GetPreparation(id)
		if err == nil && r.Status == status {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	r, _ := m.GetPreparation(id)
	t.Fatalf("准备状态未变为%s: %#v", status, r)
	return r
}

func TestPreparationDedupeCancellationAndTombstone(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	execute := func(ctx context.Context, r model.PreparationRecord, c providers.Config, mode string, delay time.Duration) (model.PreparationRecord, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-release:
			return preparedFixture(ctx, r, c, mode, delay)
		}
	}
	m := preparationManager(t, t.TempDir(), execute)
	in := prepInput("single-flight")
	var wg sync.WaitGroup
	results := make(chan model.PreparationRecord, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := m.AnalyzeTopic(context.Background(), in)
			results <- r
			failures <- err
		}()
	}
	<-started
	changed := in
	changed.Goal = "不同输入"
	if _, err := m.AnalyzeTopic(context.Background(), changed); err == nil {
		t.Fatal("同准备ID接受不同payload")
	}
	close(release)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for r := range results {
		if r.Status != "completed" || r.InputHash == "" {
			t.Fatal("没有共享完整结果")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("相同准备并发执行多次", calls.Load())
	}
	if _, err := m.AnalyzeTopic(context.Background(), in); err != nil || calls.Load() != 1 {
		t.Fatal("完成后同ID没有幂等复用", err)
	}
	if _, err := m.CancelPreparation("not-started"); err != nil {
		t.Fatal(err)
	}
	tomb := prepInput("not-started")
	if _, err := m.AnalyzeTopic(context.Background(), tomb); !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatal("Cancel tombstone仍发起分析", err)
	}
}
func TestPreparationContextAndCloseCancelActiveJobs(t *testing.T) {
	entered := make(chan struct{}, 2)
	execute := func(ctx context.Context, r model.PreparationRecord, _ providers.Config, _ string, _ time.Duration) (model.PreparationRecord, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return r, ctx.Err()
	}
	dir := t.TempDir()
	m, err := New(Options{DataDir: dir, ProviderMode: "mock", ExecutePreparation: execute})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { _, err := m.AnalyzeTopic(ctx, prepInput("ctx-cancel")); finished <- err }()
	<-entered
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitPreparation(t, m, "ctx-cancel", "cancelled")
	go func() { _, _ = m.AnalyzeTopic(context.Background(), prepInput("close-cancel")) }()
	<-entered
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	second := preparationManager(t, dir, execute)
	if r, err := second.GetPreparation("close-cancel"); err != nil || r.Status != "cancelled" {
		t.Fatal("Close未持久取消", r, err)
	}
}
func TestPreparationReconcileRunningWithoutCallingModel(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := model.PreparationRecord{ID: "interrupted", Status: "running", RequestHash: "old-request", CreatedAt: now(), UpdatedAt: now()}
	if err := store.SavePreparation(context.Background(), &record); err != nil {
		t.Fatal(err)
	}
	store.Close()
	var calls atomic.Int32
	m := preparationManager(t, dir, func(ctx context.Context, r model.PreparationRecord, c providers.Config, mode string, d time.Duration) (model.PreparationRecord, error) {
		calls.Add(1)
		return preparedFixture(ctx, r, c, mode, d)
	})
	r, err := m.GetPreparation(record.ID)
	if err != nil || r.Status != "interrupted" || calls.Load() != 0 {
		t.Fatal("重启自动重跑未完成准备", r, err)
	}
}
func TestAutoModeAdmissionAndExplicitOverride(t *testing.T) {
	cases := []struct {
		name, recommendation string
		confidence           float64
		mode                 string
		allow                bool
	}{{"missing", "", 0, "auto", false}, {"needs", "needs_clarification", 0.9, "auto", false}, {"not-suitable", "not_suitable", 0.99, "auto", false}, {"low", "free_debate", 0.59, "auto", false}, {"required-gap", "free_debate", 0.9, "auto", false}, {"estimated-not-used", "needs_clarification", 0.9, "auto", false}, {"boundary", "delphi", 0.6, "auto", true}, {"manual-override", "not_suitable", 0.2, "adjudication", true}}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			m := preparationManager(t, t.TempDir(), func(ctx context.Context, r model.PreparationRecord, c providers.Config, mode string, d time.Duration) (model.PreparationRecord, error) {
				r, _ = preparedFixture(ctx, r, c, mode, d)
				r.Classification.Recommendation = item.recommendation
				r.Classification.Confidence = item.confidence
				r.Classification.EstimatedModeAfterClarification = "free_debate"
				if item.name == "required-gap" {
					r.MaterialAnalysis.Gaps = []model.MaterialGap{{Question: "必须确认回滚窗口", Importance: "required"}}
				}
				return r, nil
			})
			in := prepInput("admission")
			record, err := m.AnalyzeTopic(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			start := model.StartRunInput{Goal: in.Goal, Materials: in.Materials, Mode: item.mode, PreparationID: record.ID, IdempotencyKey: "start"}
			if item.name == "missing" {
				start.PreparationID = ""
			}
			run, err := m.Start(start)
			if item.allow {
				if err != nil {
					t.Fatal(err)
				}
				if run.Mode == "auto" || run.RequestedMode != item.mode || run.Preparation == nil || len(run.Preparation.Materials) != 0 {
					t.Fatal("未冻结具体模式和公开准备副本")
				}
				m.Stop(run.ID)
				await(t, m, run.ID, terminal)
			} else if err == nil {
				t.Fatal("不合适或低信心自动获批")
			}
		})
	}
}
func TestPreparationChangeConsumptionBudgetAndDeadline(t *testing.T) {
	m := preparationManager(t, t.TempDir(), preparedFixture)
	in := prepInput("adopt-once")
	record, err := m.AnalyzeTopic(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	start := model.StartRunInput{Goal: in.Goal, Materials: in.Materials, Mode: "auto", PreparationID: record.ID, IdempotencyKey: "same-start"}
	changed := start
	changed.Goal += "新约束"
	if _, err := m.Start(changed); err == nil {
		t.Fatal("目标变化没有使准备失效")
	}
	changed = start
	changed.Materials = append([]model.MaterialInput(nil), start.Materials...)
	changed.Materials[0].Content += "新材料"
	if _, err := m.Start(changed); err == nil {
		t.Fatal("材料变化没有使准备失效")
	}
	// Freeze an earlier preparation start; waiting must count towards the deadline.
	record.CreatedAt = time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)
	m.mu.Lock()
	err = m.store.SavePreparation(context.Background(), &record)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Start(start)
	if err != nil {
		t.Fatal(err)
	}
	expectedCreated, _ := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if run.DeadlineAt != expectedCreated.Add(m.opt.Deadline).Format(time.RFC3339Nano) || run.CreatedAt != record.CreatedAt {
		t.Fatal("分析等待未计入原期限")
	}
	if run.Usage.Calls != 1 || run.Usage.InputTokens != 100 || run.ChainCalls != 1 {
		t.Fatal("已采用准备未计入预算或usage", run.Usage, run.ChainCalls)
	}
	repeated, err := m.Start(start)
	if err != nil || repeated.ID != run.ID {
		t.Fatal("同start幂等未返回原run", err)
	}
	other := start
	other.IdempotencyKey = "another-start"
	if _, err := m.Start(other); err == nil {
		t.Fatal("同准备批准了第二run")
	}
	consumed, _ := m.GetPreparation(record.ID)
	if consumed.UsedByRunID != run.ID || len(consumed.Materials) != 1 {
		t.Fatal("原准备未原子消费或丢失原文")
	}
	m.Stop(run.ID)
	await(t, m, run.ID, terminal)
	stored, _ := m.stored(run.ID)
	if _, ok := stored.CallReservations["preparation-"+record.ID]; !ok {
		t.Fatal("缺少唯一准备调用预留")
	}
	if err := validateFrozen(stored); err != nil {
		t.Fatal(err)
	}
	stored.Snapshot.Preparation.Classification.Summary = "被修改的结果"
	if err := validateFrozen(stored); err == nil {
		t.Fatal("准备快照篡改未被发现")
	}
	expiredIn := prepInput("expired")
	expired, _ := m.AnalyzeTopic(context.Background(), expiredIn)
	expired.CreatedAt = time.Now().Add(-m.opt.Deadline - time.Second).UTC().Format(time.RFC3339Nano)
	m.mu.Lock()
	err = m.store.SavePreparation(context.Background(), &expired)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(model.StartRunInput{Goal: expiredIn.Goal, Materials: expiredIn.Materials, Mode: "auto", PreparationID: expired.ID, IdempotencyKey: "expired"}); err == nil || !strings.Contains(err.Error(), "期限") {
		t.Fatal("过期准备仍获批", err)
	}
}
func TestPreparationAdoptionDoesNotBypassChainBudget(t *testing.T) {
	m := preparationManager(t, t.TempDir(), preparedFixture)
	root := budgetRecord(t, m, "prep-root", "", "prep-root", "completed", 30)
	_ = budgetRecord(t, m, "prep-child-a", root.Snapshot.ID, root.Snapshot.ID, "completed", 30)
	_ = budgetRecord(t, m, "prep-child-b", root.Snapshot.ID, root.Snapshot.ID, "completed", 29)
	in := prepInput("chain-preparation")
	p, err := m.AnalyzeTopic(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.start(model.StartRunInput{Goal: in.Goal, Mode: "auto", Materials: in.Materials, PreparationID: p.ID, IdempotencyKey: "adopt-over-chain"}, root.Snapshot.ID, "")
	if err == nil {
		t.Fatal("准备调用绕过了关联链预算")
	}
	still, _ := m.GetPreparation(p.ID)
	if still.UsedByRunID != "" {
		t.Fatal("预算失败仍消费了准备结果")
	}
}
func TestPreparationInputAndPrivateRecordJSON(t *testing.T) {
	m := preparationManager(t, t.TempDir(), preparedFixture)
	in := prepInput("private-record")
	record, err := m.AnalyzeTopic(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	materials, err := preparation.Normalize(in.Goal, in.Materials)
	if err != nil {
		t.Fatal(err)
	}
	if record.InputHash != preparation.HashInput(in.Goal, materials) {
		t.Fatal("准备未绑定完整输入")
	}
	record.RawDiagnostics = "PRIVATE-REASONING"
	record.DiagnosticsDir = "/private/preparation"
	body, _ := json.Marshal(record)
	if strings.Contains(string(body), "PRIVATE-REASONING") || strings.Contains(string(body), "/private/preparation") {
		t.Fatal("准备公开记录包含私有诊断")
	}
	if _, err := m.AnalyzeTopic(context.Background(), model.PreparationInput{ID: "../bad", Goal: in.Goal}); err == nil {
		t.Fatal("准备ID允许目录遍历")
	}
}
