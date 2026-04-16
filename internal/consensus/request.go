package consensus

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	DefaultPerTaskTimeout           = 20 * time.Minute
	DefaultTaskRetryAttempts        = 1
	DefaultProposalPasses           = 1
	DefaultMaxClaimsPerWorker       = 5
	DefaultMaxParallelChecks        = 4
	DefaultMaxRevisionRounds        = 2
	DefaultMaxVerificationRounds    = 2
	DefaultConfidenceDeltaEpsilon   = 0.05
	DefaultMaxAdjudicationFallbacks = 1
	DefaultDebateMinRounds          = 2
	DefaultDebateMaxRounds          = 3
	DefaultVoteThreshold            = 1.0
	DefaultDelphiMinRounds          = 2
	DefaultDelphiMaxRounds          = 3
	DefaultConvergence              = 0.8
	DefaultRatingScaleMin           = 1
	DefaultRatingScaleMax           = 5
)

type WorkflowMode string

const (
	WorkflowModeAdjudication WorkflowMode = "adjudication"
	WorkflowModeFreeDebate   WorkflowMode = "free_debate"
	WorkflowModeDelphi       WorkflowMode = "delphi"
)

type MaterialRef struct {
	ID       string         `json:"id,omitempty" yaml:"id,omitempty"`
	Title    string         `json:"title,omitempty" yaml:"title,omitempty"`
	Kind     string         `json:"kind,omitempty" yaml:"kind,omitempty"`
	Path     string         `json:"path,omitempty" yaml:"path,omitempty"`
	Content  string         `json:"content,omitempty" yaml:"content,omitempty"`
	Hash     string         `json:"hash,omitempty" yaml:"hash,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type WorkspaceSnapshot struct {
	Root     string   `json:"root,omitempty" yaml:"root,omitempty"`
	Revision string   `json:"revision,omitempty" yaml:"revision,omitempty"`
	Paths    []string `json:"paths,omitempty" yaml:"paths,omitempty"`
	Hash     string   `json:"hash,omitempty" yaml:"hash,omitempty"`
}

type TaskConstraints struct {
	Language         string   `json:"language,omitempty" yaml:"language,omitempty"`
	AllowedPaths     []string `json:"allowedPaths,omitempty" yaml:"allowed_paths,omitempty"`
	RequiredCommands []string `json:"requiredCommands,omitempty" yaml:"required_commands,omitempty"`
	Notes            []string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type TaskSpec struct {
	Goal              string             `json:"goal" yaml:"goal"`
	TaskType          CaseTaskType       `json:"taskType,omitempty" yaml:"task_type,omitempty"`
	Materials         []MaterialRef      `json:"materials,omitempty" yaml:"materials,omitempty"`
	Constraints       TaskConstraints    `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	SuccessCriteria   []string           `json:"successCriteria,omitempty" yaml:"success_criteria,omitempty"`
	OutOfScope        []string           `json:"outOfScope,omitempty" yaml:"out_of_scope,omitempty"`
	AllowedTools      []string           `json:"allowedTools,omitempty" yaml:"allowed_tools,omitempty"`
	WorkspaceSnapshot *WorkspaceSnapshot `json:"workspaceSnapshot,omitempty" yaml:"workspace_snapshot,omitempty"`
	Context           map[string]any     `json:"context,omitempty" yaml:"context,omitempty"`
}

type LoopPolicy struct {
	MaxRevisionRounds       int     `json:"maxRevisionRounds,omitempty" yaml:"max_revision_rounds,omitempty"`
	MaxVerificationRounds   int     `json:"maxVerificationRounds,omitempty" yaml:"max_verification_rounds,omitempty"`
	MaterialConfidenceDelta float64 `json:"materialConfidenceDelta,omitempty" yaml:"material_confidence_delta,omitempty"`
}

type FallbackTarget string

const (
	FallbackTargetStop   FallbackTarget = "stop"
	FallbackTargetRevise FallbackTarget = "revise"
	FallbackTargetIngest FallbackTarget = "ingest"
)

