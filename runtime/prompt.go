package runtime

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
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
	case consensus.ReviseTask:
		return revisePromptHints(typed)
	case consensus.ArbiterTask:
		return arbiterPromptHints(typed)
	case consensus.DebateRoundTask:
		return debateRoundPromptHints(typed)
	case consensus.SemanticDedupTask:
		return semanticDedupPromptHints(typed)
	case consensus.FinalVoteTask:
		return finalVotePromptHints(typed)
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
	case consensus.ReviseTask:
		return reviseRepairHints(typed)
	case consensus.ArbiterTask:
		return arbiterRepairHints(typed)
	case consensus.DebateRoundTask:
		return debateRoundRepairHints(typed)
	case consensus.SemanticDedupTask:
		return semanticDedupRepairHints(typed)
	case consensus.FinalVoteTask:
		return finalVoteRepairHints()
	default:
		return nil
	}
}

func proposalPromptHints() []string {
	return []string{
		"- Proposal claim fields are limited to: title, statement, claimType, confidence, applicability, boundaryConditions.",
		"- Do not emit dependencies or parentClaimIds in proposal outputs.",
		"- If a claim only applies under certain assumptions, put those assumptions in applicability or boundaryConditions instead of creating claim graph references.",
		"- Claims must answer the user's task. Do not create claims about this run's debate process, peer claim counts, dedup hygiene, prompt behavior, or system workflow.",
		"- Do not prefix claim titles or statements with status labels such as [Status: keep], [Status: revise], or 裁决状态：keep. Status belongs in judgement/action fields, not claim text.",
	}
}

func proposalRepairHints() []string {
	return []string{
		"- If the previous output used dependencies or parentClaimIds, remove those fields.",
		"- Preserve prerequisites as plain language in applicability or boundaryConditions when they are already stated in the previous output.",
		"- Remove claims about debate process, peer claim counts, dedup hygiene, prompt behavior, or system workflow. If useful, move them into summary only.",
		"- Remove status prefixes from claim titles and statements. Keep only the substantive claim text.",
	}
}

func semanticDedupPromptHints(task consensus.SemanticDedupTask) []string {
	threshold := fmt.Sprintf("%.2f", task.SimilarityThreshold)
	return []string{
		"- Semantic dedup compares only the provided active claims.",
		"- Return merges only when two claims have the same practical meaning or one is a strict paraphrase of the other.",
		"- Do not merge claims that are merely related, complementary, or in tension.",
		"- Do not create, rewrite, split, or delete claim text. Only emit sourceClaimId -> targetClaimId merge decisions.",
		"- targetClaimId must be the better canonical claim: clearer, broader provenance, or earlier round if otherwise equal.",
		"- similarity must be a JSON number and must be >= " + threshold + ".",
		"- Every sourceClaimId and targetClaimId must exactly match one input claimId.",
		"- Each sourceClaimId may appear at most once.",
		"- A claim must not appear as both sourceClaimId and targetClaimId across merges; do not emit chained or cyclic merges.",
		"- If there are no semantic duplicates above the threshold, return merges as an empty array.",
	}
}

func semanticDedupRepairHints(task consensus.SemanticDedupTask) []string {
	return []string{
		"- Keep the same merge intent, but remove any merge below the similarity threshold.",
		"- Do not invent new claim IDs. Use only claim IDs from the task context.",
		"- Remove chained or cyclic merges where a claim appears as both sourceClaimId and targetClaimId.",
		"- Keep merges as sourceClaimId, targetClaimId, similarity, rationale.",
	}
}

func finalVotePromptHints(task consensus.FinalVoteTask) []string {
	ids := make([]string, 0, len(task.Claims))
	for _, claim := range task.Claims {
		if strings.TrimSpace(claim.ClaimID) != "" {
			ids = append(ids, claim.ClaimID)
		}
	}
	lines := []string{
		"- Final vote must return one votes[] row for each active claim you evaluate.",
		"- Each vote requires claimId, vote, confidence, and rationale.",
		"- vote is the coarse label: accept, reject, or abstain.",
		"- confidence is the continuous support score from 0.0 to 1.0: 0.0 strongly rejects the claim, 0.5 is uncertain/abstain, 1.0 strongly accepts the claim.",
		"- confidence is NOT certainty in your vote label. Keep them coherent: vote=accept requires confidence >= 0.5, vote=reject requires confidence <= 0.5, vote=abstain should sit near 0.5.",
		"- Use intermediate confidence values to express partial support, caveated support, or live disagreement; do not collapse everything to 0 or 1.",
		"- Calibrate scores comparatively across the whole ballot: the claims you would actually stake the conclusion on belong near the top of your range, weaker or more redundant ones lower. Giving every claim the same confidence is invalid.",
		"- confidence must be a JSON number, not a string.",
	}
	if len(ids) > 0 {
		lines = append(lines,
			"- Valid final-vote claim IDs for this task: "+strings.Join(ids, ", ")+".",
			`- Valid example: {"summary":"Final vote with continuous confidence scores.","votes":[{"claimId":"`+ids[0]+`","vote":"accept","confidence":0.74,"rationale":"The claim is directionally sound but still depends on execution constraints."}]}`,
		)
	}
	return lines
}

