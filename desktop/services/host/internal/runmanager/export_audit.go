package runmanager

import (
	"fmt"
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func writeExportProvenance(b *strings.Builder, r *model.StoredRun) {
	b.WriteString("\n## 参与者、CLI 与模型来源\n\n")
	if r.Snapshot.Mode == "delphi" {
		b.WriteString("此导出供本机所有者审计，包含身份映射；参与者收到的是单独的匿名材料投影。\n\n")
	}
	b.WriteString("| 参与者 | 角色 | CLI 可执行文件 | CLI 冻结版本 | 实际报告版本 | 请求模型选择 ID | 实际报告模型 | 来源 | 请求推理 | 实际报告推理 | 配置来源 |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	configured := r.Frozen.Providers
	if len(configured) == 0 {
		configured = r.Snapshot.Providers
	}
	for _, provider := range configured {
		versions, models, sources, efforts := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, meta := range r.ActualMetadata {
			if exportString(meta, "agentId") != provider.ID {
				continue
			}
			if version := exportString(meta, "cliVersion"); version != "" {
				versions[version] = true
			}
			reported := exportString(meta, "actualModel")
			if reported == "" {
				reported = exportString(meta, "reportedModel")
			}
			if reported == "" && exportString(meta, "modelSource") == "cli-report" {
				reported = exportString(meta, "model")
			}
			if reported != "" {
				models[reported] = true
			}
			if effort := exportString(meta, "reportedReasoning"); effort != "" {
				efforts[effort] = true
			}
			if source := exportString(meta, "modelSource"); source != "" {
				sources[source] = true
			}
		}
		if binding := r.ModelBindings[provider.ID]; binding.Model != "" {
			models[binding.Model] = true
			sources["cli-report（已校验绑定）"] = true
		}
		configSources := []string{}
		for _, key := range []string{"enabled", "model", "reasoning"} {
			if source, ok := provider.Sources[key]; ok {
				value := key + ":" + source.Kind
				if source.Env != "" {
					value += "(" + source.Env + ")"
				}
				configSources = append(configSources, value)
			}
		}
		requestedEffort := provider.Reasoning
		if requestedEffort == "" {
			requestedEffort = "CLI 默认（未传覆盖）"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", cell(provider.ID), cell(exportRoles(r.Request, provider.ID)), cell(exportKnown(provider.Command)), cell(exportKnown(provider.Version)), cell(exportSet(versions)), cell(exportKnown(provider.Model)), cell(exportSet(models)), cell(exportSet(sources)), cell(requestedEffort), cell(exportSet(efforts)), cell(strings.Join(configSources, "；")))
	}
	if len(configured) == 0 {
		b.WriteString("| 未记录 | 未记录 | 未报告 | 未报告 | 未报告 | 未指定 | 未报告 | 未记录 | 未报告 | 未记录 | 历史记录无来源元数据 |\n")
	}
	b.WriteString("\n“请求模型选择 ID”仅用于 CLI 参数；实际报告模型与其可能不同。未报告不代表零消耗或已确认模型身份。请求推理只表示发给 CLI 的覆盖参数，未报告实际推理时不得以请求值或模型名后缀推定。\n")
}

func writeParticipationCoverage(b *strings.Builder, r *model.StoredRun) {
	b.WriteString("\n## 有效响应、缺席与无效票\n\n")
	if r.Snapshot.Mode == "free_debate" {
		participants := workflow.RolePlanFor(r.Frozen).Reviewers
		if r.Request != nil && len(r.Request.Roles.Participants) > 0 {
			participants = r.Request.Roles.Participants
		}
		proposalHash, manifestHash := "", ""
		if r.Snapshot.Proposal != nil {
			proposalHash = r.Snapshot.Proposal.Hash
		}
		if r.Snapshot.Decision != nil {
			manifestHash = r.Snapshot.Decision.ManifestHash
		}
		if r.CoreResult != nil && r.CoreResult.FreeDebate != nil {
			manifestHash = ballotManifestHash(r.CoreResult.FreeDebate.Claims)
		}
		b.WriteString("| 评审 | 最终票状态 | 原因 |\n| --- | --- | --- |\n")
		for _, agent := range participants {
			valid := false
			reasons := []string{}
			for _, ballot := range r.Snapshot.Ballots {
				if ballot.AgentID != agent {
					continue
				}
				if ballot.Valid && ballot.ProposalHash == proposalHash && ballot.ManifestHash == manifestHash && proposalHash != "" && manifestHash != "" {
					valid = true
					continue
				}
				if ballot.Reason != "" {
					reasons = append(reasons, ballot.Reason)
				} else {
					reasons = append(reasons, "无效或未绑定当前冻结方案／清单")
				}
			}
			coreAbsent := false
			if r.CoreResult != nil && r.CoreResult.FreeDebate != nil {
				for _, absent := range r.CoreResult.FreeDebate.AbsentVoters {
					if absent == agent {
						coreAbsent = true
					}
				}
			}
			status := "有效完整票"
			if !valid || coreAbsent {
				status = "缺席／无效"
				if r.Snapshot.Status == "running" || r.Snapshot.Status == "queued" {
					status = "尚未提交有效完整票"
				}
				if r.CoreResult != nil {
					for _, degradation := range r.CoreResult.Degradations {
						if degradation.AgentID == agent && strings.Contains(degradation.Phase, "vote") && degradation.Reason != "" {
							reasons = append(reasons, degradation.Reason)
						}
					}
				}
				if len(reasons) == 0 {
					reasons = append(reasons, "未收到对当前冻结版本的有效完整 ballot；不计为支持")
				}
			} else if len(reasons) == 0 {
				reasons = append(reasons, "方案与清单哈希匹配")
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", cell(agent), status, cell(strings.Join(reasons, "；")))
		}
	} else if r.Snapshot.Mode == "delphi" {
		b.WriteString("德尔菲使用匿名评分响应，不使用自由辩论 ballot。\n\n")
		if r.CoreResult != nil && r.CoreResult.Delphi != nil {
			b.WriteString("| 轮次 | 有效响应人数 | 缺席人数 | quorum | 候选最少评分 |\n| --- | --- | --- | --- | --- |\n")
			for _, round := range r.CoreResult.Delphi.Rounds {
				fmt.Fprintf(b, "| %d | %d | %d | %t | %t |\n", round.Round, round.ResponderCount, round.AbsentResponderCount, round.ResponseQuorumMet, round.MinResponsesMet)
			}
		} else {
			b.WriteString("尚未形成可用的匿名响应统计。\n")
		}
	} else {
		b.WriteString("裁决使用质疑、验证与仲裁记录，不使用自由辩论 ballot。\n")
	}
	if r.CoreResult == nil || len(r.CoreResult.Degradations) == 0 {
		b.WriteString("\n未记录额外的阶段缺席或降级；此项不能替代上方完整票／评分覆盖检查。\n")
		return
	}
	b.WriteString("\n### 阶段缺席与降级原因\n\n| 阶段／轮次 | 参与者 | 类型 | 原因 | 对结论的影响 |\n| --- | --- | --- | --- | --- |\n")
	for _, degradation := range r.CoreResult.Degradations {
		fmt.Fprintf(b, "| %s / %d | %s | %s | %s | %s |\n", cell(degradation.Phase), degradation.Round, cell(exportKnown(degradation.AgentID)), cell(string(degradation.Kind)), cell(exportKnown(degradation.Reason)), cell(degradation.Impact))
	}
}

func exportRoles(request *consensus.StartRequest, agent string) string {
	if request == nil {
		return "未记录"
	}
	roles := []string{}
	for _, group := range []struct {
		name   string
		agents []string
	}{{"提案", request.Roles.Proposers}, {"质疑", request.Roles.Challengers}, {"评审／评分", request.Roles.Participants}} {
		for _, id := range group.agents {
			if id == agent {
				roles = append(roles, group.name)
				break
			}
		}
	}
	for _, role := range []struct{ name, agent string }{{"综合", request.Roles.Synthesizer}, {"仲裁", request.Roles.Arbiter}, {"语义验证", request.Roles.SemanticVerifier}, {"匿名主持", request.Roles.Facilitator}, {"报告", request.Roles.Reporter}} {
		if role.agent == agent {
			roles = append(roles, role.name)
		}
	}
	if len(roles) == 0 {
		return "未分配"
	}
	return strings.Join(roles, "、")
}
func exportString(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}
func exportKnown(value string) string {
	if value == "" {
		return "未报告"
	}
	return value
}
func exportSet(values map[string]bool) string {
	items := make([]string, 0, len(values))
	for item := range values {
		items = append(items, item)
	}
	sort.Strings(items)
	if len(items) == 0 {
		return "未报告"
	}
	return strings.Join(items, "、")
}
