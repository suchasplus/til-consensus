package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

type ResolvedRunPlan struct {
	RequestID    string
	Task         string
	Mode         consensus.WorkflowMode
	Roles        consensus.RoleAssignments
	LedgerPath   string
	ManifestPath string
	EventsPath   string
	ResultPath   string
	SummaryPath  string
	ErrorPath    string
	ArtifactsDir string
	Verbose      bool
	StartRequest consensus.StartRequest
}

type RunArtifactPaths struct {
	RunDir       string
	LedgerPath   string
	ManifestPath string
	EventsPath   string
	ResultPath   string
	SummaryPath  string
	ErrorPath    string
	ArtifactsDir string
}

func ResolveRunPlan(loaded LoadedConfig, input RunInput, overrides RunOverrides, now time.Time) (ResolvedRunPlan, error) {
	cfg := loaded.Config
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		requestID = artifact.NewRequestID(now)
	}
	task := firstNonEmpty(strings.TrimSpace(overrides.Task), strings.TrimSpace(input.TaskSpec.Goal))
	if task == "" {
		return ResolvedRunPlan{}, fmt.Errorf("missing task")
	}
	mode := pickMode(overrides.Mode, input.Mode, cfg.Defaults.Mode)
	roles, err := resolveRoles(mode, cfg.Roles, input.Roles, overrides)
	if err != nil {
		return ResolvedRunPlan{}, err
	}
	taskSpec := consensus.TaskSpec{
		Goal:              task,
		TaskType:          pickTaskType(input.TaskSpec.TaskType, cfg.Defaults.TaskType),
		Materials:         input.TaskSpec.Materials,
		Constraints:       mergeConstraints(cfg.Defaults.TaskConstraints, input.TaskSpec.Constraints),
		SuccessCriteria:   pickStrings(overrides.SuccessCriteria, input.TaskSpec.SuccessCriteria, cfg.Defaults.SuccessCriteria),
		OutOfScope:        pickStrings(input.TaskSpec.OutOfScope, cfg.Defaults.OutOfScope),
		AllowedTools:      pickStrings(nil, input.TaskSpec.AllowedTools, cfg.Defaults.AllowedTools),
		WorkspaceSnapshot: resolveWorkspaceSnapshot(cfg.Defaults.WorkspaceSnapshot, input.TaskSpec.WorkspaceSnapshot, overrides.WorkspaceSnapshot, loaded.ConfigDir),
		Context:           input.TaskSpec.Context,
	}
	proposalPolicy := consensus.ProposalPolicy{
		MaxPasses:          pickInt(input.ProposalPolicy.MaxPasses, cfg.Defaults.ProposalPolicy.MaxPasses, consensus.DefaultProposalPasses),
		MaxClaimsPerWorker: pickInt(input.ProposalPolicy.MaxClaimsPerWorker, cfg.Defaults.ProposalPolicy.MaxClaimsPerWorker, consensus.DefaultMaxClaimsPerWorker),
		DedupeStrategy:     firstNonEmpty(input.ProposalPolicy.DedupeStrategy, cfg.Defaults.ProposalPolicy.DedupeStrategy),
	}
	verificationPolicy := consensus.VerificationPolicy{
		RequiredChecks:        pickChecks(input.VerificationPolicy.RequiredChecks, cfg.Defaults.VerificationPolicy.RequiredChecks),
		AllowSemanticVerifier: cfg.Defaults.VerificationPolicy.AllowSemanticVerifier || input.VerificationPolicy.AllowSemanticVerifier,
		MaxParallelChecks:     pickInt(input.VerificationPolicy.MaxParallelChecks, cfg.Defaults.VerificationPolicy.MaxParallelChecks, consensus.DefaultMaxParallelChecks),
	}
	arbiterPolicy := consensus.ArbiterPolicy{
		AllowUndetermined: true,
		BlindReview:       true,
	}
	if cfg.Defaults.ArbiterPolicy.AllowUndetermined || input.ArbiterPolicy.AllowUndetermined {
		arbiterPolicy.AllowUndetermined = true
	}
	if cfg.Defaults.ArbiterPolicy.BlindReview || input.ArbiterPolicy.BlindReview {
		arbiterPolicy.BlindReview = true
	}
	perTaskTimeout := pickDuration(overrides.Timeout, cfg.Defaults.PerTaskTimeout.Duration, consensus.DefaultPerTaskTimeout)
	taskRetryAttempts := pickInt(input.TaskRetryAttempts, cfg.Defaults.TaskRetryAttempts, consensus.DefaultTaskRetryAttempts)
	globalDeadline := pickDuration(overrides.GlobalDeadline, cfg.Defaults.GlobalDeadline.Duration, 0)
	actionPrompt := firstNonEmpty(strings.TrimSpace(overrides.Action), strings.TrimSpace(input.Action))
	debatePolicy := consensus.DebatePolicy{
		MinRounds:       pickInt(overrides.MinRounds, input.DebatePolicy.MinRounds, cfg.Defaults.DebatePolicy.MinRounds, consensus.DefaultDebateMinRounds),
		MaxRounds:       pickInt(overrides.MaxRounds, input.DebatePolicy.MaxRounds, cfg.Defaults.DebatePolicy.MaxRounds, consensus.DefaultDebateMaxRounds),
		VoteThreshold:   pickFloat(overrides.VoteThreshold, input.DebatePolicy.VoteThreshold, cfg.Defaults.DebatePolicy.VoteThreshold, consensus.DefaultVoteThreshold),
		EnableEarlyStop: cfg.Defaults.DebatePolicy.EnableEarlyStop || input.DebatePolicy.EnableEarlyStop,
		PeerContextMode: firstNonEmpty(input.DebatePolicy.PeerContextMode, cfg.Defaults.DebatePolicy.PeerContextMode, "summary+active_claims"),
	}
	if !debatePolicy.EnableEarlyStop {
		debatePolicy.EnableEarlyStop = true
	}
	delphiPolicy := consensus.DelphiPolicy{
		MinRounds:               pickInt(overrides.MinRounds, input.DelphiPolicy.MinRounds, cfg.Defaults.DelphiPolicy.MinRounds, consensus.DefaultDelphiMinRounds),
		MaxRounds:               pickInt(overrides.MaxRounds, input.DelphiPolicy.MaxRounds, cfg.Defaults.DelphiPolicy.MaxRounds, consensus.DefaultDelphiMaxRounds),
		ConvergenceThreshold:    pickFloat(overrides.ConvergenceThreshold, input.DelphiPolicy.ConvergenceThreshold, cfg.Defaults.DelphiPolicy.ConvergenceThreshold, consensus.DefaultConvergence),
		RatingScaleMin:          pickInt(input.DelphiPolicy.RatingScaleMin, cfg.Defaults.DelphiPolicy.RatingScaleMin, consensus.DefaultRatingScaleMin),
		RatingScaleMax:          pickInt(input.DelphiPolicy.RatingScaleMax, cfg.Defaults.DelphiPolicy.RatingScaleMax, consensus.DefaultRatingScaleMax),
		Anonymous:               cfg.Defaults.DelphiPolicy.Anonymous || input.DelphiPolicy.Anonymous,
		FacilitatorSummaryStyle: firstNonEmpty(input.DelphiPolicy.FacilitatorSummaryStyle, cfg.Defaults.DelphiPolicy.FacilitatorSummaryStyle, "anonymous-aggregate"),
	}
	if !delphiPolicy.Anonymous {
		delphiPolicy.Anonymous = true
	}

	artifactPaths := ResolveRunArtifacts(loaded, requestID)

	startRequest := consensus.StartRequest{
		Mode:               mode,
		RequestID:          requestID,
		TaskSpec:           taskSpec,
		Roles:              roles,
		ProposalPolicy:     proposalPolicy,
		VerificationPolicy: verificationPolicy,
		ArbiterPolicy:      arbiterPolicy,
		IngestPolicy:       pickIngestPolicy(input.IngestPolicy, cfg.Defaults.IngestPolicy),
		FallbackPolicy:     pickFallbackPolicy(input.FallbackPolicy, cfg.Defaults.FallbackPolicy),
		ObservePolicy:      pickObservePolicy(input.ObservePolicy, cfg.Defaults.ObservePolicy),
		DebatePolicy:       debatePolicy,
		DelphiPolicy:       delphiPolicy,
		ReportPolicy: consensus.ReportPolicy{
			Style: "builtin",
		},
		WaitingPolicy: consensus.WaitingPolicy{
			PerTaskTimeout: perTaskTimeout,
			GlobalDeadline: globalDeadline,
			RetryAttempts:  taskRetryAttempts,
		},
	}
	if actionPrompt != "" {
		startRequest.ActionPolicy = &consensus.ActionPolicy{
			Prompt:        actionPrompt,
			ActorID:       roles.Actor,
			IncludeResult: true,
		}
	}
	normalized, err := consensus.NormalizeStartRequest(startRequest)
	if err != nil {
		return ResolvedRunPlan{}, err
	}
	return ResolvedRunPlan{
		RequestID:    requestID,
		Task:         task,
		Mode:         mode,
		Roles:        roles,
		LedgerPath:   artifactPaths.LedgerPath,
		ManifestPath: artifactPaths.ManifestPath,
		EventsPath:   artifactPaths.EventsPath,
		ResultPath:   artifactPaths.ResultPath,
		SummaryPath:  artifactPaths.SummaryPath,
		ErrorPath:    artifactPaths.ErrorPath,
		ArtifactsDir: artifactPaths.ArtifactsDir,
		Verbose:      overrides.Verbose,
		StartRequest: normalized,
	}, nil
}

