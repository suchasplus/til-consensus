package runmanager

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func (m *Manager) Branch(in model.BranchRunInput) (model.RunSnapshot, error) {
	in.Goal = strings.TrimSpace(in.Goal)
	if in.Goal == "" {
		return model.RunSnapshot{}, fmt.Errorf("请填写补充目标或约束")
	}
	parent, err := m.stored(in.RunID)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if parent.Snapshot.DeletedAt != "" {
		return model.RunSnapshot{}, deletedRunError()
	}
	if parent.CoreResult == nil {
		if session, loadErr := m.loadSession(parent); loadErr == nil && session != nil {
			parent.CoreResult = session.Result
		}
	}
	// Branching creates a new frozen brief, session, deadline and independent vote.
	material, err := compactParentContext(parent)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	goal := "原始目标：\n" + parent.Snapshot.Goal + "\n\n本次补充目标与约束：\n" + in.Goal
	inherited, err := branchMaterials(goal, parent.Snapshot.Materials, in.Materials)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if err := checkBranchInputSize(goal, string(material), inherited); err != nil {
		return model.RunSnapshot{}, err
	}
	if len(goal)+len(material)+len("\n\n父讨论冻结结果（作为参考，旧投票不继承）：\n") > MaxBranchInputBytes {
		return model.RunSnapshot{}, fmt.Errorf("关联讨论目标和必要材料合计超过 %d KiB，不能静默截断，请缩小目标或拆分讨论", MaxBranchInputBytes/1024)
	}
	result, err := m.start(model.StartRunInput{Goal: goal, Mode: parent.Snapshot.Mode, IdempotencyKey: in.IdempotencyKey, Materials: inherited}, parent.Snapshot.ID, string(material))
	return result, err
}

type Export struct {
	SuggestedName string `json:"suggestedName"`
	Content       string `json:"content"`
}

