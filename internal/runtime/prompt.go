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
		schemaJSON, _ := json.MarshalIndent(TaskOutputJSONSchemaForAgent(task, agent), "", "  ")
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
		schemaJSON, _ := json.MarshalIndent(TaskOutputJSONSchemaForAgent(task, agent), "", "  ")
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
	case consensus.ProposalTask:
		return proposalPromptHints()
	case consensus.InitialProposalTask:
		return proposalPromptHints()
	case consensus.SemanticVerificationTask:
		return semanticVerificationPromptHints(typed)
	case consensus.DebateRoundTask:
		return debateRoundPromptHints(typed)
	default:
		return nil
	}
}

func taskRepairHints(task consensus.Task) []string {
	switch typed := task.(type) {
	case consensus.ProposalTask:
		return proposalRepairHints()
	case consensus.InitialProposalTask:
		return proposalRepairHints()
	case consensus.SemanticVerificationTask:
		return semanticVerificationRepairHints(typed)
	case consensus.DebateRoundTask:
		return debateRoundRepairHints(typed)
	default:
		return nil
	}
}

func proposalPromptHints() []string {
	return []string{
		"- Proposal claim fields are limited to: title, statement, claimType, confidence, applicability, boundaryConditions.",
		"- Do not emit dependencies or parentClaimIds in proposal outputs.",
		"- If a claim only applies under certain assumptions, put those assumptions in applicability or boundaryConditions instead of creating claim graph references.",
	}
}

func proposalRepairHints() []string {
	return []string{
		"- If the previous output used dependencies or parentClaimIds, remove those fields.",
		"- Preserve prerequisites as plain language in applicability or boundaryConditions when they are already stated in the previous output.",
	}
}

func semanticVerificationPromptHints(task consensus.SemanticVerificationTask) []string {
	claimID := strings.TrimSpace(task.Claim.ClaimID)
	lines := []string{
		"- Return exactly one semantic result row for the current claim. Do not emit separate rows for challenges, source materials, or other targets.",
		"- If targetType is included, it must be exactly \"claim\".",
		"- supported: use only when the current materials directly back the claim after considering caveats and open challenges.",
		"- supported confidence must be between 0.60 and 1.00.",
		"- refuted: use when the claim is contradicted, materially overstated, or internally inconsistent with the available materials.",
		"- refuted confidence must be between 0.60 and 1.00.",
		"- insufficient_evidence: use when the claim may be plausible but the available materials are too weak to support or refute it.",
		"- insufficient_evidence confidence must be between 0.01 and 0.60.",
		"- undetermined: use only for genuinely mixed or ambiguous evidence after considering the full record; do not use it as a safe default.",
		"- undetermined confidence must stay between 0.35 and 0.65.",
		"- Prefer supported, refuted, or insufficient_evidence whenever the evidence direction is clear.",
		"- confidence measures certainty in the verdict classification, not confidence that the underlying project decision is good.",
		"- rationale must explain why this verdict follows from the current claim and challenges; do not only say \"need more evidence\".",
	}
	if claimID != "" {
		lines = append(lines, fmt.Sprintf("- The only valid claimId for this task is %s.", claimID))
		lines = append(lines, semanticVerificationExampleLines(claimID)...)
		lines = append(lines, fmt.Sprintf(`- Invalid example: {"summary":"...","results":[{"claimId":"challenge-1","targetType":"challenge","verdict":"supported","rationale":"looked relevant"},{"claimId":"%s","verdict":"undetermined","rationale":"Need more evidence"}]}`, claimID))
	}
	return lines
}

func semanticVerificationRepairHints(task consensus.SemanticVerificationTask) []string {
	claimID := strings.TrimSpace(task.Claim.ClaimID)
	lines := []string{
		"- Rewrite the output to exactly one canonical result row for the current claim.",
		"- Drop any rows about challenges, evidence, or source materials. Keep only the claim-level judgement.",
		"- If targetType is present, set it to \"claim\".",
		"- If the previous verdict was a vague safe fallback, choose insufficient_evidence when support is missing, and choose undetermined only when the evidence is genuinely mixed.",
		"- Repair confidence to match the canonical verdict bands: supported/refuted 0.60-1.00, insufficient_evidence 0.01-0.60, undetermined 0.35-0.65.",
		"- Preserve the original judgment intent, but make the rationale concrete and claim-focused.",
	}
	if claimID != "" {
		lines = append(lines, fmt.Sprintf("- The repaired results[0].claimId must be exactly %s.", claimID))
	}
	return lines
}

func semanticVerificationExampleLines(claimID string) []string {
	return []string{
		fmt.Sprintf(`- Valid supported example: {"summary":"The current claim is directly backed by the record.","results":[{"claimId":"%s","targetType":"claim","verdict":"supported","confidence":0.78,"rationale":"The source materials explicitly document repeated cross-repo coordination delays, so the claim's diagnosis is directly supported."}]}`, claimID),
		fmt.Sprintf(`- Valid refuted example: {"summary":"The current claim overstates what the record proves.","results":[{"claimId":"%s","targetType":"claim","verdict":"refuted","confidence":0.73,"rationale":"The claim says the migration will eliminate version drift, but the materials only show it could reduce drift under additional governance constraints."}]}`, claimID),
		fmt.Sprintf(`- Valid insufficient_evidence example: {"summary":"The current claim is plausible but under-supported.","results":[{"claimId":"%s","targetType":"claim","verdict":"insufficient_evidence","confidence":0.42,"rationale":"The record shows coordination pain, but it does not quantify whether monorepo would improve throughput enough to justify the migration."}]}`, claimID),
		fmt.Sprintf(`- Valid undetermined example: {"summary":"The current claim has genuinely mixed evidence.","results":[{"claimId":"%s","targetType":"claim","verdict":"undetermined","confidence":0.5,"rationale":"The materials support the diagnosis of repository friction, but they also show unresolved release-governance tradeoffs that could negate the claimed benefit."}]}`, claimID),
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
