package consensus

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	DefaultMinParticipants      = 2
	DefaultMinRounds            = 2
	DefaultMaxRounds            = 3
	DefaultConsensusThreshold   = 1.0
	DefaultPeerChars            = 6000
	DefaultPeerCount            = 10
	DefaultPerTaskTimeout       = 20 * time.Minute
	DefaultPerRoundTimeout      = 20 * time.Minute
	DefaultSessionKeyPrefix     = "consensus"
	DefaultPeerOverflowStrategy = OverflowTruncateTail
)

type Participant struct {
	ID   string `json:"id" yaml:"id"`
	Role string `json:"role,omitempty" yaml:"role,omitempty"`
}

type ParticipantsPolicy struct {
	MinParticipants int `json:"minParticipants" yaml:"min_participants"`
}

type RoundPolicy struct {
	MinRounds int `json:"minRounds" yaml:"min_rounds"`
	MaxRounds int `json:"maxRounds" yaml:"max_rounds"`
}

type SessionPolicy struct {
	Mode             string `json:"mode" yaml:"mode"`
	SessionKeyPrefix string `json:"sessionKeyPrefix,omitempty" yaml:"session_key_prefix,omitempty"`
}

type OverflowStrategy string

const (
	OverflowTruncateTail   OverflowStrategy = "truncate-tail"
	OverflowTruncateMiddle OverflowStrategy = "truncate-middle"
)

type PeerContextPolicy struct {
	PassMode                string           `json:"passMode" yaml:"pass_mode"`
	MaxCharsPerPeerResponse int              `json:"maxCharsPerPeerResponse" yaml:"max_chars_per_peer_response"`
	MaxPeersPerRound        int              `json:"maxPeersPerRound" yaml:"max_peers_per_round"`
	OverflowStrategy        OverflowStrategy `json:"overflowStrategy" yaml:"overflow_strategy"`
}

type RubricWeights struct {
	Correctness   float64 `json:"correctness" yaml:"correctness"`
	Completeness  float64 `json:"completeness" yaml:"completeness"`
	Actionability float64 `json:"actionability" yaml:"actionability"`
	Consistency   float64 `json:"consistency" yaml:"consistency"`
}

type TieBreaker string

const (
	TieBreakerLatestRoundScore TieBreaker = "latest-round-score"
	TieBreakerLeastObjection   TieBreaker = "least-objection"
)

type ScoringPolicy struct {
	Enabled    bool          `json:"enabled" yaml:"enabled"`
	TieBreaker TieBreaker    `json:"tieBreaker" yaml:"tie_breaker"`
	Rubric     RubricWeights `json:"rubric" yaml:"rubric"`
}

type ConsensusPolicy struct {
	Threshold float64 `json:"threshold" yaml:"threshold"`
}

type ReportComposer string

const (
	ReportComposerBuiltin        ReportComposer = "builtin"
	ReportComposerRepresentative ReportComposer = "representative"
)

type TraceLevel string

const (
	TraceLevelCompact TraceLevel = "compact"
	TraceLevelFull    TraceLevel = "full"
)

type ReportPolicy struct {
	IncludeDeliberationTrace bool           `json:"includeDeliberationTrace" yaml:"include_deliberation_trace"`
	TraceLevel               TraceLevel     `json:"traceLevel" yaml:"trace_level"`
	Composer                 ReportComposer `json:"composer" yaml:"composer"`
	RepresentativeID         string         `json:"representativeId,omitempty" yaml:"representative_id,omitempty"`
}

type ActionPolicy struct {
	Prompt            string `json:"prompt" yaml:"prompt"`
	ActorID           string `json:"actorId,omitempty" yaml:"actor_id,omitempty"`
	IncludeFullResult bool   `json:"includeFullResult" yaml:"include_full_result"`
}

type WaitingPolicy struct {
	PerTaskTimeout  time.Duration `json:"perTaskTimeoutMs" yaml:"per_task_timeout"`
	PerRoundTimeout time.Duration `json:"perRoundTimeoutMs" yaml:"per_round_timeout"`
	GlobalDeadline  time.Duration `json:"globalDeadlineMs,omitempty" yaml:"global_deadline,omitempty"`
}

