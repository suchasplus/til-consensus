package runmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestProgressRefusesCLIChangesAgainstFrozenVersion(t *testing.T) {
	for _, phase := range []string{"started", "later-metadata", "missing-start-version"} {
		t.Run(phase, func(t *testing.T) {
			m, err := New(Options{DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "version-run", ProviderMode: "cli", Status: "running"}, AttemptID: "attempt-frozen", Messages: []model.ChatMessage{}}, Frozen: model.FrozenConfig{ProviderMode: "cli", Providers: []model.ProviderConfig{{ID: "grok", Version: "1.0.62"}}}}
			if err := m.saveLocked(r, "test", nil, "", "", ""); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.active[r.Snapshot.ID] = cancel
			defer delete(m.active, r.Snapshot.ID)
			event := delegate.ProgressEvent{InvocationID: "inv-1", AgentID: "grok", Kind: "started", CLIVersion: "1.0.62", Model: "grok-4.7-build"}
			if phase == "later-metadata" {
				if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
					t.Fatal(err)
				}
				event.Kind = "usage"
			}
			event.CLIVersion = "1.0.63"
			if phase == "missing-start-version" {
				event.CLIVersion = ""
			}
			err = m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event)
			if err == nil || !strings.Contains(err.Error(), "新建关联讨论") {
				t.Fatalf("没有拒绝版本混用: %v", err)
			}
			snapshot, _ := m.Get(r.Snapshot.ID)
			if snapshot.Status != "failed" || !strings.Contains(snapshot.Error, "版本") {
				t.Fatal("版本错误没有持久化", snapshot)
			}
			if ctx.Err() != context.Canceled {
				t.Fatal("版本漂移没有取消整个活跃讨论")
			}
			record, _ := m.stored(r.Snapshot.ID)
			if phase == "later-metadata" {
				if record.Snapshot.Usage.Calls != 1 || record.ActualMetadata["inv-1"]["cliVersion"] != "1.0.62" {
					t.Fatal("已拒绝的版本污染了原始元数据")
				}
			} else if record.Snapshot.Usage.Calls != 0 {
				t.Fatal("未启动调用错误消耗预算")
			}
			events, _ := m.Events(r.Snapshot.ID, 0)
			if events[len(events)-1].Type != "run.version_changed" {
				t.Fatal("没有审计版本变化事件")
			}
			// In-flight events from another task cannot reopen the failed run.
			event.CLIVersion = "1.0.62"
			if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err == nil {
				t.Fatal("版本异常后仍接收运行事件")
			}
		})
	}
}

func TestParentContextSelectsDecisionMaterialWithoutHistory(t *testing.T) {
	accepted := true
	rejected := false
	rawMarker := "RAW-HISTORY-MUST-NOT-ENTER-CONTEXT"
	current := model.Proposal{ID: "parent-p3", Content: "冻结当前方案完整正文", Hash: workflow.Hash("冻结当前方案完整正文"), Version: 3}
	parent := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "parent", ProviderMode: "cli", Goal: "目标原文及不得联网的硬约束"}, Proposal: &current, ProposalHistory: []model.Proposal{{Content: strings.Repeat(rawMarker, 10000)}}, Messages: []model.ChatMessage{{Content: strings.Repeat(rawMarker, 10000)}}, Decision: &model.Decision{Outcome: "needs_human", Summary: "关键决策原文", MinorityOpinions: []string{"少数意见必须保留"}, Assumptions: []string{"尚待验证的假设"}, Votes: []model.Vote{{AgentID: "old-voter", ClaimID: "old-vote"}}}, Claims: []model.ClaimView{{ID: "accepted", Statement: "已接受旧主张", Accepted: &accepted}, {ID: "unresolved", Statement: "尚未解决的关键主张", Accepted: &rejected}}, Reviews: []model.ReviewIssue{{ID: "resolved", Status: "resolved", Statement: rawMarker}, {ID: "blocking", Severity: "blocking", Status: "open", Statement: "阻塞问题必须完整保留"}}, Evidence: []model.Evidence{{ID: "e1", Summary: strings.Repeat(rawMarker, 10000), Kind: "semantic_verification", Source: "verifier", VerificationStatus: "model_opinion", ArtifactRef: "parent/ledger#e1"}}}, CoreResult: &consensus.RunResult{Report: consensus.AdjudicationReport{UnresolvedQuestions: []string{"报告明确保留的待回答问题"}}}, Request: &consensus.StartRequest{TaskSpec: consensus.TaskSpec{Constraints: consensus.TaskConstraints{Language: "zh-CN", Notes: []string{"不能删除的重要硬约束"}}}}}
	compact, err := compactParentContext(parent)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compact, []byte(rawMarker)) || bytes.Contains(compact, []byte("old-voter")) {
		t.Fatal("历史原文或旧投票进入新上下文")
	}
	for _, required := range []string{current.Content, "少数意见必须保留", "尚待验证的假设", "阻塞问题必须完整保留", "不能删除的重要硬约束", "尚未解决的关键主张", "parent/ledger#e1", "报告明确保留的待回答问题"} {
		if !bytes.Contains(compact, []byte(required)) {
			t.Fatalf("遗漏必要材料：%s", required)
		}
	}
	var decoded parentContext
	if err := json.Unmarshal(compact, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ContextManifest.FieldContentTruncated || decoded.ContextManifest.MaxInputBytes != MaxBranchInputBytes || decoded.ContextManifest.SourceSnapshotHash == "" {
		t.Fatal("缺少明确选材manifest")
	}
	if len(decoded.UnresolvedIssues) != 1 || len(decoded.UnresolvedClaims) != 1 {
		t.Fatal("没有选取未决问题")
	}
	for _, selection := range decoded.ContextManifest.Selections {
		if selection.Source == "reviews" {
			if len(selection.SelectedRanges) != 1 || selection.SelectedRanges[0] != [2]int{1, 2} || len(selection.OmittedRanges) != 1 || selection.OmittedRanges[0] != [2]int{0, 1} {
				t.Fatalf("选材范围错误: %#v", selection)
			}
		}
	}
	again, err := compactParentContext(parent)
	if err != nil || !bytes.Equal(compact, again) {
		t.Fatal("上下文压缩不是确定性的", err)
	}
	// Necessary content is never shortened to fit the cap.
	parent.Snapshot.Proposal.Content = strings.Repeat("超", 50000)
	if _, err := compactParentContext(parent); err == nil || !strings.Contains(err.Error(), "不能静默") {
		t.Fatalf("超限材料被静默截断: %v", err)
	}
}
