package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err == nil {
		if strings.TrimSpace(text) == "" {
			d.Duration = 0
			return nil
		}
		value, err := time.ParseDuration(text)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", text, err)
		}
		d.Duration = value
		return nil
	}
	var millis int64
	if err := unmarshal(&millis); err == nil {
		d.Duration = time.Duration(millis) * time.Millisecond
		return nil
	}
	return fmt.Errorf("duration must be string or integer milliseconds")
}

func (d Duration) MarshalYAML() (any, error) {
	if d.Duration == 0 {
		return "", nil
	}
	return d.String(), nil
}

type Config struct {
	SchemaVersion int                       `yaml:"schema_version"`
	Include       []string                  `yaml:"include,omitempty"`
	Profile       string                    `yaml:"profile,omitempty"`
	Profiles      map[string]ProfileConfig  `yaml:"profiles,omitempty"`
	Defaults      DefaultsConfig            `yaml:"defaults"`
	Output        OutputConfig              `yaml:"output"`
	Providers     map[string]ProviderConfig `yaml:"providers"`
	Agents        []AgentConfig             `yaml:"agents"`
	Roles         RolesConfig               `yaml:"roles"`
}

type ProfileConfig struct {
	Defaults  DefaultsConfig            `yaml:"defaults,omitempty"`
	Output    OutputConfig              `yaml:"output,omitempty"`
	Providers map[string]ProviderConfig `yaml:"providers,omitempty"`
	Agents    []AgentConfig             `yaml:"agents,omitempty"`
	Roles     RolesConfig               `yaml:"roles,omitempty"`
}

type DefaultsConfig struct {
	Mode               consensus.WorkflowMode               `yaml:"mode,omitempty"`
	TaskType           consensus.CaseTaskType               `yaml:"task_type,omitempty"`
	SuccessCriteria    []string                             `yaml:"success_criteria"`
	OutOfScope         []string                             `yaml:"out_of_scope,omitempty"`
	AllowedTools       []string                             `yaml:"allowed_tools"`
	PerTaskTimeout     Duration                             `yaml:"per_task_timeout"`
	TaskRetryAttempts  int                                  `yaml:"task_retry_attempts,omitempty"`
	GlobalDeadline     Duration                             `yaml:"global_deadline"`
	ProposalPolicy     ProposalPolicyConfig                 `yaml:"proposal_policy"`
	VerificationPolicy VerificationPolicyConfig             `yaml:"verification_policy"`
	ArbiterPolicy      ArbiterPolicyConfig                  `yaml:"arbiter_policy"`
	IngestPolicy       consensus.IngestPolicy               `yaml:"ingest_policy,omitempty"`
	FallbackPolicy     consensus.AdjudicationFallbackPolicy `yaml:"fallback_policy,omitempty"`
	ObservePolicy      consensus.ObservePolicy              `yaml:"observe_policy,omitempty"`
	DebatePolicy       DebatePolicyConfig                   `yaml:"debate_policy,omitempty"`
	DelphiPolicy       DelphiPolicyConfig                   `yaml:"delphi_policy,omitempty"`
	WorkspaceSnapshot  *consensus.WorkspaceSnapshot         `yaml:"workspace_snapshot,omitempty"`
	TaskConstraints    consensus.TaskConstraints            `yaml:"task_constraints,omitempty"`
}

type RolesConfig struct {
	Adjudication AdjudicationRolesConfig `yaml:"adjudication,omitempty"`
	FreeDebate   DebateRolesConfig       `yaml:"free_debate,omitempty"`
	Delphi       DelphiRolesConfig       `yaml:"delphi,omitempty"`

	// Computed compatibility fields for internal call sites. They are not part
	// of the YAML schema; new config files must use the mode-scoped fields above.
	Proposers        []string `yaml:"-" json:"-"`
	Challengers      []string `yaml:"-" json:"-"`
	Participants     []string `yaml:"-" json:"-"`
	Arbiter          string   `yaml:"-" json:"-"`
	SemanticVerifier string   `yaml:"-" json:"-"`
	SemanticDeduper  string   `yaml:"-" json:"-"`
	Facilitator      string   `yaml:"-" json:"-"`
	Reporter         string   `yaml:"-" json:"-"`
	Actor            string   `yaml:"-" json:"-"`
}

type AdjudicationRolesConfig struct {
	Proposers        []string `yaml:"proposers,omitempty"`
	Challengers      []string `yaml:"challengers,omitempty"`
	Arbiter          string   `yaml:"arbiter,omitempty"`
	SemanticVerifier string   `yaml:"semantic_verifier,omitempty"`
	Reporter         string   `yaml:"reporter,omitempty"`
	Actor            string   `yaml:"actor,omitempty"`
}

type DebateRolesConfig struct {
	Participants    []string `yaml:"participants,omitempty"`
	SemanticDeduper string   `yaml:"semantic_deduper,omitempty"`
	Reporter        string   `yaml:"reporter,omitempty"`
	Actor           string   `yaml:"actor,omitempty"`
}

