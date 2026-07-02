package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TestBuildTaskPromptAddsDebateRoundHintsAndExamples(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-a"},
		PeerClaims: []consensus.DebateClaim{
			{ClaimID: "claim-a", Statement: "Prefer monorepo", OwnerID: "peer-1", Active: true},
			{ClaimID: "claim-b", Statement: "Prefer polyrepo", OwnerID: "peer-2", Active: true},
		},
	}, ResolvedAgentRuntime{}, true)

	for _, fragment := range []string{
		"Task-specific output rules:",
		"Valid peer claim IDs for this task: claim-a, claim-b.",
		`"claimId":"claim-a","judgement":"disagree"`,
		`"claimId":"claim-b","judgement":"revise"`,
		`"judgement":"accepted"`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected debate prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildTaskPromptAddsProposalFieldRestrictions(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, ResolvedAgentRuntime{}, true)

	for _, fragment := range []string{
		"Task-specific output rules:",
		"Proposal claim fields are limited to: title, statement, claimType, confidence, applicability, boundaryConditions.",
		"Do not emit dependencies or parentClaimIds in proposal outputs.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected proposal prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildRepairPromptAddsProposalRepairHints(t *testing.T) {
	prompt := BuildRepairPrompt(consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
	}, ResolvedAgentRuntime{}, `{"summary":"proposal","claims":[{"statement":"prefer monorepo","dependencies":["material-1"]}]}`, errors.New("claims[0].dependencies is not allowed"), true)

	for _, fragment := range []string{
		"Task-specific repair rules:",
		"If the previous output used dependencies or parentClaimIds, remove those fields.",
		"Preserve prerequisites as plain language in applicability or boundaryConditions",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected proposal repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildTaskPromptAddsReviseHintsAndExamples(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
		Claims: []consensus.ClaimNode{
			{ClaimID: "claim-a", Statement: "A monorepo migration should finish in one quarter."},
			{ClaimID: "claim-b", Statement: "Polyrepo is the only scalable option."},
		},
	}, ResolvedAgentRuntime{}, true)

	for _, fragment := range []string{
		"Task-specific output rules:",
		"Prefer action=revise when you can remove unsupported numbers, timelines, universals, causal strength, or rollout scope while preserving the evidence-backed core.",
		"Valid targetClaimId values for this task: claim-a, claim-b.",
		`"targetClaimId":"claim-a","action":"revise","revisedText":"Cross-repo changes currently create measurable coordination overhead, but the available record does not yet prove monorepo is the right remedy."`,
		`"targetClaimId":"claim-a","action":"mark_unresolved","revisedText":"A phased monorepo migration remains a candidate path`,
		`"action":"mark_unresolved","reason":"Need more evidence"`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected revise prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildRepairPromptAddsReviseRepairHints(t *testing.T) {
	prompt := BuildRepairPrompt(consensus.ReviseTask{
		TaskMeta: consensus.TaskMeta{AgentID: "proposer-a"},
		Claims: []consensus.ClaimNode{
			{ClaimID: "claim-a", Statement: "A monorepo migration should finish in one quarter."},
		},
	}, ResolvedAgentRuntime{}, `{"summary":"revise","revisions":[{"targetClaimId":"claim-a","action":"mark_unresolved","reason":"Need more evidence"}]}`, errors.New("revisions[0].revisedText is required when action=mark_unresolved"), true)

	for _, fragment := range []string{
		"Task-specific repair rules:",
		"prefer narrower revisedText over unresolved whenever a smaller supported claim can be stated.",
		"mark_unresolved requires unresolved=true and revisedText.",
		"Valid targetClaimId values for repair: claim-a.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected revise repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildRepairPromptAddsDebateRoundRepairHints(t *testing.T) {
	prompt := BuildRepairPrompt(consensus.DebateRoundTask{
		TaskMeta: consensus.TaskMeta{AgentID: "participant-a"},
		PeerClaims: []consensus.DebateClaim{
			{ClaimID: "claim-a", Statement: "Prefer monorepo", OwnerID: "peer-1", Active: true},
		},
	}, ResolvedAgentRuntime{}, `{"summary":"...","judgements":[{"claim":"claim-a","verdict":"support"}]}`, errors.New("judgements[0].claimId is required"), true)

	for _, fragment := range []string{
		"Task-specific repair rules:",
		"Valid peer claim IDs for repair: claim-a.",
		"Replace any non-canonical judgement literal with the closest canonical value: agree, disagree, revise, or no_change, while preserving the original stance.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected debate repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildTaskPromptUsesCodexSpecificStrictSchema(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-a"},
	}, ResolvedAgentRuntime{
		Provider: config.ProviderConfig{
			Type:    config.ProviderTypeCLI,
			CLIType: "codex",
		},
	}, true)

	for _, fragment := range []string{
		`"required": [`,
		`"claimId"`,
		`"targetType"`,
		`"confidence"`,
		`"rationale"`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected codex prompt schema to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildTaskPromptAddsSemanticVerificationHints(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-a"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-current"},
	}, ResolvedAgentRuntime{}, true)

	for _, fragment := range []string{
		"Return exactly one semantic result row for the current claim.",
		`The only valid claimId for this task is claim-current.`,
		"Before choosing a verdict, identify the narrowest evidence-backed core that still survives",
		"For recommendation claims, separate the directional recommendation from rollout mechanics.",
		"supported confidence must be between 0.60 and 1.00.",
		"refuted confidence must be between 0.60 and 1.00.",
		"insufficient_evidence confidence must be between 0.01 and 0.60.",
		"undetermined confidence must stay between 0.35 and 0.65.",
		"rationale must use exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
		`Valid supported example: {"summary":"The current claim is directly backed by the record.","results":[{"claimId":"claim-current","targetType":"claim","verdict":"supported"`,
		`Valid refuted example: {"summary":"The current claim overstates what the record proves.","results":[{"claimId":"claim-current","targetType":"claim","verdict":"refuted"`,
		`Valid insufficient_evidence example: {"summary":"The current claim is plausible but under-supported.","results":[{"claimId":"claim-current","targetType":"claim","verdict":"insufficient_evidence"`,
		`Valid undetermined example: {"summary":"The current claim has genuinely mixed evidence.","results":[{"claimId":"claim-current","targetType":"claim","verdict":"undetermined"`,
		"undetermined: use only for genuinely mixed or ambiguous evidence",
		"Invalid example:",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected semantic prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildRepairPromptAddsSemanticVerificationRepairHints(t *testing.T) {
	prompt := BuildRepairPrompt(consensus.SemanticVerificationTask{
		TaskMeta: consensus.TaskMeta{AgentID: "verifier-a"},
		Claim:    consensus.ClaimNode{ClaimID: "claim-current"},
	}, ResolvedAgentRuntime{}, `{"summary":"semantic","results":[{"claimId":"challenge-1","targetType":"challenge","verdict":"supported","rationale":"wrong target"}]}`, errors.New("results must contain exactly one claim-level entry"), true)

	for _, fragment := range []string{
		"Task-specific repair rules:",
		"Rewrite the output to exactly one canonical result row for the current claim.",
		"Repair confidence to match the canonical verdict bands",
		"Rewrite rationale into exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
		`The repaired results[0].claimId must be exactly claim-current.`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected semantic repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildTaskPromptAddsArbiterHintsAndExamples(t *testing.T) {
	prompt := BuildTaskPrompt(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-a"},
		Claims: []consensus.ClaimNode{
			{
				ClaimID:            "claim-a",
				Statement:          "渐进式收敛方向优于大爆炸式迁移，但具体层级优先级仍需数据验证。",
				ClaimType:          consensus.ClaimTypeRecommendation,
				Applicability:      "仅在补齐归因数据后适用",
				BoundaryConditions: []string{"需确认平台团队容量"},
				Caveats:            []string{"具体路径仍待验证"},
			},
		},
	}, ResolvedAgentRuntime{}, true)

	for _, fragment := range []string{
		"Task-specific output rules:",
		"Prefer verdict=supported when the revised claim already narrows itself to the evidence-backed directional core",
		"For recommendation claims, first ask whether a directional candidate path is still supported after all narrowing.",
		"For strategy or operational recommendations, 'direction is supported but path details remain conditional' should usually be treated as supported with caveats",
		`"claimId":"claim-a","verdict":"supported","confidence":0.68`,
		`"claimId":"claim-a","verdict":"insufficient_evidence","confidence":0.45`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected arbiter prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestBuildRepairPromptAddsArbiterRepairHints(t *testing.T) {
	prompt := BuildRepairPrompt(consensus.ArbiterTask{
		TaskMeta: consensus.TaskMeta{AgentID: "arbiter-a"},
		Claims: []consensus.ClaimNode{
			{
				ClaimID:            "claim-a",
				Statement:          "渐进式收敛方向优于大爆炸式迁移，但具体层级优先级仍需数据验证。",
				ClaimType:          consensus.ClaimTypeRecommendation,
				Applicability:      "仅在补齐归因数据后适用",
				BoundaryConditions: []string{"需确认平台团队容量"},
				Caveats:            []string{"具体路径仍待验证"},
			},
		},
	}, ResolvedAgentRuntime{}, `{"summary":"arbiter","taskVerdict":"undetermined","decisions":[{"claimId":"claim-a","verdict":"insufficient_evidence","confidence":0.45,"rationale":"Need more detail.","evidenceRefs":["ledger-1"]}]}`, errors.New("prefer supported directional core"), true)

	for _, fragment := range []string{
		"Task-specific repair rules:",
		"avoids collapsing caveated directional support into insufficient_evidence",
		"For recommendation claims, preserve the supported directional path whenever it survives",
		"If the revised claim text is cautiously worded and the evidence supports that cautious core, prefer verdict=supported",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected arbiter repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}
