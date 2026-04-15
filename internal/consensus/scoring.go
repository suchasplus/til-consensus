package consensus

import (
	"fmt"
	"math"
	"sort"
)

type weightedRoundScore struct {
	Round     int
	Score     float64
	Breakdown ParticipantScoreBreakdown
}

func ComputeParticipantScores(participants []string, rounds []RoundRecord, finalClaims []Claim, policy ScoringPolicy) []ParticipantScore {
	byParticipant := map[string][]weightedRoundScore{}
	for _, participant := range participants {
		byParticipant[participant] = nil
	}
	weights := normalizeWeights(policy.Rubric)
	correctness := computePeerReviewCorrectness(participants, rounds, finalClaims)

	for _, round := range rounds {
		target := 1
		for _, output := range round.Outputs {
			size := maxInt(len(output.Judgements), len(output.ExtractedClaims), 1)
			if size > target {
				target = size
			}
		}
		for _, output := range round.Outputs {
			breakdown := scoreOutputNonCorrectness(output, target)
			breakdown.Correctness = correctness[output.ParticipantID]
			byParticipant[output.ParticipantID] = append(byParticipant[output.ParticipantID], weightedRoundScore{
				Round:     round.Round,
				Score:     weightedTotal(breakdown, weights),
				Breakdown: breakdown,
			})
		}
	}

	result := make([]ParticipantScore, 0, len(participants))
	for _, participant := range participants {
		roundScores := byParticipant[participant]
		sort.SliceStable(roundScores, func(i, j int) bool { return roundScores[i].Round < roundScores[j].Round })
		byRound := make([]ParticipantRoundScore, 0, len(roundScores))
		breakdowns := make([]ParticipantScoreBreakdown, 0, len(roundScores))
		total := 0.0
		for _, roundScore := range roundScores {
			byRound = append(byRound, ParticipantRoundScore{Round: roundScore.Round, Score: roundTo2(roundScore.Score)})
			breakdowns = append(breakdowns, roundScore.Breakdown)
			total += roundScore.Score
		}
		score := 0.0
		if len(byRound) > 0 {
			score = roundTo2(total / float64(len(byRound)))
		}
		result = append(result, ParticipantScore{
			ParticipantID: participant,
			Total:         score,
			ByRound:       byRound,
			Breakdown:     aggregateBreakdowns(breakdowns),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Total != result[j].Total {
			return result[i].Total > result[j].Total
		}
		return result[i].ParticipantID < result[j].ParticipantID
	})
	return result
}

func ChooseRepresentative(scores []ParticipantScore, rounds []RoundRecord, tieBreaker TieBreaker) (Representative, error) {
	if len(scores) == 0 {
		return Representative{}, fmt.Errorf("cannot choose representative without scores")
	}
	topScore := scores[0].Total
	ties := make([]ParticipantScore, 0)
	for _, score := range scores {
		if score.Total == topScore {
			ties = append(ties, score)
		}
	}
	if len(ties) == 1 {
		return Representative{
			ParticipantID: ties[0].ParticipantID,
			Reason:        RepresentativeReasonTopScore,
			Score:         ties[0].Total,
		}, nil
	}
	winner := ties[0]
	if tieBreaker == TieBreakerLeastObjection {
		winner = breakTieByLeastObjection(ties, rounds)
	} else {
		winner = breakTieByLatestRoundScore(ties)
	}
	return Representative{
		ParticipantID: winner.ParticipantID,
		Reason:        RepresentativeReasonTieBreaker,
		Score:         winner.Total,
	}, nil
}

func computePeerReviewCorrectness(participants []string, rounds []RoundRecord, finalClaims []Claim) map[string]float64 {
	canonical := buildCanonicalClaimMap(finalClaims)
	owners := map[string][]string{}
	for _, claim := range finalClaims {
		id := canonical[claim.ClaimID]
		if id == "" {
			id = claim.ClaimID
		}
		owners[id] = uniqueStrings(append(owners[id], claim.ProposedBy...))
	}
	type stat struct{ agree, total int }
	stats := map[string]stat{}
	for _, participant := range participants {
		stats[participant] = stat{}
	}
	for _, round := range rounds {
		for _, output := range round.Outputs {
			for _, judgement := range output.Judgements {
				id := canonical[judgement.ClaimID]
				if id == "" {
					id = judgement.ClaimID
				}
				for _, owner := range owners[id] {
					if owner == output.ParticipantID {
						continue
					}
					item := stats[owner]
					item.total++
					if judgement.Stance == ClaimStanceAgree {
						item.agree++
					}
					stats[owner] = item
				}
			}
		}
	}
	out := map[string]float64{}
	for _, participant := range participants {
		item := stats[participant]
		if item.total == 0 {
			out[participant] = 50
			continue
		}
		out[participant] = roundTo2((float64(item.agree) / float64(item.total)) * 100)
	}
	return out
}

func buildCanonicalClaimMap(finalClaims []Claim) map[string]string {
	direct := map[string]string{}
	for _, claim := range finalClaims {
		if claim.Status == ClaimStatusMerged && claim.MergedInto != "" {
			direct[claim.ClaimID] = claim.MergedInto
		}
	}
	resolve := func(id string) string {
		current := id
		seen := map[string]struct{}{}
		for {
			next, ok := direct[current]
			if !ok {
				return current
			}
			if _, dup := seen[current]; dup {
				return current
			}
			seen[current] = struct{}{}
			current = next
		}
	}
	out := map[string]string{}
	for _, claim := range finalClaims {
		out[claim.ClaimID] = resolve(claim.ClaimID)
	}
	return out
}

func breakTieByLatestRoundScore(candidates []ParticipantScore) ParticipantScore {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := latestRoundScore(candidates[i])
		right := latestRoundScore(candidates[j])
		if left != right {
			return left > right
		}
		return candidates[i].ParticipantID < candidates[j].ParticipantID
	})
	return candidates[0]
}