type DelphiRolesConfig struct {
	Participants []string `yaml:"participants,omitempty"`
	Facilitator  string   `yaml:"facilitator,omitempty"`
	Reporter     string   `yaml:"reporter,omitempty"`
	Actor        string   `yaml:"actor,omitempty"`
}

func (r AdjudicationRolesConfig) IsZero() bool {
	return len(r.Proposers) == 0 &&
		len(r.Challengers) == 0 &&
		strings.TrimSpace(r.Arbiter) == "" &&
		strings.TrimSpace(r.SemanticVerifier) == "" &&
		strings.TrimSpace(r.Reporter) == "" &&
		strings.TrimSpace(r.Actor) == ""
}

func (r DebateRolesConfig) IsZero() bool {
	return len(r.Participants) == 0 &&
		strings.TrimSpace(r.SemanticDeduper) == "" &&
		strings.TrimSpace(r.Reporter) == "" &&
		strings.TrimSpace(r.Actor) == ""
}

func (r DelphiRolesConfig) IsZero() bool {
	return len(r.Participants) == 0 &&
		strings.TrimSpace(r.Facilitator) == "" &&
		strings.TrimSpace(r.Reporter) == "" &&
		strings.TrimSpace(r.Actor) == ""
}

func (r RolesConfig) IsZero() bool {
	return r.Adjudication.IsZero() && r.FreeDebate.IsZero() && r.Delphi.IsZero()
}

type ProposalPolicyConfig struct {
	MaxPasses          int    `yaml:"max_passes"`
	MaxClaimsPerWorker int    `yaml:"max_claims_per_worker"`
	DedupeStrategy     string `yaml:"dedupe_strategy,omitempty"`
}

type VerificationPolicyConfig struct {
	RequiredChecks        []consensus.VerificationCheck `yaml:"required_checks,omitempty"`
	AllowSemanticVerifier bool                          `yaml:"allow_semantic_verifier"`
	MaxParallelChecks     int                           `yaml:"max_parallel_checks,omitempty"`
}

type ArbiterPolicyConfig struct {
	AllowUndetermined bool `yaml:"allow_undetermined"`
	BlindReview       bool `yaml:"blind_review"`
}

type DebatePolicyConfig struct {
	MinRounds       int                       `yaml:"min_rounds,omitempty"`
	MaxRounds       int                       `yaml:"max_rounds,omitempty"`
	VoteThreshold   float64                   `yaml:"vote_threshold,omitempty"`
	EnableEarlyStop bool                      `yaml:"enable_early_stop"`
	PeerContextMode string                    `yaml:"peer_context_mode,omitempty"`
	SemanticDedup   DebateSemanticDedupConfig `yaml:"semantic_dedup,omitempty"`
}

type DebateSemanticDedupConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SimilarityThreshold float64 `yaml:"similarity_threshold,omitempty"`
}

type DelphiPolicyConfig struct {
	MinRounds               int     `yaml:"min_rounds,omitempty"`
	MaxRounds               int     `yaml:"max_rounds,omitempty"`
	ConvergenceThreshold    float64 `yaml:"convergence_threshold,omitempty"`
	RatingScaleMin          int     `yaml:"rating_scale_min,omitempty"`
	RatingScaleMax          int     `yaml:"rating_scale_max,omitempty"`
	Anonymous               bool    `yaml:"anonymous"`
	FacilitatorSummaryStyle string  `yaml:"facilitator_summary_style,omitempty"`
}

type OutputConfig struct {
	Directory    string `yaml:"directory"`
	LedgerPath   string `yaml:"ledger_path"`
	EventsPath   string `yaml:"events_path"`
	ResultPath   string `yaml:"result_path"`
	SummaryPath  string `yaml:"summary_path"`
	ErrorPath    string `yaml:"error_path"`
	ArtifactsDir string `yaml:"artifacts_dir"`
}

type ProviderModelConfig struct {
	Enabled            *bool    `yaml:"enabled,omitempty"`
	ProviderModel      string   `yaml:"provider_model,omitempty"`
	ContextWindow      int      `yaml:"context_window,omitempty"`
	MaxOutputTokens    int      `yaml:"max_output_tokens,omitempty"`
	MaxOutputTokensSet bool     `yaml:"-" json:"-"`
	Temperature        *float64 `yaml:"temperature,omitempty"`
	Reasoning          string   `yaml:"reasoning,omitempty"`
}

type ProviderConfig struct {
	Enabled      *bool                              `yaml:"enabled,omitempty"`
	Type         string                             `yaml:"type"`
	Protocol     string                             `yaml:"protocol,omitempty"`
	CLIType      string                             `yaml:"cli_type,omitempty"`
	BaseURL      string                             `yaml:"base_url,omitempty"`
	APIKeyEnv    string                             `yaml:"api_key_env,omitempty"`
	Headers      map[string]string                  `yaml:"headers,omitempty"`
	Model        string                             `yaml:"model,omitempty"`
	Models       map[string]ProviderModelConfig     `yaml:"models,omitempty"`
	Command      string                             `yaml:"command,omitempty"`
	Args         []string                           `yaml:"args,omitempty"`
	Env          map[string]string                  `yaml:"env,omitempty"`
	Adapter      string                             `yaml:"adapter,omitempty"`
	Options      map[string]any                     `yaml:"options,omitempty"`
	Behavior     string                             `yaml:"behavior,omitempty"`
	Delay        Duration                           `yaml:"delay,omitempty"`
	Error        string                             `yaml:"error,omitempty"`
	Participants map[string]MockParticipantScenario `yaml:"participants,omitempty"`
}

