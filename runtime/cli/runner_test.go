package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TestRunCommandReturnsStructuredCommandError(t *testing.T) {
	_, err := runCommand(context.Background(), config.CLITypeCodex, "sh", []string{"-c", "printf 'stdout 429'; printf 'ordinary failure' >&2; exit 7"}, nil, "")
	if err == nil {
		t.Fatal("expected command failure")
	}
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("expected CommandError, got %T: %v", err, err)
	}
	if commandErr.CLIType != config.CLITypeCodex || commandErr.ExitCode != 7 {
		t.Fatalf("unexpected command metadata: %#v", commandErr)
	}
	if commandErr.Stderr != "ordinary failure" || commandErr.Stdout != "stdout 429" {
		t.Fatalf("unexpected output capture: %#v", commandErr)
	}
	if len(commandErr.Command) < 1 || commandErr.Command[0] != "sh" {
		t.Fatalf("unexpected command: %#v", commandErr.Command)
	}
}

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

func TestBuildBaseArgsClaudeMapsReasoningToEffort(t *testing.T) {
	args := buildBaseArgs("claude", "claude-opus-4-6", "max")
	want := []string{"--print", "--output-format", "json", "--model", "claude-opus-4-6", "--effort", "max"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected claude args: %#v", args)
	}
}

func TestBuildBaseArgsInheritsDefaultModelWithoutEmptyOverride(t *testing.T) {
	for _, cliType := range []string{config.CLITypeCodex, config.CLITypeClaude, config.CLITypeGrok, config.CLITypeAntigravity} {
		t.Run(cliType, func(t *testing.T) {
			args := buildBaseArgs(cliType, "", "")
			for _, arg := range args {
				if arg == "--model" || arg == "-m" || arg == "" {
					t.Fatalf("空模型应继承 CLI 原配置，不能传空覆盖参数: %#v", args)
				}
			}
		})
	}
}

func TestExecutionProfileMapsReadOnlyAndStreamingContracts(t *testing.T) {
	count := func(args []string, target string) int {
		result := 0
		for _, arg := range args {
			if arg == target {
				result++
			}
		}
		return result
	}
	for _, cliType := range []string{config.CLITypeCodex, config.CLITypeClaude, config.CLITypeGrok, config.CLITypeAntigravity} {
		t.Run(cliType, func(t *testing.T) {
			base := buildBaseArgs(cliType, "", "")
			original := strings.Join(base, "\x00")
			args := applyExecutionProfile(config.ProviderConfig{
				CLIType: cliType, PermissionMode: "read-only", OutputFormat: "stream-json",
			}, base)
			if strings.Join(base, "\x00") != original {
				t.Fatal("执行 profile 不应污染默认 argv")
			}
			for _, forbidden := range []string{"--dangerously-skip-permissions", "--disable-slash-commands", "--full-auto"} {
				if count(args, forbidden) != 0 {
					t.Fatalf("只读 profile 出现不允许的参数 %q: %#v", forbidden, args)
				}
			}
			switch cliType {
			case config.CLITypeCodex:
				if argumentAfter(base, "--sandbox") != "workspace-write" || argumentAfter(args, "--sandbox") != "read-only" || count(args, "--sandbox") != 1 || count(args, "--json") != 1 {
					t.Fatalf("Codex 默认 workspace-write 或只读覆盖错误: base=%#v profile=%#v", base, args)
				}
			case config.CLITypeClaude:
				if argumentAfter(args, "--output-format") != "stream-json" || count(args, "--output-format") != 1 || count(args, "--verbose") != 1 || count(args, "--include-partial-messages") != 1 {
					t.Fatalf("Claude 流式参数错误: %#v", args)
				}
				if count(args, "--tools") != 1 || argumentAfter(args, "--tools") != "" || count(args, "--safe-mode") != 1 || count(args, "--strict-mcp-config") != 1 || argumentAfter(args, "--mcp-config") != `{"mcpServers":{}}` || argumentAfter(args, "--permission-mode") != "dontAsk" || argumentAfter(args, "--permission-prompts") != "none" {
					t.Fatalf("Claude 只读工具限制错误: %#v", args)
				}
			case config.CLITypeGrok:
				if argumentAfter(args, "--output-format") != "streaming-json" || count(args, "--output-format") != 1 || argumentAfter(args, "--permission-mode") != "plan" {
					t.Fatalf("Grok 公共 stream-json 应映射到原生 streaming-json: %#v", args)
				}
				if count(args, "--tools") != 1 || argumentAfter(args, "--tools") != "" || argumentAfter(args, "--deny") != "MCPTool(*)" || count(args, "--disable-web-search") != 1 || count(args, "--no-subagents") != 1 {
					t.Fatalf("Grok 只读工具限制错误: %#v", args)
				}
			case config.CLITypeAntigravity:
				if argumentAfter(args, "--mode") != "plan" || argumentAfter(args, "--input-format") != "stream-json" || argumentAfter(args, "--output-format") != "stream-json" || count(args, "--sandbox") != 1 {
					t.Fatalf("agy 应保留经实测的 plan/sandbox 原生双向流协议: %#v", args)
				}
			}
		})
	}
}

