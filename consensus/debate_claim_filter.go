package consensus

import (
	"strings"
	"unicode/utf8"
)

// isDebateProcessClaim decides whether a draft is a process/meta observation
// about the run itself. The model's own category label is the primary signal
// (schema-level self-classification beats keyword guessing at phrasing
// diversity); the keyword heuristic below stays as a backstop for unlabeled
// or mislabeled drafts.
func isDebateProcessClaim(draft ClaimDraft) bool {
	if draft.Category == DebateClaimCategoryProcess {
		return true
	}
	return IsDebateProcessMetaClaimDraft(draft)
}

// debateProcessNote renders a process claim as a one-line coordination note.
func debateProcessNote(draft ClaimDraft) string {
	statement := strings.TrimSpace(draft.Statement)
	if statement != "" {
		return statement
	}
	return strings.TrimSpace(draft.Title)
}

// IsDebateProcessMetaClaimDraft is the keyword backstop behind the
// category=process self-classification: it identifies operational
// observations about the debate run itself. Those observations are useful as
// process notes, but they must not become active business claims that enter
// dedup and final vote.
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
	draft.Title = stripDebateClaimStatusAffixes(draft.Title)
	draft.Statement = stripDebateClaimStatusAffixes(draft.Statement)
	return draft
}

// stripDebateClaimStatusAffixes removes status labels from both ends of a
// claim text. Models instructed to report verdict states tend to leak them as
// prefixes ("[Status: keep] ...") and as suffixes ("...。裁决：keep。"); both
// are protocol residue, not claim content.
func stripDebateClaimStatusAffixes(value string) string {
	trimmed := stripDebateClaimStatusPrefix(value)
	for {
		next := stripOneDebateClaimStatusSuffix(trimmed)
		if next == trimmed {
			return trimmed
		}
		trimmed = strings.TrimSpace(next)
	}
}

// stripOneDebateClaimStatusSuffix strips one trailing status label such as
// "裁决：keep。", "[Status: keep]", or "（裁决状态：保留）". It only strips
// when the text after the marker is exactly a known status word, so sentences
// that merely mention a status ("最终裁决：保留原方案不变") stay intact.
func stripOneDebateClaimStatusSuffix(value string) string {
	trimmed := strings.TrimRight(value, " \t\r\n")
	tail := strings.TrimRight(trimmed, "。.;；!！?？ \t\r\n]】)）")
	lowerTail := strings.ToLower(tail)
	for _, marker := range []string{"裁决状态", "裁决", "状态", "status"} {
		idx := strings.LastIndex(lowerTail, strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		rest := trimStatusPrefixSeparator(tail[idx+len(marker):])
		if rest == "" || !isKnownDebateStatus(rest) {
			continue
		}
		head := strings.TrimRight(tail[:idx], " \t\r\n[【(（:：-—")
		for _, qualifier := range []string{"最终", "final"} {
			head = strings.TrimRight(strings.TrimSuffix(head, qualifier), " \t\r\n")
		}
		if strings.TrimSpace(head) == "" {
			// The whole text is just a status marker; leave it for the
			// prefix/empty-statement handling instead of returning "".
			return value
		}
		return head
	}
	return value
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
