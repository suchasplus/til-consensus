package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func TestDelphiRequestProjectsParentMaterialWithoutChangingAudit(t *testing.T) {
	parent := `{"parentRunId":"parent-run","unresolvedClaims":[{"id":"claim-old","statement":"保留关键约束"}],"unresolvedIssues":[{"id":"claude-review","raisedBy":"claude","claimId":"claim-old","invocationId":"private-native-invocation","statement":"我是 Claude Code，模型 claude-opus-5-5；session_id=private-native-session，在 /Users/private/.claude/runs/123","resolutionRefs":["claim-old"]}],"proposal":{"id":"parent-p1","invocationId":"native-codex","content":"Grok Build 建议独立验证"},"evidenceIndex":[{"id":"e1","artifactRef":"/Users/private/.grok/leader.sock"}]}`
	goal := "比较 Codex、Claude Code、Grok 与 agy 的调用边界"
	r := model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "child", Goal: goal, Mode: "delphi"}, ParentRunID: "parent"}, Frozen: model.FrozenConfig{ProviderMode: "cli"}, ParentMaterial: parent}
	p := model.Proposal{ID: "child-p0", Content: "Codex 初稿指出需要验证 gpt-6.1-sol", Hash: "original-owner-hash"}
	request, err := Request(r, p, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if request.TaskSpec.Goal != goal {
		t.Fatal("用户任务中的产品名称被改写")
	}
	for _, material := range request.TaskSpec.Materials {
		for _, secret := range []string{"Claude", "claude", "Codex", "Grok", "gpt-6", "private-native", "/Users/", "raisedBy", "invocationId", "artifactRef"} {
			if strings.Contains(material.Title+material.Content, secret) {
				t.Fatalf("匿名材料包含来源标识 %q: %s", secret, material.Content)
			}
		}
		if material.Hash != Hash(material.Content) {
			t.Fatal("匿名投影没有自己的内容摘要")
		}
	}
	if r.ParentMaterial != parent || p.Content != "Codex 初稿指出需要验证 gpt-6.1-sol" {
		t.Fatal("匿名投影改写了所有者的原始审计材料")
	}
	var projected map[string]any
	if err := json.Unmarshal([]byte(request.TaskSpec.Materials[1].Content), &projected); err != nil {
		t.Fatal(err)
	}
	claim := projected["unresolvedClaims"].([]any)[0].(map[string]any)["id"]
	issue := projected["unresolvedIssues"].([]any)[0].(map[string]any)
	if claim != issue["claimId"] || claim != issue["resolutionRefs"].([]any)[0] || !strings.HasPrefix(claim.(string), "anonymous-ref-") {
		t.Fatal("匿名化破坏了必要的主张引用关系")
	}
}

func TestDelphiGeneratedSummaryAndReasonsAreUntrusted(t *testing.T) {
	statement := consensus.DelphiStatement{StatementID: "statement-1", Statement: "GROK BUILD 建议缩小范围", RepresentativeReasons: []string{"作为 agy，我使用 gemini-3.8-flash-medium；thread_id=private-thread；/tmp/tcview-call/attempt"}}
	original := consensus.DelphiRevisionTask{TaskMeta: consensus.TaskMeta{AgentID: "claude"}, TaskSpec: consensus.TaskSpec{Goal: "用户原始问题"}, PreviousSummary: "Claude Code 自报 claude-opus-5-5；conversation_id=private-conversation", StatementSummaries: []consensus.DelphiStatement{statement}}
	projected := AnonymousDelphiTask(original).(consensus.DelphiRevisionTask)
	body, _ := json.Marshal(map[string]any{"summary": projected.PreviousSummary, "statements": projected.StatementSummaries})
	for _, identity := range []string{"Claude", "claude-opus", "GROK", "agy", "gemini-3", "/tmp/", "private-thread", "private-conversation"} {
		if strings.Contains(string(body), identity) {
			t.Fatalf("不可信摘要自报身份未投影：%s", body)
		}
	}
	if projected.AgentID != original.AgentID || projected.StatementSummaries[0].StatementID != "statement-1" {
		t.Fatal("改写了当前参与者或评分目标的必要标识")
	}
	if original.StatementSummaries[0].RepresentativeReasons[0] != statement.RepresentativeReasons[0] || !strings.Contains(original.PreviousSummary, "Claude") {
		t.Fatal("修改了内核的共享上下文／审计原文")
	}
}

func TestAnonymousIdentifiersDoNotEraseAdjacentConstraints(t *testing.T) {
	projected := AnonymousPeerText("路径 /tmp/tcview-call/run，必须保留链级预算；thread_id=private-session；不得静默降级。")
	if !strings.Contains(projected, "必须保留链级预算") || !strings.Contains(projected, "不得静默降级") {
		t.Fatal("匿名化删除了来源标识旁边的约束")
	}
	if strings.Contains(projected, "/tmp/") || strings.Contains(projected, "private-session") {
		t.Fatal("匿名化仍包含调用标识")
	}
}
