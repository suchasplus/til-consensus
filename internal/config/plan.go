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
	RequestID      string
	Task           string
	ParticipantIDs []string
	EventsPath     string
	ResultPath     string
	SummaryPath    string
	ErrorPath      string
	Verbose        bool
	StartRequest   consensus.StartRequest
}

func ResolveRunPlan(loaded LoadedConfig, input RunInput, overrides RunOverrides, now time.Time) (ResolvedRunPlan, error) {
	cfg := loaded.Config
	requestID := firstNonEmpty(overridesTaskID(overrides), input.RequestID)
	if requestID == "" {
		requestID = artifact.NewRequestID(now)
	}
	task := firstNonEmpty(strings.TrimSpace(overrides.Task), strings.TrimSpace(input.Task))
	if task == "" {
		return ResolvedRunPlan{}, fmt.Errorf("missing task")
	}
	participantIDs, err := resolveParticipants(cfg, input, overrides)
	if err != nil {
		return ResolvedRunPlan{}, err
	}
	participants := make([]consensus.Participant, 0, len(participantIDs))
	for _, id := range participantIDs {
		for _, agent := range cfg.Agents {
			if agent.ID == id {
				participants = append(participants, consensus.Participant{ID: agent.ID, Role: agent.Role})
				break
			}
		}
	}

	minRounds := pickInt(overrides.MinRounds, input.MinRounds, cfg.Defaults.MinRounds, consensus.DefaultMinRounds)
	maxRounds := pickInt(overrides.MaxRounds, input.MaxRounds, cfg.Defaults.MaxRounds, consensus.DefaultMaxRounds)
	if maxRounds < minRounds {
		return ResolvedRunPlan{}, fmt.Errorf("max rounds must be >= min rounds")
	}
	threshold := pickFloat(overrides.Threshold, input.Threshold, cfg.Defaults.Threshold, consensus.DefaultConsensusThreshold)
	timeout := pickDuration(overrides.Timeout, input.Timeout.Duration, cfg.Defaults.PerRoundTimeout.Duration, consensus.DefaultPerRoundTimeout)
	perTaskTimeout := pickDuration(0, input.Timeout.Duration, cfg.Defaults.PerTaskTimeout.Duration, timeout)
	if overrides.Timeout > 0 {
		perTaskTimeout = overrides.Timeout
	}
	globalDeadline := pickDuration(overrides.GlobalDeadline, input.GlobalDeadline.Duration, cfg.Defaults.GlobalDeadline.Duration, 0)
	language := firstNonEmpty(input.Language, cfg.Defaults.Language)
	tokenBudgetHint := pickInt(0, input.TokenBudgetHint, cfg.Defaults.TokenBudgetHint, 0)
	composer := firstNonEmpty(input.Composer, cfg.Defaults.Composer)
	if composer == "" {
		composer = string(consensus.ReportComposerBuiltin)
	}
	traceLevel := firstNonEmpty(input.TraceLevel, cfg.Defaults.TraceLevel)
	if traceLevel == "" {
		traceLevel = string(consensus.TraceLevelCompact)
	}
	includeTrace := cfg.Defaults.IncludeDeliberationTrace
	if input.IncludeDeliberationTrace != nil {
		includeTrace = *input.IncludeDeliberationTrace
	}

	baseDir := cfg.Output.Directory
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./out/{requestId}"
	}
	baseDir = resolveOutputPath(baseDir, loaded.ConfigDir, requestID)
	eventsPath := resolveOutputPath(fallbackPath(cfg.Output.EventsPath, filepath.Join(baseDir, "events.jsonl")), loaded.ConfigDir, requestID)
	resultPath := resolveOutputPath(fallbackPath(cfg.Output.ResultPath, filepath.Join(baseDir, "result.json")), loaded.ConfigDir, requestID)
	summaryPath := resolveOutputPath(fallbackPath(cfg.Output.SummaryPath, filepath.Join(baseDir, "summary.md")), loaded.ConfigDir, requestID)
	errorPath := resolveOutputPath(fallbackPath(cfg.Output.ErrorPath, filepath.Join(baseDir, "error.json")), loaded.ConfigDir, requestID)

	startRequest := consensus.StartRequest{
		RequestID:    requestID,
		Task:         task,
		Participants: participants,
		RoundPolicy: consensus.RoundPolicy{
			MinRounds: minRounds,
			MaxRounds: maxRounds,
		},
		ParticipantsPolicy: consensus.ParticipantsPolicy{MinParticipants: consensus.DefaultMinParticipants},
		SessionPolicy: consensus.SessionPolicy{
			Mode:             "sticky-per-participant",
			SessionKeyPrefix: "consensus",
		},
		PeerContextPolicy: consensus.PeerContextPolicy{
			PassMode:                "full-response-preferred",
			MaxCharsPerPeerResponse: consensus.DefaultPeerChars,
			MaxPeersPerRound:        consensus.DefaultPeerCount,
			OverflowStrategy:        consensus.DefaultPeerOverflowStrategy,
		},
		ScoringPolicy: consensus.ScoringPolicy{
			Enabled:    true,
			TieBreaker: consensus.TieBreakerLatestRoundScore,
			Rubric: consensus.RubricWeights{
				Correctness:   0.35,
				Completeness:  0.25,
				Actionability: 0.25,
				Consistency:   0.15,
			},
		},
		ConsensusPolicy: consensus.ConsensusPolicy{Threshold: threshold},
		ReportPolicy: consensus.ReportPolicy{
			Composer:                 consensus.ReportComposer(composer),
			RepresentativeID:         firstNonEmpty(input.RepresentativeID, cfg.Defaults.RepresentativeID),
			IncludeDeliberationTrace: includeTrace,
			TraceLevel:               consensus.TraceLevel(traceLevel),
		},
		WaitingPolicy: consensus.WaitingPolicy{
			PerTaskTimeout:  perTaskTimeout,
			PerRoundTimeout: timeout,
			GlobalDeadline:  globalDeadline,
		},
		Context: input.Context,
	}
	if language != "" || tokenBudgetHint > 0 {
		startRequest.Constraints = &consensus.Constraints{
			Language:        language,
			TokenBudgetHint: tokenBudgetHint,
		}
	}
	actionPrompt := firstNonEmpty(strings.TrimSpace(overrides.Action), strings.TrimSpace(input.Action))
	if actionPrompt != "" {
		startRequest.ActionPolicy = &consensus.ActionPolicy{
			Prompt:            actionPrompt,
			IncludeFullResult: true,
		}
	}

	return ResolvedRunPlan{
		RequestID:      requestID,
		Task:           task,
		ParticipantIDs: participantIDs,
		EventsPath:     eventsPath,
		ResultPath:     resultPath,
		SummaryPath:    summaryPath,
		ErrorPath:      errorPath,
		Verbose:        overrides.Verbose,
		StartRequest:   startRequest,
	}, nil
}

func resolveParticipants(cfg Config, input RunInput, overrides RunOverrides) ([]string, error) {
	source := overrides.Agents
	if len(source) == 0 {
		source = input.Agents
	}
	if len(source) == 0 {
		source = cfg.Defaults.DefaultAgents
	}
	if len(source) == 0 {
		for _, agent := range cfg.Agents {
			source = append(source, agent.ID)
		}
	}
	source = dedupe(source)
	if len(source) < 2 {
		return nil, fmt.Errorf("at least two agents are required")
	}
	known := map[string]struct{}{}
	for _, agent := range cfg.Agents {
		known[agent.ID] = struct{}{}
	}
	for _, id := range source {
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("unknown agent id: %s", id)
		}
	}
	return source, nil
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
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
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

func pickFloat(values ...float64) float64 {
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

func fallbackPath(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func overridesTaskID(_ RunOverrides) string {
	return ""
}
