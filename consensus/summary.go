package consensus

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// BuildRunSummary 为已完成的 run 渲染人类可读的 Markdown 摘要。
// 布局约定：结论在前（Conclusion / Retained Claims），过程明细（Final
// Vote、Claims、Statements）在后，读者不需要翻到文件底部才能拿到结果。
func BuildRunSummary(result *RunResult) string {
	lines := []string{
		"# til-consensus run " + result.RequestID,
		"",
		"- mode: " + string(result.Mode),
		"- goal: " + result.TaskSpec.Goal,
		"- elapsed: " + formatSummaryDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
	}
	if result.Lineage != nil {
		lines = append(lines,
			"- parent request: "+firstNonEmpty(result.Lineage.ParentRequestID, "-"),
			"- parent session: "+firstNonEmpty(result.Lineage.ParentSessionID, "-"),
			"- parent case: "+firstNonEmpty(result.Lineage.ParentCaseID, "-"),
			"- lineage trigger: "+firstNonEmpty(result.Lineage.Trigger, "-"),
		)
	}
	if result.TerminalState != "" {
		lines = append(lines, "- terminal state: "+string(result.TerminalState))
	}
	if result.CaseManifest != nil {
		lines = append(lines,
			"- task type: "+string(result.CaseManifest.TaskType),
			"- risk level: "+string(result.CaseManifest.RiskLevel),
			"- required evidence: "+string(result.CaseManifest.RequiredEvidenceLevel),
		)
	}
	lines = appendModeSummaryBullets(lines, result)
	lines = appendDegradationSection(lines, result)

	statements := runClaimStatements(result)
	lines = append(lines,
		"",
		"## Conclusion",
		"",
		result.Report.Summary,
	)
	if len(result.Report.RetainedClaims) > 0 {
		lines = append(lines, "", "## Retained Claims", "")
		for _, item := range result.Report.RetainedClaims {
			lines = append(lines, summarizeClaimRef(item, statements))
		}
	}
	if len(result.Report.DowngradedClaims) > 0 {
		lines = append(lines, "", "## Downgraded Claims", "")
		for _, item := range result.Report.DowngradedClaims {
			lines = append(lines, summarizeClaimRef(item, statements))
		}
	}
	if len(result.Report.UnresolvedQuestions) > 0 {
		lines = append(lines, "", "## Unresolved Questions", "")
		for _, item := range result.Report.UnresolvedQuestions {
			lines = append(lines, "- "+item)
		}
	}

	lines = appendModeDetailSections(lines, result)

	lines = append(lines,
		"",
		"## Task",
		"",
		result.TaskSpec.Goal,
	)
	if len(result.Observations) > 0 {
		lines = append(lines, "", "## Observe", "")
		for _, item := range result.Observations {
			lines = append(lines, fmt.Sprintf("- %s | %s", item.Outcome, item.Summary))
			if item.FollowUpCaseID != "" {
				lines = append(lines, "  - follow-up case: "+item.FollowUpCaseID)
			}
			if item.FollowUpRequestID != "" {
				lines = append(lines, "  - follow-up request: "+item.FollowUpRequestID)
			}
			if item.FollowUpArtifact != nil && strings.TrimSpace(item.FollowUpArtifact.Path) != "" {
				lines = append(lines, "  - follow-up artifact: "+item.FollowUpArtifact.Path)
			}
		}
	}
	lines = append(lines,
		"",
		"## Metrics",
		"",
		"- elapsed: "+formatSummaryDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
		fmt.Sprintf("- claims proposed: %d", result.Metrics.ClaimsProposed),
		fmt.Sprintf("- challenges opened: %d", result.Metrics.ChallengesOpened),
		fmt.Sprintf("- verifications run: %d", result.Metrics.VerificationsRun),
		fmt.Sprintf("- tasks dispatched: %d", result.Metrics.TasksDispatched),
		fmt.Sprintf("- timeouts: %d", result.Metrics.WaitTimeouts),
		fmt.Sprintf("- global deadline: %t", result.Metrics.GlobalDeadlineHit),
	)

	return strings.Join(lines, "\n") + "\n"
}

