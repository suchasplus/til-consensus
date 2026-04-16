package consensus

import "strings"

type TaskTypePolicy struct {
	TaskType              CaseTaskType
	DefaultRiskLevel      RiskLevel
	RequiredEvidenceLevel EvidenceLevel
	DefaultAllowedTools   []string
}

func EffectiveTaskPolicy(spec TaskSpec) TaskTypePolicy {
	taskType := DetectTaskType(spec)
	switch taskType {
	case CaseTaskTypeCoding:
		return TaskTypePolicy{
			TaskType:              taskType,
			DefaultRiskLevel:      RiskLevelMedium,
			RequiredEvidenceLevel: EvidenceLevelHigh,
			DefaultAllowedTools:   []string{"repo", "tests", "static-analysis"},
		}
	case CaseTaskTypeStrategy:
		return TaskTypePolicy{
			TaskType:              taskType,
			DefaultRiskLevel:      RiskLevelHigh,
			RequiredEvidenceLevel: EvidenceLevelHigh,
			DefaultAllowedTools:   []string{"docs", "compare", "counterfactuals"},
		}
	case CaseTaskTypeOperational:
		return TaskTypePolicy{
			TaskType:              taskType,
			DefaultRiskLevel:      RiskLevelHigh,
			RequiredEvidenceLevel: EvidenceLevelHigh,
			DefaultAllowedTools:   []string{"runbook", "dry-run", "checks"},
		}
	default:
		return TaskTypePolicy{
			TaskType:              CaseTaskTypeFactual,
			DefaultRiskLevel:      RiskLevelMedium,
			RequiredEvidenceLevel: EvidenceLevelMedium,
			DefaultAllowedTools:   []string{"sources", "validate", "cross-check"},
		}
	}
}

func DetectTaskType(spec TaskSpec) CaseTaskType {
	if spec.TaskType != "" && spec.TaskType != CaseTaskTypeUnknown {
		return normalizeCaseTaskType(spec.TaskType)
	}
	goal := strings.ToLower(strings.TrimSpace(spec.Goal))
	if spec.WorkspaceSnapshot != nil || strings.TrimSpace(spec.Constraints.Language) != "" {
		return CaseTaskTypeCoding
	}
	if strings.Contains(goal, "架构") || strings.Contains(goal, "strategy") || strings.Contains(goal, "decision") || strings.Contains(goal, "monorepo") || strings.Contains(goal, "polyrepo") {
		return CaseTaskTypeStrategy
	}
	if strings.Contains(goal, "发布") || strings.Contains(goal, "执行") || strings.Contains(goal, "上线") || strings.Contains(goal, "rollback") || strings.Contains(goal, "runbook") {
		return CaseTaskTypeOperational
	}
	return CaseTaskTypeFactual
}

func BuildCaseManifest(request StartRequest) CaseManifest {
	policy := EffectiveTaskPolicy(request.TaskSpec)
	allowedTools := dedupeStrings(request.TaskSpec.AllowedTools)
	if len(allowedTools) == 0 {
		allowedTools = append([]string(nil), policy.DefaultAllowedTools...)
	}
	unresolved := dedupeStrings(defaultUnresolvedQuestions(request.TaskSpec, policy.TaskType))
	return CaseManifest{
		CaseID:                    "case_" + request.RequestID,
		CanonicalProblemStatement: strings.TrimSpace(request.TaskSpec.Goal),
		TaskType:                  policy.TaskType,
		Constraints:               request.TaskSpec.Constraints,
		SuccessCriteria:           dedupeStrings(request.TaskSpec.SuccessCriteria),
		OutOfScope:                dedupeStrings(request.TaskSpec.OutOfScope),
		RiskLevel:                 policy.DefaultRiskLevel,
		RequiredEvidenceLevel:     policy.RequiredEvidenceLevel,
		AllowedTools:              allowedTools,
		UnresolvedQuestions:       unresolved,
	}
}

func defaultUnresolvedQuestions(spec TaskSpec, taskType CaseTaskType) []string {
	out := make([]string, 0, 4)
	if len(spec.Materials) == 0 && spec.WorkspaceSnapshot == nil {
		out = append(out, "尚未提供足够的一手材料或工作区快照")
	}
	switch taskType {
	case CaseTaskTypeStrategy:
		out = append(out, "方案选择的约束优先级是否已经明确")
	case CaseTaskTypeOperational:
		out = append(out, "是否具备可审计的 dry-run / rollback 计划")
	case CaseTaskTypeCoding:
		out = append(out, "是否已有可复现的失败用例与通过标准")
	default:
		out = append(out, "是否存在更新、更权威或相互矛盾的来源")
	}
	return out
}

func RiskGateAllows(gate ActionRiskGate, risk RiskLevel) bool {
	switch gate {
	case ActionRiskGateAllowHigh:
		return true
	case ActionRiskGateAllowMedium:
		return risk == RiskLevelLow || risk == RiskLevelMedium
	default:
		return risk == RiskLevelLow
	}
}
