package config

import "github.com/suchasplus/til-consensus/internal/consensus"

func mergeConfig(base Config, overlay Config) Config {
	out := base
	out.Include = nil
	if overlay.SchemaVersion != 0 {
		out.SchemaVersion = overlay.SchemaVersion
	}
	if overlay.Profile != "" {
		out.Profile = overlay.Profile
	}
	out.Profiles = mergeProfiles(out.Profiles, overlay.Profiles)
	out.Defaults = mergeDefaults(out.Defaults, overlay.Defaults)
	out.Output = mergeOutput(out.Output, overlay.Output)
	out.Providers = mergeProviders(out.Providers, overlay.Providers)
	out.Agents = mergeAgents(out.Agents, overlay.Agents)
	out.Roles = mergeRoles(out.Roles, overlay.Roles)
	return out
}

func mergeProfiles(base map[string]ProfileConfig, overlay map[string]ProfileConfig) map[string]ProfileConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]ProfileConfig, len(base)+len(overlay))
	for key, value := range base {
		out[key] = cloneProfile(value)
	}
	for key, value := range overlay {
		if existing, ok := out[key]; ok {
			out[key] = mergeProfile(existing, value)
			continue
		}
		out[key] = cloneProfile(value)
	}
	return out
}

func mergeProfile(base ProfileConfig, overlay ProfileConfig) ProfileConfig {
	out := base
	out.Defaults = mergeDefaults(out.Defaults, overlay.Defaults)
	out.Output = mergeOutput(out.Output, overlay.Output)
	out.Providers = mergeProviders(out.Providers, overlay.Providers)
	out.Agents = mergeAgents(out.Agents, overlay.Agents)
	out.Roles = mergeRoles(out.Roles, overlay.Roles)
	return out
}

func mergeDefaults(base DefaultsConfig, overlay DefaultsConfig) DefaultsConfig {
	out := base
	if overlay.Mode != "" {
		out.Mode = overlay.Mode
	}
	if overlay.TaskType != "" {
		out.TaskType = overlay.TaskType
	}
	if len(overlay.SuccessCriteria) > 0 {
		out.SuccessCriteria = cloneStrings(overlay.SuccessCriteria)
	}
	if len(overlay.OutOfScope) > 0 {
		out.OutOfScope = cloneStrings(overlay.OutOfScope)
	}
	if len(overlay.AllowedTools) > 0 {
		out.AllowedTools = cloneStrings(overlay.AllowedTools)
	}
	if overlay.PerTaskTimeout.Duration != 0 {
		out.PerTaskTimeout = overlay.PerTaskTimeout
	}
	if overlay.TaskRetryAttempts != 0 {
		out.TaskRetryAttempts = overlay.TaskRetryAttempts
	}
	if overlay.GlobalDeadline.Duration != 0 {
		out.GlobalDeadline = overlay.GlobalDeadline
	}
	out.ProposalPolicy = mergeProposalPolicy(out.ProposalPolicy, overlay.ProposalPolicy)
	out.VerificationPolicy = mergeVerificationPolicy(out.VerificationPolicy, overlay.VerificationPolicy)
	out.ArbiterPolicy = mergeArbiterPolicy(out.ArbiterPolicy, overlay.ArbiterPolicy)
	out.IngestPolicy = mergeIngestPolicy(out.IngestPolicy, overlay.IngestPolicy)
	out.FallbackPolicy = mergeFallbackPolicy(out.FallbackPolicy, overlay.FallbackPolicy)
	out.ObservePolicy = mergeObservePolicy(out.ObservePolicy, overlay.ObservePolicy)
	out.DebatePolicy = mergeDebatePolicy(out.DebatePolicy, overlay.DebatePolicy)
	out.DelphiPolicy = mergeDelphiPolicy(out.DelphiPolicy, overlay.DelphiPolicy)
	out.WorkspaceSnapshot = mergeWorkspaceSnapshot(out.WorkspaceSnapshot, overlay.WorkspaceSnapshot)
	out.TaskConstraints = mergeTaskConstraints(out.TaskConstraints, overlay.TaskConstraints)
	return out
}