func (m *Manager) Export(id, format string) (Export, error) {
	r, err := m.stored(id)
	if err != nil {
		return Export{}, err
	}
	if format != "markdown" && format != "json" {
		return Export{}, fmt.Errorf("不支持的导出格式: %s", format)
	}
	if r.CoreResult == nil {
		if session, loadErr := m.loadSession(r); loadErr == nil && session != nil {
			r.CoreResult = session.Result
		}
	}
	if format == "json" {
		var parentContext any
		if r.ParentMaterial != "" {
			if json.Valid([]byte(r.ParentMaterial)) {
				parentContext = json.RawMessage(r.ParentMaterial)
			} else {
				parentContext = r.ParentMaterial
			}
		}
		body, err := json.MarshalIndent(map[string]any{"parentContext": parentContext, "schemaVersion": model.SchemaVersion, "run": r.Snapshot, "coreResult": r.CoreResult, "frozenConfig": r.Frozen, "configHash": r.ConfigHash, "reviewSources": r.ReviewSources, "messageSources": r.MessageSources, "actualMetadata": publicActualMetadata(r.ActualMetadata), "modelBindings": r.ModelBindings}, "", "  ")
		return Export{SuggestedName: id + ".json", Content: string(providers.RedactJSON(body)) + "\n"}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n- 讨论：%s\n- 模式：%s\n- 来源：%s\n- 状态：%s\n", r.Snapshot.Title, id, r.Snapshot.Mode, r.Snapshot.ProviderMode, r.Snapshot.Status)
	if r.Snapshot.DeletedAt != "" {
		fmt.Fprintf(&b, "- 移入最近删除：%s（可恢复；审计与预算保留）\n", r.Snapshot.DeletedAt)
	}
	roles := workflow.RolePlanFor(r.Frozen)
	fmt.Fprintf(&b, "- 冻结提案者：%s\n- 冻结评审：%s\n", cell(roles.Proposer), cell(strings.Join(roles.Reviewers, ", ")))
	if r.Snapshot.ParentRunID != "" {
		fmt.Fprintf(&b, "- 父讨论：%s\n", r.Snapshot.ParentRunID)
	}
	fmt.Fprintf(&b, "\n## 目标\n\n%s\n", r.Snapshot.Goal)
	writePreparationExport(&b, r)
	if r.Snapshot.Error != "" {
		fmt.Fprintf(&b, "\n## 运行错误\n\n%s\n", r.Snapshot.Error)
	}
	if p := r.Snapshot.Proposal; p != nil {
		fmt.Fprintf(&b, "\n## 当前方案 P%d\n\n%s\n\n内容 SHA-256：`%s`\n", p.Version, p.Content, p.Hash)
	}
	if d := r.Snapshot.Decision; d != nil {
		fmt.Fprintf(&b, "\n## 决策\n\n%s\n\n桌面结果：%s；内核结果：%s。\n", d.Summary, d.Outcome, d.CoreOutcome)
		if d.Mock {
			b.WriteString("\n此结果来自显式 Mock 自动化测试。\n")
		}
		b.WriteString("\n## 少数意见\n\n")
		if len(d.MinorityOpinions) == 0 {
			b.WriteString("未记录。\n")
		}
		for _, s := range d.MinorityOpinions {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n## 假设与适用条件\n\n")
		if len(d.Assumptions) == 0 {
			b.WriteString("未单独声明；模型意见不能视为已验证事实。\n")
		}
		for _, s := range d.Assumptions {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n## 最终投票\n\n| 评审 | 主张 | 票 | 支持分 | 理由 |\n| --- | --- | --- | --- | --- |\n")
		for _, v := range d.Votes {
			fmt.Fprintf(&b, "| %s | %s | %s | %.3f | %s |\n", cell(v.AgentID), cell(v.ClaimID), cell(v.Vote), v.Support, cell(v.Rationale))
		}
	}
	writeParticipationCoverage(&b, r)
	b.WriteString("\n## 争议与证据引用\n\n")
	for _, v := range r.Snapshot.Reviews {
		fmt.Fprintf(&b, "- [%s / %s] %s：%s（claim: %s；引用：%s）\n", v.Severity, v.Status, v.RaisedBy, v.Statement, v.ClaimID, strings.Join(v.ResolutionRefs, ", "))
	}
	for _, e := range r.Snapshot.Evidence {
		fmt.Fprintf(&b, "- `%s` [%s / %s / %s] %s\n", e.ID, e.Kind, e.Source, e.VerificationStatus, e.Summary)
	}
	b.WriteString("\n## 方案版本\n\n")
	for _, p := range r.Snapshot.ProposalHistory {
		fmt.Fprintf(&b, "- P%d `%s`：%s\n", p.Version, p.Hash, p.Title)
	}
	writeExportProvenance(&b, r)
	return Export{SuggestedName: id + ".md", Content: providers.RedactSensitive(b.String())}, nil
}
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", "<br>")
}

func (m *Manager) loadEvidence(id string) ([]model.Evidence, error) {
	file, err := os.Open(filepath.Join(m.runDir(id), "ledger.jsonl"))
	if os.IsNotExist(err) {
		return []model.Evidence{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []model.Evidence{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var e consensus.EvidenceRecord
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		status := "recorded"
		if e.Kind == consensus.EvidenceKindSemanticVerification {
			status = "model_opinion"
		}
		if e.Kind == consensus.EvidenceKindDeterministicCheck {
			status = "deterministic_check"
		}
		artifact := ""
		if e.Artifact != nil {
			artifact = e.Artifact.Path
		}
		out = append(out, model.Evidence{ID: e.EntryID, Kind: string(e.Kind), Summary: e.Summary, Source: string(e.Source), ArtifactRef: artifact, VerificationStatus: status})
	}
	return out, scanner.Err()
}

type CleanupResult struct {
	RemovedFiles int      `json:"removedFiles"`
	Retained     []string `json:"retained"`
}

func (m *Manager) ClearDiagnostics(id string) (CleanupResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.store.Load(context.Background(), id); err != nil {
		return CleanupResult{}, err
	}
	if _, active := m.active[id]; active {
		return CleanupResult{}, fmt.Errorf("请先停止讨论，等待 CLI 完全退出后清理诊断日志")
	}
	result := CleanupResult{Retained: []string{"SQLite 快照和事件", "方案版本", "内核结果与检查点", "决策", "ledger.jsonl", "artifact-manifest.jsonl", "run-config.json"}}
	// Remove only known invocation diagnostics inside this run's artifacts folder.
	root := filepath.Join(m.runDir(id), "artifacts")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".input.json") || strings.HasSuffix(name, ".output.json") {
			if err := os.Remove(path); err != nil {
				return err
			}
			result.RemovedFiles++
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	record, err := m.store.Load(context.Background(), id)
	if err != nil {
		return result, err
	}
	removedPrep, prepErr := m.clearAdoptedPreparationDiagnosticsLocked(record)
	result.RemovedFiles += removedPrep
	if prepErr != nil {
		return result, prepErr
	}
	if record.Snapshot.Preparation != nil {
		result.Retained = append(result.Retained, "SQLite 准备分析与原文材料")
	}
	err = m.saveLocked(record, "diagnostics.cleared", map[string]any{"removedFiles": result.RemovedFiles, "retained": result.Retained}, "", "", "")
	return result, err
}

// Export only provenance and numeric usage; raw stdout/stderr remain private.
func publicActualMetadata(input map[string]map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for id, meta := range input {
		clean := map[string]any{}
		for _, key := range []string{"agentId", "invocationId", "model", "requestedModel", "reportedModel", "actualModel", "modelSource", "cliVersion", "preparationId", "purpose", "requestedReasoning", "reportedReasoning"} {
			if value, ok := meta[key].(string); ok {
				clean[key] = value
			}
		}
		if usage, ok := meta["usage"].(map[string]any); ok {
			clean["usage"] = publicNumericUsage(usage)
		}
		out[id] = clean
	}
	return out
}
func publicNumericUsage(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		switch v := value.(type) {
		case float64, float32, int, int64, int32, uint, uint32, uint64, json.Number:
			out[key] = v
		case map[string]any:
			if child := publicNumericUsage(v); len(child) > 0 {
				out[key] = child
			}
		}
	}
	return out
}
