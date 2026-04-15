package consensus

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func UpdateClaims(base []Claim, outputs []ParticipantRoundOutput) ([]Claim, int, []RoundAppliedMerge) {
	claimMap := make(map[string]Claim, len(base))
	order := make(map[string]int, len(base))
	for idx, claim := range base {
		claimMap[claim.ClaimID] = claim
		order[claim.ClaimID] = idx
	}
	nextOrder := len(order)
	newClaimCount := 0
	seqByParticipant := map[string]int{}

	for _, output := range outputs {
		for _, extracted := range output.ExtractedClaims {
			seq := seqByParticipant[output.ParticipantID]
			seqByParticipant[output.ParticipantID] = seq + 1
			claimID := fmt.Sprintf("%s:%d:%d", output.ParticipantID, output.Round, seq)
			claimMap[claimID] = Claim{
				ClaimID:    claimID,
				Title:      extracted.Title,
				Statement:  extracted.Statement,
				Category:   extracted.Category,
				ProposedBy: []string{output.ParticipantID},
				Status:     ClaimStatusActive,
			}
			order[claimID] = nextOrder
			nextOrder++
			newClaimCount++
		}
	}

	if len(claimMap) == 0 {
		for _, output := range outputs {
			claimID := fmt.Sprintf("seed:%s:%d", output.ParticipantID, output.Round)
			claimMap[claimID] = Claim{
				ClaimID:    claimID,
				Title:      fmt.Sprintf("Seed from %s", output.ParticipantID),
				Statement:  output.Summary,
				Category:   ClaimCategoryTodo,
				ProposedBy: []string{output.ParticipantID},
				Status:     ClaimStatusActive,
			}
			order[claimID] = nextOrder
			nextOrder++
			newClaimCount++
		}
	}

	type mergeState struct {
		SourceClaimID  string
		TargetClaimID  string
		ParticipantIDs map[string]struct{}
	}
	mergeBySource := map[string]*mergeState{}
	appendParticipant := func(source, target, participantID string, createIfMissing bool) {
		if existing, ok := mergeBySource[source]; ok && existing.TargetClaimID == target {
			existing.ParticipantIDs[participantID] = struct{}{}
			return
		}
		if !createIfMissing {
			return
		}
		mergeBySource[source] = &mergeState{
			SourceClaimID:  source,
			TargetClaimID:  target,
			ParticipantIDs: map[string]struct{}{participantID: {}},
		}
	}

	for _, output := range outputs {
		for _, judgement := range output.Judgements {
			directClaim, directOK := claimMap[judgement.ClaimID]
			targetID := resolveClaimID(claimMap, judgement.ClaimID)
			targetClaim, targetOK := claimMap[targetID]
			if !directOK && !targetOK {
				continue
			}

			if judgement.RevisedStatement != "" && (judgement.Stance == ClaimStanceRevise || judgement.Stance == ClaimStanceDisagree) {
				revisionTarget := directClaim
				if !directOK {
					revisionTarget = targetClaim
				}
				revisionTarget.Statement = judgement.RevisedStatement
				claimMap[revisionTarget.ClaimID] = revisionTarget
			}

			if judgement.MergesWith == "" {
				continue
			}
			mergeIntoID := resolveClaimID(claimMap, judgement.MergesWith)
			if _, ok := claimMap[mergeIntoID]; !ok {
				continue
			}
			if targetID == mergeIntoID {
				if directOK && directClaim.Status == ClaimStatusMerged && directClaim.MergedInto == mergeIntoID {
					appendParticipant(directClaim.ClaimID, mergeIntoID, output.ParticipantID, false)
				}
				continue
			}

			leftOrder := order[targetID]
			rightOrder := order[mergeIntoID]
			survivorID := targetID
			loserID := mergeIntoID
			if rightOrder < leftOrder {
				survivorID, loserID = mergeIntoID, targetID
			}
			survivor, ok1 := claimMap[survivorID]
			loser, ok2 := claimMap[loserID]
			if !ok1 || !ok2 {
				continue
			}
			loser.Status = ClaimStatusMerged
			loser.MergedInto = survivorID
			survivor.ProposedBy = uniqueStrings(append(survivor.ProposedBy, loser.ProposedBy...))
			claimMap[loser.ClaimID] = loser
			claimMap[survivor.ClaimID] = survivor

			for id, claim := range claimMap {
				if claim.Status == ClaimStatusMerged && claim.MergedInto == loserID {
					claim.MergedInto = survivorID
					claimMap[id] = claim
				}
			}
			appendParticipant(loserID, survivorID, output.ParticipantID, true)
		}
	}

	claims := make([]Claim, 0, len(claimMap))
	for _, claim := range claimMap {
		claims = append(claims, claim)
	}
	sort.SliceStable(claims, func(i, j int) bool {
		leftOrder, lok := order[claims[i].ClaimID]
		rightOrder, rok := order[claims[j].ClaimID]
		if lok && rok && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return claims[i].ClaimID < claims[j].ClaimID
	})

	merges := make([]RoundAppliedMerge, 0, len(mergeBySource))
	for _, state := range mergeBySource {
		participantIDs := make([]string, 0, len(state.ParticipantIDs))
		for participantID := range state.ParticipantIDs {
			participantIDs = append(participantIDs, participantID)
		}
		sort.Strings(participantIDs)
		merges = append(merges, RoundAppliedMerge{
			SourceClaimID:  state.SourceClaimID,
			TargetClaimID:  state.TargetClaimID,
			ParticipantIDs: participantIDs,
		})
	}
	sort.SliceStable(merges, func(i, j int) bool {
		leftOrder := order[merges[i].SourceClaimID]
		rightOrder := order[merges[j].SourceClaimID]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return merges[i].SourceClaimID < merges[j].SourceClaimID
	})

	return claims, newClaimCount, merges
}

