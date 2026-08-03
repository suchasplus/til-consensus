package consensus

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateDelphiResponsesRequiresUniqueInRangeRatings(t *testing.T) {
	valid := []DelphiResponseDraft{
		{StatementID: "statement-1", Rating: ptr(1.0)},
		{StatementID: "statement-2", Rating: ptr(5.0)},
	}
	if err := ValidateDelphiResponses(valid, 1, 5); err != nil {
		t.Fatalf("expected boundary ratings to pass: %v", err)
	}
	for _, test := range []struct {
		name      string
		responses []DelphiResponseDraft
	}{
		{name: "empty", responses: nil},
		{name: "missing rating", responses: []DelphiResponseDraft{{StatementID: "statement-1"}}},
		{name: "below range", responses: []DelphiResponseDraft{{StatementID: "statement-1", Rating: ptr(0.0)}}},
		{name: "above range", responses: []DelphiResponseDraft{{StatementID: "statement-1", Rating: ptr(6.0)}}},
		{name: "duplicate", responses: []DelphiResponseDraft{
			{StatementID: "statement-1", Rating: ptr(4.0)},
			{StatementID: "statement-1", Rating: ptr(5.0)},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateDelphiResponses(test.responses, 1, 5); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestDelphiSampleGuardsRejectSingletonAndUndersampledStatements(t *testing.T) {
	if got := consensusFromRatings([]float64{5}, 1, 5); got != 0 {
		t.Fatalf("singleton must not report measurable consensus, got %.2f", got)
	}
	if delphiMinResponsesMet([]DelphiStatement{{StatementID: "s1", ResponseCount: 2}, {StatementID: "s2", ResponseCount: 1}}, 2) {
		t.Fatal("every active statement must meet the minimum response count")
	}
	if !delphiMinResponsesMet([]DelphiStatement{{StatementID: "s1", ResponseCount: 2}, {StatementID: "s2", ResponseCount: 3}}, 2) {
		t.Fatal("expected all statements to satisfy minimum response count")
	}
}

func TestDelphiResponseRecordDoesNotSerializeParticipantIdentity(t *testing.T) {
	body, err := json.Marshal(DelphiResponseRecord{agentID: "participant-secret", StatementID: "s1", Rating: 4})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(body), "participant-secret") || strings.Contains(string(body), "agentId") {
		t.Fatalf("anonymous Delphi response leaked participant identity: %s", body)
	}
}
