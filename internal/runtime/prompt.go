package runtime

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func BuildTaskPrompt(task consensus.Task, agent ResolvedAgentRuntime, includeJSONSchema bool) string {
	sections := []string{
		"You are executing one isolated task inside til-consensus.",
		"Treat this as a one-shot assignment. Do not assume any hidden memory or prior turns.",
		"Return exactly one JSON object only. Do not add markdown, code fences, or extra commentary.",
	}
	if agent.Role != "" {
		sections = append(sections, "Role: "+agent.Role)
	}
	if agent.SystemPrompt != "" {
		sections = append(sections, "", "System instructions:", agent.SystemPrompt)
	}
	sections = append(sections, "", "Task context JSON:")
	taskJSON, _ := json.MarshalIndent(task, "", "  ")
	sections = append(sections, string(taskJSON))
	if includeJSONSchema {
		schemaJSON, _ := json.MarshalIndent(TaskOutputJSONSchema(task), "", "  ")
		sections = append(sections, "", "Expected output JSON schema:", string(schemaJSON))
	}
	if hints := taskPromptHints(task); len(hints) > 0 {
		sections = append(sections, "", "Task-specific output rules:")
		sections = append(sections, hints...)
	}
	return strings.Join(sections, "\n")
}

func BuildRepairPrompt(task consensus.Task, agent ResolvedAgentRuntime, rawOutput string, decodeErr error, includeJSONSchema bool) string {
	sections := []string{
		"You are repairing your own previous output for the same til-consensus task.",
		"Do not change the business judgment, claim content, verdict meaning, or reasoning intent unless the previous output was structurally invalid.",
		"Only fix JSON/schema problems so the output can be parsed successfully.",
		"Return exactly one JSON object only. Do not add markdown, code fences, or extra commentary.",
	}
	if agent.Role != "" {
		sections = append(sections, "Role: "+agent.Role)
	}
	if agent.SystemPrompt != "" {
		sections = append(sections, "", "System instructions:", agent.SystemPrompt)
	}
	sections = append(sections,
		"",
		"Task context JSON:",
	)
	taskJSON, _ := json.MarshalIndent(task, "", "  ")
	sections = append(sections, string(taskJSON))
	if includeJSONSchema {
		schemaJSON, _ := json.MarshalIndent(TaskOutputJSONSchema(task), "", "  ")
		sections = append(sections, "", "Expected output JSON schema:", string(schemaJSON))
	}
	if decodeErr != nil {
		sections = append(sections, "", "The previous output caused this system error:", decodeErr.Error())
	}
	sections = append(sections,
		"",
		"Previous raw output to repair:",
		rawOutput,
		"",
		"Repair instructions:",
		"- Keep the same semantic meaning as much as possible.",
		"- Do not invent new evidence, rationale, or fields unless required to satisfy the schema using the information already present.",
		"- If a field used the wrong alias but the canonical target is exact and unambiguous, rename it to the canonical schema field.",
		"- Do not broaden, narrow, or reinterpret verdict strength just to satisfy the schema.",
		"- If a required field is missing but the value is clearly present under another key, move it instead of rewriting the content.",
	)
	if hints := taskRepairHints(task); len(hints) > 0 {
		sections = append(sections, "", "Task-specific repair rules:")
		sections = append(sections, hints...)
	}
	return strings.Join(sections, "\n")
}

func taskPromptHints(task consensus.Task) []string {
	switch typed := task.(type) {
	case consensus.DebateRoundTask:
		return debateRoundPromptHints(typed)
	default:
		return nil
	}
}

func taskRepairHints(task consensus.Task) []string {
	switch typed := task.(type) {
	case consensus.DebateRoundTask:
		return debateRoundRepairHints(typed)
	default:
		return nil
	}
}