func mergeOutput(base OutputConfig, overlay OutputConfig) OutputConfig {
	out := base
	out.Directory = pickString(out.Directory, overlay.Directory)
	out.LedgerPath = pickString(out.LedgerPath, overlay.LedgerPath)
	out.EventsPath = pickString(out.EventsPath, overlay.EventsPath)
	out.ResultPath = pickString(out.ResultPath, overlay.ResultPath)
	out.SummaryPath = pickString(out.SummaryPath, overlay.SummaryPath)
	out.ErrorPath = pickString(out.ErrorPath, overlay.ErrorPath)
	out.ArtifactsDir = pickString(out.ArtifactsDir, overlay.ArtifactsDir)
	return out
}

func mergeProviders(base map[string]ProviderConfig, overlay map[string]ProviderConfig) map[string]ProviderConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]ProviderConfig, len(base)+len(overlay))
	for key, value := range base {
		out[key] = cloneProvider(value)
	}
	for key, value := range overlay {
		if existing, ok := out[key]; ok {
			out[key] = mergeProvider(existing, value)
			continue
		}
		out[key] = cloneProvider(value)
	}
	return out
}

func mergeProvider(base ProviderConfig, overlay ProviderConfig) ProviderConfig {
	out := base
	if overlay.Enabled != nil {
		out.Enabled = cloneBoolPtr(overlay.Enabled)
	}
	out.Type = pickString(out.Type, overlay.Type)
	out.Protocol = pickString(out.Protocol, overlay.Protocol)
	out.CLIType = pickString(out.CLIType, overlay.CLIType)
	out.BaseURL = pickString(out.BaseURL, overlay.BaseURL)
	out.APIKeyEnv = pickString(out.APIKeyEnv, overlay.APIKeyEnv)
	out.Headers = mergeStringMap(out.Headers, overlay.Headers)
	out.Model = pickString(out.Model, overlay.Model)
	out.Models = mergeProviderModels(out.Models, overlay.Models)
	out.Command = pickString(out.Command, overlay.Command)
	if len(overlay.Args) > 0 {
		out.Args = cloneStrings(overlay.Args)
	}
	out.Env = mergeStringMap(out.Env, overlay.Env)
	out.Adapter = pickString(out.Adapter, overlay.Adapter)
	out.Options = mergeAnyMap(out.Options, overlay.Options)
	out.Behavior = pickString(out.Behavior, overlay.Behavior)
	if overlay.Delay.Duration != 0 {
		out.Delay = overlay.Delay
	}
	out.Error = pickString(out.Error, overlay.Error)
	out.Participants = mergeMockParticipants(out.Participants, overlay.Participants)
	return out
}

func mergeProviderModels(base map[string]ProviderModelConfig, overlay map[string]ProviderModelConfig) map[string]ProviderModelConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]ProviderModelConfig, len(base)+len(overlay))
	for key, value := range base {
		out[key] = cloneProviderModel(value)
	}
	for key, value := range overlay {
		if existing, ok := out[key]; ok {
			out[key] = mergeProviderModel(existing, value)
			continue
		}
		out[key] = cloneProviderModel(value)
	}
	return out
}

func mergeProviderModel(base ProviderModelConfig, overlay ProviderModelConfig) ProviderModelConfig {
	out := base
	if overlay.Enabled != nil {
		out.Enabled = cloneBoolPtr(overlay.Enabled)
	}
	out.ProviderModel = pickString(out.ProviderModel, overlay.ProviderModel)
	if overlay.ContextWindow != 0 {
		out.ContextWindow = overlay.ContextWindow
	}
	if overlay.MaxOutputTokensSet || overlay.MaxOutputTokens != 0 {
		out.MaxOutputTokens = overlay.MaxOutputTokens
		out.MaxOutputTokensSet = overlay.MaxOutputTokensSet || overlay.MaxOutputTokens != 0
	}
	if overlay.Temperature != nil {
		value := *overlay.Temperature
		out.Temperature = &value
	}
	out.Reasoning = pickString(out.Reasoning, overlay.Reasoning)
	return out
}

func mergeAgents(base []AgentConfig, overlay []AgentConfig) []AgentConfig {
	out := make([]AgentConfig, 0, len(base)+len(overlay))
	index := map[string]int{}
	for _, agent := range base {
		cloned := cloneAgent(agent)
		index[cloned.ID] = len(out)
		out = append(out, cloned)
	}
	for _, agent := range overlay {
		cloned := cloneAgent(agent)
		if existing, ok := index[cloned.ID]; ok && cloned.ID != "" {
			out[existing] = mergeAgent(out[existing], cloned)
			continue
		}
		index[cloned.ID] = len(out)
		out = append(out, cloned)
	}
	return out
}