func TestExecutionProfileWorkspaceWriteUsesProviderNativeModes(t *testing.T) {
	for _, test := range []struct {
		cliType string
		flag    string
		want    string
	}{
		{config.CLITypeClaude, "--permission-mode", "acceptEdits"},
		{config.CLITypeAntigravity, "--mode", "accept-edits"},
		{config.CLITypeGrok, "--permission-mode", "default"},
	} {
		t.Run(test.cliType, func(t *testing.T) {
			args := applyExecutionProfile(config.ProviderConfig{CLIType: test.cliType, PermissionMode: "workspace-write"}, buildBaseArgs(test.cliType, "", ""))
			if got := argumentAfter(args, test.flag); got != test.want {
				t.Fatalf("%s = %q，期望 %q: %#v", test.flag, got, test.want, args)
			}
		})
	}
}

func TestPreviewNativeSchemaHasOneOutputFormat(t *testing.T) {
	for _, cliType := range []string{config.CLITypeClaude, config.CLITypeGrok, config.CLITypeAntigravity} {
		t.Run(cliType, func(t *testing.T) {
			for _, outputFormat := range []string{"json", "stream-json"} {
				command, err := PreviewCommand(config.ProviderConfig{
					Type: config.ProviderTypeCLI, CLIType: cliType, Command: "preview-only", OutputFormat: outputFormat,
				}, realPromptTransportTestTask{}, "private prompt", "agent", "role", "", "", nil, map[string]any{"type": "object"})
				if err != nil {
					t.Fatal(err)
				}
				formatCount, schemaCount := 0, 0
				for _, arg := range command {
					if arg == "--output-format" {
						formatCount++
					}
					if arg == "--json-schema" {
						schemaCount++
					}
				}
				if formatCount != 1 {
					t.Fatalf("schema 不能追加第二个 output-format: %#v", command)
				}
				wantSchemaCount := 1
				if cliType == config.CLITypeGrok && outputFormat == "stream-json" {
					// Grok 的 native schema 会强制 JSON 模式，流模式应把 schema 放到上下文。
					wantSchemaCount = 0
				}
				if schemaCount != wantSchemaCount {
					t.Fatalf("native schema 参数错误: %#v", command)
				}
			}
		})
	}
}

func TestBuildBaseArgsCodexUsesCanonicalSandboxFlags(t *testing.T) {
	tests := []struct {
		name      string
		reasoning string
		want      []string
	}{
		{
			name: "without reasoning",
			want: []string{"exec", "-m", "gpt-5", "--sandbox", "workspace-write", "-c", `approval_policy="never"`, "--color", "never", "--skip-git-repo-check", "--json"},
		},
		{
			name:      "with reasoning",
			reasoning: "xhigh",
			want:      []string{"exec", "-m", "gpt-5", "-c", "model_reasoning_effort=xhigh", "--sandbox", "workspace-write", "-c", `approval_policy="never"`, "--color", "never", "--skip-git-repo-check", "--json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildBaseArgs("codex", "gpt-5", tt.reasoning)
			if strings.Join(args, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("unexpected codex args: %#v", args)
			}
			for _, arg := range args {
				if arg == "--full-auto" || strings.Contains(arg, "resume") || strings.Contains(arg, "session") {
					t.Fatalf("expected canonical one-shot args, got %#v", args)
				}
			}
		})
	}
}

