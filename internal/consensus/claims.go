package consensus

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

func UpsertClaim(claims []ClaimNode, draft ClaimDraft, proposedBy string, evidenceRef string, idFactory IDFactory) ([]ClaimNode, ClaimNode, bool) {
	key := normalizedClaimKey(draft.Statement)
	for idx := range claims {
		if normalizedClaimKey(claims[idx].Statement) != key {
			continue
		}
		claims[idx].ProposedBy = appendUnique(claims[idx].ProposedBy, proposedBy)
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, evidenceRef)
		claims[idx].Dependencies = appendUnique(claims[idx].Dependencies, draft.Dependencies...)
		if claims[idx].Title == "" {
			claims[idx].Title = strings.TrimSpace(draft.Title)
		}
		if claims[idx].Scope == "" {
			claims[idx].Scope = strings.TrimSpace(draft.Scope)
		}
		if claims[idx].Applicability == "" {
			claims[idx].Applicability = strings.TrimSpace(draft.Applicability)
		}
		if claims[idx].Metadata == nil && draft.Metadata != nil {
			claims[idx].Metadata = maps.Clone(draft.Metadata)
		}
		return claims, claims[idx], false
	}
	claimID := idFactory.NewEntityID("claim")
	claim := ClaimNode{
		ClaimID:       claimID,
		Title:         strings.TrimSpace(draft.Title),
		Statement:     strings.TrimSpace(draft.Statement),
		Scope:         strings.TrimSpace(draft.Scope),
		Dependencies:  dedupeStrings(draft.Dependencies),
		Applicability: strings.TrimSpace(draft.Applicability),
		Status:        ClaimStatusProposed,
		ProposedBy:    []string{proposedBy},
		EvidenceRefs:  filterEmpty([]string{evidenceRef}),
		Metadata:      maps.Clone(draft.Metadata),
	}
	return append(claims, claim), claim, true
}

func ResolveClaimRef(claims []ClaimNode, claimID string, statement string) (ClaimNode, bool) {
	if strings.TrimSpace(claimID) != "" {
		for _, claim := range claims {
			if claim.ClaimID == strings.TrimSpace(claimID) {
				return claim, true
			}
		}
	}
	key := normalizedClaimKey(statement)
	for _, claim := range claims {
		if normalizedClaimKey(claim.Statement) == key {
			return claim, true
		}
	}
	return ClaimNode{}, false
}

func UpsertChallenge(tickets []ChallengeTicket, draft ChallengeDraft, claimID string, openedBy string, evidenceRef string, idFactory IDFactory) ([]ChallengeTicket, ChallengeTicket, bool) {
	key := claimID + "::" + normalizedClaimKey(draft.Statement) + "::" + strings.ToLower(strings.TrimSpace(draft.Kind))
	for idx := range tickets {
		existing := tickets[idx]
		if existing.ClaimID+"::"+normalizedClaimKey(existing.Statement)+"::"+strings.ToLower(existing.Kind) != key {
			continue
		}
		tickets[idx].EvidenceRefs = appendUnique(tickets[idx].EvidenceRefs, evidenceRef)
		tickets[idx].RequestedChecks = appendUnique(tickets[idx].RequestedChecks, draft.RequestedChecks...)
		return tickets, tickets[idx], false
	}
	ticket := ChallengeTicket{
		TicketID:        idFactory.NewEntityID("challenge"),
		ClaimID:         claimID,
		Kind:            strings.TrimSpace(draft.Kind),
		OpenedBy:        openedBy,
		Statement:       strings.TrimSpace(draft.Statement),
		Status:          ChallengeStatusOpen,
		EvidenceRefs:    filterEmpty([]string{evidenceRef}),
		RequestedChecks: dedupeStrings(draft.RequestedChecks),
	}
	return append(tickets, ticket), ticket, true
}

func AttachEvidenceToClaim(claims []ClaimNode, claimID string, evidenceRef string) []ClaimNode {
	for idx := range claims {
		if claims[idx].ClaimID != claimID {
			continue
		}
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, evidenceRef)
		return claims
	}
	return claims
}