func mergeAgent(base AgentConfig, overlay AgentConfig) AgentConfig {
	out := base
	out.Provider = pickString(out.Provider, overlay.Provider)
	out.Model = pickString(out.Model, overlay.Model)
	out.Role = pickString(out.Role, overlay.Role)
	out.SystemPrompt = pickString(out.SystemPrompt, overlay.SystemPrompt)
	if overlay.Timeout.Duration != 0 {
		out.Timeout = overlay.Timeout
	}
	if overlay.Temperature != nil {
		value := *overlay.Temperature
		out.Temperature = &value
	}
	out.Reasoning = pickString(out.Reasoning, overlay.Reasoning)
	return out
}

func mergeRoles(base RolesConfig, overlay RolesConfig) RolesConfig {
	base = normalizeRoles(base)
	overlay = normalizeRoles(overlay)
	out := base
	if len(overlay.Adjudication.Proposers) > 0 {
		out.Adjudication.Proposers = cloneStrings(overlay.Adjudication.Proposers)
	}
	if len(overlay.Adjudication.Challengers) > 0 {
		out.Adjudication.Challengers = cloneStrings(overlay.Adjudication.Challengers)
	}
	out.Adjudication.Arbiter = pickString(out.Adjudication.Arbiter, overlay.Adjudication.Arbiter)
	out.Adjudication.SemanticVerifier = pickString(out.Adjudication.SemanticVerifier, overlay.Adjudication.SemanticVerifier)
	out.Adjudication.Reporter = pickString(out.Adjudication.Reporter, overlay.Adjudication.Reporter)
	out.Adjudication.Actor = pickString(out.Adjudication.Actor, overlay.Adjudication.Actor)
	if len(overlay.FreeDebate.Participants) > 0 {
		out.FreeDebate.Participants = cloneStrings(overlay.FreeDebate.Participants)
	}
	out.FreeDebate.Reporter = pickString(out.FreeDebate.Reporter, overlay.FreeDebate.Reporter)
	out.FreeDebate.Actor = pickString(out.FreeDebate.Actor, overlay.FreeDebate.Actor)
	if len(overlay.Delphi.Participants) > 0 {
		out.Delphi.Participants = cloneStrings(overlay.Delphi.Participants)
	}
	out.Delphi.Facilitator = pickString(out.Delphi.Facilitator, overlay.Delphi.Facilitator)
	out.Delphi.Reporter = pickString(out.Delphi.Reporter, overlay.Delphi.Reporter)
	out.Delphi.Actor = pickString(out.Delphi.Actor, overlay.Delphi.Actor)
	return normalizeRoles(out)
}

func mergeProposalPolicy(base ProposalPolicyConfig, overlay ProposalPolicyConfig) ProposalPolicyConfig {
	out := base
	if overlay.MaxPasses != 0 {
		out.MaxPasses = overlay.MaxPasses
	}
	if overlay.MaxClaimsPerWorker != 0 {
		out.MaxClaimsPerWorker = overlay.MaxClaimsPerWorker
	}
	out.DedupeStrategy = pickString(out.DedupeStrategy, overlay.DedupeStrategy)
	return out
}

func mergeVerificationPolicy(base VerificationPolicyConfig, overlay VerificationPolicyConfig) VerificationPolicyConfig {
	out := base
	if len(overlay.RequiredChecks) > 0 {
		out.RequiredChecks = cloneVerificationChecks(overlay.RequiredChecks)
	}
	if overlay.AllowSemanticVerifier {
		out.AllowSemanticVerifier = true
	}
	if overlay.MaxParallelChecks != 0 {
		out.MaxParallelChecks = overlay.MaxParallelChecks
	}
	return out
}

func mergeArbiterPolicy(base ArbiterPolicyConfig, overlay ArbiterPolicyConfig) ArbiterPolicyConfig {
	out := base
	if overlay.AllowUndetermined {
		out.AllowUndetermined = true
	}
	if overlay.BlindReview {
		out.BlindReview = true
	}
	return out
}