func ResolveRunArtifacts(loaded LoadedConfig, requestID string) RunArtifactPaths {
	baseDir := loaded.Config.Output.Directory
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./out/{requestId}"
	}
	baseDir = resolveOutputPath(baseDir, loaded.ConfigDir, requestID)
	artifactsDir := resolveOutputPath(fallbackPath(loaded.Config.Output.ArtifactsDir, filepath.Join(baseDir, "artifacts")), loaded.ConfigDir, requestID)
	return RunArtifactPaths{
		RunDir:       baseDir,
		LedgerPath:   resolveOutputPath(fallbackPath(loaded.Config.Output.LedgerPath, filepath.Join(baseDir, "ledger.jsonl")), loaded.ConfigDir, requestID),
		ManifestPath: resolveOutputPath(filepath.Join(artifactsDirPlaceholder(loaded.Config.Output.ArtifactsDir, baseDir), "manifest.jsonl"), loaded.ConfigDir, requestID),
		EventsPath:   resolveOutputPath(fallbackPath(loaded.Config.Output.EventsPath, filepath.Join(baseDir, "events.jsonl")), loaded.ConfigDir, requestID),
		ResultPath:   resolveOutputPath(fallbackPath(loaded.Config.Output.ResultPath, filepath.Join(baseDir, "result.json")), loaded.ConfigDir, requestID),
		SummaryPath:  resolveOutputPath(fallbackPath(loaded.Config.Output.SummaryPath, filepath.Join(baseDir, "summary.md")), loaded.ConfigDir, requestID),
		ErrorPath:    resolveOutputPath(fallbackPath(loaded.Config.Output.ErrorPath, filepath.Join(baseDir, "error.json")), loaded.ConfigDir, requestID),
		ArtifactsDir: artifactsDir,
	}
}