func BuildClaimResolutions(claims []Claim, finalVoteOutputs []ParticipantRoundOutput, threshold float64, forceUnresolved bool) []ClaimResolution {
	activeClaims := make([]Claim, 0)
	active := map[string]struct{}{}
	for _, claim := range claims {
		if claim.Status == ClaimStatusActive {
			activeClaims = append(activeClaims, claim)
			active[claim.ClaimID] = struct{}{}
		}
	}
	votesByClaim := map[string]map[string]ClaimVote{}
	for _, claim := range activeClaims {
		votesByClaim[claim.ClaimID] = map[string]ClaimVote{}
	}
	for _, output := range finalVoteOutputs {
		if output.Phase != PhaseFinalVote {
			continue
		}
		for _, vote := range output.ClaimVotes {
			if _, ok := active[vote.ClaimID]; !ok {
				continue
			}
			votesByClaim[vote.ClaimID][output.ParticipantID] = ClaimVote{
				ParticipantID: output.ParticipantID,
				ClaimID:       vote.ClaimID,
				Vote:          vote.Vote,
				Reason:        vote.Reason,
			}
		}
	}

	out := make([]ClaimResolution, 0, len(activeClaims))
	for _, claim := range activeClaims {
		voteMap := votesByClaim[claim.ClaimID]
		votes := make([]ClaimVote, 0, len(voteMap))
		acceptCount := 0
		rejectCount := 0
		for _, vote := range voteMap {
			votes = append(votes, vote)
			if vote.Vote == "accept" {
				acceptCount++
			}
			if vote.Vote == "reject" {
				rejectCount++
			}
		}
		sort.SliceStable(votes, func(i, j int) bool {
			return votes[i].ParticipantID < votes[j].ParticipantID
		})
		totalVoters := len(votes)
		ratio := 0.0
		if totalVoters > 0 {
			ratio = float64(acceptCount) / float64(totalVoters)
		}
		status := ClaimResolutionUnresolved
		if !forceUnresolved && totalVoters > 0 && ratio >= threshold {
			status = ClaimResolutionResolved
		}
		out = append(out, ClaimResolution{
			ClaimID:     claim.ClaimID,
			Status:      status,
			AcceptCount: acceptCount,
			RejectCount: rejectCount,
			TotalVoters: totalVoters,
			Votes:       votes,
		})
	}
	return out
}