func mergeIngestPolicy(base consensus.IngestPolicy, overlay consensus.IngestPolicy) consensus.IngestPolicy {
	out := base
	if len(overlay.Sources) > 0 {
		out.Sources = cloneExternalCommandSources(overlay.Sources)
	}
	return out
}

func mergeFallbackPolicy(base consensus.AdjudicationFallbackPolicy, overlay consensus.AdjudicationFallbackPolicy) consensus.AdjudicationFallbackPolicy {
	out := base
	if overlay.MaxFallbackRounds != 0 {
		out.MaxFallbackRounds = overlay.MaxFallbackRounds
	}
	if overlay.OnInsufficientEvidence != "" {
		out.OnInsufficientEvidence = overlay.OnInsufficientEvidence
	}
	if overlay.OnUnresolvedConflict != "" {
		out.OnUnresolvedConflict = overlay.OnUnresolvedConflict
	}
	if overlay.OnUnresolvedClaims != "" {
		out.OnUnresolvedClaims = overlay.OnUnresolvedClaims
	}
	if overlay.OnKeepWithCaveat != "" {
		out.OnKeepWithCaveat = overlay.OnKeepWithCaveat
	}
	return out
}

func mergeObservePolicy(base consensus.ObservePolicy, overlay consensus.ObservePolicy) consensus.ObservePolicy {
	out := base
	if len(overlay.Sources) > 0 {
		out.Sources = cloneExternalCommandSources(overlay.Sources)
	}
	if overlay.OnContradiction != "" {
		out.OnContradiction = overlay.OnContradiction
	}
	return out
}

func mergeDebatePolicy(base DebatePolicyConfig, overlay DebatePolicyConfig) DebatePolicyConfig {
	out := base
	if overlay.MinRounds != 0 {
		out.MinRounds = overlay.MinRounds
	}
	if overlay.MaxRounds != 0 {
		out.MaxRounds = overlay.MaxRounds
	}
	if overlay.VoteThreshold != 0 {
		out.VoteThreshold = overlay.VoteThreshold
	}
	if overlay.EnableEarlyStop {
		out.EnableEarlyStop = true
	}
	out.PeerContextMode = pickString(out.PeerContextMode, overlay.PeerContextMode)
	return out
}

func mergeDelphiPolicy(base DelphiPolicyConfig, overlay DelphiPolicyConfig) DelphiPolicyConfig {
	out := base
	if overlay.MinRounds != 0 {
		out.MinRounds = overlay.MinRounds
	}
	if overlay.MaxRounds != 0 {
		out.MaxRounds = overlay.MaxRounds
	}
	if overlay.ConvergenceThreshold != 0 {
		out.ConvergenceThreshold = overlay.ConvergenceThreshold
	}
	if overlay.RatingScaleMin != 0 {
		out.RatingScaleMin = overlay.RatingScaleMin
	}
	if overlay.RatingScaleMax != 0 {
		out.RatingScaleMax = overlay.RatingScaleMax
	}
	if overlay.Anonymous {
		out.Anonymous = true
	}
	out.FacilitatorSummaryStyle = pickString(out.FacilitatorSummaryStyle, overlay.FacilitatorSummaryStyle)
	return out
}

func mergeWorkspaceSnapshot(base *consensus.WorkspaceSnapshot, overlay *consensus.WorkspaceSnapshot) *consensus.WorkspaceSnapshot {
	if base == nil && overlay == nil {
		return nil
	}
	if base == nil {
		return cloneWorkspaceSnapshot(overlay)
	}
	out := cloneWorkspaceSnapshot(base)
	if overlay == nil {
		return out
	}
	out.Root = pickString(out.Root, overlay.Root)
	out.Revision = pickString(out.Revision, overlay.Revision)
	if len(overlay.Paths) > 0 {
		out.Paths = cloneStrings(overlay.Paths)
	}
	out.Hash = pickString(out.Hash, overlay.Hash)
	return out
}

func mergeTaskConstraints(base consensus.TaskConstraints, overlay consensus.TaskConstraints) consensus.TaskConstraints {
	out := base
	out.Language = pickString(out.Language, overlay.Language)
	if len(overlay.AllowedPaths) > 0 {
		out.AllowedPaths = cloneStrings(overlay.AllowedPaths)
	}
	if len(overlay.RequiredCommands) > 0 {
		out.RequiredCommands = cloneStrings(overlay.RequiredCommands)
	}
	if len(overlay.Notes) > 0 {
		out.Notes = cloneStrings(overlay.Notes)
	}
	return out
}

