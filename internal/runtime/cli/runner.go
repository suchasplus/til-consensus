package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Runner struct {
	provider config.ProviderConfig
}

type RunTaskResult struct {
	Output  string
	Command []string
}

func NewRunner(provider config.ProviderConfig) *Runner {
	return &Runner{provider: provider}
}

func PreviewCommand(
	provider config.ProviderConfig,
	task consensus.Task,
	prompt string,
	agentID string,
	role string,
	providerModel string,
	reasoning string,
	temperature *float64,
	outputSchema map[string]any,
) ([]string, error) {
	cliType := provider.CLIType
	if cliType == "" {
		cliType = config.CLITypeGeneric
	}
	finalPrompt, err := buildFinalPrompt(cliType, task, prompt, agentID, role, providerModel, reasoning, temperature)
	if err != nil {
		return nil, err
	}
	commandName := provider.Command
	if commandName == "" {
		commandName = cliType
	}
	args, _ := buildBaseArgs(cliType, providerModel, finalPrompt, reasoning)
	schemaArgs, err := buildStructuredOutputArgsPreview(cliType, outputSchema)
	if err != nil {
		return nil, err
	}
	args = append(args, schemaArgs...)
	args = append(args, buildFinalOutputCaptureArgsPreview(cliType)...)
	args = append(args, renderArgs(provider.Args, task, agentID, role, providerModel)...)
	return append([]string{commandName}, args...), nil
}

