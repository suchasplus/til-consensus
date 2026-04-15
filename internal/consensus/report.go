package consensus

import (
	"strconv"
	"strings"
)

func BuildBuiltinReport(includeTrace bool, traceLevel TraceLevel, status ConsensusStatus, representativeSpeech string, rounds []RoundRecord, representativeID string, finalClaims []Claim, claimResolutions []ClaimResolution) FinalReport {
	_ = representativeID
	report := FinalReport{
		Mode:                 "builtin",
		TraceIncluded:        includeTrace,
		TraceLevel:           traceLevel,
		FinalSummary:         buildFinalSummary(status, finalClaims, claimResolutions, rounds),
		RepresentativeSpeech: representativeSpeech,
	}
	if includeTrace {
		report.OpinionShiftTimeline = buildOpinionShiftTimeline(rounds)
		report.RoundHighlights = buildRoundHighlights(rounds, traceLevel)
	}
	return report
}

func buildFinalSummary(status ConsensusStatus, finalClaims []Claim, claimResolutions []ClaimResolution, rounds []RoundRecord) string {
	activeClaims := make([]Claim, 0)
	for _, claim := range finalClaims {
		if claim.Status == ClaimStatusActive {
			activeClaims = append(activeClaims, claim)
		}
	}
	resolved := 0
	for _, resolution := range claimResolutions {
		if resolution.Status == ClaimResolutionResolved {
			resolved++
		}
	}
	unresolved := len(claimResolutions) - resolved
	label := map[ConsensusStatus]string{
		ConsensusStatusConsensus:        "Consensus reached",
		ConsensusStatusPartialConsensus: "Partial consensus",
		ConsensusStatusUnresolved:       "Unresolved",
		ConsensusStatusFailed:           "Failed",
	}[status]
	lines := []string{strings.TrimSpace(label + ". " + strconv.Itoa(len(activeClaims)) + " claims: " + strconv.Itoa(resolved) + " resolved, " + strconv.Itoa(unresolved) + " unresolved.")}
	for _, claim := range activeClaims {
		voteStr := ""
		for _, resolution := range claimResolutions {
			if resolution.ClaimID == claim.ClaimID {
				voteStr = " (" + strconv.Itoa(resolution.AcceptCount) + "/" + strconv.Itoa(resolution.TotalVoters) + " accept)"
				break
			}
		}
		lines = append(lines, "- "+claim.ClaimID+": "+claim.Title+voteStr)
	}
	if len(rounds) > 0 {
		last := rounds[0]
		for _, round := range rounds[1:] {
			if round.Round > last.Round {
				last = round
			}
		}
		lines = append(lines, "", "Final round summaries:")
		for _, output := range last.Outputs {
			lines = append(lines, "- "+output.ParticipantID+": "+singleLine(output.Summary))
		}
	}
	return strings.Join(lines, "\n")
}

func buildOpinionShiftTimeline(rounds []RoundRecord) []OpinionShift {
	last := map[string]ClaimStance{}
	out := make([]OpinionShift, 0)
	for _, round := range rounds {
		for _, output := range round.Outputs {
			for _, judgement := range output.Judgements {
				key := output.ParticipantID + "::" + judgement.ClaimID
				prev, ok := last[key]
				if !ok || prev != judgement.Stance {
					from := "unknown"
					if ok {
						from = string(prev)
					}
					out = append(out, OpinionShift{
						ClaimID:       judgement.ClaimID,
						ParticipantID: output.ParticipantID,
						From:          from,
						To:            judgement.Stance,
						Round:         round.Round,
						Reason:        judgement.Rationale,
					})
					last[key] = judgement.Stance
				}
			}
		}
	}
	return out
}

func buildRoundHighlights(rounds []RoundRecord, level TraceLevel) []RoundHighlight {
	limit := 140
	if level == TraceLevelFull {
		limit = 280
	}
	out := make([]RoundHighlight, 0)
	for _, round := range rounds {
		for _, output := range round.Outputs {
			out = append(out, RoundHighlight{
				Round:         round.Round,
				ParticipantID: output.ParticipantID,
				Summary:       truncate(output.Summary, limit),
			})
		}
	}
	return out
}

func truncate(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars-1] + "…"
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
