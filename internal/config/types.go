package config

import (
	"fmt"
	"strings"
	"time"
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
}

type DefaultsConfig struct {
	DefaultAgents            []string `yaml:"default_agents"`
	Language                 string   `yaml:"language"`
	TokenBudgetHint          int      `yaml:"token_budget_hint"`
	MinRounds                int      `yaml:"min_rounds"`
	MaxRounds                int      `yaml:"max_rounds"`
	Threshold                float64  `yaml:"threshold"`
	PerTaskTimeout           Duration `yaml:"per_task_timeout"`
	PerRoundTimeout          Duration `yaml:"per_round_timeout"`
	GlobalDeadline           Duration `yaml:"global_deadline"`
	Composer                 string   `yaml:"composer"`
	RepresentativeID         string   `yaml:"representative_id"`
	IncludeDeliberationTrace bool     `yaml:"include_deliberation_trace"`
	TraceLevel               string   `yaml:"trace_level"`
}

type OutputConfig struct {
	Directory   string `yaml:"directory"`
	EventsPath  string `yaml:"events_path"`
	ResultPath  string `yaml:"result_path"`
	SummaryPath string `yaml:"summary_path"`
	ErrorPath   string `yaml:"error_path"`
}

type ProviderConfig struct {
	Type         string                             `yaml:"type"`
	BaseURL      string                             `yaml:"base_url"`
	APIKeyEnv    string                             `yaml:"api_key_env"`
	Model        string                             `yaml:"model"`
	Command      string                             `yaml:"command"`
	Args         []string                           `yaml:"args"`
	Env          map[string]string                  `yaml:"env"`
	Behavior     string                             `yaml:"behavior"`
	Delay        Duration                           `yaml:"delay"`
	Error        string                             `yaml:"error"`
	Participants map[string]MockParticipantScenario `yaml:"participants"`
}

type MockParticipantScenario struct {
	Initial   MockAction `yaml:"initial"`
	Debate    MockAction `yaml:"debate"`
	FinalVote MockAction `yaml:"final_vote"`
	Report    MockAction `yaml:"report"`
	Action    MockAction `yaml:"action"`
}

type MockAction struct {
	Behavior string   `yaml:"behavior"`
	Delay    Duration `yaml:"delay"`
	Error    string   `yaml:"error"`
}

type AgentConfig struct {
	ID           string   `yaml:"id"`
	Provider     string   `yaml:"provider"`
	Model        string   `yaml:"model"`
	Role         string   `yaml:"role"`
	SystemPrompt string   `yaml:"system_prompt"`
	Timeout      Duration `yaml:"timeout"`
}

type LoadedConfig struct {
	Path      string
	ConfigDir string
	Config    Config
}

type RunInput struct {
	RequestID                string         `yaml:"request_id" json:"request_id"`
	Task                     string         `yaml:"task" json:"task"`
	Agents                   []string       `yaml:"agents" json:"agents"`
	MinRounds                int            `yaml:"min_rounds" json:"min_rounds"`
	MaxRounds                int            `yaml:"max_rounds" json:"max_rounds"`
	Threshold                float64        `yaml:"threshold" json:"threshold"`
	Timeout                  Duration       `yaml:"timeout" json:"timeout"`
	GlobalDeadline           Duration       `yaml:"global_deadline" json:"global_deadline"`
	Action                   string         `yaml:"action" json:"action"`
	Language                 string         `yaml:"language" json:"language"`
	TokenBudgetHint          int            `yaml:"token_budget_hint" json:"token_budget_hint"`
	RepresentativeID         string         `yaml:"representative_id" json:"representative_id"`
	Composer                 string         `yaml:"composer" json:"composer"`
	IncludeDeliberationTrace *bool          `yaml:"include_deliberation_trace" json:"include_deliberation_trace"`
	TraceLevel               string         `yaml:"trace_level" json:"trace_level"`
	Context                  map[string]any `yaml:"context" json:"context"`
}

type RunOverrides struct {
	ConfigPath     string
	InputPath      string
	Task           string
	Agents         []string
	MinRounds      int
	MaxRounds      int
	Threshold      float64
	Timeout        time.Duration
	GlobalDeadline time.Duration
	Action         string
	Verbose        bool
}