func (r *Runner) RunTask(
	ctx context.Context,
	task consensus.Task,
	prompt string,
	agentID string,
	role string,
	providerModel string,
	reasoning string,
	temperature *float64,
	outputSchema map[string]any,
) (string, error) {
	result, err := r.RunTaskDetailed(ctx, task, prompt, agentID, role, providerModel, reasoning, temperature, outputSchema)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (r *Runner) RunTaskDetailed(
	ctx context.Context,
	task consensus.Task,
	prompt string,
	agentID string,
	role string,
	providerModel string,
	reasoning string,
	temperature *float64,
	outputSchema map[string]any,
) (RunTaskResult, error) {
	cliType := r.provider.CLIType
	if cliType == "" {
		cliType = config.CLITypeGeneric
	}
	finalPrompt, err := buildFinalPrompt(cliType, task, prompt, agentID, role, providerModel, reasoning, temperature)
	if err != nil {
		return RunTaskResult{}, err
	}
	commandName := r.provider.Command
	if commandName == "" {
		commandName = cliType
	}
	args, stdin := buildBaseArgs(cliType, providerModel, finalPrompt, reasoning)
	schemaArgs, cleanup, err := buildStructuredOutputArgs(cliType, outputSchema)
	if err != nil {
		return RunTaskResult{}, err
	}
	defer cleanup()
	args = append(args, schemaArgs...)
	captureArgs, readCapturedOutput, cleanupCapture, err := buildFinalOutputCaptureArgs(cliType)
	if err != nil {
		return RunTaskResult{}, err
	}
	defer cleanupCapture()
	args = append(args, captureArgs...)
	args = append(args, renderArgs(r.provider.Args, task, agentID, role, providerModel)...)
	command := append([]string{commandName}, args...)
	env := append(os.Environ(), renderEnv(r.provider.Env, task, agentID, role, providerModel)...)
	raw, err := runCommand(ctx, commandName, args, env, stdin)
	if err != nil {
		return RunTaskResult{Command: command}, err
	}
	if captured, err := readCapturedOutput(); err != nil {
		return RunTaskResult{Command: command}, err
	} else if strings.TrimSpace(captured) != "" {
		raw = captured
	}
	output, err := normalizeStructuredCLIOutput(cliType, outputSchema, raw)
	if err != nil {
		return RunTaskResult{Command: command}, err
	}
	return RunTaskResult{Output: output, Command: command}, nil
}

func buildFinalPrompt(cliType string, task consensus.Task, prompt string, agentID string, role string, providerModel string, reasoning string, temperature *float64) (string, error) {
	finalPrompt := hardenPromptForCLI(cliType, task, prompt)
	if cliType != config.CLITypeGeneric {
		return finalPrompt, nil
	}
	envelope, err := json.MarshalIndent(map[string]any{
		"version": 1,
		"agent": map[string]any{
			"id":            agentID,
			"role":          role,
			"providerModel": providerModel,
			"reasoning":     reasoning,
			"temperature":   temperature,
		},
		"task":   task,
		"prompt": prompt,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal generic cli envelope: %w", err)
	}
	return string(envelope), nil
}

func buildStructuredOutputArgs(cliType string, outputSchema map[string]any) ([]string, func(), error) {
	if len(outputSchema) == 0 {
		return nil, func() {}, nil
	}
	switch cliType {
	case "claude":
		body, err := json.Marshal(outputSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal claude json schema: %w", err)
		}
		return []string{"--json-schema", string(body), "--output-format", "json"}, func() {}, nil
	case "codex":
		body, err := json.MarshalIndent(outputSchema, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("marshal codex output schema: %w", err)
		}
		file, err := os.CreateTemp("", "til-consensus-codex-schema-*.json")
		if err != nil {
			return nil, nil, fmt.Errorf("create codex output schema temp file: %w", err)
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return nil, nil, fmt.Errorf("write codex output schema temp file: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(file.Name())
			return nil, nil, fmt.Errorf("close codex output schema temp file: %w", err)
		}
		return []string{"--output-schema", file.Name()}, func() { _ = os.Remove(file.Name()) }, nil
	default:
		return nil, func() {}, nil
	}
}

func buildStructuredOutputArgsPreview(cliType string, outputSchema map[string]any) ([]string, error) {
	if len(outputSchema) == 0 {
		return nil, nil
	}
	switch cliType {
	case "claude":
		body, err := json.Marshal(outputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal claude json schema preview: %w", err)
		}
		return []string{"--json-schema", string(body), "--output-format", "json"}, nil
	case "codex":
		return []string{"--output-schema", "<schema-file>"}, nil
	default:
		return nil, nil
	}
}

func buildFinalOutputCaptureArgs(cliType string) ([]string, func() (string, error), func(), error) {
	if cliType != "codex" {
		return nil, func() (string, error) { return "", nil }, func() {}, nil
	}
	file, err := os.CreateTemp("", "til-consensus-codex-last-message-*.txt")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create codex output-last-message temp file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, nil, nil, fmt.Errorf("close codex output-last-message temp file: %w", err)
	}
	read := func() (string, error) {
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil
			}
			return "", fmt.Errorf("read codex output-last-message file: %w", err)
		}
		return string(body), nil
	}
	return []string{"--output-last-message", path}, read, func() { _ = os.Remove(path) }, nil
}

func buildFinalOutputCaptureArgsPreview(cliType string) []string {
	if cliType != "codex" {
		return nil
	}
	return []string{"--output-last-message", "<last-message-file>"}
}

func normalizeStructuredCLIOutput(cliType string, outputSchema map[string]any, raw string) (string, error) {
	if cliType != "claude" || len(outputSchema) == 0 {
		return raw, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, nil
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return raw, nil
	}
	if structured, ok := envelope["structured_output"]; ok && structured != nil {
		body, err := json.Marshal(structured)
		if err != nil {
			return "", fmt.Errorf("marshal claude structured_output: %w", err)
		}
		return string(body), nil
	}
	if result, ok := envelope["result"].(string); ok && strings.TrimSpace(result) != "" {
		return result, nil
	}
	return raw, nil
}