func ResolveResultTemplate(loaded LoadedConfig) string {
	requestToken := "{requestId}"
	baseDir := loaded.Config.Output.Directory
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./out/{requestId}"
	}
	baseDir = resolveOutputPath(baseDir, loaded.ConfigDir, requestToken)
	return resolveOutputPath(
		fallbackPath(loaded.Config.Output.ResultPath, filepath.Join(baseDir, "result.json")),
		loaded.ConfigDir,
		requestToken,
	)
}

func resolveRoles(mode consensus.WorkflowMode, cfg RolesConfig, input RolesConfig, overrides RunOverrides) (consensus.RoleAssignments, error) {
	roles := consensus.RoleAssignments{
		Proposers:        pickStrings(overrides.Proposers, input.Proposers, cfg.Proposers),
		Challengers:      pickStrings(overrides.Challengers, input.Challengers, cfg.Challengers),
		Participants:     pickStrings(overrides.Participants, input.Participants, cfg.Participants),
		Arbiter:          firstNonEmpty(overrides.Arbiter, input.Arbiter, cfg.Arbiter),
		SemanticVerifier: firstNonEmpty(overrides.SemanticVerifier, input.SemanticVerifier, cfg.SemanticVerifier),
		Facilitator:      firstNonEmpty(overrides.Facilitator, input.Facilitator, cfg.Facilitator),
		Reporter:         firstNonEmpty(overrides.Reporter, input.Reporter, cfg.Reporter),
		Actor:            firstNonEmpty(overrides.Actor, input.Actor, cfg.Actor),
	}
	switch mode {
	case consensus.WorkflowModeFreeDebate, consensus.WorkflowModeDelphi:
		if len(roles.Participants) == 0 {
			return consensus.RoleAssignments{}, fmt.Errorf("at least one participant is required")
		}
	default:
		if len(roles.Proposers) == 0 {
			return consensus.RoleAssignments{}, fmt.Errorf("at least one proposer is required")
		}
		if len(roles.Challengers) == 0 {
			return consensus.RoleAssignments{}, fmt.Errorf("at least one challenger is required")
		}
	}
	return roles, nil
}

