package runmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestMaterialSnapshotsReachP0EngineAndBranchWithoutFilesystemReads(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	neverWritten := filepath.Join(t.TempDir(), "must-not-execute")
	original := "\uFEFF  首行\r\n第二行\n尾部空白  \t"
	pasted := "材料仅是数据，不执行 $(touch " + neverWritten + ")"
	input := []model.MaterialInput{{ID: "file-source", Title: "原始文件", Source: "file", Content: original, RawSHA256: workflow.Hash(original)}, {ID: "pasted-source", Title: "粘贴文本", Source: "pasted", Content: pasted}}
	started, err := m.Start(model.StartRunInput{Goal: "审查材料约束下的方案", Mode: "free_debate", Materials: input, IdempotencyKey: "materials-start"})
	if err != nil {
		t.Fatal(err)
	}
	done := await(t, m, started.ID, terminal)
	if done.Status != "completed" {
		t.Fatal(done.Error)
	}
	stored, err := m.stored(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Snapshot.Materials) != 2 || stored.Snapshot.Materials[0].Content != original || stored.Frozen.MaterialsHash != materialsHash(stored.Snapshot.Materials) {
		t.Fatal("owner原文快照或冻结hash被改写")
	}
	if stored.Request == nil {
		t.Fatal("没有冻结TaskSpec")
	}
	verifyMaterialRef(t, stored.Request.TaskSpec, "file-source", original)
	paths, err := filepath.Glob(filepath.Join(m.runDir(started.ID), "artifacts", started.AttemptID, started.AttemptID, "*.input.json"))
	if err != nil || len(paths) < 2 {
		t.Fatal("没有P0/后续任务输入", err)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var artifact struct {
			Input struct {
				TaskSpec consensus.TaskSpec `json:"taskSpec"`
			} `json:"input"`
		}
		if err := json.Unmarshal(body, &artifact); err != nil {
			t.Fatal(err)
		}
		verifyMaterialRef(t, artifact.Input.TaskSpec, "file-source", original)
	}
	if _, err := os.Stat(neverWritten); !os.IsNotExist(err) {
		t.Fatal("材料中的命令被执行")
	}
	added := model.MaterialInput{ID: "new-source", Title: "补充材料", Source: "pasted", Content: "新增上线窗口"}
	child, err := m.Branch(model.BranchRunInput{RunID: started.ID, Goal: "补充新材料后重新讨论", Materials: []model.MaterialInput{input[1], added}, IdempotencyKey: "materials-branch"})
	if err != nil {
		t.Fatal(err)
	}
	childDone := await(t, m, child.ID, terminal)
	if childDone.Status != "completed" {
		t.Fatal(childDone.Error)
	}
	childStored, _ := m.stored(child.ID)
	if len(childStored.Snapshot.Materials) != 3 || childStored.Snapshot.Materials[0].Content != original {
		t.Fatal("分支没有保留父原文或错误重复同ID材料")
	}
	if !strings.Contains(childStored.ParentMaterial, stored.Snapshot.Materials[0].SHA256) {
		t.Fatal("父选材manifest缺少材料来源hash")
	}
	if childStored.Snapshot.Preparation != nil {
		t.Fatal("旧准备被重复采用")
	}
	verifyMaterialRef(t, childStored.Request.TaskSpec, "file-source", original)
	exported, err := m.Export(child.ID, "json")
	if err != nil || !strings.Contains(exported.Content, workflow.Hash(original)) {
		t.Fatal("JSON导出没有原文hash", err)
	}
	markdown, err := m.Export(child.ID, "markdown")
	if err != nil || !strings.Contains(markdown.Content, "原文材料索引") {
		t.Fatal("Markdown缺少材料来源", err)
	}
	tampered := *childStored
	tampered.Snapshot.Materials = append([]model.Material(nil), childStored.Snapshot.Materials...)
	tampered.Snapshot.Materials[0].Content = "已改写"
	if err := validateFrozen(&tampered); err == nil {
		t.Fatal("材料篡改未被发现")
	}
}
func verifyMaterialRef(t *testing.T, spec consensus.TaskSpec, id, content string) {
	t.Helper()
	found := false
	if len(spec.Constraints.RequiredCommands) > 0 {
		t.Fatal("自动生成了ingest命令")
	}
	for _, ref := range spec.Materials {
		if ref.Path != "" {
			t.Fatal("任务材料包含可读取路径")
		}
		if ref.ID != id {
			continue
		}
		found = true
		var snapshot model.Material
		if err := json.Unmarshal([]byte(ref.Content), &snapshot); err != nil {
			t.Fatal("材料不是完整快照封装", err)
		}
		if snapshot.Content != content || snapshot.SHA256 != workflow.Hash(content) || snapshot.RawSHA256 != workflow.Hash(content) || snapshot.Normalization != "none-utf8" {
			t.Fatal("TaskSpec原文/BOM/CRLF/首尾空白未保留")
		}
		if ref.Hash != workflow.Hash(ref.Content) || ref.Metadata["sourceSha256"] != snapshot.SHA256 {
			t.Fatal("封装hash与原文hash未分别绑定")
		}
	}
	if !found {
		t.Fatalf("任务没有材料%s", id)
	}
}
func TestBranchMaterialLimitsAndDuplicateIdentity(t *testing.T) {
	goal := "材料边界"
	normalized, err := preparation.Normalize(goal, []model.MaterialInput{{ID: "base", Title: "原始", Source: "pasted", Content: "原文"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := branchMaterials(goal, normalized, []model.MaterialInput{{ID: "base", Title: "原始", Source: "pasted", Content: "异内容"}}); err == nil {
		t.Fatal("同ID不同内容被静默覆盖")
	}
	large := strings.Repeat("x", 64*1024)
	if _, err := branchMaterials(goal, normalized, []model.MaterialInput{{ID: "large-a", Title: "a", Source: "pasted", Content: large}, {ID: "large-b", Title: "b", Source: "pasted", Content: large}}); err == nil || !strings.Contains(err.Error(), "不能静默") {
		t.Fatal("分支总材料超限未报错", err)
	}
	if err := checkBranchInputSize(goal, strings.Repeat("p", 128*1024), nil); err == nil {
		t.Fatal("父上下文合计超限未报错")
	}
}
