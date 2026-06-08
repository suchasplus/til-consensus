package config

import (
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func Validate(cfg Config) error {
	if err := validateProviderProfiles(cfg); err != nil {
		return err
	}
	knownAgents, err := validateAgentProfiles(cfg, true)
	if err != nil {
		return err
	}
	if err := validateRoles(cfg, knownAgents); err != nil {
		return err
	}
	if err := validateDefaults(cfg); err != nil {
		return err
	}
	return nil
}

func ValidateProfiles(cfg Config) error {
	return validateProviderProfiles(cfg)
}

func validateProviderProfiles(cfg Config) error {
	if cfg.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version: %d", cfg.SchemaVersion)
	}
	if len(cfg.Providers) == 0 {
		return fmt.Errorf("providers must not be empty")
	}
	for name, provider := range cfg.Providers {
		switch provider.Type {
		case ProviderTypeMock:
		case ProviderTypeAPI:
			if provider.Protocol != APIProtocolOpenAICompatible &&
				provider.Protocol != APIProtocolAnthropicCompatible &&
				provider.Protocol != APIProtocolGemini {
				return fmt.Errorf("provider %s: unsupported protocol %q", name, provider.Protocol)
			}
			if len(provider.Models) == 0 {
				return fmt.Errorf("provider %s: models must not be empty", name)
			}
		case ProviderTypeCLI:
			if strings.TrimSpace(provider.CLIType) == "" {
				return fmt.Errorf("provider %s: cli_type is required", name)
			}
			if len(provider.Models) == 0 {
				return fmt.Errorf("provider %s: models must not be empty", name)
			}
			if provider.CLIType == CLITypeGeneric && strings.TrimSpace(provider.Command) == "" {
				return fmt.Errorf("provider %s: command is required for generic cli provider", name)
			}
		case ProviderTypeSDK:
			if strings.TrimSpace(provider.Adapter) == "" {
				return fmt.Errorf("provider %s: adapter is required", name)
			}
			if len(provider.Models) == 0 {
				return fmt.Errorf("provider %s: models must not be empty", name)
			}
		default:
			return fmt.Errorf("provider %s: unsupported type %q", name, provider.Type)
		}
		for modelID, model := range provider.Models {
			if strings.TrimSpace(modelID) == "" {
				return fmt.Errorf("provider %s: model id must not be empty", name)
			}
			if model.MaxOutputTokens < 0 {
				return fmt.Errorf("provider %s model %s: max_output_tokens must be >= 0", name, modelID)
			}
			if model.Temperature != nil && (*model.Temperature < 0 || *model.Temperature > 2) {
				return fmt.Errorf("provider %s model %s: temperature must be in [0,2]", name, modelID)
			}
		}
	}
	return nil
}

func validateAgentProfiles(cfg Config, requireAgents bool) (map[string]struct{}, error) {
	if requireAgents && len(cfg.Agents) == 0 {
		return nil, fmt.Errorf("agents must not be empty")
	}
	knownAgents := map[string]struct{}{}
	for _, agent := range cfg.Agents {
		if strings.TrimSpace(agent.ID) == "" {
			return nil, fmt.Errorf("agent id is required")
		}
		if _, ok := knownAgents[agent.ID]; ok {
			return nil, fmt.Errorf("duplicate agent id: %s", agent.ID)
		}
		knownAgents[agent.ID] = struct{}{}
		if agent.Temperature != nil && (*agent.Temperature < 0 || *agent.Temperature > 2) {
			return nil, fmt.Errorf("agent %s: temperature must be in [0,2]", agent.ID)
		}
		provider, ok := cfg.Providers[agent.Provider]
		if !ok {
			return nil, fmt.Errorf("agent %s: unknown provider %s", agent.ID, agent.Provider)
		}
		if len(provider.Models) > 0 {
			if strings.TrimSpace(agent.Model) == "" {
				return nil, fmt.Errorf("agent %s: model is required", agent.ID)
			}
			if _, ok := provider.Models[agent.Model]; !ok {
				return nil, fmt.Errorf("agent %s: unknown model %s for provider %s", agent.ID, agent.Model, agent.Provider)
			}
		}
	}
	return knownAgents, nil
}

