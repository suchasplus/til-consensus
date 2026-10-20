package delegate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/runtime/mock"
)

func testTasks() []consensus.Task {
	meta := consensus.TaskMeta{AgentID: "claude", Role: "reviewer"}
	spec := consensus.TaskSpec{Goal: "为团队设计可追溯的讨论桌面工具"}
	claim := consensus.ClaimNode{ClaimID: "claim-1", Statement: "明确验收边界", Confidence: 0.7}
	debate := consensus.DebateClaim{ClaimID: "claim-1", Statement: "明确验收边界", Active: true}
	statement := consensus.DelphiStatement{StatementID: "statement-1", Statement: "最小闭环"}
	return []consensus.Task{
		consensus.ProposalTask{TaskMeta: meta, TaskSpec: spec, MaxClaims: 2},
		consensus.InitialProposalTask{TaskMeta: meta, TaskSpec: spec, MaxClaims: 2, Round: 1},
		consensus.ChallengeTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.ClaimNode{claim}},
		consensus.ReviseTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.ClaimNode{claim}, Round: 1},
		consensus.SemanticVerificationTask{TaskMeta: meta, TaskSpec: spec, Claim: claim},
		consensus.ArbiterTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.ClaimNode{claim}},
		consensus.ReportTask{TaskMeta: meta, TaskSpec: spec},
		consensus.DebateRoundTask{TaskMeta: meta, TaskSpec: spec, PeerClaims: []consensus.DebateClaim{debate}, Round: 1},
		consensus.DebateRoundTask{TaskMeta: meta, TaskSpec: spec, PeerClaims: []consensus.DebateClaim{debate}, Round: 3, SynthesisReview: true},
		consensus.SemanticDedupTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.DebateClaim{debate}, SimilarityThreshold: 0.9},
		consensus.SynthesisTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.DebateClaim{debate}},
		consensus.SynthesisTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.DebateClaim{debate}, Draft: &debate},
		consensus.FinalVoteTask{TaskMeta: meta, TaskSpec: spec, Claims: []consensus.DebateClaim{debate}},
		consensus.DelphiQuestionnaireTask{TaskMeta: meta, TaskSpec: spec, RatingScaleMin: 1, RatingScaleMax: 5, Round: 1},
		consensus.DelphiRevisionTask{TaskMeta: meta, TaskSpec: spec, RatingScaleMin: 1, RatingScaleMax: 5, StatementSummaries: []consensus.DelphiStatement{statement}, Round: 2},
		consensus.DelphiFacilitatorSummaryTask{TaskMeta: meta, TaskSpec: spec, StatementSummaries: []consensus.DelphiStatement{statement}},
		consensus.ActionTask{TaskMeta: meta, Prompt: "生成实施任务"},
	}
}

