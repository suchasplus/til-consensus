package consensus

import "testing"

func TestChooseRepresentativeTieBreakers(t *testing.T) {
	scores := []ParticipantScore{
		{ParticipantID: "a", Total: 80, ByRound: []ParticipantRoundScore{{Round: 0, Score: 70}, {Round: 1, Score: 80}}},
		{ParticipantID: "b", Total: 80, ByRound: []ParticipantRoundScore{{Round: 0, Score: 90}, {Round: 1, Score: 70}}},
	}
	rounds := []RoundRecord{
		{
			Round: 1,
			Outputs: []ParticipantRoundOutput{
				{ParticipantID: "a", Judgements: []ClaimJudgement{{ClaimID: "c1", Stance: ClaimStanceAgree}}},
				{ParticipantID: "b", Judgements: []ClaimJudgement{{ClaimID: "c1", Stance: ClaimStanceDisagree}}},
			},
		},
	}
	rep, err := ChooseRepresentative(append([]ParticipantScore(nil), scores...), rounds, TieBreakerLatestRoundScore)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ParticipantID != "a" {
		t.Fatalf("expected a, got %s", rep.ParticipantID)
	}
	rep, err = ChooseRepresentative(append([]ParticipantScore(nil), scores...), rounds, TieBreakerLeastObjection)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ParticipantID != "a" {
		t.Fatalf("expected a, got %s", rep.ParticipantID)
	}
}
