package runmanager

import (
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func TestMarkdownExportStatesAbsenceInvalidBallotsAndActualSources(t *testing.T) {
	m := manager(t, t.TempDir(), 0)
	claim := consensus.DebateClaim{ClaimID: "canonical", Statement: "当前方案", Category: consensus.DebateClaimCategorySynthesis, Active: true}
	proposal := &model.Proposal{ID: "p1", Content: claim.Statement, Hash: workflow.Hash(claim.Statement)}
	manifest := ballotManifestHash([]consensus.DebateClaim{claim})
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "export-audit", Mode: "free_debate", ProviderMode: "cli", Status: "completed"}, Proposal: proposal, Decision: &model.Decision{Outcome: "quorum_not_met", ManifestHash: manifest}, Ballots: []model.BallotBinding{{AgentID: "claude", Valid: true, ProposalHash: proposal.Hash, ManifestHash: manifest}, {AgentID: "agy", Valid: false, Reason: "旧版本 ballot 不可批准新方案"}}}, Request: &consensus.StartRequest{Roles: consensus.RoleAssignments{Proposers: []string{"codex"}, Participants: []string{"claude", "grok", "agy"}, Synthesizer: "codex"}}, Frozen: model.FrozenConfig{Providers: []model.ProviderConfig{{ID: "codex", Model: "gpt-6.1-sol", Version: "0.142.5"}, {ID: "claude", Model: "claude-opus-5-5", Version: "2.1.114"}, {ID: "grok", Model: "grok-4.7", Version: "1.0.62"}, {ID: "agy", Model: "gemini-3.8-flash-medium", Version: "1.112.0"}}}, ActualMetadata: map[string]map[string]any{"grok-call": {"agentId": "grok", "actualModel": "grok-4.7-build", "requestedModel": "grok-4.7", "cliVersion": "1.0.62", "modelSource": "cli-report", "rawOutput": "RAW-DIAGNOSTICS-MUST-NOT-EXPORT"}, "agy-call": {"agentId": "agy", "model": "gemini-3.8-flash-medium", "modelSource": "explicit-request"}}, CoreResult: &consensus.RunResult{FreeDebate: &consensus.FreeDebateResultSection{Claims: []consensus.DebateClaim{claim}, Voters: []string{"claude"}, AbsentVoters: []string{"grok", "agy"}}, Degradations: []consensus.Degradation{{Kind: consensus.DegradationParticipantAbsent, Phase: "final_vote", AgentID: "grok", Reason: "schema 校验失败", Impact: "缺少一张完整评审票"}}}}
	if err := m.saveLocked(r, "test", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	export, err := m.Export(r.Snapshot.ID, "markdown")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"参与者、CLI 与模型来源", "请求模型选择 ID", "实际报告模型", "grok-4.7-build", "grok-4.7", "1.0.62", "cli-report", "explicit-request", "| claude | 有效完整票 |", "| grok | 缺席／无效 | schema 校验失败", "旧版本 ballot 不可批准新方案", "缺少一张完整评审票", "未报告"} {
		if !strings.Contains(export.Content, required) {
			t.Fatalf("Markdown 导出遗漏 %q", required)
		}
	}
	if strings.Contains(export.Content, "RAW-DIAGNOSTICS-MUST-NOT-EXPORT") {
		t.Fatal("默认 Markdown 导出混入原始诊断日志")
	}
	// An explicit selector is not evidence that the CLI reported that model.
	for _, line := range strings.Split(export.Content, "\n") {
		if strings.HasPrefix(line, "| agy |") && strings.Contains(line, "explicit-request") && !strings.Contains(line, "| gemini-3.8-flash-medium | 未报告 | explicit-request |") {
			t.Fatalf("把选择模型当成实际报告：%s", line)
		}
	}
}

func TestMarkdownDelphiExportUsesAnonymousResponseCoverage(t *testing.T) {
	var output strings.Builder
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{Mode: "delphi"}}, CoreResult: &consensus.RunResult{Delphi: &consensus.DelphiResultSection{Rounds: []consensus.DelphiRoundRecord{{Round: 2, ResponderCount: 2, AbsentResponderCount: 1, ResponseQuorumMet: false, MinResponsesMet: false}}}}}
	writeParticipationCoverage(&output, r)
	if !strings.Contains(output.String(), "| 2 | 2 | 1 | false | false |") || strings.Contains(output.String(), "| claude |") {
		t.Fatal("Delphi 导出捏造自由辩论 ballot 或丢失缺席统计")
	}
}
