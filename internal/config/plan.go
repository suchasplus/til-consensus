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
	Roles        consensus.RoleAssignments
	LedgerPath   string
	EventsPath   string
	ResultPath   string
	SummaryPath  string
	ErrorPath    string
	ArtifactsDir string
	Verbose      bool
	StartRequest consensus.StartRequest
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
	roles, err := resolveRoles(cfg.Roles, input.Roles, overrides)
	if err != nil {
		return ResolvedRunPlan{}, err
	}
	taskSpec := consensus.TaskSpec{
		Goal:              task,
		Materials:         input.TaskSpec.Materials,
		Constraints:       mergeConstraints(cfg.Defaults.TaskConstraints, input.TaskSpec.Constraints),
		SuccessCriteria:   pickStrings(overrides.SuccessCriteria, input.TaskSpec.SuccessCriteria, cfg.Defaults.SuccessCriteria),
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
	globalDeadline := pickDuration(overrides.GlobalDeadline, cfg.Defaults.GlobalDeadline.Duration, 0)
	actionPrompt := firstNonEmpty(strings.TrimSpace(overrides.Action), strings.TrimSpace(input.Action))

	baseDir := cfg.Output.Directory
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./out/{requestId}"
	}
	baseDir = resolveOutputPath(baseDir, loaded.ConfigDir, requestID)
	ledgerPath := resolveOutputPath(fallbackPath(cfg.Output.LedgerPath, filepath.Join(baseDir, "ledger.jsonl")), loaded.ConfigDir, requestID)
	eventsPath := resolveOutputPath(fallbackPath(cfg.Output.EventsPath, filepath.Join(baseDir, "events.jsonl")), loaded.ConfigDir, requestID)
	resultPath := resolveOutputPath(fallbackPath(cfg.Output.ResultPath, filepath.Join(baseDir, "result.json")), loaded.ConfigDir, requestID)
	summaryPath := resolveOutputPath(fallbackPath(cfg.Output.SummaryPath, filepath.Join(baseDir, "summary.md")), loaded.ConfigDir, requestID)
	errorPath := resolveOutputPath(fallbackPath(cfg.Output.ErrorPath, filepath.Join(baseDir, "error.json")), loaded.ConfigDir, requestID)
	artifactsDir := resolveOutputPath(fallbackPath(cfg.Output.ArtifactsDir, filepath.Join(baseDir, "artifacts")), loaded.ConfigDir, requestID)

	startRequest := consensus.StartRequest{
		RequestID:          requestID,
		TaskSpec:           taskSpec,
		Roles:              roles,
		ProposalPolicy:     proposalPolicy,
		VerificationPolicy: verificationPolicy,
		ArbiterPolicy:      arbiterPolicy,
		ReportPolicy: consensus.ReportPolicy{
			Style: "builtin",
		},
		WaitingPolicy: consensus.WaitingPolicy{
			PerTaskTimeout: perTaskTimeout,
			GlobalDeadline: globalDeadline,
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
		Roles:        roles,
		LedgerPath:   ledgerPath,
		EventsPath:   eventsPath,
		ResultPath:   resultPath,
		SummaryPath:  summaryPath,
		ErrorPath:    errorPath,
		ArtifactsDir: artifactsDir,
		Verbose:      overrides.Verbose,
		StartRequest: normalized,
	}, nil
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

func resolveRoles(cfg RolesConfig, input RolesConfig, overrides RunOverrides) (consensus.RoleAssignments, error) {
	roles := consensus.RoleAssignments{
		Proposers:        pickStrings(overrides.Proposers, input.Proposers, cfg.Proposers),
		Challengers:      pickStrings(overrides.Challengers, input.Challengers, cfg.Challengers),
		Arbiter:          firstNonEmpty(overrides.Arbiter, input.Arbiter, cfg.Arbiter),
		SemanticVerifier: firstNonEmpty(overrides.SemanticVerifier, input.SemanticVerifier, cfg.SemanticVerifier),
		Reporter:         firstNonEmpty(overrides.Reporter, input.Reporter, cfg.Reporter),
		Actor:            firstNonEmpty(overrides.Actor, input.Actor, cfg.Actor),
	}
	if len(roles.Proposers) == 0 {
		return consensus.RoleAssignments{}, fmt.Errorf("at least one proposer is required")
	}
	if len(roles.Challengers) == 0 {
		return consensus.RoleAssignments{}, fmt.Errorf("at least one challenger is required")
	}
	return roles, nil
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
