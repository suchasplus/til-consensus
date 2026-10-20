package runmanager

import (
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestP0CommitReplacesStreamWithCompleteProposal(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "p0-progress", ProviderMode: "mock", Status: "running", Phase: "preparing"}, HostPhase: "preparing", AttemptID: "attempt-p0", Messages: []model.ChatMessage{}}, Frozen: model.FrozenConfig{ProviderMode: "mock"}}
	if err := m.saveLocked(r, "test", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	event := delegate.ProgressEvent{InvocationID: "attempt-p0-inv-0001", AgentID: "codex", TaskKind: "propose", Phase: "propose", Kind: "started", Model: "gpt-6.1-sol"}
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal(err)
	}
	for _, delta := range []string{"初稿摘要", "仍有完整正文等待提交"} {
		event.Kind = "text.delta"
		event.Text = delta
		event.Delta = delta
		if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
			t.Fatal(err)
		}
	}
	streamed, _ := m.Get(r.Snapshot.ID)
	if len(streamed.Messages) != 1 || !streamed.Messages[0].Streaming {
		t.Fatal("未建立单条流式P0")
	}
	event.Kind = "completed"
	event.Text = ""
	event.Delta = ""
	event.Complete = true
	if err := m.progress(r.Snapshot.ID, r.Snapshot.AttemptID, event); err != nil {
		t.Fatal(err)
	}
	ended, _ := m.Get(r.Snapshot.ID)
	if ended.Messages[0].Streaming {
		t.Fatal("无文本终态没有结束生成状态")
	}
	full := strings.Repeat("完整P0正文与可验收主张。", 100)
	proposal := model.Proposal{ID: r.Snapshot.ID + "-p0", Content: full, Hash: workflow.Hash(full), Version: 0}
	if err := m.update(r.Snapshot.ID, "proposal.created", func(current *model.StoredRun) error {
		commitP0Message(current, &proposal, r.Snapshot.AttemptID)
		current.Snapshot.Proposal = &proposal
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	done, _ := m.Get(r.Snapshot.ID)
	if len(done.Messages) != 1 {
		t.Fatalf("P0流与最终正文重复为%d条", len(done.Messages))
	}
	message := done.Messages[0]
	if message.ID != streamed.Messages[0].ID || message.InvocationID != event.InvocationID || message.Streaming || message.Content != full || message.Phase != "preparing" || message.Model != "gpt-6.1-sol" {
		t.Fatalf("P0未以原invocation完成提交: %#v", message)
	}
	if done.Proposal.InvocationID != event.InvocationID {
		t.Fatal("P0方案没有绑定原始调用")
	}
	record, _ := m.stored(r.Snapshot.ID)
	if !record.MessageSources[message.ID].Complete {
		t.Fatal("P0完整来源状态未持久化")
	}
}
func TestMockP0CommitStillHasOneMessage(t *testing.T) {
	r := &model.StoredRun{Snapshot: model.RunSnapshot{Messages: []model.ChatMessage{}}}
	proposal := model.Proposal{ID: "mock-p0", Content: "显式Mock初稿"}
	commitP0Message(r, &proposal, "mock-attempt")
	commitP0Message(r, &proposal, "mock-attempt")
	if len(r.Snapshot.Messages) != 1 || r.Snapshot.Messages[0].Streaming || r.Snapshot.Messages[0].Content != proposal.Content {
		t.Fatal("Mock P0重复或未结束")
	}
}
