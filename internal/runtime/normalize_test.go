package runtime

import (
	"testing"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestNormalizeProposalOutputFromText(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"proposal","claims":[{"title":"A","statement":"claim"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ProposalTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if len(typed.Output.Claims) != 1 || typed.Output.Claims[0].Statement != "claim" {
		t.Fatalf("unexpected proposal output: %#v", typed.Output)
	}
}

func TestNormalizeProposalOutputCoercesStringConfidence(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"proposal","claims":[{"title":"A","statement":"claim","confidence":"0.8"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ProposalTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.Claims[0].Confidence != 0.8 {
		t.Fatalf("unexpected coerced confidence: %#v", typed.Output.Claims[0])
	}
}

func TestNormalizeSemanticVerificationOutput(t *testing.T) {
	result, err := NormalizeTaskOutput(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
	}, map[string]any{
		"summary": "semantic",
		"results": []map[string]any{{
			"claimId":    "claim-1",
			"verdict":    "supported",
			"confidence": 0.7,
			"rationale":  "looks good",
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTaskOutput failed: %v", err)
	}
	typed, ok := result.(consensus.SemanticVerificationTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.Results[0].Verdict != consensus.ClaimVerdictSupported {
		t.Fatalf("unexpected semantic verification output: %#v", typed.Output)
	}
}

func TestNormalizeArbiterOutputRejectsMissingVerdict(t *testing.T) {
	_, err := NormalizeTaskOutput(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-1"},
	}, map[string]any{
		"summary":   "arbiter",
		"decisions": []map[string]any{},
	})
	if err == nil {
		t.Fatal("expected missing task verdict to fail")
	}
}

func TestNormalizeDelphiQuestionnaireOutputCoercesStringRating(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.DelphiQuestionnaireTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"responses","responses":[{"statement":"Use monorepo","rating":"4"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.DelphiQuestionnaireTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.Responses[0].Rating != 4 {
		t.Fatalf("unexpected coerced rating: %#v", typed.Output.Responses[0])
	}
}