type ExternalCommandSource struct {
	Name           string                 `json:"name" yaml:"name"`
	Command        string                 `json:"command" yaml:"command"`
	Args           []string               `json:"args,omitempty" yaml:"args,omitempty"`
	Workdir        string                 `json:"workdir,omitempty" yaml:"workdir,omitempty"`
	Env            map[string]string      `json:"env,omitempty" yaml:"env,omitempty"`
	SourceType     string                 `json:"sourceType,omitempty" yaml:"source_type,omitempty"`
	Reference      string                 `json:"reference,omitempty" yaml:"reference,omitempty"`
	SuccessPattern string                 `json:"successPattern,omitempty" yaml:"success_pattern,omitempty"`
	FailurePattern string                 `json:"failurePattern,omitempty" yaml:"failure_pattern,omitempty"`
	Parsing        ExternalCommandParsing `json:"parsing,omitempty" yaml:"parsing,omitempty"`
}

type ExternalCommandParseMode string

const (
	ExternalCommandParseModeText ExternalCommandParseMode = "text"
	ExternalCommandParseModeJSON ExternalCommandParseMode = "json"
	ExternalCommandParseModeYAML ExternalCommandParseMode = "yaml"
	ExternalCommandParseModeXML  ExternalCommandParseMode = "xml"
)

type ExternalCommandParsing struct {
	Mode          ExternalCommandParseMode `json:"mode,omitempty" yaml:"mode,omitempty"`
	SuccessPath   string                   `json:"successPath,omitempty" yaml:"success_path,omitempty"`
	FailurePath   string                   `json:"failurePath,omitempty" yaml:"failure_path,omitempty"`
	SummaryPath   string                   `json:"summaryPath,omitempty" yaml:"summary_path,omitempty"`
	ExcerptPath   string                   `json:"excerptPath,omitempty" yaml:"excerpt_path,omitempty"`
	NotesPath     string                   `json:"notesPath,omitempty" yaml:"notes_path,omitempty"`
	MetadataPaths map[string]string        `json:"metadataPaths,omitempty" yaml:"metadata_paths,omitempty"`
	RequiredPaths []string                 `json:"requiredPaths,omitempty" yaml:"required_paths,omitempty"`
}

type IngestPolicy struct {
	Sources []ExternalCommandSource `json:"sources,omitempty" yaml:"sources,omitempty"`
}

type ObserveContradictionMode string

const (
	ObserveContradictionReopen     ObserveContradictionMode = "reopen"
	ObserveContradictionRecordOnly ObserveContradictionMode = "record_only"
)

type ObservePolicy struct {
	Sources         []ExternalCommandSource  `json:"sources,omitempty" yaml:"sources,omitempty"`
	OnContradiction ObserveContradictionMode `json:"onContradiction,omitempty" yaml:"on_contradiction,omitempty"`
}

type AdjudicationFallbackPolicy struct {
	MaxFallbackRounds      int            `json:"maxFallbackRounds,omitempty" yaml:"max_fallback_rounds,omitempty"`
	OnInsufficientEvidence FallbackTarget `json:"onInsufficientEvidence,omitempty" yaml:"on_insufficient_evidence,omitempty"`
	OnUnresolvedConflict   FallbackTarget `json:"onUnresolvedConflict,omitempty" yaml:"on_unresolved_conflict,omitempty"`
	OnUnresolvedClaims     FallbackTarget `json:"onUnresolvedClaims,omitempty" yaml:"on_unresolved_claims,omitempty"`
	OnKeepWithCaveat       FallbackTarget `json:"onKeepWithCaveat,omitempty" yaml:"on_keep_with_caveat,omitempty"`
}

type RoleAssignments struct {
	Proposers        []string `json:"proposers" yaml:"proposers"`
	Challengers      []string `json:"challengers" yaml:"challengers"`
	Participants     []string `json:"participants,omitempty" yaml:"participants,omitempty"`
	Arbiter          string   `json:"arbiter,omitempty" yaml:"arbiter,omitempty"`
	SemanticVerifier string   `json:"semanticVerifier,omitempty" yaml:"semantic_verifier,omitempty"`
	Facilitator      string   `json:"facilitator,omitempty" yaml:"facilitator,omitempty"`
	Reporter         string   `json:"reporter,omitempty" yaml:"reporter,omitempty"`
	Actor            string   `json:"actor,omitempty" yaml:"actor,omitempty"`
}

type ProposalPolicy struct {
	MaxPasses          int    `json:"maxPasses" yaml:"max_passes"`
	MaxClaimsPerWorker int    `json:"maxClaimsPerWorker" yaml:"max_claims_per_worker"`
	DedupeStrategy     string `json:"dedupeStrategy,omitempty" yaml:"dedupe_strategy,omitempty"`
}