func validateRoles(cfg Config, knownAgents map[string]struct{}) error {
	for _, id := range cfg.Roles.Proposers {
		if _, ok := knownAgents[id]; !ok {
			return fmt.Errorf("roles.proposers references unknown agent %s", id)
		}
	}
	for _, id := range cfg.Roles.Challengers {
		if _, ok := knownAgents[id]; !ok {
			return fmt.Errorf("roles.challengers references unknown agent %s", id)
		}
	}
	for _, id := range cfg.Roles.Participants {
		if _, ok := knownAgents[id]; !ok {
			return fmt.Errorf("roles.participants references unknown agent %s", id)
		}
	}
	for _, id := range []string{cfg.Roles.Arbiter, cfg.Roles.SemanticVerifier, cfg.Roles.Facilitator, cfg.Roles.Reporter, cfg.Roles.Actor} {
		if id == "" {
			continue
		}
		if _, ok := knownAgents[id]; !ok {
			return fmt.Errorf("roles references unknown agent %s", id)
		}
	}
	mode := cfg.Defaults.Mode
	if mode == "" {
		mode = consensus.WorkflowModeAdjudication
	}
	switch mode {
	case consensus.WorkflowModeFreeDebate, consensus.WorkflowModeDelphi:
		if len(cfg.Roles.Participants) == 0 {
			return fmt.Errorf("roles.participants must not be empty for mode %s", cfg.Defaults.Mode)
		}
	default:
		if len(cfg.Roles.Proposers) == 0 {
			return fmt.Errorf("roles.proposers must not be empty")
		}
		if len(cfg.Roles.Challengers) == 0 {
			return fmt.Errorf("roles.challengers must not be empty")
		}
	}
	return nil
}

func validateDefaults(cfg Config) error {
	if cfg.Defaults.ProposalPolicy.MaxPasses < 0 {
		return fmt.Errorf("defaults.proposal_policy.max_passes must be >= 0")
	}
	if cfg.Defaults.ProposalPolicy.MaxClaimsPerWorker < 0 {
		return fmt.Errorf("defaults.proposal_policy.max_claims_per_worker must be >= 0")
	}
	if cfg.Defaults.VerificationPolicy.MaxParallelChecks < 0 {
		return fmt.Errorf("defaults.verification_policy.max_parallel_checks must be >= 0")
	}
	for _, source := range append(append([]consensus.ExternalCommandSource(nil), cfg.Defaults.IngestPolicy.Sources...), cfg.Defaults.ObservePolicy.Sources...) {
		if strings.TrimSpace(source.Name) == "" {
			return fmt.Errorf("defaults external source name is required")
		}
		if strings.TrimSpace(source.Command) == "" {
			return fmt.Errorf("defaults external source %s: command is required", source.Name)
		}
	}
	switch cfg.Defaults.ObservePolicy.OnContradiction {
	case "", consensus.ObserveContradictionReopen, consensus.ObserveContradictionRecordOnly:
	default:
		return fmt.Errorf("defaults.observe_policy.on_contradiction is invalid: %s", cfg.Defaults.ObservePolicy.OnContradiction)
	}
	for _, target := range []consensus.FallbackTarget{
		cfg.Defaults.FallbackPolicy.OnInsufficientEvidence,
		cfg.Defaults.FallbackPolicy.OnUnresolvedConflict,
		cfg.Defaults.FallbackPolicy.OnUnresolvedClaims,
		cfg.Defaults.FallbackPolicy.OnKeepWithCaveat,
	} {
		switch target {
		case "", consensus.FallbackTargetStop, consensus.FallbackTargetRevise, consensus.FallbackTargetIngest:
		default:
			return fmt.Errorf("defaults.fallback_policy contains invalid target: %s", target)
		}
	}
	if cfg.Defaults.FallbackPolicy.MaxFallbackRounds < 0 {
		return fmt.Errorf("defaults.fallback_policy.max_fallback_rounds must be >= 0")
	}
	if cfg.Defaults.TaskRetryAttempts < 0 {
		return fmt.Errorf("defaults.task_retry_attempts must be >= 0")
	}
	return nil
}