func debateRoundPromptHints(task consensus.DebateRoundTask) []string {
	validIDs := debateRoundPeerClaimIDs(task)
	lines := []string{
		"- The canonical debate-round fields are: summary, newClaims, judgements[].claimId, judgements[].judgement, judgements[].rationale, judgements[].revisedStatement, judgements[].mergeWithClaims.",
		"- judgements[].claimId must copy an existing peerClaims[].claimId exactly. Never invent claim IDs and never use aliases like claim, targetId, verdict, stance, or opinion.",
		"- judgements[].judgement must be exactly one of: agree, disagree, revise, no_change.",
		"- If judgement is revise, revisedStatement is required and must contain the revised claim text.",
		"- If judgement is not revise, omit revisedStatement entirely.",
		"- Prefer one judgement entry per peer claim in this round. If you want to keep a peer claim unchanged, use judgement=no_change.",
	}
	if len(validIDs) == 0 {
		lines = append(lines,
			"- There are no peer claims in this round. Return judgements: [].",
			`- Valid empty example: {"summary":"No peer claims this round.","judgements":[]}`,
		)
		return lines
	}
	lines = append(lines, fmt.Sprintf("- Valid peer claim IDs for this task: %s.", strings.Join(validIDs, ", ")))
	lines = append(lines, "- Use only those IDs in judgements[].claimId.")
	lines = append(lines, debateRoundExampleLines(validIDs)...)
	return lines
}

func debateRoundRepairHints(task consensus.DebateRoundTask) []string {
	lines := []string{
		"- Preserve the same debate intent, but rewrite every judgement row into canonical fields only.",
		"- Do not invent new claim IDs. If a row is about a peer claim, claimId must be copied verbatim from the valid peer claim IDs listed below.",
		"- Replace any non-canonical judgement literal with the closest canonical value: agree, disagree, revise, or no_change, while preserving the original stance.",
		"- If a row intends to propose a textual rewrite, use judgement=revise and move the rewritten text into revisedStatement.",
	}
	validIDs := debateRoundPeerClaimIDs(task)
	if len(validIDs) == 0 {
		lines = append(lines, "- This task has no peer claims, so the repaired output must use judgements: [].")
		return lines
	}
	lines = append(lines, fmt.Sprintf("- Valid peer claim IDs for repair: %s.", strings.Join(validIDs, ", ")))
	return lines
}

func debateRoundPeerClaimIDs(task consensus.DebateRoundTask) []string {
	ids := make([]string, 0, len(task.PeerClaims))
	for _, claim := range task.PeerClaims {
		claimID := strings.TrimSpace(claim.ClaimID)
		if claimID == "" || slices.Contains(ids, claimID) {
			continue
		}
		ids = append(ids, claimID)
	}
	return ids
}

func debateRoundExampleLines(validIDs []string) []string {
	exampleID := validIDs[0]
	lines := []string{
		fmt.Sprintf(`- Valid example: {"summary":"Compared peer claims and kept one unchanged.","judgements":[{"claimId":"%s","judgement":"no_change","rationale":"No stronger objection this round."}]}`, exampleID),
		fmt.Sprintf(`- Invalid example: {"summary":"...","judgements":[{"claim":"%s","verdict":"accept"}]}`, exampleID),
	}
	if len(validIDs) > 1 {
		lines[0] = fmt.Sprintf(`- Valid example: {"summary":"Compared peer claims and proposed one revision.","judgements":[{"claimId":"%s","judgement":"disagree","rationale":"The operational cost is understated."},{"claimId":"%s","judgement":"revise","rationale":"Narrow the scope to teams already using trunk-based workflows.","revisedStatement":"A monorepo is a better default only when the organization already has strong trunk-based development and shared build tooling."}]}`, validIDs[0], validIDs[1])
		lines = append(lines, fmt.Sprintf(`- Invalid example: {"summary":"...","judgements":[{"claimId":"%s","judgement":"accepted"},{"claimId":"%s","judgement":"support"}]}`, validIDs[0], validIDs[1]))
	}
	return lines
}
