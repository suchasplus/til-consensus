package config

import (
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

const (
	ProviderTypeAPI     = "api"
	ProviderTypeCLI     = "cli"
	ProviderTypeSDK     = "sdk"
	ProviderTypeMock    = "mock"
	ProviderTypeOpenAI  = "openai"
	ProviderTypeCommand = "command"
)

const (
	APIProtocolOpenAICompatible    = "openai-compatible"
	APIProtocolAnthropicCompatible = "anthropic-compatible"
	APIProtocolGemini              = "gemini-api"
)

const (
	CLITypeGeneric = "generic"
)

func Normalize(cfg Config) Config {
	out := cfg
	out.Defaults.Mode = consensus.WorkflowMode(strings.TrimSpace(string(out.Defaults.Mode)))
	out.Providers = make(map[string]ProviderConfig, len(cfg.Providers))
	for name, provider := range cfg.Providers {
		out.Providers[name] = normalizeProvider(provider)
	}
	out.Agents = make([]AgentConfig, 0, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		normalized := agent
		if provider, ok := out.Providers[agent.Provider]; ok && normalized.Model == "" {
			if modelID, ok := singleModelID(provider); ok {
				normalized.Model = modelID
			}
		}
		out.Agents = append(out.Agents, normalized)
	}
	out.Roles = normalizeRoles(cfg.Roles)
	return out
}

func normalizeProvider(provider ProviderConfig) ProviderConfig {
	out := provider
	switch out.Type {
	case ProviderTypeOpenAI:
		out.Type = ProviderTypeAPI
		if out.Protocol == "" {
			out.Protocol = APIProtocolOpenAICompatible
		}
	case ProviderTypeCommand:
		out.Type = ProviderTypeCLI
		if out.CLIType == "" {
			out.CLIType = CLITypeGeneric
		}
	}
	if out.Type == ProviderTypeCLI && out.CLIType == "" {
		out.CLIType = CLITypeGeneric
	}
	if out.Type == ProviderTypeCLI && out.Command == "" && out.CLIType != CLITypeGeneric {
		out.Command = out.CLIType
	}
	if len(out.Models) == 0 && out.Model != "" {
		out.Models = map[string]ProviderModelConfig{
			out.Model: {ProviderModel: out.Model},
		}
	}
	if out.Type == ProviderTypeMock && len(out.Models) == 0 {
		out.Models = map[string]ProviderModelConfig{
			"default": {ProviderModel: "default"},
		}
	}
	if out.Headers == nil {
		out.Headers = map[string]string{}
	}
	if out.Env == nil {
		out.Env = map[string]string{}
	}
	if out.Options == nil {
		out.Options = map[string]any{}
	}
	if out.Participants == nil {
		out.Participants = map[string]MockParticipantScenario{}
	}
	return out
}

func normalizeRoles(roles RolesConfig) RolesConfig {
	roles.Proposers = dedupe(roles.Proposers)
	roles.Challengers = dedupe(roles.Challengers)
	roles.Participants = dedupe(roles.Participants)
	return roles
}

func singleModelID(provider ProviderConfig) (string, bool) {
	if len(provider.Models) != 1 {
		return "", false
	}
	keys := ModelIDs(provider)
	if len(keys) != 1 {
		return "", false
	}
	return keys[0], true
}

func ModelIDs(provider ProviderConfig) []string {
	out := make([]string, 0, len(provider.Models))
	for modelID := range provider.Models {
		out = append(out, modelID)
	}
	sort.Strings(out)
	return out
}
