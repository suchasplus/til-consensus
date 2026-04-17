package config

import (
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

const (
	TemplatePresetQuickstart = "quickstart"
	TemplatePresetOpenAI     = "openai"
	TemplatePresetCoding     = "coding"
	TemplatePresetDebate     = "debate"
	TemplatePresetDelphi     = "delphi"
	TemplatePresetGeneric    = "generic"
	TemplatePresetCodex      = "codex"
	TemplatePresetClaude     = "claude"
	TemplatePresetGemini     = "gemini"
)

func InitTemplate() Config {
	return Config{
		SchemaVersion: 1,
		Defaults: DefaultsConfig{
			SuccessCriteria:   []string{"给出 claim 级裁决", "对证据不足部分明确保留 unresolved 或 undetermined"},
			AllowedTools:      []string{"sources", "compare", "cross-check"},
			PerTaskTimeout:    Duration{Duration: 20 * time.Minute},
			TaskRetryAttempts: consensus.DefaultTaskRetryAttempts,
			ProposalPolicy: ProposalPolicyConfig{
				MaxPasses:          1,
				MaxClaimsPerWorker: 3,
				DedupeStrategy:     "normalized-statement",
			},
			VerificationPolicy: VerificationPolicyConfig{
				AllowSemanticVerifier: true,
				MaxParallelChecks:     4,
			},
			ArbiterPolicy: ArbiterPolicyConfig{
				AllowUndetermined: true,
				BlindReview:       true,
			},
		},
		Output: OutputConfig{
			Directory: "./out/{requestId}",
		},
		Providers: map[string]ProviderConfig{
			"mock": {
				Type:     ProviderTypeMock,
				Behavior: "deterministic",
				Models: map[string]ProviderModelConfig{
					"default": {
						ProviderModel: "mock-default",
					},
				},
			},
		},
		Agents: []AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default", Role: "proposer"},
			{ID: "challenger-a", Provider: "mock", Model: "default", Role: "challenger"},
			{ID: "arbiter-a", Provider: "mock", Model: "default", Role: "arbiter"},
			{ID: "verifier-a", Provider: "mock", Model: "default", Role: "semantic-verifier"},
			{ID: "reporter-a", Provider: "mock", Model: "default", Role: "reporter"},
			{ID: "actor-a", Provider: "mock", Model: "default", Role: "actor"},
		},
		Roles: RolesConfig{
			Proposers:        []string{"proposer-a"},
			Challengers:      []string{"challenger-a"},
			Arbiter:          "arbiter-a",
			SemanticVerifier: "verifier-a",
			Reporter:         "reporter-a",
			Actor:            "actor-a",
		},
	}
}

func RenderTemplate(preset string) (string, error) {
	body, _, err := RenderTemplateRequest(preset, "", "", "")
	return body, err
}

func WritePresetTemplate(path string, preset string, force bool) error {
	selection, err := ResolveTemplateSelection(preset, "", "", "")
	if err != nil {
		return err
	}
	return WriteTemplateSelection(path, selection, force)
}

func normalizePreset(preset string) string {
	value := strings.TrimSpace(strings.ToLower(preset))
	if value == "" {
		return TemplatePresetQuickstart
	}
	return value
}