func finalVoteRepairHints() []string {
	return []string{
		"- Add a numeric confidence field to every votes[] row.",
		"- Repair confidence into the inclusive [0,1] range.",
		"- Keep vote as accept, reject, or abstain, but use confidence to preserve partial support.",
		"- If a row's vote label contradicts its confidence (accept with confidence < 0.5, or reject with confidence > 0.5), the confidence was probably meant as certainty in the label. Reconcile using the rationale: keep the vote label and move confidence into the matching band (accept: 0.5-1.0 where higher means stronger support; reject: 0.0-0.5 where lower means stronger rejection).",
		"- If every vote carries an identical confidence, spread the scores to reflect relative strength while keeping each vote label.",
		"- Do not quote confidence numbers.",
	}
}

func semanticVerificationPromptHints(task consensus.SemanticVerificationTask) []string {
	claimID := strings.TrimSpace(task.Claim.ClaimID)
	lines := []string{
		"- Return exactly one semantic result row for the current claim. Do not emit separate rows for challenges, source materials, or other targets.",
		"- If targetType is included, it must be exactly \"claim\".",
		"- Before choosing a verdict, identify the narrowest evidence-backed core that still survives in the current claim as written.",
		"- For recommendation claims, separate the directional recommendation from rollout mechanics. If the directional core is supported and the claim already exposes its caveats, do not downgrade to insufficient_evidence just because execution details remain open.",
		"- For inference claims, distinguish supported observations from unsupported causal strength. If only the causal strength is uncertain, keep the supported core explicit instead of defaulting to undetermined.",
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
		"- rationale must use exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
		"- The supported_core clause must name the surviving claim core, even when the final verdict is insufficient_evidence or undetermined.",
		"- The missing_or_conflict clause must name the concrete missing data or conflicting record; do not only say \"need more evidence\".",
		"- The verdict_reason clause must explain why this verdict wins over the other three canonical verdicts.",
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
		"- Rewrite rationale into exactly this structure: supported_core: ... | missing_or_conflict: ... | verdict_reason: ...",
		"- Do not leave supported_core blank. Even for insufficient_evidence or undetermined, say which narrow core remains supported or plausible.",
	}
	if claimID != "" {
		lines = append(lines, fmt.Sprintf("- The repaired results[0].claimId must be exactly %s.", claimID))
	}
	return lines
}

func semanticVerificationExampleLines(claimID string) []string {
	return []string{
		fmt.Sprintf(`- Valid supported example: {"summary":"The current claim is directly backed by the record.","results":[{"claimId":"%s","targetType":"claim","verdict":"supported","confidence":0.78,"rationale":"supported_core: The record directly documents repeated cross-repo coordination delays. | missing_or_conflict: none beyond already-stated caveats. | verdict_reason: The directional diagnosis survives as written, so supported is stronger than insufficient_evidence or undetermined."}]}`, claimID),
		fmt.Sprintf(`- Valid refuted example: {"summary":"The current claim overstates what the record proves.","results":[{"claimId":"%s","targetType":"claim","verdict":"refuted","confidence":0.73,"rationale":"supported_core: The materials only support a potential reduction in version drift under added governance constraints. | missing_or_conflict: The claim says migration will eliminate drift outright, which the record does not support. | verdict_reason: Because the claim overstates the evidence rather than merely lacking detail, refuted is stronger than insufficient_evidence."}]}`, claimID),
		fmt.Sprintf(`- Valid insufficient_evidence example: {"summary":"The current claim is plausible but under-supported.","results":[{"claimId":"%s","targetType":"claim","verdict":"insufficient_evidence","confidence":0.42,"rationale":"supported_core: The record supports that repository coordination pain exists and that phased consolidation remains a plausible path. | missing_or_conflict: The materials do not quantify whether the migration benefit is large enough to justify the cost for this organization. | verdict_reason: The evidence direction is incomplete rather than conflicting, so insufficient_evidence fits better than supported or undetermined."}]}`, claimID),
		fmt.Sprintf(`- Valid undetermined example: {"summary":"The current claim has genuinely mixed evidence.","results":[{"claimId":"%s","targetType":"claim","verdict":"undetermined","confidence":0.5,"rationale":"supported_core: The materials support the diagnosis of repository friction and some case for gradual convergence. | missing_or_conflict: Other materials show unresolved release-governance tradeoffs that could erase the claimed benefit. | verdict_reason: The record points in two live directions at once, so undetermined is more accurate than supported or insufficient_evidence."}]}`, claimID),
	}
}