func appendModeSummaryBullets(lines []string, result *RunResult) []string {
	switch result.Mode {
	case WorkflowModeAdjudication:
		section := result.Adjudication
		if section == nil {
			return lines
		}
		counts := map[ClaimVerdict]int{}
		dispositions := map[ClaimDisposition]int{}
		for _, claim := range section.ClaimGraph {
			counts[claim.Verdict]++
			dispositions[claim.Disposition]++
		}
		lines = append(lines,
			"- task verdict: "+string(section.TaskVerdict),
			fmt.Sprintf("- claims: %d", len(section.ClaimGraph)),
			fmt.Sprintf("- supported: %d", counts[ClaimVerdictSupported]),
			fmt.Sprintf("- refuted: %d", counts[ClaimVerdictRefuted]),
			fmt.Sprintf("- insufficient evidence: %d", counts[ClaimVerdictInsufficientEvidence]),
			fmt.Sprintf("- undetermined: %d", counts[ClaimVerdictUndetermined]),
			fmt.Sprintf("- keep: %d", dispositions[ClaimDispositionKeep]),
			fmt.Sprintf("- keep with caveat: %d", dispositions[ClaimDispositionKeepWithCaveat]),
			fmt.Sprintf("- unresolved: %d", dispositions[ClaimDispositionUnresolved]),
			fmt.Sprintf("- reject: %d", dispositions[ClaimDispositionReject]),
		)
	case WorkflowModeFreeDebate:
		section := result.FreeDebate
		if section == nil {
			return lines
		}
		acceptedLine := fmt.Sprintf("- accepted claims: %d", countAcceptedDebate(section.ClaimResolutions))
		if section.BallotSize > 0 {
			accepted := countAcceptedDebate(section.ClaimResolutions)
			acceptedLine = fmt.Sprintf("- accepted claims: %d/%d ballot (%.0f%%)", accepted, section.BallotSize, float64(accepted)/float64(section.BallotSize)*100)
		}
		lines = append(lines,
			"- outcome: "+string(section.Outcome),
			fmt.Sprintf("- rounds: %d", len(section.Rounds)),
			fmt.Sprintf("- claims: %d", len(section.Claims)),
			acceptedLine,
		)
		if total := len(section.Voters) + len(section.AbsentVoters); total > 0 {
			voterLine := fmt.Sprintf("- voters: %d/%d", len(section.Voters), total)
			if len(section.AbsentVoters) > 0 {
				voterLine += " (absent: " + strings.Join(section.AbsentVoters, ", ") + ")"
			}
			lines = append(lines, voterLine)
		}
	case WorkflowModeDelphi:
		section := result.Delphi
		if section == nil {
			return lines
		}
		lines = append(lines,
			fmt.Sprintf("- rounds: %d", len(section.Rounds)),
			fmt.Sprintf("- consensus level: %.2f", section.ConsensusLevel),
			"- recommendation: "+firstNonEmpty(section.Recommendation, "未形成明确推荐"),
		)
	}
	return lines
}

func appendDegradationSection(lines []string, result *RunResult) []string {
	if len(result.Degradations) == 0 {
		return lines
	}
	lines = append(lines, "", "## Degradations", "")
	for _, item := range result.Degradations {
		header := "- ⚠ " + string(item.Kind) + " | " + item.Phase
		if item.Round > 0 {
			header += fmt.Sprintf(" (round %d)", item.Round)
		}
		if item.AgentID != "" {
			header += " | " + item.AgentID
		}
		if item.Reason != "" {
			header += " | " + item.Reason
		}
		lines = append(lines, header)
		if item.Impact != "" {
			lines = append(lines, "  impact: "+item.Impact)
		}
	}
	return lines
}

func appendModeDetailSections(lines []string, result *RunResult) []string {
	switch result.Mode {
	case WorkflowModeAdjudication:
		section := result.Adjudication
		if section == nil || len(section.ClaimGraph) == 0 {
			return lines
		}
		lines = append(lines, "", "## Claims")
		for _, claim := range section.ClaimGraph {
			lines = append(lines, "", "### "+firstNonEmpty(claim.Title, claim.ClaimID), "", claim.Statement)
			if claim.ClaimType != "" {
				lines = append(lines, "- claim type: "+string(claim.ClaimType))
			}
			lines = append(lines, "- verdict: "+string(claim.Verdict))
			if claim.Disposition != "" {
				lines = append(lines, "- disposition: "+string(claim.Disposition))
			}
			lines = append(lines, fmt.Sprintf("- confidence: %.2f", claim.Confidence))
			if claim.Scope != "" {
				lines = append(lines, "- scope: "+claim.Scope)
			}
			if len(claim.Caveats) > 0 {
				lines = append(lines, "- caveats: "+strings.Join(claim.Caveats, "; "))
			}
			if claim.Rationale != "" {
				lines = append(lines, "- rationale: "+claim.Rationale)
			}
		}
	case WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			lines = appendFreeDebateVoteSection(lines, result.FreeDebate)
		}
	case WorkflowModeDelphi:
		section := result.Delphi
		if section == nil {
			return lines
		}
		if len(section.Statements) > 0 {
			lines = append(lines, "", "## Statements")
			for _, item := range section.Statements {
				lines = append(lines, fmt.Sprintf("- %s | mean=%.2f | consensus=%.2f", item.Statement, item.MeanRating, item.ConsensusLevel))
			}
		}
		if len(section.DissentSummary) > 0 {
			lines = append(lines, "", "## Dissent")
			for _, item := range section.DissentSummary {
				lines = append(lines, "- "+item)
			}
		}
	}
	return lines
}

