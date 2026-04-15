package consensus

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	DefaultPerTaskTimeout     = 20 * time.Minute
	DefaultProposalPasses     = 1
	DefaultMaxClaimsPerWorker = 5
	DefaultMaxParallelChecks  = 4
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
	Materials         []MaterialRef      `json:"materials,omitempty" yaml:"materials,omitempty"`
	Constraints       TaskConstraints    `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	SuccessCriteria   []string           `json:"successCriteria,omitempty" yaml:"success_criteria,omitempty"`
	AllowedTools      []string           `json:"allowedTools,omitempty" yaml:"allowed_tools,omitempty"`
	WorkspaceSnapshot *WorkspaceSnapshot `json:"workspaceSnapshot,omitempty" yaml:"workspace_snapshot,omitempty"`
	Context           map[string]any     `json:"context,omitempty" yaml:"context,omitempty"`
}

type RoleAssignments struct {
	Proposers        []string `json:"proposers" yaml:"proposers"`
	Challengers      []string `json:"challengers" yaml:"challengers"`
	Arbiter          string   `json:"arbiter,omitempty" yaml:"arbiter,omitempty"`
	SemanticVerifier string   `json:"semanticVerifier,omitempty" yaml:"semantic_verifier,omitempty"`
	Reporter         string   `json:"reporter,omitempty" yaml:"reporter,omitempty"`
	Actor            string   `json:"actor,omitempty" yaml:"actor,omitempty"`
}

type ProposalPolicy struct {
	MaxPasses          int    `json:"maxPasses" yaml:"max_passes"`
	MaxClaimsPerWorker int    `json:"maxClaimsPerWorker" yaml:"max_claims_per_worker"`
	DedupeStrategy     string `json:"dedupeStrategy,omitempty" yaml:"dedupe_strategy,omitempty"`
}

type VerificationCheck struct {
	Name    string            `json:"name" yaml:"name"`
	Kind    string            `json:"kind" yaml:"kind"`
	Command string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Workdir string            `json:"workdir,omitempty" yaml:"workdir,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
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

type ReportPolicy struct {
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
}

type ActionPolicy struct {
	Prompt        string `json:"prompt" yaml:"prompt"`
	ActorID       string `json:"actorId,omitempty" yaml:"actor_id,omitempty"`
	IncludeResult bool   `json:"includeResult" yaml:"include_result"`
}

type WaitingPolicy struct {
	PerTaskTimeout time.Duration `json:"perTaskTimeoutMs" yaml:"per_task_timeout"`
	GlobalDeadline time.Duration `json:"globalDeadlineMs,omitempty" yaml:"global_deadline,omitempty"`
}

type StartRequest struct {
	RequestID          string             `json:"requestId" yaml:"request_id"`
	TaskSpec           TaskSpec           `json:"taskSpec" yaml:"task_spec"`
	Roles              RoleAssignments    `json:"roles" yaml:"roles"`
	ProposalPolicy     ProposalPolicy     `json:"proposalPolicy" yaml:"proposal_policy"`
	VerificationPolicy VerificationPolicy `json:"verificationPolicy" yaml:"verification_policy"`
	ArbiterPolicy      ArbiterPolicy      `json:"arbiterPolicy" yaml:"arbiter_policy"`
	ReportPolicy       ReportPolicy       `json:"reportPolicy" yaml:"report_policy"`
	ActionPolicy       *ActionPolicy      `json:"actionPolicy,omitempty" yaml:"action_policy,omitempty"`
	WaitingPolicy      WaitingPolicy      `json:"waitingPolicy" yaml:"waiting_policy"`
}

func NormalizeStartRequest(in StartRequest) (StartRequest, error) {
	out := in
	out.RequestID = strings.TrimSpace(out.RequestID)
	out.TaskSpec.Goal = strings.TrimSpace(out.TaskSpec.Goal)
	out.TaskSpec.SuccessCriteria = dedupeStrings(out.TaskSpec.SuccessCriteria)
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
	out.Roles.Arbiter = strings.TrimSpace(out.Roles.Arbiter)
	out.Roles.SemanticVerifier = strings.TrimSpace(out.Roles.SemanticVerifier)
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
		out.VerificationPolicy.RequiredChecks[idx].Args = slices.Clone(out.VerificationPolicy.RequiredChecks[idx].Args)
		out.VerificationPolicy.RequiredChecks[idx].Env = cloneStringMap(out.VerificationPolicy.RequiredChecks[idx].Env)
	}
	if out.VerificationPolicy.MaxParallelChecks == 0 {
		out.VerificationPolicy.MaxParallelChecks = DefaultMaxParallelChecks
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
	if out.WaitingPolicy.PerTaskTimeout == 0 {
		out.WaitingPolicy.PerTaskTimeout = DefaultPerTaskTimeout
	}
	if out.ActionPolicy != nil {
		clone := *out.ActionPolicy
		clone.Prompt = strings.TrimSpace(clone.Prompt)
		clone.ActorID = strings.TrimSpace(clone.ActorID)
		if !clone.IncludeResult {
			clone.IncludeResult = true
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
	for _, check := range in.VerificationPolicy.RequiredChecks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("verification check name is required")
		}
		switch check.Kind {
		case "command", "workspace_snapshot", "allowed_paths":
		default:
			return fmt.Errorf("unsupported verification check kind: %s", check.Kind)
		}
		if check.Kind == "command" && strings.TrimSpace(check.Command) == "" {
			return fmt.Errorf("verification check %s: command is required", check.Name)
		}
	}
	if in.WaitingPolicy.PerTaskTimeout <= 0 {
		return fmt.Errorf("waiting_policy.per_task_timeout must be positive")
	}
	if in.WaitingPolicy.GlobalDeadline < 0 {
		return fmt.Errorf("waiting_policy.global_deadline must be >= 0")
	}
	if in.ActionPolicy != nil && strings.TrimSpace(in.ActionPolicy.Prompt) == "" {
		return fmt.Errorf("action_policy.prompt is required")
	}
	if in.TaskSpec.WorkspaceSnapshot != nil {
		snapshot := in.TaskSpec.WorkspaceSnapshot
		if strings.TrimSpace(snapshot.Root) == "" && strings.TrimSpace(snapshot.Hash) == "" && strings.TrimSpace(snapshot.Revision) == "" {
			return fmt.Errorf("workspace_snapshot requires at least one of root/hash/revision")
		}
	}
	return nil
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
