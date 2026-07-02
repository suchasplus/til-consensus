package artifact

import (
	"fmt"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func BuildSummary(result *consensus.RunResult) string {
	lines := []string{
		"# til-consensus run " + result.RequestID,
		"",
		"- mode: " + string(result.Mode),
		"- goal: " + result.TaskSpec.Goal,
		"- elapsed: " + FormatDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
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
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		section := result.Adjudication
		if section != nil {
			counts := map[consensus.ClaimVerdict]int{}
			dispositions := map[consensus.ClaimDisposition]int{}
			for _, claim := range section.ClaimGraph {
				counts[claim.Verdict]++
				dispositions[claim.Disposition]++
			}
			lines = append(lines,
				"- task verdict: "+string(section.TaskVerdict),
				fmt.Sprintf("- claims: %d", len(section.ClaimGraph)),
				fmt.Sprintf("- supported: %d", counts[consensus.ClaimVerdictSupported]),
				fmt.Sprintf("- refuted: %d", counts[consensus.ClaimVerdictRefuted]),
				fmt.Sprintf("- insufficient evidence: %d", counts[consensus.ClaimVerdictInsufficientEvidence]),
				fmt.Sprintf("- undetermined: %d", counts[consensus.ClaimVerdictUndetermined]),
				fmt.Sprintf("- keep: %d", dispositions[consensus.ClaimDispositionKeep]),
				fmt.Sprintf("- keep with caveat: %d", dispositions[consensus.ClaimDispositionKeepWithCaveat]),
				fmt.Sprintf("- unresolved: %d", dispositions[consensus.ClaimDispositionUnresolved]),
				fmt.Sprintf("- reject: %d", dispositions[consensus.ClaimDispositionReject]),
			)
			if len(section.ClaimGraph) > 0 {
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
			}
		}
	case consensus.WorkflowModeFreeDebate:
		section := result.FreeDebate
		if section != nil {
			lines = append(lines,
				"- outcome: "+string(section.Outcome),
				fmt.Sprintf("- rounds: %d", len(section.Rounds)),
				fmt.Sprintf("- claims: %d", len(section.Claims)),
				fmt.Sprintf("- accepted claims: %d", countAcceptedDebate(section.ClaimResolutions)),
			)
			if len(section.ClaimResolutions) > 0 {
				lines = append(lines, "", "## Final Vote")
				for _, item := range section.ClaimResolutions {
					lines = append(lines, fmt.Sprintf("- %s | accepted=%t | support=%.2f | confidenceMean=%.2f | confidenceVariance=%.4f", item.ClaimID, item.Accepted, item.SupportRatio, item.ConfidenceMean, item.ConfidenceVariance))
					if item.FinalStatement != "" {
						lines = append(lines, "  "+item.FinalStatement)
					}
				}
			}
		}
	case consensus.WorkflowModeDelphi:
		section := result.Delphi
		if section != nil {
			lines = append(lines,
				fmt.Sprintf("- rounds: %d", len(section.Rounds)),
				fmt.Sprintf("- consensus level: %.2f", section.ConsensusLevel),
				"- recommendation: "+firstNonEmpty(section.Recommendation, "未形成明确推荐"),
			)
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
	}

	lines = append(lines,
		"",
		"## Task",
		"",
		result.TaskSpec.Goal,
		"",
		"## Conclusion",
		"",
		result.Report.Summary,
	)
	if len(result.Report.RetainedClaims) > 0 {
		lines = append(lines, "", "## Retained Claims", "")
		for _, item := range result.Report.RetainedClaims {
			lines = append(lines, "- "+item)
		}
	}
	if len(result.Report.DowngradedClaims) > 0 {
		lines = append(lines, "", "## Downgraded Claims", "")
		for _, item := range result.Report.DowngradedClaims {
			lines = append(lines, "- "+item)
		}
	}
	if len(result.Report.UnresolvedQuestions) > 0 {
		lines = append(lines, "", "## Unresolved Questions", "")
		for _, item := range result.Report.UnresolvedQuestions {
			lines = append(lines, "- "+item)
		}
	}
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

func countAcceptedDebate(items []consensus.DebateClaimResolution) int {
	total := 0
	for _, item := range items {
		if item.Accepted {
			total++
		}
	}
	return total
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