type VerificationCheck struct {
	Name          string            `json:"name" yaml:"name"`
	Kind          string            `json:"kind" yaml:"kind"`
	Command       string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args          []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Workdir       string            `json:"workdir,omitempty" yaml:"workdir,omitempty"`
	Env           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	BaseRevision  string            `json:"baseRevision,omitempty" yaml:"base_revision,omitempty"`
	Pattern       string            `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Threshold     float64           `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	ThresholdMode string            `json:"thresholdMode,omitempty" yaml:"threshold_mode,omitempty"`
	Paths         []string          `json:"paths,omitempty" yaml:"paths,omitempty"`
}

type VerificationPolicy struct {
	RequiredChecks        []VerificationCheck `json:"requiredChecks,omitempty" yaml:"required_checks,omitempty"`
	AllowSemanticVerifier bool                `json:"allowSemanticVerifier" yaml:"allow_semantic_verifier"`
	MaxParallelChecks     int                 `json:"maxParallelChecks,omitempty" yaml:"max_parallel_checks,omitempty"`
}

type ArbiterPolicy struct {
	AllowUndetermined bool `json:"allowUndetermined" yaml:"allow_undetermined"`
	BlindReview       bool `json:"blindReview" yaml:"blind_review"`
}

type DebatePolicy struct {
	MinRounds       int     `json:"minRounds,omitempty" yaml:"min_rounds,omitempty"`
	MaxRounds       int     `json:"maxRounds,omitempty" yaml:"max_rounds,omitempty"`
	VoteThreshold   float64 `json:"voteThreshold,omitempty" yaml:"vote_threshold,omitempty"`
	EnableEarlyStop bool    `json:"enableEarlyStop" yaml:"enable_early_stop"`
	PeerContextMode string  `json:"peerContextMode,omitempty" yaml:"peer_context_mode,omitempty"`
}

type DelphiPolicy struct {
	MinRounds               int     `json:"minRounds,omitempty" yaml:"min_rounds,omitempty"`
	MaxRounds               int     `json:"maxRounds,omitempty" yaml:"max_rounds,omitempty"`
	ConvergenceThreshold    float64 `json:"convergenceThreshold,omitempty" yaml:"convergence_threshold,omitempty"`
	RatingScaleMin          int     `json:"ratingScaleMin,omitempty" yaml:"rating_scale_min,omitempty"`
	RatingScaleMax          int     `json:"ratingScaleMax,omitempty" yaml:"rating_scale_max,omitempty"`
	Anonymous               bool    `json:"anonymous" yaml:"anonymous"`
	FacilitatorSummaryStyle string  `json:"facilitatorSummaryStyle,omitempty" yaml:"facilitator_summary_style,omitempty"`
}

type ReportPolicy struct {
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
}

type ActionPolicy struct {
	Prompt        string         `json:"prompt" yaml:"prompt"`
	ActorID       string         `json:"actorId,omitempty" yaml:"actor_id,omitempty"`
	IncludeResult bool           `json:"includeResult" yaml:"include_result"`
	RiskGate      ActionRiskGate `json:"riskGate,omitempty" yaml:"risk_gate,omitempty"`
}

type WaitingPolicy struct {
	PerTaskTimeout time.Duration `json:"perTaskTimeoutMs" yaml:"per_task_timeout"`
	GlobalDeadline time.Duration `json:"globalDeadlineMs,omitempty" yaml:"global_deadline,omitempty"`
	RetryAttempts  int           `json:"retryAttempts,omitempty" yaml:"retry_attempts,omitempty"`
}

type RunLineage struct {
	ParentRequestID string `json:"parentRequestId,omitempty" yaml:"parent_request_id,omitempty"`
	ParentSessionID string `json:"parentSessionId,omitempty" yaml:"parent_session_id,omitempty"`
	ParentCaseID    string `json:"parentCaseId,omitempty" yaml:"parent_case_id,omitempty"`
	Trigger         string `json:"trigger,omitempty" yaml:"trigger,omitempty"`
}

