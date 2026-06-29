package cli

import (
	"context"
	"os"
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
	}, "hello-cli", "agent-1", "role", "model-x", "", nil, nil)
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

func TestBuildBaseArgsAntigravityUsesPrintPrompt(t *testing.T) {
	args, stdin := buildBaseArgs(config.CLITypeAntigravity, "Gemini 3.5 Flash (High)", "prompt", "medium")
	if stdin != "" {
		t.Fatalf("expected antigravity prompt in args, got stdin %q", stdin)
	}
	want := []string{"--model", "Gemini 3.5 Flash (High)", "-p", "prompt"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected antigravity args: %#v", args)
	}
}

func TestBuildBaseArgsGeminiUsesPromptFlag(t *testing.T) {
	args, stdin := buildBaseArgs("gemini", "gemini-3.1-pro-preview", "prompt", "medium")
	if stdin != "" {
		t.Fatalf("expected gemini prompt in args, got stdin %q", stdin)
	}
	want := []string{"--approval-mode", "yolo", "-m", "gemini-3.1-pro-preview", "-p", "prompt"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected gemini args: %#v", args)
	}
}

func TestBuildStructuredOutputArgsClaudeInlinesSchema(t *testing.T) {
	args, cleanup, err := buildStructuredOutputArgs("claude", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required": []string{"ok"},
	})
	if err != nil {
		t.Fatalf("buildStructuredOutputArgs failed: %v", err)
	}
	defer cleanup()
	if len(args) != 4 || args[0] != "--json-schema" || args[2] != "--output-format" || args[3] != "json" {
		t.Fatalf("unexpected claude schema args: %#v", args)
	}
	if !strings.Contains(args[1], `"ok"`) {
		t.Fatalf("expected inlined schema, got %q", args[1])
	}
}

func TestNormalizeStructuredCLIOutputExtractsClaudeStructuredOutput(t *testing.T) {
	out, err := normalizeStructuredCLIOutput("claude", map[string]any{"type": "object"}, `{"type":"result","structured_output":{"ok":true}}`)
	if err != nil {
		t.Fatalf("normalizeStructuredCLIOutput failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected normalized output: %s", out)
	}
}

func TestBuildStructuredOutputArgsCodexWritesTempFile(t *testing.T) {
	args, cleanup, err := buildStructuredOutputArgs("codex", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required": []string{"ok"},
	})
	if err != nil {
		t.Fatalf("buildStructuredOutputArgs failed: %v", err)
	}
	if len(args) != 2 || args[0] != "--output-schema" {
		t.Fatalf("unexpected codex schema args: %#v", args)
	}
	body, readErr := os.ReadFile(args[1])
	if readErr != nil {
		t.Fatalf("read schema file: %v", readErr)
	}
	if !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("expected schema file body, got %s", string(body))
	}
	cleanup()
	if _, statErr := os.Stat(args[1]); !os.IsNotExist(statErr) {
		t.Fatalf("expected schema temp file removed, statErr=%v", statErr)
	}
}

func TestBuildFinalOutputCaptureArgsCodexReadsLastMessage(t *testing.T) {
	args, read, cleanup, err := buildFinalOutputCaptureArgs("codex")
	if err != nil {
		t.Fatalf("buildFinalOutputCaptureArgs failed: %v", err)
	}
	if len(args) != 2 || args[0] != "--output-last-message" {
		t.Fatalf("unexpected codex capture args: %#v", args)
	}
	if err := os.WriteFile(args[1], []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write capture file: %v", err)
	}
	out, err := read()
	if err != nil {
		t.Fatalf("read capture failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected captured output: %s", out)
	}
	cleanup()
	if _, statErr := os.Stat(args[1]); !os.IsNotExist(statErr) {
		t.Fatalf("expected capture temp file removed, statErr=%v", statErr)
	}
}

func TestBuildFinalOutputCaptureArgsNonCodexNoops(t *testing.T) {
	args, read, cleanup, err := buildFinalOutputCaptureArgs("claude")
	if err != nil {
		t.Fatalf("buildFinalOutputCaptureArgs failed: %v", err)
	}
	defer cleanup()
	if len(args) != 0 {
		t.Fatalf("expected no capture args, got %#v", args)
	}
	out, err := read()
	if err != nil {
		t.Fatalf("read capture failed: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty capture output, got %q", out)
	}
}