func breakTieByLeastObjection(candidates []ParticipantScore, rounds []RoundRecord) ParticipantScore {
	objections := map[string]int{}
	for _, candidate := range candidates {
		objections[candidate.ParticipantID] = 0
	}
	for _, round := range rounds {
		for _, output := range round.Outputs {
			if _, ok := objections[output.ParticipantID]; !ok {
				continue
			}
			count := 0
			for _, judgement := range output.Judgements {
				if judgement.Stance == ClaimStanceDisagree {
					count++
				}
			}
			objections[output.ParticipantID] += count
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := objections[candidates[i].ParticipantID]
		right := objections[candidates[j].ParticipantID]
		if left != right {
			return left < right
		}
		return candidates[i].ParticipantID < candidates[j].ParticipantID
	})
	return candidates[0]
}

func scoreOutputNonCorrectness(output ParticipantRoundOutput, roundClaimTarget int) ParticipantScoreBreakdown {
	stanceAvg := 0.7
	if len(output.Judgements) > 0 {
		sum := 0.0
		for _, judgement := range output.Judgements {
			sum += stanceFactor(judgement.Stance)
		}
		stanceAvg = sum / float64(len(output.Judgements))
	}
	completeness := roundTo2((clamp(float64(len(output.Judgements))/float64(maxInt(roundClaimTarget, 1)), 0, 1)*0.5 +
		clamp(float64(len(output.FullResponse))/160.0, 0, 1)*0.3 +
		clamp(float64(len(output.Summary))/80.0, 0, 1)*0.2) * 100)
	hasRevision := false
	for _, judgement := range output.Judgements {
		if judgement.RevisedStatement != "" {
			hasRevision = true
			break
		}
	}
	hasVotes := output.Phase == PhaseFinalVote && len(output.ClaimVotes) > 0
	actionability := roundTo2((clamp(float64(len(output.Summary))/60.0, 0, 1)*0.45 +
		clamp(float64(len(output.FullResponse))/180.0, 0, 1)*0.25 +
		valueBool(hasRevision, 1, 0.55)*0.2 +
		valueBool(hasVotes, 1, 0.7)*0.1) * 100)
	consistency := roundTo2(clamp(stanceAvg*100, 0, 100))
	return ParticipantScoreBreakdown{
		Completeness:  completeness,
		Actionability: actionability,
		Consistency:   consistency,
	}
}

func aggregateBreakdowns(items []ParticipantScoreBreakdown) *ParticipantScoreBreakdown {
	if len(items) == 0 {
		return nil
	}
	return &ParticipantScoreBreakdown{
		Correctness:   roundTo2(average(project(items, func(v ParticipantScoreBreakdown) float64 { return v.Correctness }))),
		Completeness:  roundTo2(average(project(items, func(v ParticipantScoreBreakdown) float64 { return v.Completeness }))),
		Actionability: roundTo2(average(project(items, func(v ParticipantScoreBreakdown) float64 { return v.Actionability }))),
		Consistency:   roundTo2(average(project(items, func(v ParticipantScoreBreakdown) float64 { return v.Consistency }))),
	}
}

func weightedTotal(breakdown ParticipantScoreBreakdown, weights RubricWeights) float64 {
	totalWeight := weights.Correctness + weights.Completeness + weights.Actionability + weights.Consistency
	if totalWeight <= 0 {
		totalWeight = 1
	}
	return roundTo2((breakdown.Correctness*weights.Correctness +
		breakdown.Completeness*weights.Completeness +
		breakdown.Actionability*weights.Actionability +
		breakdown.Consistency*weights.Consistency) / totalWeight)
}

func normalizeWeights(weights RubricWeights) RubricWeights {
	if weights.Correctness+weights.Completeness+weights.Actionability+weights.Consistency > 0 {
		return weights
	}
	return RubricWeights{
		Correctness:   0.35,
		Completeness:  0.25,
		Actionability: 0.25,
		Consistency:   0.15,
	}
}

func stanceFactor(stance ClaimStance) float64 {
	switch stance {
	case ClaimStanceAgree:
		return 1
	case ClaimStanceRevise:
		return 0.8
	default:
		return 0.6
	}
}

func latestRoundScore(score ParticipantScore) float64 {
	if len(score.ByRound) == 0 {
		return 0
	}
	return score.ByRound[len(score.ByRound)-1].Score
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func clamp(value, min, max float64) float64 {
	return math.Max(min, math.Min(max, value))
}

func roundTo2(value float64) float64 {
	return math.Round(value*100) / 100
}

func project[T any](items []T, fn func(T) float64) []float64 {
	out := make([]float64, 0, len(items))
	for _, item := range items {
		out = append(out, fn(item))
	}
	return out
}

func valueBool(ok bool, ifTrue, ifFalse float64) float64 {
	if ok {
		return ifTrue
	}
	return ifFalse
}

func maxInt(values ...int) int {
	current := values[0]
	for _, value := range values[1:] {
		if value > current {
			current = value
		}
	}
	return current
}