type StartRequest struct {
	Mode               WorkflowMode               `json:"mode,omitempty" yaml:"mode,omitempty"`
	RequestID          string                     `json:"requestId" yaml:"request_id"`
	Lineage            *RunLineage                `json:"lineage,omitempty" yaml:"lineage,omitempty"`
	TaskSpec           TaskSpec                   `json:"taskSpec" yaml:"task_spec"`
	Roles              RoleAssignments            `json:"roles" yaml:"roles"`
	ProposalPolicy     ProposalPolicy             `json:"proposalPolicy" yaml:"proposal_policy"`
	VerificationPolicy VerificationPolicy         `json:"verificationPolicy" yaml:"verification_policy"`
	ArbiterPolicy      ArbiterPolicy              `json:"arbiterPolicy" yaml:"arbiter_policy"`
	IngestPolicy       IngestPolicy               `json:"ingestPolicy,omitempty" yaml:"ingest_policy,omitempty"`
	FallbackPolicy     AdjudicationFallbackPolicy `json:"fallbackPolicy,omitempty" yaml:"fallback_policy,omitempty"`
	ObservePolicy      ObservePolicy              `json:"observePolicy,omitempty" yaml:"observe_policy,omitempty"`
	DebatePolicy       DebatePolicy               `json:"debatePolicy,omitempty" yaml:"debate_policy,omitempty"`
	DelphiPolicy       DelphiPolicy               `json:"delphiPolicy,omitempty" yaml:"delphi_policy,omitempty"`
	LoopPolicy         LoopPolicy                 `json:"loopPolicy,omitempty" yaml:"loop_policy,omitempty"`
	ReportPolicy       ReportPolicy               `json:"reportPolicy" yaml:"report_policy"`
	ActionPolicy       *ActionPolicy              `json:"actionPolicy,omitempty" yaml:"action_policy,omitempty"`
	WaitingPolicy      WaitingPolicy              `json:"waitingPolicy" yaml:"waiting_policy"`
}

