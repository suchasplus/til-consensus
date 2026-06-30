package consensus

import "strings"

// IsDebateProcessMetaClaimDraft identifies operational observations about the
// debate run itself. Those observations are useful in summaries/debug logs, but
// they must not become active business claims that enter dedup and final vote.
func IsDebateProcessMetaClaimDraft(draft ClaimDraft) bool {
	text := strings.ToLower(strings.Join(filterEmpty([]string{
		draft.Title,
		draft.Statement,
		draft.Applicability,
		strings.Join(draft.BoundaryConditions, " "),
	}), " "))
	if strings.TrimSpace(text) == "" {
		return false
	}

	if containsAny(text, []string{"辩论流程", "流程优化", "本辩论系统"}) &&
		containsAny(text, []string{"peer claims", "claim", "claims", "声明", "主张", "去重", "轮次", "讨论效率", "可审阅性"}) {
		return true
	}
	if strings.Contains(text, "peer claims") &&
		containsAny(text, []string{"本轮", "去重", "dedup", "声明数量", "claim count", "独立论点", "后续轮次", "讨论效率", "可审阅性"}) {
		return true
	}
	if containsAny(text, []string{"声明数量", "claim 数量", "claim数量"}) &&
		containsAny(text, []string{"去重", "dedup", "控制在", "讨论效率", "可审阅性"}) {
		return true
	}
	return false
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
