package artifact

import (
	"fmt"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func BuildSummary(result *consensus.AdjudicationResult) string {
	counts := map[consensus.ClaimVerdict]int{}
	for _, claim := range result.ClaimGraph {
		counts[claim.Verdict]++
	}
	lines := []string{
		"# til-consensus run " + result.RequestID,
		"",
		"- goal: " + result.TaskSpec.Goal,
		"- task verdict: " + string(result.TaskVerdict),
		"- elapsed: " + FormatDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
		fmt.Sprintf("- claims: %d", len(result.ClaimGraph)),
		fmt.Sprintf("- supported: %d", counts[consensus.ClaimVerdictSupported]),
		fmt.Sprintf("- refuted: %d", counts[consensus.ClaimVerdictRefuted]),
		fmt.Sprintf("- insufficient evidence: %d", counts[consensus.ClaimVerdictInsufficientEvidence]),
		fmt.Sprintf("- undetermined: %d", counts[consensus.ClaimVerdictUndetermined]),
		"",
		"## Task",
		"",
		result.TaskSpec.Goal,
		"",
		"## Conclusion",
		"",
		result.Report.Summary,
	}

	if len(result.ClaimGraph) > 0 {
		lines = append(lines, "", "## Claims")
		for _, claim := range result.ClaimGraph {
			lines = append(lines, "", "### "+firstNonEmpty(claim.Title, claim.ClaimID), "", claim.Statement)
			lines = append(lines, "- verdict: "+string(claim.Verdict))
			lines = append(lines, fmt.Sprintf("- confidence: %.2f", claim.Confidence))
			if claim.Scope != "" {
				lines = append(lines, "- scope: "+claim.Scope)
			}
			if claim.Rationale != "" {
				lines = append(lines, "- rationale: "+claim.Rationale)
			}
		}
	}

	if len(result.ChallengeTickets) > 0 {
		lines = append(lines, "", "## Challenges", "")
		for _, ticket := range result.ChallengeTickets {
			lines = append(lines, fmt.Sprintf("- %s | %s | %s", ticket.ClaimID, ticket.Kind, ticket.Status))
		}
	}

	lines = append(lines,
		"",
		"## Metrics",
		"",
		"- elapsed: "+FormatDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
		fmt.Sprintf("- claims proposed: %d", result.Metrics.ClaimsProposed),
		fmt.Sprintf("- challenges opened: %d", result.Metrics.ChallengesOpened),
		fmt.Sprintf("- verifications run: %d", result.Metrics.VerificationsRun),
		fmt.Sprintf("- tasks dispatched: %d", result.Metrics.TasksDispatched),
		fmt.Sprintf("- timeouts: %d", result.Metrics.WaitTimeouts),
		fmt.Sprintf("- global deadline: %t", result.Metrics.GlobalDeadlineHit),
	)

	return strings.Join(lines, "\n") + "\n"
}

func FormatDuration(value time.Duration) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
