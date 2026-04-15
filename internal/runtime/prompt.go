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