func NormalizeStartRequest(in StartRequest) (StartRequest, error) {
	out := in
	out.Mode = normalizeWorkflowMode(out.Mode)
	out.RequestID = strings.TrimSpace(out.RequestID)
	if out.Lineage != nil {
		clone := *out.Lineage
		clone.ParentRequestID = strings.TrimSpace(clone.ParentRequestID)
		clone.ParentSessionID = strings.TrimSpace(clone.ParentSessionID)
		clone.ParentCaseID = strings.TrimSpace(clone.ParentCaseID)
		clone.Trigger = strings.TrimSpace(clone.Trigger)
		if clone.ParentRequestID == "" && clone.ParentSessionID == "" && clone.ParentCaseID == "" && clone.Trigger == "" {
			out.Lineage = nil
		} else {
			out.Lineage = &clone
		}
	}
	out.TaskSpec.Goal = strings.TrimSpace(out.TaskSpec.Goal)
	out.TaskSpec.TaskType = normalizeCaseTaskType(out.TaskSpec.TaskType)
	out.TaskSpec.SuccessCriteria = dedupeStrings(out.TaskSpec.SuccessCriteria)
	out.TaskSpec.OutOfScope = dedupeStrings(out.TaskSpec.OutOfScope)
	out.TaskSpec.AllowedTools = dedupeStrings(out.TaskSpec.AllowedTools)
	out.TaskSpec.Context = cloneAnyMap(out.TaskSpec.Context)
	for idx := range out.TaskSpec.Materials {
		out.TaskSpec.Materials[idx].Metadata = cloneAnyMap(out.TaskSpec.Materials[idx].Metadata)
		out.TaskSpec.Materials[idx].Path = strings.TrimSpace(out.TaskSpec.Materials[idx].Path)
		out.TaskSpec.Materials[idx].Content = strings.TrimSpace(out.TaskSpec.Materials[idx].Content)
	}
	if out.TaskSpec.WorkspaceSnapshot != nil {
		clone := *out.TaskSpec.WorkspaceSnapshot
		clone.Paths = dedupeStrings(clone.Paths)
		clone.Root = strings.TrimSpace(clone.Root)
		clone.Revision = strings.TrimSpace(clone.Revision)
		clone.Hash = strings.TrimSpace(clone.Hash)
		out.TaskSpec.WorkspaceSnapshot = &clone
	}
	out.TaskSpec.Constraints.AllowedPaths = dedupeStrings(out.TaskSpec.Constraints.AllowedPaths)
	out.TaskSpec.Constraints.RequiredCommands = dedupeStrings(out.TaskSpec.Constraints.RequiredCommands)
	out.TaskSpec.Constraints.Notes = dedupeStrings(out.TaskSpec.Constraints.Notes)
	out.Roles.Proposers = dedupeStrings(out.Roles.Proposers)
	out.Roles.Challengers = dedupeStrings(out.Roles.Challengers)
	out.Roles.Participants = dedupeStrings(out.Roles.Participants)
	out.Roles.Arbiter = strings.TrimSpace(out.Roles.Arbiter)
	out.Roles.SemanticVerifier = strings.TrimSpace(out.Roles.SemanticVerifier)
	out.Roles.Facilitator = strings.TrimSpace(out.Roles.Facilitator)
	out.Roles.Reporter = strings.TrimSpace(out.Roles.Reporter)
	out.Roles.Actor = strings.TrimSpace(out.Roles.Actor)
	if out.ProposalPolicy.MaxPasses == 0 {
		out.ProposalPolicy.MaxPasses = DefaultProposalPasses
	}
	if out.ProposalPolicy.MaxClaimsPerWorker == 0 {
		out.ProposalPolicy.MaxClaimsPerWorker = DefaultMaxClaimsPerWorker
	}
	if strings.TrimSpace(out.ProposalPolicy.DedupeStrategy) == "" {
		out.ProposalPolicy.DedupeStrategy = "normalized-statement"
	}
	for idx := range out.VerificationPolicy.RequiredChecks {
		out.VerificationPolicy.RequiredChecks[idx].Name = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].Name)
		out.VerificationPolicy.RequiredChecks[idx].Kind = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].Kind)
		out.VerificationPolicy.RequiredChecks[idx].Command = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].Command)
		out.VerificationPolicy.RequiredChecks[idx].Workdir = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].Workdir)
		out.VerificationPolicy.RequiredChecks[idx].BaseRevision = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].BaseRevision)
		out.VerificationPolicy.RequiredChecks[idx].Pattern = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].Pattern)
		out.VerificationPolicy.RequiredChecks[idx].ThresholdMode = strings.TrimSpace(out.VerificationPolicy.RequiredChecks[idx].ThresholdMode)
		out.VerificationPolicy.RequiredChecks[idx].Args = slices.Clone(out.VerificationPolicy.RequiredChecks[idx].Args)
		out.VerificationPolicy.RequiredChecks[idx].Env = cloneStringMap(out.VerificationPolicy.RequiredChecks[idx].Env)
		out.VerificationPolicy.RequiredChecks[idx].Paths = dedupeStrings(out.VerificationPolicy.RequiredChecks[idx].Paths)
		if out.VerificationPolicy.RequiredChecks[idx].ThresholdMode == "" && out.VerificationPolicy.RequiredChecks[idx].Kind == "benchmark_threshold" {
			out.VerificationPolicy.RequiredChecks[idx].ThresholdMode = "max"
		}
	}
	for idx := range out.IngestPolicy.Sources {
		out.IngestPolicy.Sources[idx] = normalizeExternalCommandSource(out.IngestPolicy.Sources[idx])
	}
	for idx := range out.ObservePolicy.Sources {
		out.ObservePolicy.Sources[idx] = normalizeExternalCommandSource(out.ObservePolicy.Sources[idx])
	}
	if out.VerificationPolicy.MaxParallelChecks == 0 {
		out.VerificationPolicy.MaxParallelChecks = DefaultMaxParallelChecks
	}
	if out.LoopPolicy.MaxRevisionRounds == 0 {
		out.LoopPolicy.MaxRevisionRounds = DefaultMaxRevisionRounds
	}
	if out.LoopPolicy.MaxVerificationRounds == 0 {
		out.LoopPolicy.MaxVerificationRounds = DefaultMaxVerificationRounds
	}
	if out.LoopPolicy.MaterialConfidenceDelta == 0 {
		out.LoopPolicy.MaterialConfidenceDelta = DefaultConfidenceDeltaEpsilon
	}
	if out.FallbackPolicy.MaxFallbackRounds == 0 {
		out.FallbackPolicy.MaxFallbackRounds = DefaultMaxAdjudicationFallbacks
	}
	if out.FallbackPolicy.OnInsufficientEvidence == "" {
		out.FallbackPolicy.OnInsufficientEvidence = FallbackTargetIngest
	}
	if out.FallbackPolicy.OnUnresolvedConflict == "" {
		out.FallbackPolicy.OnUnresolvedConflict = FallbackTargetIngest
	}
	if out.FallbackPolicy.OnUnresolvedClaims == "" {
		out.FallbackPolicy.OnUnresolvedClaims = FallbackTargetRevise
	}
	if out.FallbackPolicy.OnKeepWithCaveat == "" {
		out.FallbackPolicy.OnKeepWithCaveat = FallbackTargetRevise
	}
	if out.ObservePolicy.OnContradiction == "" {
		out.ObservePolicy.OnContradiction = ObserveContradictionReopen
	}
	if out.VerificationPolicy.AllowSemanticVerifier || out.Roles.SemanticVerifier == "" {
		// preserve explicit true, otherwise infer below
	} else {
		out.VerificationPolicy.AllowSemanticVerifier = true
	}
	if !out.ArbiterPolicy.AllowUndetermined {
		out.ArbiterPolicy.AllowUndetermined = true
	}
	if !out.ArbiterPolicy.BlindReview {
		out.ArbiterPolicy.BlindReview = true
	}
	if out.DebatePolicy.MinRounds == 0 {
		out.DebatePolicy.MinRounds = DefaultDebateMinRounds
	}
	if out.DebatePolicy.MaxRounds == 0 {
		out.DebatePolicy.MaxRounds = DefaultDebateMaxRounds
	}
	if out.DebatePolicy.VoteThreshold == 0 {
		out.DebatePolicy.VoteThreshold = DefaultVoteThreshold
	}
	if !out.DebatePolicy.EnableEarlyStop {
		out.DebatePolicy.EnableEarlyStop = true
	}
	if strings.TrimSpace(out.DebatePolicy.PeerContextMode) == "" {
		out.DebatePolicy.PeerContextMode = "summary+active_claims"
	}
	if out.DelphiPolicy.MinRounds == 0 {
		out.DelphiPolicy.MinRounds = DefaultDelphiMinRounds
	}
	if out.DelphiPolicy.MaxRounds == 0 {
		out.DelphiPolicy.MaxRounds = DefaultDelphiMaxRounds
	}
	if out.DelphiPolicy.ConvergenceThreshold == 0 {
		out.DelphiPolicy.ConvergenceThreshold = DefaultConvergence
	}
	if out.DelphiPolicy.RatingScaleMin == 0 {
		out.DelphiPolicy.RatingScaleMin = DefaultRatingScaleMin
	}
	if out.DelphiPolicy.RatingScaleMax == 0 {
		out.DelphiPolicy.RatingScaleMax = DefaultRatingScaleMax
	}
	if !out.DelphiPolicy.Anonymous {
		out.DelphiPolicy.Anonymous = true
	}
	if strings.TrimSpace(out.DelphiPolicy.FacilitatorSummaryStyle) == "" {
		out.DelphiPolicy.FacilitatorSummaryStyle = "anonymous-aggregate"
	}
	if out.WaitingPolicy.PerTaskTimeout == 0 {
		out.WaitingPolicy.PerTaskTimeout = DefaultPerTaskTimeout
	}
	if out.WaitingPolicy.RetryAttempts == 0 {
		out.WaitingPolicy.RetryAttempts = DefaultTaskRetryAttempts
	}
	if out.ActionPolicy != nil {
		clone := *out.ActionPolicy
		clone.Prompt = strings.TrimSpace(clone.Prompt)
		clone.ActorID = strings.TrimSpace(clone.ActorID)
		if !clone.IncludeResult {
			clone.IncludeResult = true
		}
		if clone.RiskGate == "" {
			clone.RiskGate = ActionRiskGateLowOnly
		}
		out.ActionPolicy = &clone
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
	if strings.TrimSpace(in.TaskSpec.Goal) == "" {
		return fmt.Errorf("task_spec.goal is required")
	}
	switch normalizeWorkflowMode(in.Mode) {
	case WorkflowModeAdjudication:
		if len(in.Roles.Proposers) == 0 {
			return fmt.Errorf("at least one proposer is required")
		}
		if len(in.Roles.Challengers) == 0 {
			return fmt.Errorf("at least one challenger is required")
		}
		if in.ProposalPolicy.MaxPasses < 1 {
			return fmt.Errorf("proposal_policy.max_passes must be >= 1")
		}
		if in.ProposalPolicy.MaxClaimsPerWorker < 1 {
			return fmt.Errorf("proposal_policy.max_claims_per_worker must be >= 1")
		}
		if in.VerificationPolicy.MaxParallelChecks < 1 {
			return fmt.Errorf("verification_policy.max_parallel_checks must be >= 1")
		}
	case WorkflowModeFreeDebate:
		if len(in.Roles.Participants) < 2 {
			return fmt.Errorf("free_debate requires at least two participants")
		}
		if in.DebatePolicy.MinRounds < 1 {
			return fmt.Errorf("debate_policy.min_rounds must be >= 1")
		}
		if in.DebatePolicy.MaxRounds < in.DebatePolicy.MinRounds {
			return fmt.Errorf("debate_policy.max_rounds must be >= min_rounds")
		}
		if in.DebatePolicy.VoteThreshold <= 0 || in.DebatePolicy.VoteThreshold > 1 {
			return fmt.Errorf("debate_policy.vote_threshold must be in (0,1]")
		}
	case WorkflowModeDelphi:
		if len(in.Roles.Participants) < 2 {
			return fmt.Errorf("delphi requires at least two participants")
		}
		if in.DelphiPolicy.MinRounds < 1 {
			return fmt.Errorf("delphi_policy.min_rounds must be >= 1")
		}
		if in.DelphiPolicy.MaxRounds < in.DelphiPolicy.MinRounds {
			return fmt.Errorf("delphi_policy.max_rounds must be >= min_rounds")
		}
		if in.DelphiPolicy.ConvergenceThreshold <= 0 || in.DelphiPolicy.ConvergenceThreshold > 1 {
			return fmt.Errorf("delphi_policy.convergence_threshold must be in (0,1]")
		}
		if in.DelphiPolicy.RatingScaleMin >= in.DelphiPolicy.RatingScaleMax {
			return fmt.Errorf("delphi_policy.rating_scale_min must be < rating_scale_max")
		}
	default:
		return fmt.Errorf("unsupported mode: %s", in.Mode)
	}
	for _, check := range in.VerificationPolicy.RequiredChecks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("verification check name is required")
		}
		switch check.Kind {
		case "command", "workspace_snapshot", "allowed_paths", "git_diff_paths", "benchmark_threshold":
		default:
			return fmt.Errorf("unsupported verification check kind: %s", check.Kind)
		}
		if check.Kind == "command" && strings.TrimSpace(check.Command) == "" {
			return fmt.Errorf("verification check %s: command is required", check.Name)
		}
		if check.Kind == "benchmark_threshold" {
			if strings.TrimSpace(check.Command) == "" {
				return fmt.Errorf("verification check %s: command is required", check.Name)
			}
			if check.Threshold == 0 {
				return fmt.Errorf("verification check %s: threshold is required", check.Name)
			}
			if check.ThresholdMode != "" && check.ThresholdMode != "max" && check.ThresholdMode != "min" {
				return fmt.Errorf("verification check %s: threshold_mode must be max or min", check.Name)
			}
		}
	}
	if in.WaitingPolicy.PerTaskTimeout <= 0 {
		return fmt.Errorf("waiting_policy.per_task_timeout must be positive")
	}
	if in.LoopPolicy.MaxRevisionRounds < 0 {
		return fmt.Errorf("loop_policy.max_revision_rounds must be >= 0")
	}
	if in.LoopPolicy.MaxVerificationRounds < 1 {
		return fmt.Errorf("loop_policy.max_verification_rounds must be >= 1")
	}
	if in.LoopPolicy.MaterialConfidenceDelta < 0 {
		return fmt.Errorf("loop_policy.material_confidence_delta must be >= 0")
	}
	if in.FallbackPolicy.MaxFallbackRounds < 0 {
		return fmt.Errorf("fallback_policy.max_fallback_rounds must be >= 0")
	}
	for _, value := range []FallbackTarget{
		in.FallbackPolicy.OnInsufficientEvidence,
		in.FallbackPolicy.OnUnresolvedConflict,
		in.FallbackPolicy.OnUnresolvedClaims,
		in.FallbackPolicy.OnKeepWithCaveat,
	} {
		switch value {
		case "", FallbackTargetStop, FallbackTargetRevise, FallbackTargetIngest:
		default:
			return fmt.Errorf("fallback_policy contains invalid target: %s", value)
		}
	}
	switch in.ObservePolicy.OnContradiction {
	case "", ObserveContradictionReopen, ObserveContradictionRecordOnly:
	default:
		return fmt.Errorf("observe_policy.on_contradiction is invalid: %s", in.ObservePolicy.OnContradiction)
	}
	for _, source := range append(append([]ExternalCommandSource(nil), in.IngestPolicy.Sources...), in.ObservePolicy.Sources...) {
		if strings.TrimSpace(source.Name) == "" {
			return fmt.Errorf("external source name is required")
		}
		if strings.TrimSpace(source.Command) == "" {
			return fmt.Errorf("external source %s: command is required", source.Name)
		}
		switch source.Parsing.Mode {
		case "", ExternalCommandParseModeText, ExternalCommandParseModeJSON, ExternalCommandParseModeYAML, ExternalCommandParseModeXML:
		default:
			return fmt.Errorf("external source %s: parsing.mode is invalid: %s", source.Name, source.Parsing.Mode)
		}
	}
	if in.WaitingPolicy.RetryAttempts < 0 {
		return fmt.Errorf("waiting_policy.retry_attempts must be >= 0")
	}
	if in.WaitingPolicy.GlobalDeadline < 0 {
		return fmt.Errorf("waiting_policy.global_deadline must be >= 0")
	}
	if in.ActionPolicy != nil && strings.TrimSpace(in.ActionPolicy.Prompt) == "" {
		return fmt.Errorf("action_policy.prompt is required")
	}
	if in.ActionPolicy != nil {
		switch in.ActionPolicy.RiskGate {
		case "", ActionRiskGateLowOnly, ActionRiskGateAllowMedium, ActionRiskGateAllowHigh:
		default:
			return fmt.Errorf("action_policy.risk_gate is invalid: %s", in.ActionPolicy.RiskGate)
		}
	}
	if in.TaskSpec.WorkspaceSnapshot != nil {
		snapshot := in.TaskSpec.WorkspaceSnapshot
		if strings.TrimSpace(snapshot.Root) == "" && strings.TrimSpace(snapshot.Hash) == "" && strings.TrimSpace(snapshot.Revision) == "" {
			return fmt.Errorf("workspace_snapshot requires at least one of root/hash/revision")
		}
	}
	return nil
}

