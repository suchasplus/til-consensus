package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
)

func TestNormalizedRequestMaterialPreservesOriginalUTF8Snapshot(t *testing.T) {
	original := "\uFEFF  BOM\r\nCRLF\n 空白尾部\t "
	materials, err := preparation.Normalize("保留原文", []model.MaterialInput{{ID: "raw", Title: "文件", Source: "file", Content: original, RawSHA256: Hash(original)}})
	if err != nil {
		t.Fatal(err)
	}
	r := model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "request", Goal: "保留原文", Mode: "free_debate"}, Materials: materials}, Frozen: model.FrozenConfig{ProviderMode: "mock"}}
	request, err := Request(r, model.Proposal{ID: "p0", Content: "初稿", Hash: Hash("初稿")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	refs := request.TaskSpec.Materials
	if len(refs) != 2 {
		t.Fatal("缺少材料")
	}
	ref := refs[1]
	var decoded model.Material
	if err := json.Unmarshal([]byte(ref.Content), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Content != original || decoded.SHA256 != Hash(original) || decoded.RawSHA256 != Hash(original) || ref.Hash != Hash(ref.Content) {
		t.Fatal("内核NormalizeStartRequest破坏了原文或封装hash")
	}
	p0 := P0Materials(r)
	if len(p0) != 1 || p0[0].Content != ref.Content {
		t.Fatal("P0和后续任务没有收到同一原文快照")
	}
}
func TestDelphiUserMaterialPreservesFactsButProjectsSourceMetadata(t *testing.T) {
	original := " 用户材料讨论 Codex 与 Grok 的能力，不应改写事实\r\n "
	materials, err := preparation.Normalize("对比工具", []model.MaterialInput{{ID: "user-material", Title: "Claude /Users/private/author.md", Source: "pasted", Content: original}})
	if err != nil {
		t.Fatal(err)
	}
	ref := MaterialRefs(materials)[0]
	ref.Path = "/Users/private/should-not-read"
	ref.Metadata["provider"] = "Claude"
	ref.Metadata["private"] = "/Users/private/secret"
	task := consensus.DelphiQuestionnaireTask{TaskSpec: consensus.TaskSpec{Goal: "对比工具", Materials: []consensus.MaterialRef{ref}}}
	projected := AnonymousDelphiTask(task).(consensus.DelphiQuestionnaireTask).TaskSpec.Materials[0]
	if projected.Path != "" || strings.Contains(projected.Title, "Claude") || projected.Metadata["provider"] != nil || projected.Metadata["private"] != nil {
		t.Fatal("Delphi留下来源身份或路径")
	}
	var snapshot model.Material
	if err := json.Unmarshal([]byte(projected.Content), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Content != original || snapshot.SHA256 != Hash(original) || strings.Contains(snapshot.Title, "Claude") {
		t.Fatal("用户事实被改写或来源title没有匿名")
	}
	if projected.Hash != Hash(projected.Content) {
		t.Fatal("匿名投影后的封装hash无效")
	}
}