func TestAllThreeModeTaskContracts(t *testing.T) {
	for _, task := range testTasks() {
		t.Run(string(task.Kind())+"/"+phase(task), func(t *testing.T) {
			var messages []Message
			d := New(Options{Mode: "mock", AttemptID: "attempt-1", ArtifactDir: t.TempDir(), OnMessage: func(m Message) error { messages = append(messages, m); return nil }})
			defer d.Close()
			receipt, err := d.Dispatch(context.Background(), task)
			if err != nil {
				t.Fatal(err)
			}
			result, err := d.Await(context.Background(), receipt.TaskID, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if !result.OK {
				t.Fatal(result.Error)
			}
			if result.Output.Kind() != task.Kind() {
				t.Fatalf("wrong core type: %T", result.Output)
			}
			if len(messages) != 1 || !strings.Contains(messages[0].Content, "Mock") {
				t.Fatalf("missing readable mock message: %#v", messages)
			}
			if taskGoal(task) != "" && task.Kind() != consensus.TaskKindDelphiFacilitatorSummary && !strings.Contains(messages[0].Content, shortGoal(taskGoal(task))) {
				t.Fatalf("message lost task context: %s", messages[0].Content)
			}
			data, err := os.ReadFile(result.Artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			var artifact struct {
				Complete bool            `json:"complete"`
				Raw      json.RawMessage `json:"rawOutput"`
			}
			if err := json.Unmarshal(data, &artifact); err != nil {
				t.Fatal(err)
			}
			if !artifact.Complete {
				t.Fatal("completed task artifact marked incomplete")
			}
			if _, _, err := ValidateEnvelope(task, artifact.Raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func validEnvelope(t *testing.T, task consensus.Task) map[string]any {
	t.Helper()
	raw, err := mock.RunTask(context.Background(), task, config.AgentConfig{ID: task.Meta().AgentID}, config.ProviderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	core, err := localizeCore(task, raw)
	if err != nil {
		t.Fatal(err)
	}
	env := Envelope{SchemaVersion: 1, Mock: true, InvocationID: "invocation-1", TaskKind: string(task.Kind()), ContextHash: strings.Repeat("a", 64), CoreOutput: core, DesktopReview: buildReviews(task, "invocation-1", core)}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEnvelopeRejectsInvalidDesktopExtensions(t *testing.T) {
	task := testTasks()[1]
	cases := map[string]func(map[string]any){
		"severity":                func(e map[string]any) { firstReview(e)["severity"] = "whatever" },
		"stringDraftIndex":        func(e map[string]any) { firstReview(e)["claimDraftIndex"] = "0" },
		"unknownDraft":            func(e map[string]any) { firstReview(e)["claimDraftIndex"] = 999 },
		"mismatchedDraftText":     func(e map[string]any) { firstReview(e)["draftStatement"] = "other claim" },
		"ambiguousClaimRef":       func(e map[string]any) { firstReview(e)["claimId"] = "claim-1" },
		"missingDraftRef":         func(e map[string]any) { delete(firstReview(e), "invocationId") },
		"resolutionType":          func(e map[string]any) { firstReview(e)["resolutionRefs"] = []any{17} },
		"resolvedWithoutEvidence": func(e map[string]any) { firstReview(e)["status"] = "resolved" },
		"exclusiveUnknown":        func(e map[string]any) { firstReview(e)["exclusiveWith"] = []any{"unknown"} },
		"unknownExtension":        func(e map[string]any) { firstReview(e)["madeUp"] = true },
		"unknownCoreField":        func(e map[string]any) { e["coreOutput"].(map[string]any)["madeUp"] = true },
		"wrongSource":             func(e map[string]any) { firstReview(e)["raisedBy"] = "grok" },
		"mockWrongType":           func(e map[string]any) { e["mock"] = "false" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			env := validEnvelope(t, task)
			mutate(env)
			data, _ := json.Marshal(env)
			if _, _, err := ValidateEnvelope(task, data); err == nil {
				t.Fatal("accepted invalid envelope")
			}
		})
	}
	data, _ := json.Marshal(validEnvelope(t, task))
	if _, _, err := ValidateEnvelope(task, data); err != nil {
		t.Fatal(err)
	}
}

func firstReview(e map[string]any) map[string]any {
	return e["desktopReview"].([]any)[0].(map[string]any)
}

func TestCancellationPersistsIncompleteOutput(t *testing.T) {
	dir := t.TempDir()
	d := New(Options{Mode: "mock", AttemptID: "cancel", ArtifactDir: dir, Delay: time.Second})
	defer d.Close()
	r, err := d.Dispatch(context.Background(), testTasks()[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Cancel(context.Background(), r.TaskID); err != nil {
		t.Fatal(err)
	}
	result, err := d.Await(context.Background(), r.TaskID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !strings.Contains(result.Error, "canceled") {
		t.Fatalf("unexpected cancel result: %#v", result)
	}
	data, err := os.ReadFile(result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Complete bool   `json:"complete"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Complete || artifact.Status != "cancelled" {
		t.Fatalf("cancellation artifact incorrect: %s", data)
	}
}

func TestParallelIndependentContextsAndUniqueArtifacts(t *testing.T) {
	var mu sync.Mutex
	messages := make([]Message, 0, 24)
	d := New(Options{Mode: "mock", AttemptID: "parallel", ArtifactDir: t.TempDir(), Delay: 10 * time.Millisecond, OnMessage: func(m Message) error { mu.Lock(); defer mu.Unlock(); messages = append(messages, m); return nil }})
	defer d.Close()
	var receipts []consensus.DispatchReceipt
	for i := 0; i < 24; i++ {
		task := testTasks()[1].(consensus.InitialProposalTask)
		task.Metadata = map[string]any{"independent": true, "round": 1}
		task.TaskSpec.Goal = fmtGoal(i)
		r, err := d.Dispatch(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, r)
		task.Metadata["round"] = 99
	}
	paths := make(map[string]bool)
	hashes := make(map[string]bool)
	for _, r := range receipts {
		result, err := d.Await(context.Background(), r.TaskID, time.Second)
		if err != nil || !result.OK {
			t.Fatalf("parallel task: %#v %v", result, err)
		}
		if paths[result.Artifact.Path] {
			t.Fatal("artifact collision")
		}
		paths[result.Artifact.Path] = true
	}
	for _, message := range messages {
		if hashes[message.ContextHash] {
			t.Fatal("different task context shared a hash")
		}
		hashes[message.ContextHash] = true
		input, err := os.ReadFile(strings.TrimSuffix(message.ArtifactPath, ".output.json") + ".input.json")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(input), `"round": 99`) {
			t.Fatal("independent context mutated after dispatch")
		}
	}
	if len(messages) != 24 {
		t.Fatalf("expected 24 complete independent messages, got %d", len(messages))
	}
}

func fmtGoal(i int) string { return "独立任务 " + string(rune('A'+i)) }

func TestCallbackFailureStopsFurtherDispatch(t *testing.T) {
	d := New(Options{Mode: "mock", OnMessage: func(Message) error { return errors.New("sqlite commit failed") }})
	defer d.Close()
	r, err := d.Dispatch(context.Background(), testTasks()[1])
	if err != nil {
		t.Fatal(err)
	}
	result, err := d.Await(context.Background(), r.TaskID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !strings.Contains(result.Error, "sqlite commit failed") {
		t.Fatalf("lost callback error: %#v", result)
	}
	if _, err := d.Dispatch(context.Background(), testTasks()[1]); err == nil {
		t.Fatal("continued dispatch after persistence failure")
	}
}

func TestP0DoesNotPublishOutsideHostTransaction(t *testing.T) {
	d := New(Options{Mode: "mock", ArtifactDir: t.TempDir(), OnMessage: func(Message) error { t.Error("P0 message must be committed by host"); return nil }, OnReview: func(Review) error { t.Error("P0 review must not be published"); return nil }})
	defer d.Close()
	text, err := d.ProposeP0(context.Background(), "创建桌面讨论工具")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "创建桌面讨论工具") || !strings.Contains(text, "P0 · Mock") {
		t.Fatal(text)
	}
	files, err := filepath.Glob(filepath.Join(d.options.ArtifactDir, "mock", "*.output.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("P0 artifact missing: %v %v", files, err)
	}
}

func TestMessageUsesReadableClaimsAndRoundedNumbers(t *testing.T) {
	id := "claim-7368fbd8-2c6b-41e0-849b-a0a88c4c3981"
	task := consensus.FinalVoteTask{Claims: []consensus.DebateClaim{{ClaimID: id, Title: "停止后保留检查点"}}}
	core := map[string]any{"summary": "【Mock】冻结版本投票", "votes": []map[string]any{{"claimId": id, "vote": "accept", "confidence": 0.9199999999999999, "rationale": "当前方案具备明确恢复边界"}}}
	before, _ := json.Marshal(core)
	message := messageContent(task, core)
	if strings.Contains(message, id) || strings.Contains(message, "0.919") || strings.Contains(message, "accept") {
		t.Fatalf("聊天暴露原始标识或浮点尾差：%s", message)
	}
	if !strings.Contains(message, "停止后保留检查点") || !strings.Contains(message, "支持分 92%") || !strings.Contains(message, "支持") {
		t.Fatalf("缺少可读投票内容：%s", message)
	}
	after, _ := json.Marshal(core)
	if string(before) != string(after) {
		t.Fatal("展示格式化修改了原始核心结果")
	}
	delphi := consensus.DelphiRevisionTask{StatementSummaries: []consensus.DelphiStatement{{StatementID: id, Statement: "分步接入真实工具"}}}
	message = messageContent(delphi, map[string]any{"summary": "【Mock】匿名复评", "responses": []map[string]any{{"statementId": id, "rating": 4.199999999999999, "rationale": "范围可控"}, {"statementId": "missing", "rating": 3.2, "rationale": "保留待验证项"}}})
	if strings.Contains(message, id) || !strings.Contains(message, "分步接入真实工具") || !strings.Contains(message, "评分 4.2") || !strings.Contains(message, "候选 2") {
		t.Fatalf("候选展示不正确：%s", message)
	}
}