func TestPromptTransportKeepsPromptOutOfArgv(t *testing.T) {
	tests := []struct {
		name    string
		cliType string
		model   string
		want    []string
	}{
		{name: "generic", cliType: config.CLITypeGeneric, model: "local", want: nil},
		{name: "claude", cliType: "claude", model: "claude-opus-4-6", want: []string{"--print", "--output-format", "json", "--model", "claude-opus-4-6", "--effort", "medium"}},
		{name: "codex", cliType: "codex", model: "gpt-5", want: []string{"exec", "-m", "gpt-5", "-c", "model_reasoning_effort=medium", "--sandbox", "workspace-write", "-c", `approval_policy="never"`, "--color", "never", "--skip-git-repo-check", "--json"}},
		{name: "gemini", cliType: "gemini", model: "gemini-3.1-pro-preview", want: []string{"--approval-mode", "yolo", "-m", "gemini-3.1-pro-preview"}},
		{name: "opencode", cliType: "opencode", model: "google/gemini", want: []string{"run", "--yolo", "-m", "google/gemini"}},
		{name: "pi", cliType: config.CLITypePi, model: "gemini-3.6-flash", want: []string{"--mode", "json", "--print", "--no-session", "--model", "gemini-3.6-flash", "--thinking", "medium"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := preparePromptTransport(tt.cliType, tt.model, "unique-secret-prompt", "medium")
			if err != nil {
				t.Fatalf("preparePromptTransport failed: %v", err)
			}
			defer func() { _ = transport.Cleanup() }()
			if transport.Stdin != "unique-secret-prompt" {
				t.Fatalf("unexpected stdin: %q", transport.Stdin)
			}
			if strings.Join(transport.Args, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("unexpected args: %#v", transport.Args)
			}
			if strings.Contains(strings.Join(transport.Args, " "), "unique-secret-prompt") {
				t.Fatalf("prompt leaked into argv: %#v", transport.Args)
			}
		})
	}
}

func TestPiDoesNotGenerateSchemaFlags(t *testing.T) {
	args, cleanup, err := buildStructuredOutputArgs(config.CLITypePi, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("buildStructuredOutputArgs failed: %v", err)
	}
	defer cleanup()
	if len(args) != 0 {
		t.Fatalf("expected no Pi schema args, got %#v", args)
	}
}

func TestPiOutputContractRequiresSingleJSONObject(t *testing.T) {
	contract := buildCLIOutputContract(config.CLITypePi, realPromptTransportTestTask{})
	if !strings.Contains(contract, "Pi-specific") || !strings.Contains(contract, "exactly one JSON object") {
		t.Fatalf("unexpected Pi output contract: %s", contract)
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
	if len(args) != 2 || args[0] != "--json-schema" {
		t.Fatalf("unexpected claude schema args: %#v", args)
	}
	if !strings.Contains(args[1], `"ok"`) {
		t.Fatalf("expected inlined schema, got %q", args[1])
	}
}

func TestNormalizeStructuredCLIOutputExtractsClaudeStructuredOutput(t *testing.T) {
	// 复用真实成功 envelope，包含 subtype/is_error/terminal_reason。
	out, err := normalizeStructuredCLIOutput("claude", map[string]any{"type": "object"}, realClaudeNativeSuccess)
	if err != nil {
		t.Fatalf("normalizeStructuredCLIOutput failed: %v", err)
	}
	var envelope struct {
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(realClaudeNativeSuccess), &envelope); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.StructuredOutput, &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("unexpected normalized output: %s, want %s", out, wantJSON)
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

func TestRunCommandPreservesOutputOnCancellation(t *testing.T) {
	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 300*time.Millisecond)
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "cancel",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				time.AfterFunc(300*time.Millisecond, cancel)
				return ctx, cancel
			},
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.context()
			defer cancel()
			_, err := runCommand(ctx, "generic", "sh",
				[]string{"-c", `printf 'partial-stdout\n'; printf 'partial-stderr\n' >&2; sleep 30`}, nil, "")
			if err == nil {
				t.Fatal("expected cancelled command to fail")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error chain to keep %v, got %v", tt.wantErr, err)
			}
			var commandErr *CommandError
			if !errors.As(err, &commandErr) {
				t.Fatalf("expected *CommandError, got %T: %v", err, err)
			}
			if commandErr.Stdout != "partial-stdout" {
				t.Fatalf("stdout not preserved, got %q", commandErr.Stdout)
			}
			if commandErr.Stderr != "partial-stderr" {
				t.Fatalf("stderr not preserved, got %q", commandErr.Stderr)
			}
			if commandErr.ExitCode >= 0 {
				t.Fatalf("expected negative exit code for terminated process, got %d", commandErr.ExitCode)
			}
			if strings.Contains(commandErr.Error(), "exited with code") {
				t.Fatalf("terminated process must not be reported as an exit status: %s", commandErr.Error())
			}
		})
	}
}
