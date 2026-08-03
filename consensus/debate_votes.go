package consensus

import (
	"fmt"
	"math"
	"strings"
)

// ValidateDebateVotes verifies that one participant returned exactly one vote
// for every claim on the final-vote ballot and no vote for anything else.
func ValidateDebateVotes(ballot []DebateClaim, votes []DebateVoteDraft) error {
	expected := make(map[string]struct{}, len(ballot))
	for idx, claim := range ballot {
		claimID := claim.ClaimID
		if strings.TrimSpace(claimID) == "" {
			return fmt.Errorf("ballot[%d].claimId is required", idx)
		}
		if _, exists := expected[claimID]; exists {
			return fmt.Errorf("ballot[%d].claimId duplicates earlier claim %q", idx, claimID)
		}
		expected[claimID] = struct{}{}
	}
	if len(expected) == 0 {
		return fmt.Errorf("final vote ballot has no active claims")
	}
	if len(votes) == 0 {
		return fmt.Errorf("final vote ballot is empty; expected one vote for each of %d active claims", len(expected))
	}

	seen := make(map[string]struct{}, len(votes))
	for idx, vote := range votes {
		claimID := vote.ClaimID
		if strings.TrimSpace(claimID) == "" {
			return fmt.Errorf("votes[%d].claimId is required", idx)
		}
		if _, exists := expected[claimID]; !exists {
			return fmt.Errorf("votes[%d].claimId %q is not on the active ballot", idx, claimID)
		}
		if _, exists := seen[claimID]; exists {
			return fmt.Errorf("votes[%d].claimId duplicates earlier vote for %q", idx, claimID)
		}
		seen[claimID] = struct{}{}
		if !allowedDebateVote(vote.Vote) {
			return fmt.Errorf("votes[%d].vote must be one of accept|reject|abstain", idx)
		}
		if vote.Confidence == nil {
			return fmt.Errorf("votes[%d].confidence is required", idx)
		}
		if math.IsNaN(*vote.Confidence) || math.IsInf(*vote.Confidence, 0) || *vote.Confidence < 0 || *vote.Confidence > 1 {
			return fmt.Errorf("votes[%d].confidence must be a finite number within [0,1]", idx)
		}
		if vote.Vote == DebateVoteAccept && *vote.Confidence < 0.5 {
			return fmt.Errorf("votes[%d]: vote=accept requires confidence >= 0.5 because confidence is the continuous support score, got %.2f", idx, *vote.Confidence)
		}
		if vote.Vote == DebateVoteReject && *vote.Confidence > 0.5 {
			return fmt.Errorf("votes[%d]: vote=reject requires confidence <= 0.5 because confidence is the continuous support score, got %.2f", idx, *vote.Confidence)
		}
	}

	if len(seen) != len(expected) {
		missing := make([]string, 0, len(expected)-len(seen))
		for _, claim := range ballot {
			claimID := claim.ClaimID
			if _, exists := seen[claimID]; !exists {
				missing = append(missing, claimID)
			}
		}
		return fmt.Errorf("final vote ballot is incomplete; missing votes for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func allowedDebateVote(value DebateVoteChoice) bool {
	switch value {
	case DebateVoteAccept, DebateVoteReject, DebateVoteAbstain:
		return true
	default:
		return false
	}
}
