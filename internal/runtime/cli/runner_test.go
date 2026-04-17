package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestGenericCLIRunnerPassesPromptViaStdin(t *testing.T) {
	runner := NewRunner(config.ProviderConfig{
		Type:    config.ProviderTypeCLI,
		CLIType: config.CLITypeGeneric,
		Command: "sh",
		Args:    []string{"-c", "cat"},
	})
	out, err := runner.RunTask(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{RequestID: "req-1", SessionID: "session-1", AgentID: "agent-1"},
	}, "hello-cli", "agent-1", "role", "model-x", "", nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if !strings.Contains(out, "hello-cli") || !strings.Contains(out, `"task"`) {
		t.Fatalf("unexpected cli output: %s", out)
	}
}

func TestBuildBaseArgsCodexIsOneShot(t *testing.T) {
	args, stdin := buildBaseArgs("codex", "gpt-5", "prompt", "high")
	if len(args) == 0 || stdin != "prompt" {
		t.Fatalf("unexpected codex args/stdin: %#v %q", args, stdin)
	}
	for _, arg := range args {
		if strings.Contains(arg, "resume") || strings.Contains(arg, "session") {
			t.Fatalf("expected one-shot args, got %#v", args)
		}
	}
}

func TestHardenPromptForCLIAddsProviderSpecificContract(t *testing.T) {
	base := "Return exactly one JSON object only."
	prompt := hardenPromptForCLI("gemini", consensus.SemanticVerificationTask{}, base)
	for _, fragment := range []string{
		"CLI output contract (strict):",
		"Semantic fields: summary, results[].claimId, results[].verdict, results[].confidence, results[].rationale.",
		"Semantic aliases forbidden: targetId, verificationStatus, reasoning, reason.",
		"Semantic verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
		"Gemini-specific: do not replace canonical fields with aliases like verificationStatus/claim/text/accepted/verified.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected hardened prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestHardenPromptForCLILeavesGenericPromptUnchanged(t *testing.T) {
	base := "hello"
	if got := hardenPromptForCLI(config.CLITypeGeneric, consensus.ProposalTask{}, base); got != base {
		t.Fatalf("expected generic prompt to remain unchanged, got %q", got)
	}
}

func TestHardenPromptForCLIAddsArbiterAndProposalFieldContracts(t *testing.T) {
	proposalPrompt := hardenPromptForCLI("codex", consensus.ProposalTask{}, "base")
	for _, fragment := range []string{
		"Proposal fields: summary, claims[].statement, claims[].claimType, claims[].confidence.",
		"Proposal aliases forbidden: claim, text, statementText.",
		"Proposal confidence must be a JSON number, not a string.",
		"Codex-specific: suppress all preambles, reflections, and trailing notes. Output JSON only.",
	} {
		if !strings.Contains(proposalPrompt, fragment) {
			t.Fatalf("expected proposal hardened prompt to contain %q, got:\n%s", fragment, proposalPrompt)
		}
	}

	arbiterPrompt := hardenPromptForCLI("claude", consensus.ArbiterTask{}, "base")
	for _, fragment := range []string{
		"Arbiter fields: summary, taskVerdict, decisions[].claimId, decisions[].verdict, decisions[].confidence, decisions[].rationale, decisions[].evidenceRefs.",
		"Arbiter taskVerdict must be a JSON string, not an object.",
		"Arbiter aliases forbidden: targetClaimId, targetId, reasoning, reason, keyEvidenceRefs.",
		"Arbiter claim verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
		"Claude-specific: do not wrap the answer in prose or explanatory paragraphs. Output JSON only.",
	} {
		if !strings.Contains(arbiterPrompt, fragment) {
			t.Fatalf("expected arbiter hardened prompt to contain %q, got:\n%s", fragment, arbiterPrompt)
		}
	}
}