type Constraints struct {
	Language        string `json:"language,omitempty" yaml:"language,omitempty"`
	TokenBudgetHint int    `json:"tokenBudgetHint,omitempty" yaml:"token_budget_hint,omitempty"`
}

type StartRequest struct {
	RequestID          string             `json:"requestId" yaml:"request_id"`
	Task               string             `json:"task" yaml:"task"`
	Participants       []Participant      `json:"participants" yaml:"participants"`
	ParticipantsPolicy ParticipantsPolicy `json:"participantsPolicy" yaml:"participants_policy"`
	RoundPolicy        RoundPolicy        `json:"roundPolicy" yaml:"round_policy"`
	SessionPolicy      SessionPolicy      `json:"sessionPolicy" yaml:"session_policy"`
	PeerContextPolicy  PeerContextPolicy  `json:"peerContextPolicy" yaml:"peer_context_policy"`
	ScoringPolicy      ScoringPolicy      `json:"scoringPolicy" yaml:"scoring_policy"`
	ConsensusPolicy    ConsensusPolicy    `json:"consensusPolicy" yaml:"consensus_policy"`
	ReportPolicy       ReportPolicy       `json:"reportPolicy" yaml:"report_policy"`
	ActionPolicy       *ActionPolicy      `json:"actionPolicy,omitempty" yaml:"action_policy,omitempty"`
	WaitingPolicy      WaitingPolicy      `json:"waitingPolicy" yaml:"waiting_policy"`
	Constraints        *Constraints       `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	Context            map[string]any     `json:"context,omitempty" yaml:"context,omitempty"`
}

func NormalizeStartRequest(in StartRequest) (StartRequest, error) {
	out := in
	out.Task = strings.TrimSpace(out.Task)
	out.RequestID = strings.TrimSpace(out.RequestID)

	if out.ParticipantsPolicy.MinParticipants == 0 {
		out.ParticipantsPolicy.MinParticipants = DefaultMinParticipants
	}
	if out.RoundPolicy.MinRounds == 0 {
		out.RoundPolicy.MinRounds = DefaultMinRounds
	}
	if out.RoundPolicy.MaxRounds == 0 {
		out.RoundPolicy.MaxRounds = DefaultMaxRounds
	}
	if out.SessionPolicy.Mode == "" {
		out.SessionPolicy.Mode = "sticky-per-participant"
	}
	if out.SessionPolicy.SessionKeyPrefix == "" {
		out.SessionPolicy.SessionKeyPrefix = DefaultSessionKeyPrefix
	}
	if out.PeerContextPolicy.PassMode == "" {
		out.PeerContextPolicy.PassMode = "full-response-preferred"
	}
	if out.PeerContextPolicy.MaxCharsPerPeerResponse == 0 {
		out.PeerContextPolicy.MaxCharsPerPeerResponse = DefaultPeerChars
	}
	if out.PeerContextPolicy.MaxPeersPerRound == 0 {
		out.PeerContextPolicy.MaxPeersPerRound = DefaultPeerCount
	}
	if out.PeerContextPolicy.OverflowStrategy == "" {
		out.PeerContextPolicy.OverflowStrategy = DefaultPeerOverflowStrategy
	}
	if out.ConsensusPolicy.Threshold == 0 {
		out.ConsensusPolicy.Threshold = DefaultConsensusThreshold
	}
	if !out.ScoringPolicy.Enabled {
		out.ScoringPolicy.Enabled = true
	}
	if out.ScoringPolicy.TieBreaker == "" {
		out.ScoringPolicy.TieBreaker = TieBreakerLatestRoundScore
	}
	if out.ScoringPolicy.Rubric == (RubricWeights{}) {
		out.ScoringPolicy.Rubric = RubricWeights{
			Correctness:   0.35,
			Completeness:  0.25,
			Actionability: 0.25,
			Consistency:   0.15,
		}
	}
	if out.ReportPolicy.TraceLevel == "" {
		out.ReportPolicy.TraceLevel = TraceLevelCompact
	}
	if out.ReportPolicy.Composer == "" {
		out.ReportPolicy.Composer = ReportComposerBuiltin
	}
	if out.WaitingPolicy.PerTaskTimeout == 0 {
		out.WaitingPolicy.PerTaskTimeout = DefaultPerTaskTimeout
	}
	if out.WaitingPolicy.PerRoundTimeout == 0 {
		out.WaitingPolicy.PerRoundTimeout = DefaultPerRoundTimeout
	}
	if out.ActionPolicy != nil && !out.ActionPolicy.IncludeFullResult {
		// keep explicit false
	} else if out.ActionPolicy != nil {
		out.ActionPolicy.IncludeFullResult = true
	}
	if out.Context != nil {
		out.Context = maps.Clone(out.Context)
	}

	if err := ValidateStartRequest(out); err != nil {
		return StartRequest{}, err
	}
	return out, nil
}

func ValidateStartRequest(in StartRequest) error {
	if strings.TrimSpace(in.RequestID) == "" {
		return fmt.Errorf("request_id is required")
	}
	if strings.TrimSpace(in.Task) == "" {
		return fmt.Errorf("task is required")
	}
	if len(in.Participants) < 2 {
		return fmt.Errorf("at least two participants are required")
	}
	seen := map[string]struct{}{}
	for _, participant := range in.Participants {
		if strings.TrimSpace(participant.ID) == "" {
			return fmt.Errorf("participant id is required")
		}
		if _, ok := seen[participant.ID]; ok {
			return fmt.Errorf("duplicate participant id: %s", participant.ID)
		}
		seen[participant.ID] = struct{}{}
	}
	if in.ParticipantsPolicy.MinParticipants < 2 {
		return fmt.Errorf("min participants must be >= 2")
	}
	if len(in.Participants) < in.ParticipantsPolicy.MinParticipants {
		return fmt.Errorf("participants length must be >= min participants")
	}
	if in.RoundPolicy.MinRounds < 0 {
		return fmt.Errorf("min rounds must be >= 0")
	}
	if in.RoundPolicy.MaxRounds < 1 {
		return fmt.Errorf("max rounds must be >= 1")
	}
	if in.RoundPolicy.MaxRounds < in.RoundPolicy.MinRounds {
		return fmt.Errorf("max rounds must be >= min rounds")
	}
	if in.PeerContextPolicy.MaxCharsPerPeerResponse < 200 {
		return fmt.Errorf("max chars per peer response must be >= 200")
	}
	if in.PeerContextPolicy.MaxPeersPerRound < 1 {
		return fmt.Errorf("max peers per round must be >= 1")
	}
	if in.ConsensusPolicy.Threshold < 0 || in.ConsensusPolicy.Threshold > 1 {
		return fmt.Errorf("consensus threshold must be in [0,1]")
	}
	if in.WaitingPolicy.PerTaskTimeout <= 0 || in.WaitingPolicy.PerRoundTimeout <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	if in.WaitingPolicy.GlobalDeadline < 0 {
		return fmt.Errorf("global deadline must be >= 0")
	}
	if in.ReportPolicy.Composer != ReportComposerBuiltin && in.ReportPolicy.Composer != ReportComposerRepresentative {
		return fmt.Errorf("unsupported report composer: %s", in.ReportPolicy.Composer)
	}
	if in.ReportPolicy.TraceLevel != TraceLevelCompact && in.ReportPolicy.TraceLevel != TraceLevelFull {
		return fmt.Errorf("unsupported trace level: %s", in.ReportPolicy.TraceLevel)
	}
	if in.PeerContextPolicy.OverflowStrategy != OverflowTruncateTail && in.PeerContextPolicy.OverflowStrategy != OverflowTruncateMiddle {
		return fmt.Errorf("unsupported overflow strategy: %s", in.PeerContextPolicy.OverflowStrategy)
	}
	if in.ScoringPolicy.TieBreaker != TieBreakerLatestRoundScore && in.ScoringPolicy.TieBreaker != TieBreakerLeastObjection {
		return fmt.Errorf("unsupported tie breaker: %s", in.ScoringPolicy.TieBreaker)
	}
	return nil
}

func ParticipantIDs(participants []Participant) []string {
	out := make([]string, 0, len(participants))
	for _, participant := range participants {
		out = append(out, participant.ID)
	}
	slices.Sort(out)
	return out
}
