package runtime

import (
	"encoding/json"
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
		"- If a field used the wrong alias, rename it to the canonical schema field.",
		"- If an enum value was invalid, replace it with the closest canonical schema value without changing the underlying intent.",
		"- If a required field is missing but the value is clearly present under another key, move it instead of rewriting the content.",
	)
	return strings.Join(sections, "\n")
}