func AggregateSessionStatus(claimResolutions []ClaimResolution, globalDeadlineHit bool) ConsensusStatus {
	if globalDeadlineHit {
		return ConsensusStatusUnresolved
	}
	if len(claimResolutions) == 0 {
		return ConsensusStatusUnresolved
	}
	resolvedCount := 0
	for _, resolution := range claimResolutions {
		if resolution.Status == ClaimResolutionResolved {
			resolvedCount++
		}
	}
	if resolvedCount == len(claimResolutions) {
		return ConsensusStatusConsensus
	}
	if resolvedCount > 0 {
		return ConsensusStatusPartialConsensus
	}
	return ConsensusStatusUnresolved
}

func ShouldEarlyStop(outputs []ParticipantRoundOutput, newClaimCount int) bool {
	if len(outputs) == 0 || newClaimCount > 0 {
		return false
	}
	for _, output := range outputs {
		if len(output.Judgements) == 0 {
			return false
		}
		for _, judgement := range output.Judgements {
			if judgement.Stance != ClaimStanceAgree || judgement.RevisedStatement != "" {
				return false
			}
		}
	}
	return true
}

func ApplyTextBudget(text string, maxChars int, strategy OverflowStrategy) (string, bool) {
	if len(text) <= maxChars {
		return text, false
	}
	if maxChars < 10 {
		return text[:maxChars], true
	}
	if strategy == OverflowTruncateMiddle {
		head := int(math.Floor(float64(maxChars-1) / 2))
		tail := maxChars - 1 - head
		return text[:head] + "…" + text[len(text)-tail:], true
	}
	return text[:maxChars-1] + "…", true
}

func CollectDisagreements(rounds []RoundRecord) []Disagreement {
	out := make([]Disagreement, 0)
	for _, round := range rounds {
		for _, output := range round.Outputs {
			for _, judgement := range output.Judgements {
				if judgement.Stance != ClaimStanceDisagree {
					continue
				}
				out = append(out, Disagreement{
					ClaimID:       judgement.ClaimID,
					ParticipantID: output.ParticipantID,
					Reason:        judgement.Rationale,
				})
			}
		}
	}
	return out
}

func SelectTaskTitle(rounds []RoundRecord, representativeID string, scoreboard []ParticipantScore, fallbackPrompt string) string {
	var initRound *RoundRecord
	for idx := range rounds {
		if rounds[idx].Round == 0 {
			initRound = &rounds[idx]
			break
		}
	}
	if initRound != nil {
		if ownTitle := findInitialTaskTitle(initRound.Outputs, representativeID); ownTitle != "" {
			return ownTitle
		}
		for _, score := range scoreboard {
			if score.ParticipantID == representativeID {
				continue
			}
			if candidate := findInitialTaskTitle(initRound.Outputs, score.ParticipantID); candidate != "" {
				return candidate
			}
		}
	}
	trimmed := strings.TrimSpace(fallbackPrompt)
	if trimmed == "" {
		return "Untitled til-consensus session"
	}
	if len(trimmed) <= 60 {
		return trimmed
	}
	return trimmed[:59] + "…"
}

func findInitialTaskTitle(outputs []ParticipantRoundOutput, participantID string) string {
	for _, output := range outputs {
		if output.Phase == PhaseInitial && output.ParticipantID == participantID && output.TaskTitle != "" {
			return output.TaskTitle
		}
	}
	return ""
}

func resolveClaimID(claimMap map[string]Claim, claimID string) string {
	current := claimID
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[current]; ok {
			return current
		}
		seen[current] = struct{}{}
		claim, ok := claimMap[current]
		if !ok || claim.Status != ClaimStatusMerged || claim.MergedInto == "" {
			return current
		}
		current = claim.MergedInto
	}
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