func normalizeExternalCommandSource(in ExternalCommandSource) ExternalCommandSource {
	in.Name = strings.TrimSpace(in.Name)
	in.Command = strings.TrimSpace(in.Command)
	in.Workdir = strings.TrimSpace(in.Workdir)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.Reference = strings.TrimSpace(in.Reference)
	in.SuccessPattern = strings.TrimSpace(in.SuccessPattern)
	in.FailurePattern = strings.TrimSpace(in.FailurePattern)
	in.Args = slices.Clone(in.Args)
	in.Env = cloneStringMap(in.Env)
	in.Parsing.Mode = ExternalCommandParseMode(strings.TrimSpace(string(in.Parsing.Mode)))
	in.Parsing.SuccessPath = strings.TrimSpace(in.Parsing.SuccessPath)
	in.Parsing.FailurePath = strings.TrimSpace(in.Parsing.FailurePath)
	in.Parsing.SummaryPath = strings.TrimSpace(in.Parsing.SummaryPath)
	in.Parsing.ExcerptPath = strings.TrimSpace(in.Parsing.ExcerptPath)
	in.Parsing.NotesPath = strings.TrimSpace(in.Parsing.NotesPath)
	in.Parsing.MetadataPaths = cloneStringMap(in.Parsing.MetadataPaths)
	in.Parsing.RequiredPaths = dedupeStrings(in.Parsing.RequiredPaths)
	return in
}

func normalizeWorkflowMode(mode WorkflowMode) WorkflowMode {
	switch WorkflowMode(strings.TrimSpace(string(mode))) {
	case "", WorkflowModeAdjudication:
		return WorkflowModeAdjudication
	case WorkflowModeFreeDebate:
		return WorkflowModeFreeDebate
	case WorkflowModeDelphi:
		return WorkflowModeDelphi
	default:
		return WorkflowMode(strings.TrimSpace(string(mode)))
	}
}

func normalizeCaseTaskType(taskType CaseTaskType) CaseTaskType {
	switch CaseTaskType(strings.TrimSpace(string(taskType))) {
	case "", CaseTaskTypeUnknown:
		return CaseTaskTypeUnknown
	case CaseTaskTypeFactual:
		return CaseTaskTypeFactual
	case CaseTaskTypeCoding:
		return CaseTaskTypeCoding
	case CaseTaskTypeStrategy:
		return CaseTaskTypeStrategy
	case CaseTaskTypeOperational:
		return CaseTaskTypeOperational
	default:
		return CaseTaskType(strings.TrimSpace(string(taskType)))
	}
}

func dedupeStrings(items []string) []string {
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

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	return maps.Clone(in)
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	return maps.Clone(in)
}
