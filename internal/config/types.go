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
	Defaults      DefaultsConfig            `yaml:"defaults"`
	Output        OutputConfig              `yaml:"output"`
	Providers     map[string]ProviderConfig `yaml:"providers"`
	Agents        []AgentConfig             `yaml:"agents"`
	Roles         RolesConfig               `yaml:"roles"`
}

type DefaultsConfig struct {
	SuccessCriteria    []string                     `yaml:"success_criteria"`
	AllowedTools       []string                     `yaml:"allowed_tools"`
	PerTaskTimeout     Duration                     `yaml:"per_task_timeout"`
	GlobalDeadline     Duration                     `yaml:"global_deadline"`
	ProposalPolicy     ProposalPolicyConfig         `yaml:"proposal_policy"`
	VerificationPolicy VerificationPolicyConfig     `yaml:"verification_policy"`
	ArbiterPolicy      ArbiterPolicyConfig          `yaml:"arbiter_policy"`
	WorkspaceSnapshot  *consensus.WorkspaceSnapshot `yaml:"workspace_snapshot,omitempty"`
	TaskConstraints    consensus.TaskConstraints    `yaml:"task_constraints,omitempty"`
}

type RolesConfig struct {
	Proposers        []string `yaml:"proposers"`
	Challengers      []string `yaml:"challengers"`
	Arbiter          string   `yaml:"arbiter,omitempty"`
	SemanticVerifier string   `yaml:"semantic_verifier,omitempty"`
	Reporter         string   `yaml:"reporter,omitempty"`
	Actor            string   `yaml:"actor,omitempty"`
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
	ProviderModel   string   `yaml:"provider_model,omitempty"`
	ContextWindow   int      `yaml:"context_window,omitempty"`
	MaxOutputTokens int      `yaml:"max_output_tokens,omitempty"`
	Temperature     *float64 `yaml:"temperature,omitempty"`
	Reasoning       string   `yaml:"reasoning,omitempty"`
}

type ProviderConfig struct {
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
	Path      string
	ConfigDir string
	Config    Config
}

type RunInput struct {
	RequestID          string                   `yaml:"request_id" json:"request_id"`
	TaskSpec           TaskSpecInput            `yaml:"task_spec" json:"task_spec"`
	Roles              RolesConfig              `yaml:"roles" json:"roles"`
	ProposalPolicy     ProposalPolicyConfig     `yaml:"proposal_policy" json:"proposal_policy"`
	VerificationPolicy VerificationPolicyConfig `yaml:"verification_policy" json:"verification_policy"`
	ArbiterPolicy      ArbiterPolicyConfig      `yaml:"arbiter_policy" json:"arbiter_policy"`
	Action             string                   `yaml:"action" json:"action"`
}

type TaskSpecInput struct {
	Goal              string                       `yaml:"goal" json:"goal"`
	Materials         []consensus.MaterialRef      `yaml:"materials,omitempty" json:"materials,omitempty"`
	Constraints       consensus.TaskConstraints    `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	SuccessCriteria   []string                     `yaml:"success_criteria,omitempty" json:"success_criteria,omitempty"`
	AllowedTools      []string                     `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	WorkspaceSnapshot *consensus.WorkspaceSnapshot `yaml:"workspace_snapshot,omitempty" json:"workspace_snapshot,omitempty"`
	Context           map[string]any               `yaml:"context,omitempty" json:"context,omitempty"`
}

type RunOverrides struct {
	ConfigPath        string
	InputPath         string
	Task              string
	Proposers         []string
	Challengers       []string
	Arbiter           string
	SemanticVerifier  string
	Reporter          string
	Actor             string
	SuccessCriteria   []string
	WorkspaceSnapshot string
	Timeout           time.Duration
	GlobalDeadline    time.Duration
	Action            string
	Verbose           bool
}