func buildBaseArgs(cliType string, providerModel string, prompt string, reasoning string) ([]string, string) {
	switch cliType {
	case "claude":
		args := []string{"--print", "--model", providerModel}
		if reasoning != "" {
			args = append(args, "--effort", reasoning)
		}
		return args, prompt
	case "codex":
		args := []string{"exec", "-m", providerModel, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		if reasoning != "" {
			args = []string{"exec", "-m", providerModel, "-c", "model_reasoning_effort=" + reasoning, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		}
		return args, prompt
	case "gemini":
		return []string{"--approval-mode", "yolo", "-m", providerModel, "-p", prompt}, ""
	case config.CLITypeAntigravity:
		return []string{"--model", providerModel, "-p", prompt}, ""
	case "opencode":
		return []string{"run", prompt, "--dangerously-skip-permissions", "-m", providerModel}, ""
	default:
		return nil, prompt
	}
}

func hardenPromptForCLI(cliType string, task consensus.Task, prompt string) string {
	block := buildCLIOutputContract(cliType, task)
	if strings.TrimSpace(block) == "" {
		return prompt
	}
	return prompt + "\n\n" + block
}

func buildCLIOutputContract(cliType string, task consensus.Task) string {
	switch cliType {
	case "codex", "claude", "gemini", config.CLITypeAntigravity:
	default:
		return ""
	}
	lines := []string{
		"CLI output contract (strict):",
		"- Return exactly one JSON object. The first non-whitespace character must be '{' and the last must be '}'.",
		"- Do not output markdown, code fences, bullets, analysis, or any commentary before or after the JSON object.",
		"- Use exact field names from the schema. Do not rename fields, do not invent aliases, and do not add parallel fields.",
		"- Use exact enum literals from the allowed lists below. Do not use provider-specific synonyms.",
		"- If an optional field is unavailable, omit it or return an empty array/string/object as appropriate. Do not change the shape.",
	}
	lines = append(lines, taskSpecificContract(task)...)
	switch cliType {
	case "codex":
		lines = append(lines,
			"- Codex-specific: suppress all preambles, reflections, and trailing notes. Output JSON only.",
			"- Codex-specific: when a schema field exists, include it explicitly. Use empty strings, empty arrays, 0, or false for not-applicable optional values instead of omitting the field.",
		)
	case "claude":
		lines = append(lines,
			"- Claude-specific: do not wrap the answer in prose or explanatory paragraphs. Output JSON only.",
		)
	case "gemini":
		lines = append(lines,
			"- Gemini-specific: do not replace canonical fields with aliases like verificationStatus/claim/text/accepted/verified.",
		)
	case config.CLITypeAntigravity:
		lines = append(lines,
			"- Antigravity-specific: non-interactive output is parsed from stdout. Do not emit TUI notes, plans, tool logs, or artifact summaries; output JSON only.",
		)
	}
	return strings.Join(lines, "\n")
}

func taskSpecificContract(task consensus.Task) []string {
	switch typed := task.(type) {
	case consensus.ProposalTask, consensus.InitialProposalTask:
		return []string{
			"- Proposal fields: summary, claims[].title, claims[].statement, claims[].claimType, claims[].confidence, claims[].applicability, claims[].boundaryConditions.",
			"- Proposal aliases forbidden: claim, text, statementText.",
			"- Proposal relationship fields forbidden: dependencies, parentClaimIds.",
			"- Proposal claimType allowed: fact, inference, recommendation, assumption.",
			"- Proposal confidence must be a JSON number, not a string.",
			"- Proposal claims must answer the user's task, not this run's debate process, peer claim counts, dedup hygiene, prompt behavior, or system workflow.",
			"- Proposal claim titles/statements must not include status prefixes like [Status: keep], [Status: revise], or 裁决状态：keep.",
		}
	case consensus.ChallengeTask:
		return []string{
			"- Challenge fields: summary, tickets[].claimId, tickets[].statement, tickets[].kind, tickets[].attackType, tickets[].severity.",
			"- Challenge severity allowed: low, medium, high.",
		}
	case consensus.SemanticVerificationTask:
		lines := []string{
			"- Semantic fields: summary, results[].claimId, results[].verdict, results[].confidence, results[].rationale.",
			"- Semantic aliases forbidden: targetId, verificationStatus, reasoning, reason.",
			"- Semantic output must contain exactly one results[] row for the current claim.",
			"- Semantic output must not emit rows for challenges, evidence, or source materials.",
			"- Semantic targetType is optional, but if present it must be claim.",
			"- Semantic verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
			"- Semantic confidence bands: supported/refuted must be 0.60-1.00, insufficient_evidence must be 0.01-0.60, undetermined must be 0.35-0.65.",
			"- Semantic rationale must use exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
			"- Semantic supported_core must name the narrowest surviving claim core, even when the verdict is insufficient_evidence or undetermined.",
			"- Semantic missing_or_conflict must name the concrete missing data or conflicting record. Do not only say \"need more evidence\".",
			"- Semantic verdict_reason must explain why the chosen verdict beats the other three canonical verdicts.",
			"- Semantic recommendation claims should judge the directional recommendation separately from rollout mechanics. Do not downgrade to insufficient_evidence only because execution details remain open.",
			"- Semantic undetermined is only for genuinely mixed evidence. Do not use it as a default safe fallback.",
			"- Semantic insufficient_evidence is preferred when evidence is simply too weak or missing.",
			"- Semantic confidence must be a JSON number, not a string.",
		}
		if typed.Claim.ClaimID != "" {
			lines = append(lines, "- Semantic claimId must be exactly "+typed.Claim.ClaimID+".")
		}
		return lines
	case consensus.SemanticDedupTask:
		return []string{
			"- Semantic dedup fields: summary, merges[].sourceClaimId, merges[].targetClaimId, merges[].similarity, merges[].rationale.",
			"- Semantic dedup must only merge claims whose meaning is equivalent or near-equivalent above threshold.",
			"- Semantic dedup must not merge merely related claims, pro/con pairs, parent/child claims, or claims with different conditions.",
			"- Semantic dedup must not invent claim IDs or rewrite claim text.",
			"- Semantic dedup similarity must be a JSON number, not a string.",
			"- Semantic dedup must not emit chained or cyclic merges where one claim is both sourceClaimId and targetClaimId.",
		}
	case consensus.ReviseTask:
		return []string{
			"- Revise fields: summary, revisions[].targetClaimId, revisions[].action, revisions[].reason, revisions[].confidenceDelta, revisions[].unresolved.",
			"- Revise aliases forbidden: claimId, targetId, rationale, reasoning.",
			"- Revise action allowed: revise, downgrade_confidence, withdraw, mark_unresolved, unchanged.",
			"- Revise targetClaimId must copy an existing claims[].claimId exactly.",
			"- Revise should prefer action=revise when a narrower evidence-backed statement can be written.",
			"- Revise action=revise requires revisedText, and revisedText must be materially narrower than the current claim text.",
			"- Revise action=mark_unresolved requires revisedText and unresolved=true. Use it only when no narrower supported statement can be written.",
			"- Revise confidenceDelta must be a JSON number, not a string.",
		}
	case consensus.DebateRoundTask:
		return []string{
			"- Debate-round fields: summary, newClaims[], judgements[].claimId, judgements[].judgement, judgements[].rationale, judgements[].revisedStatement, judgements[].mergeWithClaims.",
			"- Debate-round claimId must copy an existing peerClaims[].claimId exactly.",
			"- Debate-round aliases forbidden: claim, targetId, verdict, vote, stance, opinion.",
			"- Debate-round judgement allowed: agree, disagree, revise, no_change.",
			"- Debate-round if judgement=revise, revisedStatement is required. Otherwise omit revisedStatement.",
			"- Debate-round prefer one judgement row per peer claim. Use no_change instead of skipping a peer claim you reviewed.",
			"- Debate-round newClaims must be substantive claims about the user's task. Put process/meta observations about peer claim counts, dedup needs, round hygiene, or workflow only in summary.",
			"- Debate-round newClaims titles/statements and revisedStatement must not include status prefixes like [Status: keep], [Status: revise], or 裁决状态：keep.",
		}
	case consensus.FinalVoteTask:
		return []string{
			"- Final-vote fields: summary, votes[].claimId, votes[].vote, votes[].confidence, votes[].rationale.",
			"- Final-vote claimId must copy an existing claims[].claimId exactly.",
			"- Final-vote vote allowed: accept, reject, abstain.",
			"- Final-vote confidence is a continuous support score from 0.0 to 1.0: 0.0 means strongly reject, 0.5 means uncertain/abstain, 1.0 means strongly accept.",
			"- Final-vote confidence must be a JSON number, not a string.",
			"- Final-vote aliases forbidden: targetId, verdict, judgement, stance.",
		}
	case consensus.ArbiterTask:
		return []string{
			"- Arbiter fields: summary, taskVerdict, decisions[].claimId, decisions[].verdict, decisions[].confidence, decisions[].rationale, decisions[].evidenceRefs.",
			"- Arbiter taskVerdict must be a JSON string, not an object.",
			"- Arbiter aliases forbidden: targetClaimId, targetId, reasoning, reason, keyEvidenceRefs.",
			"- Arbiter claim verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
			"- Arbiter task verdict allowed: supported, partially_supported, undetermined, failed.",
			"- Arbiter must judge the current revised claim text as written, not the broader superseded claim.",
			"- Arbiter should prefer supported when a narrowed strategy/operational claim keeps a supported directional core but leaves execution details or prerequisites conditional.",
			"- Arbiter recommendation claims should preserve a supported directional path and carry remaining rollout uncertainty as caveats whenever the claim already encodes those caveats.",
			"- Arbiter should use insufficient_evidence only when no narrower supported core remains even after considering caveats, applicability, and boundaryConditions.",
			"- Arbiter confidence must be a JSON number, not a string.",
		}
	case consensus.DelphiQuestionnaireTask, consensus.DelphiRevisionTask:
		return []string{
			"- Delphi fields: summary, responses[].statement, responses[].rating.",
			"- Delphi rating must be a JSON number, not a string.",
		}
	default:
		return nil
	}
}

func renderArgs(args []string, task consensus.Task, agentID string, role string, providerModel string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, renderTemplate(arg, task, agentID, role, providerModel))
	}
	return out
}

