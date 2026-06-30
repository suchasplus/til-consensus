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
	APIProtocolOpenAIResponses     = "openai-responses"
	APIProtocolAnthropicCompatible = "anthropic-compatible"
	APIProtocolGemini              = "gemini-api"
)

const (
	CLITypeGeneric     = "generic"
	CLITypeAntigravity = "antigravity"
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
	if out.Enabled == nil {
		out.Enabled = boolPtr(true)
	}
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
	if out.Type == ProviderTypeCLI && out.CLIType == CLITypeAntigravity && out.Command == "" {
		out.Command = "agy"
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
	for modelID, model := range out.Models {
		if model.Enabled == nil {
			model.Enabled = boolPtr(true)
		}
		out.Models[modelID] = model
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

func IsProviderEnabled(provider ProviderConfig) bool {
	return boolValue(provider.Enabled, true)
}

func IsProviderModelEnabled(model ProviderModelConfig) bool {
	return boolValue(model.Enabled, true)
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPtr(value bool) *bool {
	return &value
}

func normalizeRoles(roles RolesConfig) RolesConfig {
	if roles.Adjudication.IsZero() {
		roles.Adjudication = AdjudicationRolesConfig{
			Proposers:        roles.Proposers,
			Challengers:      roles.Challengers,
			Arbiter:          roles.Arbiter,
			SemanticVerifier: roles.SemanticVerifier,
			Reporter:         roles.Reporter,
			Actor:            roles.Actor,
		}
	}
	if roles.FreeDebate.IsZero() {
		roles.FreeDebate = DebateRolesConfig{
			Participants: roles.Participants,
			Reporter:     roles.Reporter,
			Actor:        roles.Actor,
		}
	}
	if roles.Delphi.IsZero() {
		roles.Delphi = DelphiRolesConfig{
			Participants: roles.Participants,
			Facilitator:  roles.Facilitator,
			Reporter:     roles.Reporter,
			Actor:        roles.Actor,
		}
	}
	roles.Adjudication.Proposers = dedupe(roles.Adjudication.Proposers)
	roles.Adjudication.Challengers = dedupe(roles.Adjudication.Challengers)
	roles.Adjudication.Arbiter = strings.TrimSpace(roles.Adjudication.Arbiter)
	roles.Adjudication.SemanticVerifier = strings.TrimSpace(roles.Adjudication.SemanticVerifier)
	roles.Adjudication.Reporter = strings.TrimSpace(roles.Adjudication.Reporter)
	roles.Adjudication.Actor = strings.TrimSpace(roles.Adjudication.Actor)
	roles.FreeDebate.Participants = dedupe(roles.FreeDebate.Participants)
	roles.FreeDebate.Reporter = strings.TrimSpace(roles.FreeDebate.Reporter)
	roles.FreeDebate.Actor = strings.TrimSpace(roles.FreeDebate.Actor)
	roles.Delphi.Participants = dedupe(roles.Delphi.Participants)
	roles.Delphi.Facilitator = strings.TrimSpace(roles.Delphi.Facilitator)
	roles.Delphi.Reporter = strings.TrimSpace(roles.Delphi.Reporter)
	roles.Delphi.Actor = strings.TrimSpace(roles.Delphi.Actor)
	roles.Proposers = cloneStrings(roles.Adjudication.Proposers)
	roles.Challengers = cloneStrings(roles.Adjudication.Challengers)
	roles.Participants = firstNonEmptyStrings(roles.FreeDebate.Participants, roles.Delphi.Participants)
	roles.Arbiter = roles.Adjudication.Arbiter
	roles.SemanticVerifier = roles.Adjudication.SemanticVerifier
	roles.Facilitator = roles.Delphi.Facilitator
	roles.Reporter = firstNonEmpty(roles.Adjudication.Reporter, roles.FreeDebate.Reporter, roles.Delphi.Reporter)
	roles.Actor = firstNonEmpty(roles.Adjudication.Actor, roles.FreeDebate.Actor, roles.Delphi.Actor)
	return roles
}

func RoleAssignmentsForMode(roles RolesConfig, mode consensus.WorkflowMode) consensus.RoleAssignments {
	roles = normalizeRoles(roles)
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		return consensus.RoleAssignments{
			Participants: cloneStrings(roles.FreeDebate.Participants),
			Reporter:     roles.FreeDebate.Reporter,
			Actor:        roles.FreeDebate.Actor,
		}
	case consensus.WorkflowModeDelphi:
		return consensus.RoleAssignments{
			Participants: cloneStrings(roles.Delphi.Participants),
			Facilitator:  roles.Delphi.Facilitator,
			Reporter:     roles.Delphi.Reporter,
			Actor:        roles.Delphi.Actor,
		}
	default:
		return consensus.RoleAssignments{
			Proposers:        cloneStrings(roles.Adjudication.Proposers),
			Challengers:      cloneStrings(roles.Adjudication.Challengers),
			Arbiter:          roles.Adjudication.Arbiter,
			SemanticVerifier: roles.Adjudication.SemanticVerifier,
			Reporter:         roles.Adjudication.Reporter,
			Actor:            roles.Adjudication.Actor,
		}
	}
}

func firstNonEmptyStrings(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return cloneStrings(value)
		}
	}
	return nil
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