func TestHardenPromptForCLIAddsProviderSpecificContract(t *testing.T) {
	base := "Return exactly one JSON object only."
	prompt := hardenPromptForCLI("gemini", consensus.SemanticVerificationTask{
		Claim: consensus.ClaimNode{ClaimID: "claim-1"},
	}, base)
	for _, fragment := range []string{
		"CLI output contract (strict):",
		"Semantic fields: summary, results[].claimId, results[].verdict, results[].confidence, results[].rationale.",
		"Semantic aliases forbidden: targetId, verificationStatus, reasoning, reason.",
		"Semantic output must contain exactly one results[] row for the current claim.",
		"Semantic output must not emit rows for challenges, evidence, or source materials.",
		"Semantic targetType is optional, but if present it must be claim.",
		"Semantic verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
		"Semantic confidence bands: supported/refuted must be 0.60-1.00, insufficient_evidence must be 0.01-0.60, undetermined must be 0.35-0.65.",
		"Semantic rationale must use exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
		"Semantic supported_core must name the narrowest surviving claim core, even when the verdict is insufficient_evidence or undetermined.",
		"Semantic verdict_reason must explain why the chosen verdict beats the other three canonical verdicts.",
		"Semantic recommendation claims should judge the directional recommendation separately from rollout mechanics.",
		"Semantic insufficient_evidence is preferred when evidence is simply too weak or missing.",
		"Semantic claimId must be exactly claim-1.",
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

func TestHardenPromptForCLIAddsAntigravityContract(t *testing.T) {
	prompt := hardenPromptForCLI(config.CLITypeAntigravity, consensus.ProposalTask{}, "base")
	for _, fragment := range []string{
		"CLI output contract (strict):",
		"Proposal fields: summary, claims[].title, claims[].statement, claims[].claimType, claims[].confidence, claims[].applicability, claims[].boundaryConditions.",
		"Antigravity-specific: non-interactive output is parsed from stdout.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected antigravity hardened prompt to contain %q, got:\n%s", fragment, prompt)
		}
	}
}

func TestHardenPromptForCLIAddsArbiterAndProposalFieldContracts(t *testing.T) {
	proposalPrompt := hardenPromptForCLI("codex", consensus.ProposalTask{}, "base")
	for _, fragment := range []string{
		"Proposal fields: summary, claims[].title, claims[].statement, claims[].claimType, claims[].confidence, claims[].applicability, claims[].boundaryConditions.",
		"Proposal aliases forbidden: claim, text, statementText.",
		"Proposal relationship fields forbidden: dependencies, parentClaimIds.",
		"Proposal confidence must be a JSON number, not a string.",
		"Codex-specific: suppress all preambles, reflections, and trailing notes. Output JSON only.",
		"Codex-specific: when a schema field exists, include it explicitly. Use empty strings, empty arrays, 0, or false for not-applicable optional values instead of omitting the field.",
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
		"Arbiter must judge the current revised claim text as written, not the broader superseded claim.",
		"Arbiter should prefer supported when a narrowed strategy/operational claim keeps a supported directional core but leaves execution details or prerequisites conditional.",
		"Arbiter recommendation claims should preserve a supported directional path and carry remaining rollout uncertainty as caveats whenever the claim already encodes those caveats.",
		"Arbiter should use insufficient_evidence only when no narrower supported core remains even after considering caveats, applicability, and boundaryConditions.",
		"Claude-specific: do not wrap the answer in prose or explanatory paragraphs. Output JSON only.",
	} {
		if !strings.Contains(arbiterPrompt, fragment) {
			t.Fatalf("expected arbiter hardened prompt to contain %q, got:\n%s", fragment, arbiterPrompt)
		}
	}
}

func TestHardenPromptForCLIAddsDebateRoundContract(t *testing.T) {
	debatePrompt := hardenPromptForCLI("gemini", consensus.DebateRoundTask{}, "base")
	for _, fragment := range []string{
		"Debate-round fields: summary, newClaims[], judgements[].claimId, judgements[].judgement, judgements[].rationale, judgements[].revisedStatement, judgements[].mergeWithClaims.",
		"Debate-round claimId must copy an existing peerClaims[].claimId exactly.",
		"Debate-round judgement allowed: agree, disagree, revise, no_change.",
		"Debate-round if judgement=revise, revisedStatement is required. Otherwise omit revisedStatement.",
		"Gemini-specific: do not replace canonical fields with aliases like verificationStatus/claim/text/accepted/verified.",
	} {
		if !strings.Contains(debatePrompt, fragment) {
			t.Fatalf("expected debate hardened prompt to contain %q, got:\n%s", fragment, debatePrompt)
		}
	}
}

func TestHardenPromptForCLIAddsReviseContract(t *testing.T) {
	revisePrompt := hardenPromptForCLI("codex", consensus.ReviseTask{}, "base")
	for _, fragment := range []string{
		"Revise fields: summary, revisions[].targetClaimId, revisions[].action, revisions[].reason, revisions[].confidenceDelta, revisions[].unresolved.",
		"Revise targetClaimId must copy an existing claims[].claimId exactly.",
		"Revise should prefer action=revise when a narrower evidence-backed statement can be written.",
		"Revise action=revise requires revisedText, and revisedText must be materially narrower than the current claim text.",
		"Revise action=mark_unresolved requires revisedText and unresolved=true. Use it only when no narrower supported statement can be written.",
	} {
		if !strings.Contains(revisePrompt, fragment) {
			t.Fatalf("expected revise hardened prompt to contain %q, got:\n%s", fragment, revisePrompt)
		}
	}
}