func AttachVerificationToClaim(claims []ClaimNode, claimID string, verificationRef string) []ClaimNode {
	for idx := range claims {
		if claims[idx].ClaimID != claimID {
			continue
		}
		claims[idx].VerificationRefs = appendUnique(claims[idx].VerificationRefs, verificationRef)
		claims[idx].Status = ClaimStatusVerified
		return claims
	}
	return claims
}

func AttachChallengeToClaim(claims []ClaimNode, claimID string, challengeRef string) []ClaimNode {
	for idx := range claims {
		if claims[idx].ClaimID != claimID {
			continue
		}
		claims[idx].ChallengeRefs = appendUnique(claims[idx].ChallengeRefs, challengeRef)
		claims[idx].Status = ClaimStatusChallenged
		return claims
	}
	return claims
}

func ApplyDecisions(claims []ClaimNode, decisions []ArbiterDecision) []ClaimNode {
	index := make(map[string]ArbiterDecision, len(decisions))
	for _, decision := range decisions {
		index[decision.ClaimID] = decision
	}
	for idx := range claims {
		decision, ok := index[claims[idx].ClaimID]
		if !ok {
			continue
		}
		claims[idx].Status = ClaimStatusAdjudicated
		claims[idx].Verdict = decision.Verdict
		claims[idx].Confidence = decision.Confidence
		claims[idx].Rationale = decision.Rationale
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, decision.EvidenceRefs...)
	}
	return claims
}

func CloseChallenges(tickets []ChallengeTicket, claimID string, verificationRefs []string, resolution string) []ChallengeTicket {
	for idx := range tickets {
		if tickets[idx].ClaimID != claimID {
			continue
		}
		tickets[idx].Status = ChallengeStatusClosed
		tickets[idx].VerificationRefs = appendUnique(tickets[idx].VerificationRefs, verificationRefs...)
		if tickets[idx].ResolutionSummary == "" {
			tickets[idx].ResolutionSummary = strings.TrimSpace(resolution)
		}
	}
	return tickets
}

func CountVerdicts(claims []ClaimNode) map[ClaimVerdict]int {
	out := map[ClaimVerdict]int{}
	for _, claim := range claims {
		out[claim.Verdict]++
	}
	return out
}

func DetermineTaskVerdict(claims []ClaimNode) TaskVerdict {
	if len(claims) == 0 {
		return TaskVerdictFailed
	}
	counts := CountVerdicts(claims)
	switch {
	case counts[ClaimVerdictSupported] == len(claims):
		return TaskVerdictSupported
	case counts[ClaimVerdictSupported] > 0:
		return TaskVerdictPartiallySupported
	case counts[ClaimVerdictRefuted] == len(claims):
		return TaskVerdictFailed
	default:
		return TaskVerdictUndetermined
	}
}

func normalizedClaimKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	return strings.Join(strings.Fields(trimmed), " ")
}

func appendUnique(base []string, values ...string) []string {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		base = append(base, trimmed)
	}
	return base
}

func filterEmpty(values []string) []string {
	return dedupeStrings(values)
}

func SortClaims(claims []ClaimNode) {
	slices.SortFunc(claims, func(left, right ClaimNode) int {
		switch {
		case left.ClaimID < right.ClaimID:
			return -1
		case left.ClaimID > right.ClaimID:
			return 1
		default:
			return 0
		}
	})
}

func SortChallenges(tickets []ChallengeTicket) {
	slices.SortFunc(tickets, func(left, right ChallengeTicket) int {
		switch {
		case left.TicketID < right.TicketID:
			return -1
		case left.TicketID > right.TicketID:
			return 1
		default:
			return 0
		}
	})
}

func ValidateClaimDependencies(claims []ClaimNode) error {
	index := map[string]struct{}{}
	for _, claim := range claims {
		index[claim.ClaimID] = struct{}{}
	}
	for _, claim := range claims {
		for _, dep := range claim.Dependencies {
			if _, ok := index[dep]; !ok {
				return fmt.Errorf("claim %s references unknown dependency %s", claim.ClaimID, dep)
			}
		}
	}
	return nil
}
