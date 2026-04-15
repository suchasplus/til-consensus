package consensus

import "testing"

func TestUpdateClaimsAssignsDeterministicIDs(t *testing.T) {
	outputs := []ParticipantRoundOutput{
		{
			ParticipantID: "a",
			Round:         0,
			ExtractedClaims: []ExtractedClaim{
				{Title: "A1", Statement: "A1"},
				{Title: "A2", Statement: "A2"},
			},
		},
		{
			ParticipantID: "b",
			Round:         0,
			ExtractedClaims: []ExtractedClaim{
				{Title: "B1", Statement: "B1"},
			},
		},
	}
	claims, newCount, _ := UpdateClaims(nil, outputs)
	if newCount != 3 {
		t.Fatalf("expected 3 new claims, got %d", newCount)
	}
	if claims[0].ClaimID != "a:0:0" || claims[1].ClaimID != "a:0:1" || claims[2].ClaimID != "b:0:0" {
		t.Fatalf("unexpected claim ids: %#v", claims)
	}
}

func TestUpdateClaimsSeedsWhenNoClaimsPresent(t *testing.T) {
	outputs := []ParticipantRoundOutput{
		{ParticipantID: "a", Round: 0, Summary: "seed a"},
		{ParticipantID: "b", Round: 0, Summary: "seed b"},
	}
	claims, newCount, _ := UpdateClaims(nil, outputs)
	if newCount != 2 {
		t.Fatalf("expected 2 seeded claims, got %d", newCount)
	}
	if claims[0].ClaimID != "seed:a:0" || claims[1].ClaimID != "seed:b:0" {
		t.Fatalf("unexpected seeded claim ids: %#v", claims)
	}
}

func TestUpdateClaimsRevisesAndMerges(t *testing.T) {
	base := []Claim{
		{ClaimID: "c1", Title: "C1", Statement: "first", ProposedBy: []string{"a"}, Status: ClaimStatusActive},
		{ClaimID: "c2", Title: "C2", Statement: "second", ProposedBy: []string{"b"}, Status: ClaimStatusActive},
	}
	outputs := []ParticipantRoundOutput{
		{
			ParticipantID: "a",
			Round:         1,
			Judgements: []ClaimJudgement{
				{
					ClaimID:          "c1",
					Stance:           ClaimStanceRevise,
					RevisedStatement: "first revised",
					MergesWith:       "c2",
				},
			},
		},
	}
	claims, _, merges := UpdateClaims(base, outputs)
	if len(merges) != 1 {
		t.Fatalf("expected 1 merge event, got %#v", merges)
	}
	if claims[0].Statement != "first revised" {
		t.Fatalf("expected revised statement, got %#v", claims[0])
	}
	if claims[1].Status != ClaimStatusMerged || claims[1].MergedInto != "c1" {
		t.Fatalf("expected c2 merged into c1, got %#v", claims[1])
	}
}
