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

func TestNormalizeProposalOutputRejectsConfidenceLabelsAndClaimAlias(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"proposal","claims":[{"id":"c1","claim":"prefer polyrepo by default","confidence":"medium","reasoning":"better default for service autonomy"}]}`)
	if err == nil {
		t.Fatal("expected non-canonical proposal output to fail normalization")
	}
}

func TestNormalizeProposalOutputEnforcesSchemaAfterSyntaxRecovery(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, "```json\n{\"summary\":\"proposal\",\"claims\":[{\"claim\":\"alias field\",\"confidence\":\"0.8\"}]}\n```")
	if err == nil {
		t.Fatal("expected syntax recovery to succeed but schema enforcement to reject alias field")
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

func TestNormalizeSemanticVerificationOutputRejectsAliasesAndVerdictLabels(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verdict":"rejected","confidence":"high","reasoning":"too strong"}]}`)
	if err == nil {
		t.Fatal("expected non-canonical semantic verification output to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsVerificationStatusAlias(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verificationStatus":"verified","reasoning":"looks verified"}]}`)
	if err == nil {
		t.Fatal("expected verificationStatus alias to fail normalization")
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

func TestNormalizeArbiterOutputRejectsStructuredTaskVerdictAndDecisionAliases(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-1"},
	}, `{"summary":"arbiter","taskVerdict":{"verdict":"undetermined","rationale":"insufficient context"},"decisions":[{"targetClaimId":"claim-1","verdict":"rejected","confidence":"high","reasoning":"too strong","keyEvidenceRefs":["ledger-1"]}]}`)
	if err == nil {
		t.Fatal("expected non-canonical arbiter output to fail normalization")
	}
}

func TestNormalizeArbiterOutputRejectsVerifiedVerdictAndAcceptedDisposition(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-1"},
	}, `{"summary":"arbiter","taskVerdict":"undetermined","decisions":[{"claimId":"claim-1","verdict":"verified","disposition":"accepted","rationale":"looks correct"}]}`)
	if err == nil {
		t.Fatal("expected semantic/disposition aliases to fail normalization")
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

func TestNormalizeReviseOutputRejectsClaimAliasAndRationale(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"revise","revisions":[{"claimId":"claim-1","action":"narrow","revisedStatement":"narrowed","rationale":"needs stronger context","verdict":"undetermined"}]}`)
	if err == nil {
		t.Fatal("expected revise aliases to fail normalization")
	}
}

func TestParseFlexibleFloatSupportsPercentOnly(t *testing.T) {
	tests := []struct {
		raw  string
		want float64
	}{
		{raw: "80%", want: 0.8},
	}
	for _, tc := range tests {
		got, ok := parseFlexibleFloat(tc.raw)
		if !ok {
			t.Fatalf("parseFlexibleFloat(%q) returned !ok", tc.raw)
		}
		if got != tc.want {
			t.Fatalf("parseFlexibleFloat(%q)=%v want %v", tc.raw, got, tc.want)
		}
	}
	if _, ok := parseFlexibleFloat("high"); ok {
		t.Fatal("expected confidence label coercion to be disabled")
	}
}

func TestStrictDecodeTaskOutputRejectsWrappedJSONAndStringNumbers(t *testing.T) {
	_, err := StrictDecodeTaskOutput(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, "```json\n{\"summary\":\"proposal\",\"claims\":[{\"statement\":\"claim\",\"confidence\":\"0.8\"}]}\n```")
	if err == nil {
		t.Fatal("expected strict decode to reject wrapped JSON and string numbers")
	}
}
