package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
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
		"supported confidence must be between 0.60 and 1.00.",
		"refuted confidence must be between 0.60 and 1.00.",
		"insufficient_evidence confidence must be between 0.01 and 0.60.",
		"undetermined confidence must stay between 0.35 and 0.65.",
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
		`The repaired results[0].claimId must be exactly claim-current.`,
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected semantic repair prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}
