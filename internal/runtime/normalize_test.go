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

func TestNormalizeProposalOutputAcceptsConfidenceLabelsAndClaimAlias(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"proposal","claims":[{"id":"c1","claim":"prefer polyrepo by default","confidence":"medium","reasoning":"better default for service autonomy"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ProposalTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if len(typed.Output.Claims) != 1 {
		t.Fatalf("unexpected claim count: %#v", typed.Output.Claims)
	}
	if typed.Output.Claims[0].Statement != "prefer polyrepo by default" {
		t.Fatalf("expected claim alias to map to statement: %#v", typed.Output.Claims[0])
	}
	if typed.Output.Claims[0].Confidence != 0.6 {
		t.Fatalf("unexpected confidence label coercion: %#v", typed.Output.Claims[0])
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

func TestNormalizeSemanticVerificationOutputAcceptsTargetAliasesAndVerdictLabels(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verdict":"rejected","confidence":"high","reasoning":"too strong"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.SemanticVerificationTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.Results[0].ClaimID != "claim-1" || typed.Output.Results[0].Verdict != consensus.ClaimVerdictRefuted || typed.Output.Results[0].Confidence != 0.82 || typed.Output.Results[0].Rationale != "too strong" {
		t.Fatalf("unexpected semantic verification normalization: %#v", typed.Output.Results[0])
	}
	if typed.Output.Results[0].TargetType != "claim" || typed.Output.Results[0].Metadata["rawVerdict"] != "rejected" {
		t.Fatalf("expected semantic metadata to preserve raw verdict: %#v", typed.Output.Results[0])
	}
}

func TestNormalizeSemanticVerificationOutputAcceptsVerificationStatusAlias(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verificationStatus":"verified","reasoning":"looks verified"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.SemanticVerificationTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.Results[0].ClaimID != "claim-1" || typed.Output.Results[0].Verdict != consensus.ClaimVerdictSupported || typed.Output.Results[0].Rationale != "looks verified" {
		t.Fatalf("unexpected semantic verification alias normalization: %#v", typed.Output.Results[0])
	}
	if typed.Output.Results[0].Metadata["rawVerdict"] != "verified" {
		t.Fatalf("expected semantic metadata to preserve verificationStatus alias: %#v", typed.Output.Results[0].Metadata)
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

func TestNormalizeArbiterOutputAcceptsStructuredTaskVerdictAndDecisionAliases(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-1"},
	}, `{"summary":"arbiter","taskVerdict":{"verdict":"undetermined","rationale":"insufficient context"},"decisions":[{"targetClaimId":"claim-1","verdict":"rejected","confidence":"high","reasoning":"too strong","keyEvidenceRefs":["ledger-1"]}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ArbiterTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if typed.Output.TaskVerdict != consensus.TaskVerdictUndetermined {
		t.Fatalf("unexpected task verdict: %#v", typed.Output)
	}
	if len(typed.Output.Decisions) != 1 {
		t.Fatalf("unexpected decisions: %#v", typed.Output.Decisions)
	}
	decision := typed.Output.Decisions[0]
	if decision.ClaimID != "claim-1" || decision.Verdict != consensus.ClaimVerdictRefuted || decision.Confidence != 0.82 || decision.Rationale != "too strong" || len(decision.EvidenceRefs) != 1 || decision.EvidenceRefs[0] != "ledger-1" {
		t.Fatalf("unexpected arbiter decision normalization: %#v", decision)
	}
	if typed.Output.Metadata["rawTaskVerdict"] == nil || decision.Metadata["rawVerdict"] != "rejected" {
		t.Fatalf("expected arbiter metadata to preserve raw verdicts: output=%#v decision=%#v", typed.Output.Metadata, decision.Metadata)
	}
}

func TestNormalizeArbiterOutputAcceptsVerifiedVerdictAndAcceptedDisposition(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-1"},
	}, `{"summary":"arbiter","taskVerdict":"undetermined","decisions":[{"claimId":"claim-1","verdict":"verified","disposition":"accepted","rationale":"looks correct"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ArbiterTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	decision := typed.Output.Decisions[0]
	if decision.Verdict != consensus.ClaimVerdictSupported {
		t.Fatalf("expected verified to normalize to supported, got %#v", decision)
	}
	if decision.Metadata["rawVerdict"] != "verified" || decision.Metadata["rawDisposition"] != "accepted" {
		t.Fatalf("expected raw verdict/disposition metadata, got %#v", decision.Metadata)
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

func TestNormalizeReviseOutputAcceptsClaimAliasAndRationale(t *testing.T) {
	result, err := NormalizeTaskOutputFromText(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"revise","revisions":[{"claimId":"claim-1","action":"narrow","revisedStatement":"narrowed","rationale":"needs stronger context","verdict":"undetermined"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.ReviseTaskResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	revision := typed.Output.Revisions[0]
	if revision.TargetClaimID != "claim-1" || revision.Reason != "needs stronger context" || !revision.Unresolved {
		t.Fatalf("unexpected revise normalization: %#v", revision)
	}
	if revision.Metadata["rawVerdict"] != "undetermined" {
		t.Fatalf("expected revision metadata to preserve raw verdict: %#v", revision.Metadata)
	}
}

func TestParseFlexibleFloatSupportsPercentAndLabels(t *testing.T) {
	tests := []struct {
		raw  string
		want float64
	}{
		{raw: "80%", want: 0.8},
		{raw: "high", want: 0.82},
		{raw: "very_high", want: 0.93},
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
}
