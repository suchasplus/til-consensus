package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func BuildTaskPrompt(task consensus.Task, agent ResolvedAgentRuntime, includeJSONSchema bool) string {
	if action, ok := task.(consensus.ActionTask); ok {
		return buildActionPrompt(action, agent)
	}
	sections := []string{
		"You are executing one task in the til-consensus CLI host.",
		"Return one JSON object only. Do not add markdown, code fences, or commentary.",
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

func buildActionPrompt(task consensus.ActionTask, agent ResolvedAgentRuntime) string {
	sections := []string{
		"You are executing a follow-up action after a consensus session.",
	}
	if agent.Role != "" {
		sections = append(sections, "Role: "+agent.Role)
	}
	if agent.SystemPrompt != "" {
		sections = append(sections, "", "System instructions:", agent.SystemPrompt)
	}
	sections = append(sections,
		"",
		"Action instructions:",
		task.Prompt,
		"",
		"Consensus summary:",
		fmt.Sprintf("Status: %s", task.Input.Status),
		task.Input.FinalSummary,
		"",
		"Representative speech:",
		task.Input.RepresentativeSpeech,
	)
	return strings.Join(sections, "\n")
}
