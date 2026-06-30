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
				provider.Protocol != APIProtocolOpenAIResponses &&
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
			if err := validateCLIProviderDoesNotDeclareTokenBudget(name, provider); err != nil {
				return err
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

func validateCLIProviderDoesNotDeclareTokenBudget(name string, provider ProviderConfig) error {
	for modelID, model := range provider.Models {
		if model.MaxOutputTokensSet || model.MaxOutputTokens != 0 {
			return fmt.Errorf("provider %s model %s: max_output_tokens is API-only and is not applied to cli providers; remove it or use an api provider", name, modelID)
		}
	}
	if path, ok := findMaxTokenOptionPath(provider.Options, "options"); ok {
		return fmt.Errorf("provider %s %s is API-only and is not applied to cli providers; remove it or use an api provider", name, path)
	}
	for i, arg := range provider.Args {
		if isMaxTokenConfigKey(arg) {
			return fmt.Errorf("provider %s args[%d]=%q declares an output-token budget, but cli providers do not expose a supported token-budget contract", name, i, arg)
		}
	}
	for key := range provider.Env {
		if isMaxTokenConfigKey(key) {
			return fmt.Errorf("provider %s env.%s declares an output-token budget, but cli providers do not expose a supported token-budget contract", name, key)
		}
	}
	return nil
}

func findMaxTokenOptionPath(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			nextPath := path + "." + key
			if isMaxTokenConfigKey(key) {
				return nextPath, true
			}
			if found, ok := findMaxTokenOptionPath(child, nextPath); ok {
				return found, true
			}
		}
	case []any:
		for i, child := range typed {
			if found, ok := findMaxTokenOptionPath(child, fmt.Sprintf("%s[%d]", path, i)); ok {
				return found, true
			}
		}
	}
	return "", false
}

func isMaxTokenConfigKey(value string) bool {
	normalized := normalizeConfigKey(value)
	switch normalized {
	case "maxtokens",
		"maxtokensfield",
		"maxoutputtokens",
		"maxoutputtokensfield",
		"maxcompletiontokens",
		"maxcompletiontokensfield":
		return true
	default:
		return false
	}
}

func normalizeConfigKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "-")
	if before, _, ok := strings.Cut(value, "="); ok {
		value = before
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
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
		if !IsProviderEnabled(provider) {
			return nil, fmt.Errorf("agent %s: provider %s is disabled", agent.ID, agent.Provider)
		}
		if len(provider.Models) > 0 {
			if strings.TrimSpace(agent.Model) == "" {
				return nil, fmt.Errorf("agent %s: model is required", agent.ID)
			}
			model, ok := provider.Models[agent.Model]
			if !ok {
				return nil, fmt.Errorf("agent %s: unknown model %s for provider %s", agent.ID, agent.Model, agent.Provider)
			}
			if !IsProviderModelEnabled(model) {
				return nil, fmt.Errorf("agent %s: model %s for provider %s is disabled", agent.ID, agent.Model, agent.Provider)
			}
		}
	}
	return knownAgents, nil
}

func validateRoles(cfg Config, knownAgents map[string]struct{}) error {
	roles := normalizeRoles(cfg.Roles)
	validateAgent := func(id string, field string) error {
		if id == "" {
			return nil
		}
		if _, ok := knownAgents[id]; !ok {
			return fmt.Errorf("%s references unknown agent %s", field, id)
		}
		return nil
	}
	validateAgents := func(ids []string, field string) error {
		for _, id := range ids {
			if err := validateAgent(id, field); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validateAgents(roles.Adjudication.Proposers, "roles.adjudication.proposers"); err != nil {
		return err
	}
	if err := validateAgents(roles.Adjudication.Challengers, "roles.adjudication.challengers"); err != nil {
		return err
	}
	for field, id := range map[string]string{
		"roles.adjudication.arbiter":           roles.Adjudication.Arbiter,
		"roles.adjudication.semantic_verifier": roles.Adjudication.SemanticVerifier,
		"roles.adjudication.reporter":          roles.Adjudication.Reporter,
		"roles.adjudication.actor":             roles.Adjudication.Actor,
	} {
		if err := validateAgent(id, field); err != nil {
			return err
		}
	}
	if err := validateAgents(roles.FreeDebate.Participants, "roles.free_debate.participants"); err != nil {
		return err
	}
	for field, id := range map[string]string{
		"roles.free_debate.reporter": roles.FreeDebate.Reporter,
		"roles.free_debate.actor":    roles.FreeDebate.Actor,
	} {
		if err := validateAgent(id, field); err != nil {
			return err
		}
	}
	if err := validateAgents(roles.Delphi.Participants, "roles.delphi.participants"); err != nil {
		return err
	}
	for field, id := range map[string]string{
		"roles.delphi.facilitator": roles.Delphi.Facilitator,
		"roles.delphi.reporter":    roles.Delphi.Reporter,
		"roles.delphi.actor":       roles.Delphi.Actor,
	} {
		if err := validateAgent(id, field); err != nil {
			return err
		}
	}
	mode := cfg.Defaults.Mode
	if mode == "" {
		mode = consensus.WorkflowModeAdjudication
	}
	activeRoles := RoleAssignmentsForMode(roles, mode)
	switch mode {
	case consensus.WorkflowModeFreeDebate, consensus.WorkflowModeDelphi:
		if len(activeRoles.Participants) == 0 {
			return fmt.Errorf("roles.%s.participants must not be empty for mode %s", mode, cfg.Defaults.Mode)
		}
	default:
		if len(activeRoles.Proposers) == 0 {
			return fmt.Errorf("roles.adjudication.proposers must not be empty")
		}
		if len(activeRoles.Challengers) == 0 {
			return fmt.Errorf("roles.adjudication.challengers must not be empty")
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
