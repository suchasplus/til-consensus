package runmanager

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/storage"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestProductionDefaultsToRealCLI(t *testing.T) {
	m, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.ProviderMode() != "cli" {
		t.Fatal("正式后台必须默认真实CLI")
	}
}
func TestFrozenBallotRejectsOldVersionAndDuplicateModels(t *testing.T) {
	claim := consensus.DebateClaim{ClaimID: "canonical", Statement: "不可变方案", Category: consensus.DebateClaimCategorySynthesis, Active: true}
	s := model.RunSnapshot{RunSummary: model.RunSummary{ProviderMode: "cli"}, Proposal: &model.Proposal{Hash: workflow.Hash(claim.Statement)}, Decision: &model.Decision{Outcome: "supported"}}
	r := &model.StoredRun{Snapshot: s, ActualMetadata: map[string]map[string]any{}}
	manifest := ballotManifestHash([]consensus.DebateClaim{claim})
	for _, id := range []string{"claude", "grok", "agy"} {
		r.Snapshot.Ballots = append(r.Snapshot.Ballots, model.BallotBinding{AgentID: id, Valid: true, ProposalHash: s.Proposal.Hash, ManifestHash: manifest})
		r.ActualMetadata[id] = map[string]any{"agentId": id, "model": id + "-model"}
	}
	result := &consensus.RunResult{FreeDebate: &consensus.FreeDebateResultSection{Claims: []consensus.DebateClaim{claim}}}
	applyBallotGate(r, result)
	if r.Snapshot.Decision.Outcome != "supported" {
		t.Fatal(r.Snapshot.Decision.Summary)
	}
	r.Snapshot.Decision.Outcome = "supported"
	r.Snapshot.Ballots[0].ProposalHash = "old-content"
	applyBallotGate(r, result)
	if r.Snapshot.Decision.Outcome == "supported" {
		t.Fatal("旧方案的票不能批准当前方案")
	}
	r.Snapshot.Decision.Outcome = "supported"
	r.Snapshot.Ballots[0].ProposalHash = s.Proposal.Hash
	r.ActualMetadata["agy"]["model"] = "grok-model"
	applyBallotGate(r, result)
	if r.Snapshot.Decision.Outcome != "needs_human" {
		t.Fatal("同一模型不能算作两张独立票")
	}
}
func TestBranchVersionsExportAndDiagnosticCleanup(t *testing.T) {
	m := manager(t, t.TempDir(), time.Millisecond)
	parent, err := m.Start(model.StartRunInput{Goal: "设计桌面讨论室", Mode: "free_debate", IdempotencyKey: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	done := await(t, m, parent.ID, terminal)
	if done.Status != "completed" {
		t.Fatal(done.Error)
	}
	if len(done.ProposalHistory) < 2 || len(done.Ballots) != 3 {
		t.Fatalf("修订版本和真实绑定缺失: versions=%d ballots=%d", len(done.ProposalHistory), len(done.Ballots))
	}
	initial := done.ProposalHistory[0]
	if initial.Version != 0 || initial.Hash != workflow.Hash(initial.Content) {
		t.Fatal("初稿不是不可变版本")
	}
	input := model.BranchRunInput{RunID: parent.ID, Goal: "新增离线查看约束", IdempotencyKey: "branch"}
	child, err := m.Branch(input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := m.Branch(input)
	if err != nil || duplicate.ID != child.ID {
		t.Fatal("关联讨论没有幂等", err)
	}
	childDone := await(t, m, child.ID, terminal)
	if childDone.Status != "completed" {
		t.Fatal(childDone.Error)
	}
	record, _ := m.stored(child.ID)
	if record.Snapshot.ParentRunID != parent.ID || record.Snapshot.SourceProposalHash != done.Proposal.Hash || record.Snapshot.SessionID == done.SessionID || !strings.Contains(record.ParentMaterial, initial.ID[:10]) {
		t.Fatal("缺少冻结lineage或复用旧session")
	}
	for _, b := range childDone.Ballots {
		for _, old := range done.Ballots {
			if b.InvocationID == old.InvocationID {
				t.Fatal("旧投票跨run复用")
			}
		}
	}
	for _, format := range []string{"markdown", "json"} {
		export, err := m.Export(child.ID, format)
		if err != nil || export.SuggestedName == "" || !strings.Contains(export.Content, child.ID) {
			t.Fatal("导出不完整", err)
		}
		if format == "json" && !json.Valid([]byte(export.Content)) {
			t.Fatal("JSON不可读")
		}
	}
	cleanup, err := m.ClearDiagnostics(child.ID)
	if err != nil || cleanup.RemovedFiles == 0 {
		t.Fatal("没有清除诊断原文", err)
	}
	for _, name := range []string{"ledger.jsonl", "result.json", "decision.json", "run-config.json"} {
		if _, err := os.Stat(filepath.Join(m.runDir(child.ID), name)); err != nil {
			t.Fatal("结果或账本被删除", name, err)
		}
	}
	if after, _ := m.Get(child.ID); after.Proposal.Hash != childDone.Proposal.Hash || len(after.Ballots) != 3 {
		t.Fatal("清理改变了结果")
	}
}
func TestUsageSnapshotsBudgetAndIncrementalUpsert(t *testing.T) {
	m := manager(t, t.TempDir(), time.Second)
	run, err := m.Start(model.StartRunInput{Goal: "验证预算和增量", Mode: "delphi", IdempotencyKey: "progress"})
	if err != nil {
		t.Fatal(err)
	}
	m.Stop(run.ID)
	await(t, m, run.ID, terminal)
	for _, delta := range []string{"你好", "，世界"} {
		if err := m.progress(run.ID, run.AttemptID, delegate.ProgressEvent{InvocationID: "live-1", AgentID: "grok", Delta: delta, Text: delta, Kind: "text.delta", Usage: map[string]any{"input_tokens": float64(100), "output_tokens": float64(5)}}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := m.Get(run.ID)
	if snapshot.Usage.InputTokens != 100 || snapshot.Usage.OutputTokens != 5 {
		t.Fatal("累计usage重复相加")
	}
	count := 0
	for _, msg := range snapshot.Messages {
		if msg.InvocationID == "live-1" {
			count++
			if msg.Content != "你好，世界" {
				t.Fatal(msg.Content)
			}
		}
	}
	if count != 1 {
		t.Fatal("流式消息重复创建")
	}
	_ = m.update(run.ID, "test", func(r *model.StoredRun) error {
		r.UsageByInvocation = map[string]model.Usage{}
		for i := 0; i < 30; i++ {
			r.UsageByInvocation[string(rune('a'+i))] = model.Usage{}
		}
		return nil
	}, nil)
	if err := m.progress(run.ID, run.AttemptID, delegate.ProgressEvent{InvocationID: "over-budget", Kind: "started"}); err == nil {
		t.Fatal("预算未阻止下一次实际调用")
	}
}
func TestSchemaMigrationPreservesHistoricalMockAndNoLiveResume(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	frozen := model.FrozenConfig{EngineVersion: model.EngineVersion, PolicyVersion: "m0-mock-v1", ProviderMode: "mock", GoalHash: workflow.Hash("旧演示"), Mode: "delphi", DeadlineAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano)}
	body, _ := json.Marshal(frozen)
	old := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "legacy", Goal: "旧演示", Mode: "delphi", Status: "running"}, DeadlineAt: frozen.DeadlineAt}, Frozen: frozen, ConfigHash: workflow.Hash(string(body))}
	if err := s.Save(context.Background(), old, nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", filepath.Join(dir, "desktop.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version=0"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	m, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	snapshot, err := m.Get("legacy")
	if err != nil || snapshot.ProviderMode != "mock" || snapshot.Status != "interrupted" {
		t.Fatal("历史Mock迁移后被伪装真实", err, snapshot)
	}
	if _, err := m.Resume(model.ResumeRunInput{RunID: "legacy", IdempotencyKey: "legacy-resume"}); err == nil {
		t.Fatal("旧Mock不能自动升级继续真实执行")
	}
}

func TestDelphiDesktopContextRemovesIdentity(t *testing.T) {
	m := manager(t, t.TempDir(), time.Second)
	run, err := m.Start(model.StartRunInput{Goal: "匿名调查", Mode: "delphi", IdempotencyKey: "anonymous-context"})
	if err != nil {
		t.Fatal(err)
	}
	m.Stop(run.ID)
	await(t, m, run.ID, terminal)
	_ = m.update(run.ID, "test", func(r *model.StoredRun) error {
		r.Snapshot.Reviews = []model.ReviewIssue{{ID: "claude-review", RaisedBy: "claude", Severity: "warning", Statement: "Grok Build 的建议需要说明", Status: "open", ResolutionRefs: []string{"agy-ref"}}}
		return nil
	}, nil)
	task, err := m.bindTask(run.ID, consensus.DelphiQuestionnaireTask{TaskMeta: consensus.TaskMeta{AgentID: "grok"}, Round: 1})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(task.Meta().Metadata["desktopReviewContext"])
	for _, identity := range []string{"claude", "grok", "agy", "Grok"} {
		if strings.Contains(string(body), identity) {
			t.Fatalf("匿名上下文泄漏身份: %s", body)
		}
	}
	// The same phase/round retains its first frozen issue set.
	_ = m.update(run.ID, "test", func(r *model.StoredRun) error { r.Snapshot.Reviews = nil; return nil }, nil)
	second, err := m.bindTask(run.ID, consensus.DelphiQuestionnaireTask{TaskMeta: consensus.TaskMeta{AgentID: "agy"}, Round: 1})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(second.Meta().Metadata["desktopReviewContext"])
	if string(body) != string(again) {
		t.Fatal("同轮获得不同问题版本")
	}
}
func TestSemanticOpinionCannotVerifyFactualClaim(t *testing.T) {
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{Mode: "adjudication", ProviderMode: "cli"}, Decision: &model.Decision{Outcome: "supported"}}}
	session := &consensus.SessionSnapshot{ClaimGraph: []consensus.ClaimNode{{ClaimID: "fact", Statement: "尚未实际检查的事实", ClaimType: consensus.ClaimTypeFact, Verdict: consensus.ClaimVerdictSupported}}, VerificationResults: []consensus.VerificationResult{{ClaimID: "fact", Kind: "semantic", Status: consensus.VerificationStatusPassed}}}
	applyEvidenceGate(r, session)
	if r.Snapshot.Decision.Outcome != "insufficient_evidence" {
		t.Fatal("模型支持不能冒充事实验证")
	}
	if validResolutionRefs(r, []string{"invented-ref"}, "") {
		t.Fatal("凭空引用不能关闭阻塞问题")
	}
}
