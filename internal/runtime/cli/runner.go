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

func NewRunner(provider config.ProviderConfig) *Runner {
	return &Runner{provider: provider}
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
) (string, error) {
	cliType := r.provider.CLIType
	if cliType == "" {
		cliType = config.CLITypeGeneric
	}
	finalPrompt := hardenPromptForCLI(cliType, task, prompt)
	if cliType == config.CLITypeGeneric {
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
		finalPrompt = string(envelope)
	}
	commandName := r.provider.Command
	if commandName == "" {
		commandName = cliType
	}
	args, stdin := buildBaseArgs(cliType, providerModel, finalPrompt, reasoning)
	args = append(args, renderArgs(r.provider.Args, task, agentID, role, providerModel)...)
	env := append(os.Environ(), renderEnv(r.provider.Env, task, agentID, role, providerModel)...)
	return runCommand(ctx, commandName, args, env, stdin)
}

func buildBaseArgs(cliType string, providerModel string, prompt string, reasoning string) ([]string, string) {
	switch cliType {
	case "claude":
		return []string{"--print", "--model", providerModel}, prompt
	case "codex":
		args := []string{"exec", "-m", providerModel, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		if reasoning != "" {
			args = []string{"exec", "-m", providerModel, "-c", "model_reasoning_effort=" + reasoning, "--full-auto", "--color", "never", "--skip-git-repo-check"}
		}
		return args, prompt
	case "gemini":
		return []string{"--approval-mode", "yolo", "-m", providerModel}, prompt
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
	case "codex", "claude", "gemini":
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
		)
	case "claude":
		lines = append(lines,
			"- Claude-specific: do not wrap the answer in prose or explanatory paragraphs. Output JSON only.",
		)
	case "gemini":
		lines = append(lines,
			"- Gemini-specific: do not replace canonical fields with aliases like verificationStatus/claim/text/accepted/verified.",
		)
	}
	return strings.Join(lines, "\n")
}

func taskSpecificContract(task consensus.Task) []string {
	switch task.(type) {
	case consensus.ProposalTask, consensus.InitialProposalTask:
		return []string{
			"- Proposal fields: summary, claims[].statement, claims[].claimType, claims[].confidence.",
			"- Proposal aliases forbidden: claim, text, statementText.",
			"- Proposal claimType allowed: fact, inference, recommendation, assumption.",
			"- Proposal confidence must be a JSON number, not a string.",
		}
	case consensus.ChallengeTask:
		return []string{
			"- Challenge fields: summary, tickets[].claimId, tickets[].statement, tickets[].kind, tickets[].attackType, tickets[].severity.",
			"- Challenge severity allowed: low, medium, high.",
		}
	case consensus.SemanticVerificationTask:
		return []string{
			"- Semantic fields: summary, results[].claimId, results[].verdict, results[].confidence, results[].rationale.",
			"- Semantic aliases forbidden: targetId, verificationStatus, reasoning, reason.",
			"- Semantic verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
			"- Semantic confidence must be a JSON number, not a string.",
		}
	case consensus.ReviseTask:
		return []string{
			"- Revise fields: summary, revisions[].targetClaimId, revisions[].action, revisions[].reason, revisions[].confidenceDelta, revisions[].unresolved.",
			"- Revise aliases forbidden: claimId, targetId, rationale, reasoning.",
			"- Revise action allowed: revise, downgrade_confidence, withdraw, mark_unresolved, unchanged.",
			"- Revise confidenceDelta must be a JSON number, not a string.",
		}
	case consensus.ArbiterTask:
		return []string{
			"- Arbiter fields: summary, taskVerdict, decisions[].claimId, decisions[].verdict, decisions[].confidence, decisions[].rationale, decisions[].evidenceRefs.",
			"- Arbiter taskVerdict must be a JSON string, not an object.",
			"- Arbiter aliases forbidden: targetClaimId, targetId, reasoning, reason, keyEvidenceRefs.",
			"- Arbiter claim verdict allowed: supported, refuted, insufficient_evidence, undetermined.",
			"- Arbiter task verdict allowed: supported, partially_supported, undetermined, failed.",
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