// appendFreeDebateVoteSection groups resolutions so a reader can tell apart
// accepted, voted-but-not-accepted, unvoted, and merged claims. Merged claims
// are folded to one line each: they were consolidated, not rejected.
func appendFreeDebateVoteSection(lines []string, section *FreeDebateResultSection) []string {
	if len(section.ClaimResolutions) == 0 {
		return lines
	}
	accepted := make([]DebateClaimResolution, 0)
	notAccepted := make([]DebateClaimResolution, 0)
	unvoted := make([]DebateClaimResolution, 0)
	merged := make([]DebateClaimResolution, 0)
	for _, item := range section.ClaimResolutions {
		switch {
		case item.MergedInto != "":
			merged = append(merged, item)
		case item.Accepted:
			accepted = append(accepted, item)
		case item.VoteCount > 0 || item.IncoherentVotes > 0:
			notAccepted = append(notAccepted, item)
		default:
			unvoted = append(unvoted, item)
		}
	}
	bySupportDesc := func(items []DebateClaimResolution) {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].SupportScore > items[j].SupportScore
		})
	}
	bySupportDesc(accepted)
	bySupportDesc(notAccepted)

	lines = append(lines, "", "## Final Vote")
	if len(accepted) > 0 {
		lines = append(lines, "", fmt.Sprintf("### Accepted (%d)", len(accepted)))
		for _, item := range accepted {
			lines = appendVoteResolutionLines(lines, item)
		}
	}
	if len(notAccepted) > 0 {
		lines = append(lines, "", fmt.Sprintf("### Not Accepted (%d)", len(notAccepted)))
		for _, item := range notAccepted {
			lines = appendVoteResolutionLines(lines, item)
		}
	}
	if len(unvoted) > 0 {
		lines = append(lines, "", fmt.Sprintf("### No Votes (%d)", len(unvoted)))
		for _, item := range unvoted {
			lines = append(lines, "- "+item.ClaimID)
			if item.FinalStatement != "" {
				lines = append(lines, "  "+item.FinalStatement)
			}
		}
	}
	if len(merged) > 0 {
		lines = append(lines, "", fmt.Sprintf("### Merged (%d)", len(merged)))
		for _, item := range merged {
			lines = append(lines, fmt.Sprintf("- %s → merged into %s", item.ClaimID, item.MergedInto))
		}
	}
	return lines
}

// appendVoteResolutionLines renders one voted claim. support= is the
// aggregated support score that the accept decision used, not the coarse
// label ratio.
func appendVoteResolutionLines(lines []string, item DebateClaimResolution) []string {
	line := fmt.Sprintf("- %s | support=%.2f | votes=%d (accept %d / reject %d / abstain %d)",
		item.ClaimID, item.SupportScore, item.VoteCount,
		len(item.SupportingVoters), len(item.OpposingVoters), len(item.AbstainingVoters))
	if item.ConfidenceStdDev >= 0.005 {
		line += fmt.Sprintf(" | stddev=%.2f", item.ConfidenceStdDev)
	}
	if item.IncoherentVotes > 0 {
		line += fmt.Sprintf(" | incoherent=%d", item.IncoherentVotes)
	}
	lines = append(lines, line)
	if item.FinalStatement != "" {
		lines = append(lines, "  "+item.FinalStatement)
	}
	return lines
}

// runClaimStatements maps claim/statement IDs to their text so report
// sections can render "id — statement" instead of bare IDs.
func runClaimStatements(result *RunResult) map[string]string {
	statements := map[string]string{}
	if result.Adjudication != nil {
		for _, claim := range result.Adjudication.ClaimGraph {
			statements[claim.ClaimID] = claim.Statement
		}
	}
	if result.FreeDebate != nil {
		for _, claim := range result.FreeDebate.Claims {
			statements[claim.ClaimID] = claim.Statement
		}
	}
	if result.Delphi != nil {
		for _, statement := range result.Delphi.Statements {
			statements[statement.StatementID] = statement.Statement
		}
	}
	return statements
}

func summarizeClaimRef(id string, statements map[string]string) string {
	statement := strings.TrimSpace(statements[id])
	if statement == "" {
		return "- " + id
	}
	return "- " + id + " — " + truncateSummaryText(statement, 240)
}

func truncateSummaryText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func countAcceptedDebate(items []DebateClaimResolution) int {
	total := 0
	for _, item := range items {
		if item.Accepted {
			total++
		}
	}
	return total
}

func formatSummaryDuration(value time.Duration) string {
	if value < time.Second {
		return fmt.Sprintf("%dms", value.Milliseconds())
	}
	if value < time.Minute {
		return fmt.Sprintf("%.1fs", value.Seconds())
	}
	minutes := int(value / time.Minute)
	seconds := int((value % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}