func revisePromptHints(task consensus.ReviseTask) []string {
	validIDs := reviseClaimIDs(task.Claims)
	lines := []string{
		"- Revisions are for shrinking or clarifying a claim into the smallest statement still supported by the current record.",
		"- Prefer action=revise when you can remove unsupported numbers, timelines, universals, causal strength, or rollout scope while preserving the evidence-backed core.",
		"- Use action=mark_unresolved only when no narrower, still-useful statement can be written without depending on missing or conflicting evidence.",
		"- revise and mark_unresolved both require revisedText. The revisedText must be materially narrower than the current claim text, not a restatement.",
		"- mark_unresolved also requires unresolved=true.",
		"- If only confidence should change and the claim text remains usable, use downgrade_confidence instead of mark_unresolved.",
		"- If the claim survives after splitting fact from attribution, put the supported fact in revisedText instead of leaving the whole claim unresolved.",
	}
	if len(validIDs) == 0 {
		lines = append(lines, "- There are no active claims in this task. Return revisions: [].")
		return lines
	}
	lines = append(lines, fmt.Sprintf("- Valid targetClaimId values for this task: %s.", strings.Join(validIDs, ", ")))
	lines = append(lines, reviseExampleLines(validIDs)...)
	return lines
}

func reviseRepairHints(task consensus.ReviseTask) []string {
	lines := []string{
		"- Repair revisions so they preserve the same evidence-backed intent, but prefer narrower revisedText over unresolved whenever a smaller supported claim can be stated.",
		"- For revise or mark_unresolved, write revisedText that removes unsupported specifics instead of repeating the original claim.",
		"- Only keep mark_unresolved when the remaining uncertainty is essential even after narrowing the claim.",
		"- mark_unresolved requires unresolved=true and revisedText.",
	}
	validIDs := reviseClaimIDs(task.Claims)
	if len(validIDs) == 0 {
		lines = append(lines, "- There are no active claims in this task. The repaired output must use revisions: [].")
		return lines
	}
	lines = append(lines, fmt.Sprintf("- Valid targetClaimId values for repair: %s.", strings.Join(validIDs, ", ")))
	return lines
}

func reviseClaimIDs(claims []consensus.ClaimNode) []string {
	ids := make([]string, 0, len(claims))
	for _, claim := range claims {
		claimID := strings.TrimSpace(claim.ClaimID)
		if claimID == "" || slices.Contains(ids, claimID) {
			continue
		}
		ids = append(ids, claimID)
	}
	return ids
}

func reviseExampleLines(validIDs []string) []string {
	exampleID := validIDs[0]
	lines := []string{
		fmt.Sprintf(`- Valid revise example: {"summary":"Narrowed the claim to the evidence-backed core.","revisions":[{"targetClaimId":"%s","action":"revise","revisedText":"Cross-repo changes currently create measurable coordination overhead, but the available record does not yet prove monorepo is the right remedy.","reason":"Removed unsupported causal strength and recommendation language.","confidenceDelta":-0.05}]}`, exampleID),
		fmt.Sprintf(`- Valid unresolved example: {"summary":"Kept only a constrained candidate path.","revisions":[{"targetClaimId":"%s","action":"mark_unresolved","revisedText":"A phased monorepo migration remains a candidate path, but it should stay unresolved until team capacity, rollout scope, and independent-release requirements are quantified.","reason":"No narrower execution-ready statement is justified by the current evidence.","confidenceDelta":-0.08,"unresolved":true}]}`, exampleID),
		fmt.Sprintf(`- Invalid example: {"summary":"...","revisions":[{"targetClaimId":"%s","action":"mark_unresolved","reason":"Need more evidence"}]}`, exampleID),
	}
	return lines
}

