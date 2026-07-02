package consensus

import (
	"strings"
	"unicode/utf8"
)

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

func canonicalizeDebateClaimDraft(draft ClaimDraft) ClaimDraft {
	draft.Title = stripDebateClaimStatusPrefix(draft.Title)
	draft.Statement = stripDebateClaimStatusPrefix(draft.Statement)
	return draft
}

func stripDebateClaimStatusPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	for {
		next := stripOneDebateClaimStatusPrefix(trimmed)
		if next == trimmed {
			return trimmed
		}
		trimmed = strings.TrimSpace(next)
	}
}

func stripOneDebateClaimStatusPrefix(value string) string {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "[status:") {
		if closeIdx := strings.Index(value, "]"); closeIdx >= 0 {
			rest := value[len("[status:"):closeIdx]
			if !isKnownDebateStatus(strings.TrimSpace(rest)) {
				return value
			}
			return trimStatusPrefixSeparator(value[closeIdx+1:])
		}
	}
	if strings.HasPrefix(lower, "status:") {
		return stripDebateStatusRest(value, value[len("status:"):])
	}
	for _, prefix := range []string{"裁决状态", "状态"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		return stripDebateStatusRest(value, value[len(prefix):])
	}
	return value
}

func stripDebateStatusRest(original string, rest string) string {
	rest = trimStatusPrefixSeparator(rest)
	for _, status := range debateStatusPrefixes() {
		if !hasDebateStatusPrefix(rest, status) {
			continue
		}
		return trimStatusPrefixSeparator(rest[len(status):])
	}
	return original
}

func hasDebateStatusPrefix(value string, status string) bool {
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, status) {
		return false
	}
	if len(value) == len(status) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(value[len(status):])
	return strings.ContainsRune(" \t\r\n]。.;；:：-—", next)
}

func isKnownDebateStatus(value string) bool {
	normalized := strings.Join(strings.Fields(strings.ToLower(value)), " ")
	for _, status := range debateStatusPrefixes() {
		if normalized == status {
			return true
		}
	}
	return false
}

func debateStatusPrefixes() []string {
	return []string{
		"accept",
		"accepted",
		"abstain",
		"keep",
		"no_change",
		"no change",
		"reject",
		"rejected",
		"revise",
		"unresolved",
		"接受",
		"保留",
		"拒绝",
		"未解决",
		"修订",
	}
}

func trimStatusPrefixSeparator(value string) string {
	return strings.TrimLeft(strings.TrimSpace(value), "。.;；:：-— \t\r\n")
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