func pickMode(values ...consensus.WorkflowMode) consensus.WorkflowMode {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return consensus.WorkflowModeAdjudication
}

func resolveWorkspaceSnapshot(defaultValue *consensus.WorkspaceSnapshot, input *consensus.WorkspaceSnapshot, override string, baseDir string) *consensus.WorkspaceSnapshot {
	if strings.TrimSpace(override) != "" {
		return &consensus.WorkspaceSnapshot{Root: resolveOutputPath(override, baseDir, "")}
	}
	if input != nil {
		clone := *input
		if clone.Root != "" {
			clone.Root = resolveOutputPath(clone.Root, baseDir, "")
		}
		return &clone
	}
	if defaultValue != nil {
		clone := *defaultValue
		if clone.Root != "" {
			clone.Root = resolveOutputPath(clone.Root, baseDir, "")
		}
		return &clone
	}
	return nil
}

func mergeConstraints(base consensus.TaskConstraints, override consensus.TaskConstraints) consensus.TaskConstraints {
	out := base
	if override.Language != "" {
		out.Language = override.Language
	}
	if len(override.AllowedPaths) > 0 {
		out.AllowedPaths = override.AllowedPaths
	}
	if len(override.RequiredCommands) > 0 {
		out.RequiredCommands = override.RequiredCommands
	}
	if len(override.Notes) > 0 {
		out.Notes = override.Notes
	}
	return out
}

func pickChecks(values ...[]consensus.VerificationCheck) []consensus.VerificationCheck {
	for _, value := range values {
		if len(value) > 0 {
			return append([]consensus.VerificationCheck(nil), value...)
		}
	}
	return nil
}

func pickFloat(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func pickTaskType(values ...consensus.CaseTaskType) consensus.CaseTaskType {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return consensus.CaseTaskTypeUnknown
}

func pickIngestPolicy(values ...consensus.IngestPolicy) consensus.IngestPolicy {
	for _, value := range values {
		if len(value.Sources) > 0 {
			return consensus.IngestPolicy{Sources: append([]consensus.ExternalCommandSource(nil), value.Sources...)}
		}
	}
	return consensus.IngestPolicy{}
}

func pickObservePolicy(values ...consensus.ObservePolicy) consensus.ObservePolicy {
	for _, value := range values {
		if len(value.Sources) > 0 || value.OnContradiction != "" {
			return consensus.ObservePolicy{
				Sources:         append([]consensus.ExternalCommandSource(nil), value.Sources...),
				OnContradiction: value.OnContradiction,
			}
		}
	}
	return consensus.ObservePolicy{}
}

func pickFallbackPolicy(values ...consensus.AdjudicationFallbackPolicy) consensus.AdjudicationFallbackPolicy {
	for _, value := range values {
		if value.MaxFallbackRounds != 0 || value.OnInsufficientEvidence != "" || value.OnUnresolvedConflict != "" || value.OnUnresolvedClaims != "" || value.OnKeepWithCaveat != "" {
			return value
		}
	}
	return consensus.AdjudicationFallbackPolicy{}
}

func resolveOutputPath(rawPath, baseDir, requestID string) string {
	path := strings.ReplaceAll(rawPath, "{requestId}", requestID)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func dedupe(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pickInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func pickDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func pickStrings(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return dedupe(value)
		}
	}
	return nil
}

func fallbackPath(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func artifactsDirPlaceholder(value string, baseDir string) string {
	if strings.TrimSpace(value) == "" {
		return filepath.Join(baseDir, "artifacts")
	}
	return value
}
