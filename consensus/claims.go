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
		if claims[idx].ClaimType == "" {
			claims[idx].ClaimType = draft.ClaimType
		}
		claims[idx].ParentClaimIDs = appendUnique(claims[idx].ParentClaimIDs, draft.ParentClaimIDs...)
		claims[idx].BoundaryConditions = appendUnique(claims[idx].BoundaryConditions, draft.BoundaryConditions...)
		if claims[idx].Applicability == "" {
			claims[idx].Applicability = strings.TrimSpace(draft.Applicability)
		}
		if draft.Confidence > claims[idx].Confidence {
			claims[idx].Confidence = draft.Confidence
		}
		if claims[idx].Metadata == nil && draft.Metadata != nil {
			claims[idx].Metadata = maps.Clone(draft.Metadata)
		}
		return claims, claims[idx], false
	}
	claimID := idFactory.NewEntityID("claim")
	claim := ClaimNode{
		ClaimID:               claimID,
		Title:                 strings.TrimSpace(draft.Title),
		Statement:             strings.TrimSpace(draft.Statement),
		ClaimText:             strings.TrimSpace(draft.Statement),
		ClaimType:             draft.ClaimType,
		Scope:                 strings.TrimSpace(draft.Scope),
		Dependencies:          dedupeStrings(draft.Dependencies),
		ParentClaimIDs:        dedupeStrings(draft.ParentClaimIDs),
		BoundaryConditions:    dedupeStrings(draft.BoundaryConditions),
		Applicability:         strings.TrimSpace(draft.Applicability),
		Status:                ClaimStatusProposed,
		ProposedBy:            []string{proposedBy},
		SourceProposalAgentID: proposedBy,
		EvidenceRefs:          filterEmpty([]string{evidenceRef}),
		SupportingEvidenceIDs: filterEmpty([]string{evidenceRef}),
		Confidence:            draft.Confidence,
		Metadata:              maps.Clone(draft.Metadata),
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
		TicketID:                     idFactory.NewEntityID("challenge"),
		ClaimID:                      claimID,
		Kind:                         strings.TrimSpace(draft.Kind),
		AttackType:                   firstNonEmpty(strings.TrimSpace(draft.AttackType), strings.TrimSpace(draft.Kind)),
		Severity:                     firstNonEmptySeverity(draft.Severity),
		OpenedBy:                     openedBy,
		Statement:                    strings.TrimSpace(draft.Statement),
		AttackText:                   strings.TrimSpace(draft.Statement),
		Status:                       ChallengeStatusOpen,
		EvidenceRefs:                 filterEmpty([]string{evidenceRef}),
		RequestedChecks:              dedupeStrings(draft.RequestedChecks),
		SuggestedFalsificationMethod: strings.TrimSpace(draft.SuggestedFalsificationMethod),
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

func AttachVerificationOutcome(claims []ClaimNode, claimID string, finding VerificationResult) []ClaimNode {
	for idx := range claims {
		if claims[idx].ClaimID != claimID {
			continue
		}
		claims[idx].VerificationRefs = appendUnique(claims[idx].VerificationRefs, finding.EvidenceRef)
		if finding.EvidenceRef != "" {
			claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, finding.EvidenceRef)
		}
		switch finding.Status {
		case VerificationStatusPassed:
			claims[idx].SupportingEvidenceIDs = appendUnique(claims[idx].SupportingEvidenceIDs, finding.EvidenceRef)
		case VerificationStatusFailed:
			claims[idx].OpposingEvidenceIDs = appendUnique(claims[idx].OpposingEvidenceIDs, finding.EvidenceRef)
		}
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

func ApplyAdjudicationRecords(claims []ClaimNode, records []AdjudicationRecord) []ClaimNode {
	index := make(map[string]AdjudicationRecord, len(records))
	for _, record := range records {
		index[record.TargetClaimID] = record
	}
	for idx := range claims {
		record, ok := index[claims[idx].ClaimID]
		if !ok {
			continue
		}
		claims[idx].Status = ClaimStatusAdjudicated
		claims[idx].Disposition = record.Disposition
		claims[idx].Confidence = record.FinalConfidence
		claims[idx].Rationale = record.Rationale
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, record.EvidenceRefs...)
		switch record.Disposition {
		case ClaimDispositionKeep:
			claims[idx].Verdict = ClaimVerdictSupported
		case ClaimDispositionKeepWithCaveat:
			claims[idx].Verdict = ClaimVerdictSupported
		case ClaimDispositionReject:
			claims[idx].Verdict = ClaimVerdictRefuted
		default:
			claims[idx].Verdict = ClaimVerdictUndetermined
		}
	}
	return claims
}

func ApplyRevisionRecords(claims []ClaimNode, records []ClaimRevisionRecord, confidenceEpsilon float64) ([]ClaimNode, []string, bool) {
	changedClaims := make([]string, 0, len(records))
	materialChange := false
	index := make(map[string]int, len(claims))
	for idx, claim := range claims {
		index[claim.ClaimID] = idx
	}
	for _, record := range records {
		idx, ok := index[record.TargetClaimID]
		if !ok {
			continue
		}
		changedClaims = appendUnique(changedClaims, record.TargetClaimID)
		claim := claims[idx]
		claim.Status = ClaimStatusRevised
		claim.LastRevisionRound = record.Round
		claim.Caveats = appendUnique(claim.Caveats, record.Caveats...)
		claim.BoundaryConditions = appendUnique(claim.BoundaryConditions, record.BoundaryConditions...)
		if text := strings.TrimSpace(record.RevisedText); text != "" && text != claim.Statement {
			claim.Statement = text
			claim.ClaimText = text
			materialChange = true
		}
		if delta := record.ConfidenceDelta; delta != 0 {
			claim.Confidence += delta
			if claim.Confidence < 0 {
				claim.Confidence = 0
			}
			if claim.Confidence > 1 {
				claim.Confidence = 1
			}
			if delta >= confidenceEpsilon || delta <= -confidenceEpsilon {
				materialChange = true
			}
		}
		if reason := strings.TrimSpace(record.Reason); reason != "" {
			claim.Rationale = reason
		}
		switch record.Action {
		case RevisionActionWithdraw:
			claim.Status = ClaimStatusWithdrawn
			claim.Verdict = ClaimVerdictRefuted
			claim.Disposition = ClaimDispositionReject
			materialChange = true
		case RevisionActionUnresolved:
			claim.Verdict = ClaimVerdictUndetermined
			claim.Disposition = ClaimDispositionUnresolved
		case RevisionActionDowngrade:
			if claim.Verdict == ClaimVerdictSupported {
				claim.Verdict = ClaimVerdictInsufficientEvidence
			}
		}
		claims[idx] = claim
	}
	return claims, changedClaims, materialChange
}

func CloseChallenges(tickets []ChallengeTicket, claimID string, findings []VerificationResult, resolution string) []ChallengeTicket {
	resolvedChecks := map[string]struct{}{}
	verificationRefs := make([]string, 0, len(findings))
	hasConclusiveFinding := false
	for _, finding := range findings {
		if finding.ClaimID != claimID {
			continue
		}
		if finding.EvidenceRef != "" {
			verificationRefs = appendUnique(verificationRefs, finding.EvidenceRef)
		}
		if finding.Status == VerificationStatusInconclusive {
			continue
		}
		hasConclusiveFinding = true
		if name := strings.TrimSpace(finding.CheckName); name != "" {
			resolvedChecks[name] = struct{}{}
		}
		if kind := strings.TrimSpace(finding.Kind); kind != "" {
			resolvedChecks[kind] = struct{}{}
		}
	}
	for idx := range tickets {
		if tickets[idx].ClaimID != claimID {
			continue
		}
		if !hasConclusiveFinding {
			continue
		}
		if !challengeChecksResolved(tickets[idx], resolvedChecks) {
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

func challengeChecksResolved(ticket ChallengeTicket, resolvedChecks map[string]struct{}) bool {
	if len(ticket.RequestedChecks) == 0 {
		return true
	}
	for _, requested := range dedupeStrings(ticket.RequestedChecks) {
		if _, ok := resolvedChecks[requested]; !ok {
			return false
		}
	}
	return true
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

func DetermineTerminalState(claims []ClaimNode, tickets []ChallengeTicket, manifest CaseManifest, action *ActionOutput) WorkflowTerminalState {
	if action != nil && action.Status == string(TerminalStateActionBlockedByRisk) {
		return TerminalStateActionBlockedByRisk
	}
	openTickets := 0
	for _, ticket := range tickets {
		if ticket.Status == ChallengeStatusOpen {
			openTickets++
		}
	}
	counts := CountVerdicts(claims)
	switch {
	case counts[ClaimVerdictSupported] == 0 && counts[ClaimVerdictUndetermined] > 0 && manifest.RequiredEvidenceLevel == EvidenceLevelHigh:
		return TerminalStateInsufficientEvidence
	case openTickets > 0 && manifest.RiskLevel == RiskLevelHigh:
		return TerminalStateRequiresHumanReview
	case openTickets > 0:
		return TerminalStateUnresolvedConflict
	default:
		return TerminalStateCompleted
	}
}

func firstNonEmptySeverity(value AttackSeverity) AttackSeverity {
	if value == "" {
		return AttackSeverityMedium
	}
	return value
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