type MockParticipantScenario struct {
	Propose        MockAction `yaml:"propose"`
	Challenge      MockAction `yaml:"challenge"`
	SemanticVerify MockAction `yaml:"semantic_verify"`
	Arbiter        MockAction `yaml:"arbiter"`
	Report         MockAction `yaml:"report"`
	Action         MockAction `yaml:"action"`
}

type MockAction struct {
	Behavior string   `yaml:"behavior"`
	Delay    Duration `yaml:"delay"`
	Error    string   `yaml:"error"`
}

type AgentConfig struct {
	ID           string   `yaml:"id"`
	Provider     string   `yaml:"provider"`
	Model        string   `yaml:"model,omitempty"`
	Role         string   `yaml:"role,omitempty"`
	SystemPrompt string   `yaml:"system_prompt,omitempty"`
	Timeout      Duration `yaml:"timeout,omitempty"`
	Temperature  *float64 `yaml:"temperature,omitempty"`
	Reasoning    string   `yaml:"reasoning,omitempty"`
}

type LoadedConfig struct {
	Path         string
	ConfigDir    string
	Profile      string
	Config       Config
	IncludeTrace []IncludeTraceEntry
}

type IncludeTraceEntry struct {
	Path       string `json:"path" yaml:"path"`
	IncludedBy string `json:"includedBy,omitempty" yaml:"included_by,omitempty"`
}

type RunInput struct {
	Mode               consensus.WorkflowMode               `yaml:"mode" json:"mode"`
	RequestID          string                               `yaml:"request_id" json:"request_id"`
	TaskRetryAttempts  int                                  `yaml:"task_retry_attempts,omitempty" json:"task_retry_attempts,omitempty"`
	TaskSpec           TaskSpecInput                        `yaml:"task_spec" json:"task_spec"`
	Roles              RolesConfig                          `yaml:"roles" json:"roles"`
	ProposalPolicy     ProposalPolicyConfig                 `yaml:"proposal_policy" json:"proposal_policy"`
	VerificationPolicy VerificationPolicyConfig             `yaml:"verification_policy" json:"verification_policy"`
	ArbiterPolicy      ArbiterPolicyConfig                  `yaml:"arbiter_policy" json:"arbiter_policy"`
	IngestPolicy       consensus.IngestPolicy               `yaml:"ingest_policy" json:"ingest_policy"`
	FallbackPolicy     consensus.AdjudicationFallbackPolicy `yaml:"fallback_policy" json:"fallback_policy"`
	ObservePolicy      consensus.ObservePolicy              `yaml:"observe_policy" json:"observe_policy"`
	DebatePolicy       DebatePolicyConfig                   `yaml:"debate_policy" json:"debate_policy"`
	DelphiPolicy       DelphiPolicyConfig                   `yaml:"delphi_policy" json:"delphi_policy"`
	Action             string                               `yaml:"action" json:"action"`
}

type TaskSpecInput struct {
	Goal              string                       `yaml:"goal" json:"goal"`
	TaskType          consensus.CaseTaskType       `yaml:"task_type,omitempty" json:"task_type,omitempty"`
	Materials         []consensus.MaterialRef      `yaml:"materials,omitempty" json:"materials,omitempty"`
	Constraints       consensus.TaskConstraints    `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	SuccessCriteria   []string                     `yaml:"success_criteria,omitempty" json:"success_criteria,omitempty"`
	OutOfScope        []string                     `yaml:"out_of_scope,omitempty" json:"out_of_scope,omitempty"`
	AllowedTools      []string                     `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	WorkspaceSnapshot *consensus.WorkspaceSnapshot `yaml:"workspace_snapshot,omitempty" json:"workspace_snapshot,omitempty"`
	Context           map[string]any               `yaml:"context,omitempty" json:"context,omitempty"`
}

type RunOverrides struct {
	ConfigPath           string
	InputPath            string
	Mode                 consensus.WorkflowMode
	Task                 string
	Proposers            []string
	Challengers          []string
	Participants         []string
	Arbiter              string
	SemanticVerifier     string
	SemanticDeduper      string
	Facilitator          string
	Reporter             string
	Actor                string
	SuccessCriteria      []string
	WorkspaceSnapshot    string
	Timeout              time.Duration
	GlobalDeadline       time.Duration
	MinRounds            int
	MaxRounds            int
	VoteThreshold        float64
	ConvergenceThreshold float64
	Action               string
	Verbose              bool
	Debug                bool
}