func renderEnv(env map[string]string, task consensus.Task, agentID string, role string, providerModel string) []string {
	out := make([]string, 0, len(env)+6)
	for key, value := range env {
		out = append(out, key+"="+renderTemplate(value, task, agentID, role, providerModel))
	}
	meta := task.Meta()
	out = append(out,
		"TIL_CONSENSUS_TASK_KIND="+string(task.Kind()),
		"TIL_CONSENSUS_REQUEST_ID="+meta.RequestID,
		"TIL_CONSENSUS_SESSION_ID="+meta.SessionID,
		"TIL_CONSENSUS_AGENT_ID="+meta.AgentID,
		"TIL_CONSENSUS_PROVIDER_MODEL="+providerModel,
	)
	if role != "" {
		out = append(out, "TIL_CONSENSUS_ROLE="+role)
	}
	return out
}

func renderTemplate(value string, task consensus.Task, agentID string, role string, providerModel string) string {
	meta := task.Meta()
	value = strings.ReplaceAll(value, "{requestId}", meta.RequestID)
	value = strings.ReplaceAll(value, "{sessionId}", meta.SessionID)
	value = strings.ReplaceAll(value, "{agentId}", meta.AgentID)
	value = strings.ReplaceAll(value, "{taskKind}", string(task.Kind()))
	value = strings.ReplaceAll(value, "{providerModel}", providerModel)
	value = strings.ReplaceAll(value, "{role}", role)
	return value
}

func runCommand(ctx context.Context, command string, args []string, env []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli provider failed: %w stderr=%s stdout=%s", err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}
	return stdout.String(), nil
}
