package consensus

import "testing"

type testIDs struct{ n int }

func (i *testIDs) NewSessionID() string { return "session-test" }
func (i *testIDs) NewEntityID(prefix string) string {
	i.n++
	return prefix + "-id-" + string(rune('0'+i.n))
}

func TestUpsertClaimDedupeByNormalizedStatement(t *testing.T) {
	ids := &testIDs{}
	claims := []ClaimNode{}
	var created bool

	claims, _, created = UpsertClaim(claims, ClaimDraft{Title: "A", Statement: " Patch fixes race condition "}, "p1", "ledger-1", ids)
	if !created || len(claims) != 1 {
		t.Fatalf("expected first claim to be created")
	}

	claims, claim, created := UpsertClaim(claims, ClaimDraft{Title: "B", Statement: "patch   fixes race condition"}, "p2", "ledger-2", ids)
	if created {
		t.Fatalf("expected duplicate statement to be merged")
	}
	if len(claims) != 1 {
		t.Fatalf("expected one merged claim, got %d", len(claims))
	}
	if len(claim.ProposedBy) != 2 {
		t.Fatalf("expected merged claim to retain both proposers, got %#v", claim.ProposedBy)
	}
	if len(claim.EvidenceRefs) != 2 {
		t.Fatalf("expected merged claim to retain both evidence refs, got %#v", claim.EvidenceRefs)
	}
}

func TestUpsertChallengeDedupesSameClaimAndStatement(t *testing.T) {
	ids := &testIDs{}
	tickets := []ChallengeTicket{}
	var created bool

	tickets, _, created = UpsertChallenge(tickets, ChallengeDraft{
		Statement: "Need more evidence",
		Kind:      "evidence-gap",
	}, "claim-1", "challenger-1", "ledger-1", ids)
	if !created || len(tickets) != 1 {
		t.Fatalf("expected first challenge to be created")
	}
	_, ticket, created := UpsertChallenge(tickets, ChallengeDraft{
		Statement: " need   more evidence ",
		Kind:      "evidence-gap",
	}, "claim-1", "challenger-2", "ledger-2", ids)
	if created {
		t.Fatalf("expected duplicate challenge to be merged")
	}
	if len(ticket.EvidenceRefs) != 2 {
		t.Fatalf("expected merged evidence refs, got %#v", ticket.EvidenceRefs)
	}
}

func TestCloseChallengesOnlyClosesResolvedRequestedChecks(t *testing.T) {
	tickets := []ChallengeTicket{
		{
			TicketID:         "ticket-1",
			ClaimID:          "claim-1",
			Status:           ChallengeStatusOpen,
			RequestedChecks:  []string{"workspace"},
			VerificationRefs: nil,
		},
		{
			TicketID:         "ticket-2",
			ClaimID:          "claim-1",
			Status:           ChallengeStatusOpen,
			RequestedChecks:  []string{"semantic"},
			VerificationRefs: nil,
		},
	}

	findings := []VerificationResult{
		{
			ClaimID:     "claim-1",
			CheckName:   "workspace",
			Kind:        "workspace_snapshot",
			Status:      VerificationStatusPassed,
			EvidenceRef: "ledger-1",
		},
		{
			ClaimID:     "claim-1",
			CheckName:   "semantic",
			Kind:        "semantic",
			Status:      VerificationStatusInconclusive,
			EvidenceRef: "ledger-2",
		},
	}

	closed := CloseChallenges(tickets, "claim-1", findings, "resolved")
	if closed[0].Status != ChallengeStatusClosed {
		t.Fatalf("expected first ticket to close, got %#v", closed[0])
	}
	if closed[1].Status != ChallengeStatusOpen {
		t.Fatalf("expected second ticket to remain open, got %#v", closed[1])
	}
}
