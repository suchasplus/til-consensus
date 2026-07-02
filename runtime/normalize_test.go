package runtime

import (
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
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

func TestNormalizeProposalOutputRejectsRelationshipFields(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
	}, `{"summary":"proposal","claims":[{"statement":"prefer monorepo","dependencies":["engineering-constraints"]}]}`)
	if err == nil {
		t.Fatal("expected proposal relationship fields to fail normalization")
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
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, map[string]any{
		"summary": "semantic",
		"results": []map[string]any{{
			"claimId":    "claim-1",
			"verdict":    "supported",
			"confidence": 0.7,
			"rationale":  "supported_core: The core claim is directly backed. | missing_or_conflict: none beyond already stated caveats. | verdict_reason: supported is the strongest fit because the claim survives as written.",
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
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verdict":"rejected","confidence":"high","reasoning":"too strong"}]}`)
	if err == nil {
		t.Fatal("expected non-canonical semantic verification output to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsVerificationStatusAlias(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"targetId":"claim-1","targetType":"claim","verificationStatus":"verified","reasoning":"looks verified"}]}`)
	if err == nil {
		t.Fatal("expected verificationStatus alias to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsMismatchedClaimID(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-2","verdict":"supported","confidence":"0.7","rationale":"supported_core: wrong target. | missing_or_conflict: none. | verdict_reason: supported."}]}`)
	if err == nil {
		t.Fatal("expected mismatched semantic claimId to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsMultipleRows(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"supported","confidence":"0.7","rationale":"supported_core: primary core. | missing_or_conflict: none. | verdict_reason: supported fits best."},{"claimId":"claim-1","verdict":"insufficient_evidence","confidence":"0.3","rationale":"supported_core: extra row core. | missing_or_conflict: missing throughput numbers. | verdict_reason: insufficient_evidence fits best."}]}`)
	if err == nil {
		t.Fatal("expected multiple semantic rows to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsNonClaimTargetType(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","targetType":"challenge","verdict":"supported","confidence":"0.7","rationale":"supported_core: wrong target type. | missing_or_conflict: none. | verdict_reason: supported."}]}`)
	if err == nil {
		t.Fatal("expected non-claim semantic targetType to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsLowConfidenceSupported(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"supported","confidence":"0.41","rationale":"supported_core: The claim core is backed. | missing_or_conflict: none beyond caveats. | verdict_reason: supported is still intended."}]}`)
	if err == nil {
		t.Fatal("expected low-confidence supported verdict to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsHighConfidenceInsufficientEvidence(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"insufficient_evidence","confidence":"0.82","rationale":"supported_core: Repository friction is plausible. | missing_or_conflict: Missing quantified migration benefit. | verdict_reason: insufficient_evidence is intended because the direction is incomplete."}]}`)
	if err == nil {
		t.Fatal("expected high-confidence insufficient_evidence verdict to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsOutOfBandUndeterminedConfidence(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"undetermined","confidence":"0.9","rationale":"supported_core: The record supports some coordination pain. | missing_or_conflict: Other evidence shows unresolved governance tradeoffs. | verdict_reason: the evidence is mixed, so undetermined is intended."}]}`)
	if err == nil {
		t.Fatal("expected out-of-band undetermined confidence to fail normalization")
	}
}

func TestNormalizeSemanticVerificationOutputRejectsRationaleWithoutCanonicalSections(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"supported","confidence":"0.7","rationale":"Need more evidence."}]}`)
	if err == nil || !strings.Contains(err.Error(), "supported_core") {
		t.Fatalf("expected rationale without canonical sections to fail, got %v", err)
	}
}

func TestNormalizeSemanticVerificationOutputRejectsInsufficientEvidenceWithoutConcreteGap(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-1"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-1"},
	}, `{"summary":"semantic","results":[{"claimId":"claim-1","verdict":"insufficient_evidence","confidence":"0.4","rationale":"supported_core: Repository friction exists. | missing_or_conflict: none. | verdict_reason: insufficient_evidence fits best."}]}`)
	if err == nil || !strings.Contains(err.Error(), "missing_or_conflict") {
		t.Fatalf("expected insufficient_evidence without concrete gap to fail, got %v", err)
	}
}

func TestNormalizeSemanticDedupOutputRequiresThresholdAndKnownClaims(t *testing.T) {
	task := consensus.SemanticDedupTask{
		TaskMeta:            consensus.TaskMeta{AgentID: "deduper-1"},
		SimilarityThreshold: 0.85,
		Claims: []consensus.DebateClaim{
			{ClaimID: "claim-1", Active: true},
			{ClaimID: "claim-2", Active: true},
			{ClaimID: "claim-3", Active: true},
		},
	}
	result, err := NormalizeTaskOutputFromText(task, `{"summary":"dedup","merges":[{"sourceClaimId":"claim-2","targetClaimId":"claim-1","similarity":0.92,"rationale":"same practical recommendation"},{"sourceClaimId":"claim-3","targetClaimId":"claim-1","similarity":0.91,"rationale":"same practical recommendation"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.SemanticDedupTaskResult)
	if !ok || len(typed.Output.Merges) != 2 {
		t.Fatalf("unexpected semantic dedup output: %#v", result)
	}
	if typed.Output.Merges[0].Similarity != 0.92 {
		t.Fatalf("unexpected similarity: %#v", typed.Output.Merges[0])
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"dedup","merges":[{"sourceClaimId":"claim-2","targetClaimId":"claim-1","similarity":0.84,"rationale":"close but below threshold"}]}`)
	if err == nil {
		t.Fatal("expected below-threshold semantic dedup merge to fail")
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"dedup","merges":[{"sourceClaimId":"claim-x","targetClaimId":"claim-1","similarity":0.95,"rationale":"unknown claim"}]}`)
	if err == nil {
		t.Fatal("expected unknown source claim to fail")
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"dedup","merges":[{"sourceClaimId":"claim-2","targetClaimId":"claim-1","similarity":0.92,"rationale":"same practical recommendation"},{"sourceClaimId":"claim-1","targetClaimId":"claim-3","similarity":0.91,"rationale":"same practical recommendation"}]}`)
	if err == nil {
		t.Fatal("expected chained semantic dedup merge to fail")
	}
}

func TestNormalizeFinalVoteRequiresNumericConfidence(t *testing.T) {
	task := consensus.FinalVoteTask{
		TaskMeta: consensus.TaskMeta{AgentID: "voter-1"},
		Claims: []consensus.DebateClaim{
			{ClaimID: "claim-1", Active: true},
		},
	}
	result, err := NormalizeTaskOutputFromText(task, `{"summary":"vote","votes":[{"claimId":"claim-1","vote":"accept","confidence":0.67,"rationale":"directionally supported"}]}`)
	if err != nil {
		t.Fatalf("NormalizeTaskOutputFromText failed: %v", err)
	}
	typed, ok := result.(consensus.FinalVoteTaskResult)
	if !ok || len(typed.Output.Votes) != 1 || typed.Output.Votes[0].Confidence == nil || *typed.Output.Votes[0].Confidence != 0.67 {
		t.Fatalf("unexpected final vote output: %#v", result)
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"vote","votes":[{"claimId":"claim-1","vote":"accept","rationale":"missing score"}]}`)
	if err == nil {
		t.Fatal("expected missing confidence to fail")
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"vote","votes":[{"claimId":"claim-1","vote":"accept","confidence":"high","rationale":"label score"}]}`)
	if err == nil {
		t.Fatal("expected confidence label to fail")
	}
	_, err = NormalizeTaskOutputFromText(task, `{"summary":"vote","votes":[{"claimId":"claim-1","vote":"accept","confidence":1.2,"rationale":"out of range"}]}`)
	if err == nil {
		t.Fatal("expected out-of-range confidence to fail")
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

func TestNormalizeReviseOutputRequiresRevisedTextForMarkUnresolved(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
		Claims: []consensus.ClaimNode{
			{ClaimID: "claim-1", Statement: "A monorepo migration should finish in one quarter."},
		},
	}, `{"summary":"revise","revisions":[{"targetClaimId":"claim-1","action":"mark_unresolved","reason":"Need more evidence","unresolved":true}]}`)
	if err == nil || !strings.Contains(err.Error(), "revisedText is required") {
		t.Fatalf("expected mark_unresolved without revisedText to fail, got %v", err)
	}
}

func TestNormalizeReviseOutputRejectsUnknownTargetClaimID(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
		Claims: []consensus.ClaimNode{
			{ClaimID: "claim-1", Statement: "Current claim"},
		},
	}, `{"summary":"revise","revisions":[{"targetClaimId":"claim-x","action":"revise","revisedText":"Narrowed claim","reason":"Removed unsupported scope"}]}`)
	if err == nil || !strings.Contains(err.Error(), "must reference an existing claim") {
		t.Fatalf("expected unknown targetClaimId to fail, got %v", err)
	}
}

func TestNormalizeReviseOutputRejectsUnchangedRestatement(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-1"},
		Claims: []consensus.ClaimNode{
			{ClaimID: "claim-1", Statement: "A monorepo migration should finish in one quarter."},
		},
	}, `{"summary":"revise","revisions":[{"targetClaimId":"claim-1","action":"revise","revisedText":"A monorepo migration should finish in one quarter.","reason":"Kept the same text"}]}`)
	if err == nil || !strings.Contains(err.Error(), "must materially narrow or clarify") {
		t.Fatalf("expected unchanged revise text to fail, got %v", err)
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

func TestDebateRoundSchemaIncludesJudgementEnumAndRequiredFields(t *testing.T) {
	schema := TaskOutputJSONSchema(consensus.DebateRoundTask{})
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected debate schema properties, got %#v", schema)
	}
	judgements, ok := properties["judgements"].(map[string]any)
	if !ok {
		t.Fatalf("expected judgements schema, got %#v", properties["judgements"])
	}
	items, ok := judgements["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected judgements item schema, got %#v", judgements["items"])
	}
	required, ok := items["required"].([]string)
	if !ok {
		t.Fatalf("expected judgements required list, got %#v", items["required"])
	}
	if len(required) != 2 || required[0] != "claimId" || required[1] != "judgement" {
		t.Fatalf("unexpected required fields: %#v", required)
	}
	itemProperties, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected judgements properties, got %#v", items["properties"])
	}
	judgementProperty, ok := itemProperties["judgement"].(map[string]any)
	if !ok {
		t.Fatalf("expected judgement property schema, got %#v", itemProperties["judgement"])
	}
	enumValues, ok := judgementProperty["enum"].([]string)
	if !ok {
		t.Fatalf("expected judgement enum, got %#v", judgementProperty["enum"])
	}
	want := []string{"agree", "disagree", "revise", "no_change"}
	for idx, value := range want {
		if enumValues[idx] != value {
			t.Fatalf("unexpected judgement enum: %#v", enumValues)
		}
	}
}

func TestTaskOutputJSONSchemaForAgentPreservesNaturalOptionalFieldsByDefault(t *testing.T) {
	schema := TaskOutputJSONSchemaForAgent(consensus.SemanticVerificationTask{}, ResolvedAgentRuntime{})
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected root properties, got %#v", schema)
	}
	results, ok := properties["results"].(map[string]any)
	if !ok {
		t.Fatalf("expected results schema, got %#v", properties["results"])
	}
	items, ok := results["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected result item schema, got %#v", results["items"])
	}
	required, ok := items["required"].([]string)
	if !ok {
		t.Fatalf("expected required list, got %#v", items["required"])
	}
	for _, field := range required {
		if field == "confidence" || field == "targetType" {
			t.Fatalf("default schema should keep optional fields optional, got required=%#v", required)
		}
	}
}

func TestTaskOutputJSONSchemaForAgentMakesAllObjectPropertiesRequiredForCodex(t *testing.T) {
	schema := TaskOutputJSONSchemaForAgent(consensus.SemanticVerificationTask{}, ResolvedAgentRuntime{
		Provider: config.ProviderConfig{
			Type:    config.ProviderTypeCLI,
			CLIType: "codex",
		},
	})
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected root properties, got %#v", schema)
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected root required list, got %#v", schema["required"])
	}
	for _, field := range []string{"summary", "results"} {
		if !containsString(required, field) {
			t.Fatalf("expected root required to include %q, got %#v", field, required)
		}
	}
	results, ok := properties["results"].(map[string]any)
	if !ok {
		t.Fatalf("expected results schema, got %#v", properties["results"])
	}
	items, ok := results["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected result item schema, got %#v", results["items"])
	}
	itemRequired, ok := items["required"].([]string)
	if !ok {
		t.Fatalf("expected item required list, got %#v", items["required"])
	}
	for _, field := range []string{"claimId", "targetType", "verdict", "confidence", "rationale"} {
		if !containsString(itemRequired, field) {
			t.Fatalf("expected codex item required to include %q, got %#v", field, itemRequired)
		}
	}
}

func TestNormalizeDebateRoundOutputRejectsDuplicateClaimIDs(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"debate","judgements":[{"claimId":"claim-1","judgement":"agree"},{"claimId":"claim-1","judgement":"no_change"}]}`)
	if err == nil {
		t.Fatal("expected duplicate claimId to fail validation")
	}
}

func TestNormalizeDebateRoundOutputRequiresRevisedStatementForRevise(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"debate","judgements":[{"claimId":"claim-1","judgement":"revise","rationale":"needs narrowing"}]}`)
	if err == nil {
		t.Fatal("expected revise judgement without revisedStatement to fail validation")
	}
}

func TestNormalizeDebateRoundOutputRejectsRevisedStatementOutsideRevise(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"debate","judgements":[{"claimId":"claim-1","judgement":"agree","revisedStatement":"unexpected"}]}`)
	if err == nil {
		t.Fatal("expected non-revise judgement with revisedStatement to fail validation")
	}
}

func TestNormalizeDebateRoundOutputRejectsRelationshipFieldsInNewClaims(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"debate","newClaims":[{"statement":"narrow scope","dependencies":["claim-1"]}],"judgements":[]}`)
	if err == nil {
		t.Fatal("expected newClaims relationship fields to fail validation")
	}
}

func TestNormalizeDebateRoundOutputRejectsProcessMetaNewClaims(t *testing.T) {
	_, err := NormalizeTaskOutputFromText(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-1"},
	}, `{"summary":"debate","newClaims":[{"title":"43 条 peer claims 可合并为约 12 条独立论点","statement":"本轮 43 条 peer claims 的实际独立论点约 12 个，建议系统层面实施去重，将声明数量控制在 15 条以内。","applicability":"辩论流程优化","claimType":"recommendation","confidence":0.95}],"judgements":[]}`)
	if err == nil {
		t.Fatal("expected process/meta newClaims to fail validation")
	}
	if !strings.Contains(err.Error(), "process/meta") {
		t.Fatalf("expected process/meta validation error, got %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
