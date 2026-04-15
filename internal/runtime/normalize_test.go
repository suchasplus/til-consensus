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
