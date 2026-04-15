package artifact

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func BuildSummary(result *consensus.ConsensusResult) string {
	activeClaims := make([]consensus.Claim, 0)
	for _, claim := range result.FinalClaims {
		if claim.Status == consensus.ClaimStatusActive {
			activeClaims = append(activeClaims, claim)
		}
	}
	resolved := 0
	for _, item := range result.ClaimResolutions {
		if item.Status == consensus.ClaimResolutionResolved {
			resolved++
		}
	}

	lines := []string{
		"# til-consensus run " + result.RequestID,
		"",
		"- task: " + result.Task.Title,
		"- status: " + string(result.Status),
		fmt.Sprintf("- representative: %s (%s, score=%.2f)", result.Representative.ParticipantID, result.Representative.Reason, result.Representative.Score),
		"- elapsed: " + FormatDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
		fmt.Sprintf("- rounds: %d", result.Metrics.TotalRounds),
		fmt.Sprintf("- turns: %d", result.Metrics.TotalTurns),
		fmt.Sprintf("- claims: %d active / %d total", len(activeClaims), len(result.FinalClaims)),
		fmt.Sprintf("- resolved: %d/%d", resolved, len(result.ClaimResolutions)),
		"",
		"## Task",
		"",
		result.Task.Prompt,
		"",
		"## Conclusion",
		"",
		result.Report.FinalSummary,
		"",
		"## Representative Statement",
		"",
		result.Report.RepresentativeSpeech,
	}

	if len(activeClaims) > 0 {
		lines = append(lines, "", "## Claims")
		resolutionMap := map[string]consensus.ClaimResolution{}
		for _, resolution := range result.ClaimResolutions {
			resolutionMap[resolution.ClaimID] = resolution
		}
		for _, claim := range activeClaims {
			lines = append(lines, "", "### "+claim.Title, "", claim.Statement)
			if claim.Category != "" {
				lines = append(lines, "- category: "+string(claim.Category))
			}
			if resolution, ok := resolutionMap[claim.ClaimID]; ok {
				lines = append(lines, fmt.Sprintf("- accept: %d / reject: %d", resolution.AcceptCount, resolution.RejectCount))
			}
		}
	}

	scoreboard := append([]consensus.ParticipantScore(nil), result.Scoreboard...)
	sort.SliceStable(scoreboard, func(i, j int) bool {
		if scoreboard[i].Total != scoreboard[j].Total {
			return scoreboard[i].Total > scoreboard[j].Total
		}
		return scoreboard[i].ParticipantID < scoreboard[j].ParticipantID
	})
	if len(scoreboard) > 0 {
		lines = append(lines, "", "## Scoreboard", "")
		for _, entry := range scoreboard {
			line := fmt.Sprintf("- %s | %.2f", entry.ParticipantID, entry.Total)
			if entry.Breakdown != nil {
				parts := make([]string, 0, 4)
				parts = append(parts, fmt.Sprintf("correctness=%.2f", entry.Breakdown.Correctness))
				parts = append(parts, fmt.Sprintf("completeness=%.2f", entry.Breakdown.Completeness))
				parts = append(parts, fmt.Sprintf("actionability=%.2f", entry.Breakdown.Actionability))
				parts = append(parts, fmt.Sprintf("consistency=%.2f", entry.Breakdown.Consistency))
				line += " (" + strings.Join(parts, ", ") + ")"
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines,
		"",
		"## Metrics",
		"",
		"- elapsed: "+FormatDuration(time.Duration(result.Metrics.ElapsedMs)*time.Millisecond),
		fmt.Sprintf("- rounds: %d", result.Metrics.TotalRounds),
		fmt.Sprintf("- turns: %d", result.Metrics.TotalTurns),
		fmt.Sprintf("- retries: %d", result.Metrics.Retries),
		fmt.Sprintf("- timeouts: %d", result.Metrics.WaitTimeouts),
		fmt.Sprintf("- early stop: %t", result.Metrics.EarlyStopTriggered),
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