func mergeMockParticipants(base map[string]MockParticipantScenario, overlay map[string]MockParticipantScenario) map[string]MockParticipantScenario {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]MockParticipantScenario, len(base)+len(overlay))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func mergeStringMap(base map[string]string, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneStringMap(base)
	if out == nil {
		out = map[string]string{}
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func mergeAnyMap(base map[string]any, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneDeepAnyMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range overlay {
		if baseValue, ok := out[key]; ok {
			if merged, ok := mergeAnyValue(baseValue, value); ok {
				out[key] = merged
				continue
			}
		}
		out[key] = cloneAny(value)
	}
	return out
}

func mergeAnyValue(base any, overlay any) (any, bool) {
	baseMap, baseOK := base.(map[string]any)
	overlayMap, overlayOK := overlay.(map[string]any)
	if baseOK && overlayOK {
		return mergeAnyMap(baseMap, overlayMap), true
	}
	return nil, false
}

func cloneProvider(provider ProviderConfig) ProviderConfig {
	out := provider
	out.Enabled = cloneBoolPtr(provider.Enabled)
	out.Headers = cloneStringMap(provider.Headers)
	out.Models = mergeProviderModels(nil, provider.Models)
	out.Args = cloneStrings(provider.Args)
	out.Env = cloneStringMap(provider.Env)
	out.Options = cloneDeepAnyMap(provider.Options)
	out.Participants = mergeMockParticipants(nil, provider.Participants)
	return out
}

func cloneProfile(profile ProfileConfig) ProfileConfig {
	return ProfileConfig{
		Defaults:  mergeDefaults(DefaultsConfig{}, profile.Defaults),
		Output:    mergeOutput(OutputConfig{}, profile.Output),
		Providers: mergeProviders(nil, profile.Providers),
		Agents:    mergeAgents(nil, profile.Agents),
		Roles:     mergeRoles(RolesConfig{}, profile.Roles),
	}
}

func cloneProviderModel(model ProviderModelConfig) ProviderModelConfig {
	out := model
	out.Enabled = cloneBoolPtr(model.Enabled)
	if model.Temperature != nil {
		value := *model.Temperature
		out.Temperature = &value
	}
	return out
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneAgent(agent AgentConfig) AgentConfig {
	out := agent
	if agent.Temperature != nil {
		value := *agent.Temperature
		out.Temperature = &value
	}
	return out
}

func cloneWorkspaceSnapshot(snapshot *consensus.WorkspaceSnapshot) *consensus.WorkspaceSnapshot {
	if snapshot == nil {
		return nil
	}
	out := *snapshot
	out.Paths = cloneStrings(snapshot.Paths)
	return &out
}

func cloneVerificationChecks(checks []consensus.VerificationCheck) []consensus.VerificationCheck {
	if len(checks) == 0 {
		return nil
	}
	out := make([]consensus.VerificationCheck, len(checks))
	for idx, check := range checks {
		out[idx] = check
		out[idx].Args = cloneStrings(check.Args)
		out[idx].Env = cloneStringMap(check.Env)
		out[idx].Paths = cloneStrings(check.Paths)
	}
	return out
}

func cloneExternalCommandSources(sources []consensus.ExternalCommandSource) []consensus.ExternalCommandSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]consensus.ExternalCommandSource, len(sources))
	for idx, source := range sources {
		out[idx] = source
		out[idx].Args = cloneStrings(source.Args)
		out[idx].Env = cloneStringMap(source.Env)
		out[idx].Parsing.MetadataPaths = cloneStringMap(source.Parsing.MetadataPaths)
		out[idx].Parsing.RequiredPaths = cloneStrings(source.Parsing.RequiredPaths)
	}
	return out
}

func cloneDeepAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneDeepAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAny(item)
		}
		return out
	case []string:
		return cloneStrings(typed)
	case map[string]string:
		return cloneStringMap(typed)
	default:
		return typed
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func pickString(base string, overlay string) string {
	if overlay != "" {
		return overlay
	}
	return base
}
