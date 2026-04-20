package runtime

import (
	"errors"
	"strings"
	"testing"

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