func arbiterPromptHints(task consensus.ArbiterTask) []string {
	lines := []string{
		"- Arbiter decisions should judge the current revised claim text as written, not the broader earlier claim it replaced.",
		"- Prefer verdict=supported when the revised claim already narrows itself to the evidence-backed directional core and explicitly leaves implementation details, rollout sequencing, or prerequisites unresolved.",
		"- For recommendation claims, first ask whether a directional candidate path is still supported after all narrowing. If yes, keep that directional core and carry remaining execution uncertainty as caveats.",
		"- Use insufficient_evidence only when no stable supported core remains even after considering the claim's caveats, applicability, and boundaryConditions.",
		"- For strategy or operational recommendations, 'direction is supported but path details remain conditional' should usually be treated as supported with caveats, not as wholly unresolved.",
		"- Use undetermined only for genuinely mixed or conflicting evidence, not merely because some execution details are still unknown.",
		"- recommendation claims should fall to insufficient_evidence or undetermined only when the directional core itself cannot be defended.",
		"- rationale must explain whether the supported core survives, which caveats still gate execution, and why the verdict is not simply unresolved by default.",
	}
	if lines = append(lines, arbiterExampleLines(task.Claims)...); len(lines) > 0 {
		return lines
	}
	return nil
}

func arbiterRepairHints(task consensus.ArbiterTask) []string {
	lines := []string{
		"- Repair arbiter output so it preserves claim-level meaning but avoids collapsing caveated directional support into insufficient_evidence when the revised claim already encodes its own uncertainty.",
		"- If the revised claim text is cautiously worded and the evidence supports that cautious core, prefer verdict=supported with caveat-style rationale.",
		"- For recommendation claims, preserve the supported directional path whenever it survives, and move the remaining execution uncertainty into caveats instead of downgrading the whole claim.",
		"- Keep insufficient_evidence or undetermined only when no narrower supported core remains after reading the revised claim text, caveats, applicability, and boundaryConditions.",
	}
	return append(lines, arbiterExampleLines(task.Claims)...)
}

func arbiterExampleLines(claims []consensus.ClaimNode) []string {
	for _, claim := range claims {
		if claim.ClaimType == consensus.ClaimTypeFact {
			continue
		}
		if strings.TrimSpace(claim.Statement) == "" {
			continue
		}
		if strings.TrimSpace(claim.Applicability) == "" && len(claim.Caveats) == 0 && len(claim.BoundaryConditions) == 0 {
			continue
		}
		return []string{
			fmt.Sprintf(`- Valid caveated support example: {"summary":"The narrowed strategy claim keeps a supported directional core.","taskVerdict":"partially_supported","decisions":[{"claimId":"%s","verdict":"supported","confidence":0.68,"rationale":"The revised claim already limits itself to a directional recommendation and explicitly names the missing prerequisites, so the directional core is supported while execution remains gated.","evidenceRefs":["ledger-1"]}]}`, claim.ClaimID),
			fmt.Sprintf(`- Invalid over-downgrade example: {"summary":"...","taskVerdict":"undetermined","decisions":[{"claimId":"%s","verdict":"insufficient_evidence","confidence":0.45,"rationale":"Some rollout details are still unknown.","evidenceRefs":["ledger-1"]}]}`, claim.ClaimID),
		}
	}
	return nil
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
		"- newClaims must be substantive claims about the user's task. Do not put process/meta observations about this run, peer claim counts, dedup needs, round hygiene, or system workflow into newClaims; put those observations in summary only.",
		"- Never restate an existing claim (yours or a peer's) in different words as a newClaim; use judgements to agree, revise, or merge instead.",
		"- Do not prefix newClaims titles or statements with status labels such as [Status: keep], [Status: revise], or 裁决状态：keep.",
	}
	switch {
	case task.MaxNewClaims < 0:
		lines = append(lines, "- The active-claim budget for this debate is full: newClaims must be an empty array this round. Express positions through judgements and merges only.")
	case task.MaxNewClaims > 0:
		lines = append(lines, fmt.Sprintf("- Propose at most %d newClaims this round; entries beyond that budget are discarded by the coordinator. Spend the budget only on genuinely new positions.", task.MaxNewClaims))
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
		"- Remove any newClaims entries that are about this run's process, peer claim counts, dedup hygiene, round hygiene, or system workflow. Preserve them in summary only if they are useful.",
		"- Remove status prefixes from newClaims titles, newClaims statements, and revisedStatement. Keep only substantive claim text.",
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
