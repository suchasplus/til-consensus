package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"gopkg.in/yaml.v3"
)

const (
	TemplateProviderProfileMock        = "mock"
	TemplateProviderProfileOpenAI      = "openai"
	TemplateProviderProfileGeneric     = "generic"
	TemplateProviderProfileCodex       = "codex"
	TemplateProviderProfileClaude      = "claude"
	TemplateProviderProfileGemini      = "gemini"
	TemplateProviderProfileAntigravity = "antigravity"

	TemplateTaskProfileGeneral = "general"
	TemplateTaskProfileCoding  = "coding"
)

type TemplateSelection struct {
	Mode            consensus.WorkflowMode
	ProviderProfile string
	TaskProfile     string
	Alias           string
}

func ResolveTemplateSelection(preset string, mode string, providerProfile string, taskProfile string) (TemplateSelection, error) {
	selection, err := resolvePresetAlias(preset)
	if err != nil {
		return TemplateSelection{}, err
	}
	if selection.Mode == "" {
		selection = TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileMock,
			TaskProfile:     TemplateTaskProfileGeneral,
		}
	}
	if normalizedMode, err := normalizeTemplateMode(mode); err != nil {
		return TemplateSelection{}, err
	} else if normalizedMode != "" {
		selection.Mode = normalizedMode
	}
	if normalizedProvider, err := normalizeProviderProfile(providerProfile); err != nil {
		return TemplateSelection{}, err
	} else if normalizedProvider != "" {
		selection.ProviderProfile = normalizedProvider
	}
	if normalizedTask, err := normalizeTaskProfile(taskProfile); err != nil {
		return TemplateSelection{}, err
	} else if normalizedTask != "" {
		selection.TaskProfile = normalizedTask
	}
	if selection.Mode == "" {
		selection.Mode = consensus.WorkflowModeAdjudication
	}
	if selection.ProviderProfile == "" {
		selection.ProviderProfile = TemplateProviderProfileMock
	}
	if selection.TaskProfile == "" {
		selection.TaskProfile = TemplateTaskProfileGeneral
	}
	if selection.TaskProfile == TemplateTaskProfileCoding && selection.Mode != consensus.WorkflowModeAdjudication {
		return TemplateSelection{}, fmt.Errorf("task profile coding 只支持 adjudication mode")
	}
	selection.Alias = canonicalTemplateAlias(selection)
	return selection, nil
}

func RenderTemplateRequest(preset string, mode string, providerProfile string, taskProfile string) (string, TemplateSelection, error) {
	selection, err := ResolveTemplateSelection(preset, mode, providerProfile, taskProfile)
	if err != nil {
		return "", TemplateSelection{}, err
	}
	body, err := RenderTemplateSelection(selection)
	if err != nil {
		return "", TemplateSelection{}, err
	}
	return body, selection, nil
}

func RenderTemplateSelection(selection TemplateSelection) (string, error) {
	cfg, err := BuildTemplateConfig(selection)
	if err != nil {
		return "", err
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal template config: %w", err)
	}
	header := []string{
		"# til-consensus 配置模板",
		fmt.Sprintf("# mode=%s provider_profile=%s task_profile=%s", selection.Mode, selection.ProviderProfile, selection.TaskProfile),
		"# 推荐优先使用 --mode / --provider-profile / --task-profile；--preset 仅保留为兼容别名。",
	}
	if selection.Alias != "" {
		header = append(header, fmt.Sprintf("# 兼容别名: %s", selection.Alias))
	}
	return strings.Join(header, "\n") + "\n" + string(body), nil
}

func WriteTemplateSelection(path string, selection TemplateSelection, force bool) error {
	body, err := RenderTemplateSelection(selection)
	if err != nil {
		return err
	}
	if !force {
		if _, statErr := os.Stat(path); statErr == nil {
			return fmt.Errorf("config already exists: %s", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write template config: %w", err)
	}
	return nil
}

func BuildTemplateConfig(selection TemplateSelection) (Config, error) {
	cfg := InitTemplate()
	cfg.Defaults.Mode = selection.Mode
	cfg.Defaults.TaskType = ""
	cfg.Defaults.WorkspaceSnapshot = nil
	cfg.Defaults.TaskConstraints = consensus.TaskConstraints{}
	cfg.Defaults.VerificationPolicy.RequiredChecks = nil

	applyBaseGeneralDefaults(&cfg)
	switch selection.TaskProfile {
	case TemplateTaskProfileGeneral:
	case TemplateTaskProfileCoding:
		applyCodingTaskProfile(&cfg)
	default:
		return Config{}, fmt.Errorf("unsupported task profile: %s", selection.TaskProfile)
	}
	switch selection.Mode {
	case consensus.WorkflowModeAdjudication:
		applyAdjudicationModeDefaults(&cfg)
	case consensus.WorkflowModeFreeDebate:
		applyFreeDebateModeDefaults(&cfg)
	case consensus.WorkflowModeDelphi:
		applyDelphiModeDefaults(&cfg)
	default:
		return Config{}, fmt.Errorf("unsupported template mode: %s", selection.Mode)
	}
	providerID, modelID, providerConfig, err := buildTemplateProvider(selection.ProviderProfile)
	if err != nil {
		return Config{}, err
	}
	cfg.Providers = map[string]ProviderConfig{
		providerID: providerConfig,
	}
	cfg.Agents, cfg.Roles = buildTemplateAgents(selection.Mode, providerID, modelID)
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyBaseGeneralDefaults(cfg *Config) {
	cfg.Defaults.SuccessCriteria = []string{
		"给出 claim 级裁决",
		"对证据不足部分明确保留 unresolved 或 undetermined",
	}
	cfg.Defaults.AllowedTools = []string{"sources", "compare", "cross-check"}
	cfg.Defaults.PerTaskTimeout = Duration{Duration: 20 * time.Minute}
	cfg.Defaults.TaskRetryAttempts = consensus.DefaultTaskRetryAttempts
	cfg.Defaults.ProposalPolicy = ProposalPolicyConfig{
		MaxPasses:          1,
		MaxClaimsPerWorker: 3,
		DedupeStrategy:     "normalized-statement",
	}
	cfg.Defaults.VerificationPolicy = VerificationPolicyConfig{
		AllowSemanticVerifier: true,
		MaxParallelChecks:     2,
	}
	cfg.Defaults.ArbiterPolicy = ArbiterPolicyConfig{
		AllowUndetermined: true,
		BlindReview:       true,
	}
}

func applyCodingTaskProfile(cfg *Config) {
	cfg.Defaults.TaskType = consensus.CaseTaskTypeCoding
	cfg.Defaults.SuccessCriteria = []string{
		"给出 claim 级裁决",
		"对测试、基准和 diff 的证据做显式引用",
		"证据不足时允许 undetermined",
	}
	cfg.Defaults.AllowedTools = []string{"repo", "git", "tests", "benchmarks"}
	cfg.Defaults.WorkspaceSnapshot = &consensus.WorkspaceSnapshot{
		Root:     ".",
		Revision: "HEAD",
		Paths:    []string{"cmd", "internal", "go.mod", "go.sum"},
	}
	cfg.Defaults.TaskConstraints = consensus.TaskConstraints{
		Language:     "go",
		AllowedPaths: []string{"cmd/", "internal/", "go.mod", "go.sum"},
		RequiredCommands: []string{
			"go",
			"git",
		},
		Notes: []string{
			"patch 必须限制在允许路径内",
			"benchmark 结果需要可复现",
		},
	}
	cfg.Defaults.VerificationPolicy.RequiredChecks = []consensus.VerificationCheck{
		{Name: "workspace-snapshot", Kind: "workspace_snapshot"},
		{Name: "allowed-paths", Kind: "allowed_paths", Paths: []string{"cmd/", "internal/", "go.mod", "go.sum"}},
		{Name: "changed-files", Kind: "git_diff_paths", BaseRevision: "origin/main", Paths: []string{"cmd/", "internal/"}},
		{Name: "unit-tests", Kind: "command", Command: "go", Args: []string{"test", "./..."}, Workdir: "."},
		{
			Name:          "benchmark-budget",
			Kind:          "benchmark_threshold",
			Command:       "go",
			Args:          []string{"test", "./...", "-run", "^$", "-bench", "."},
			Workdir:       ".",
			Pattern:       `Benchmark.*\s+(\d+(?:\.\d+)?) ns/op`,
			Threshold:     250000,
			ThresholdMode: "max",
		},
	}
}

func applyAdjudicationModeDefaults(cfg *Config) {
	cfg.Defaults.Mode = consensus.WorkflowModeAdjudication
}

func applyFreeDebateModeDefaults(cfg *Config) {
	cfg.Defaults.Mode = consensus.WorkflowModeFreeDebate
	cfg.Defaults.SuccessCriteria = []string{
		"让多个 participant 独立提出主张并交叉辩论",
		"最终通过 final vote 收敛",
		"没有共识时允许 no_consensus",
	}
	cfg.Defaults.DebatePolicy = DebatePolicyConfig{
		MinRounds:       2,
		MaxRounds:       3,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
	}
}

func applyDelphiModeDefaults(cfg *Config) {
	cfg.Defaults.Mode = consensus.WorkflowModeDelphi
	cfg.Defaults.SuccessCriteria = []string{
		"让参与者匿名给出候选结论和评分",
		"每轮基于匿名汇总修订意见",
		"输出推荐结论、收敛度和异议摘要",
	}
	cfg.Defaults.DelphiPolicy = DelphiPolicyConfig{
		MinRounds:               2,
		MaxRounds:               3,
		ConvergenceThreshold:    0.8,
		RatingScaleMin:          1,
		RatingScaleMax:          5,
		Anonymous:               true,
		FacilitatorSummaryStyle: "anonymous-aggregate",
	}
}

func buildTemplateProvider(profile string) (string, string, ProviderConfig, error) {
	switch profile {
	case TemplateProviderProfileMock:
		return "mock", "default", ProviderConfig{
			Type:     ProviderTypeMock,
			Behavior: "deterministic",
			Models: map[string]ProviderModelConfig{
				"default": {ProviderModel: "mock-default"},
			},
		}, nil
	case TemplateProviderProfileOpenAI:
		temp := 0.2
		return "openai", "default", ProviderConfig{
			Type:      ProviderTypeAPI,
			Protocol:  "openai-compatible",
			BaseURL:   "https://api.openai.com/v1",
			APIKeyEnv: "OPENAI_API_KEY",
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "your-openai-model",
					Temperature:   &temp,
					Reasoning:     "medium",
				},
			},
		}, nil
	case TemplateProviderProfileGeneric:
		return "generic-local", "default", ProviderConfig{
			Type:    ProviderTypeCLI,
			CLIType: "generic",
			Command: "python3",
			Args:    []string{"./scripts/generic_adapter.py"},
			Env: map[string]string{
				"ADAPTER_MODEL":      "{providerModel}",
				"ADAPTER_ROLE":       "{role}",
				"ADAPTER_REQUEST_ID": "{requestId}",
				"ADAPTER_SESSION_ID": "{sessionId}",
			},
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "local-generic",
					Reasoning:     "medium",
				},
			},
		}, nil
	case TemplateProviderProfileCodex:
		return "codex-cli", "default", ProviderConfig{
			Type:    ProviderTypeCLI,
			CLIType: "codex",
			Command: "codex",
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "gpt-5.4",
					Reasoning:     "medium",
				},
			},
		}, nil
	case TemplateProviderProfileClaude:
		return "claude-cli", "default", ProviderConfig{
			Type:    ProviderTypeCLI,
			CLIType: "claude",
			Command: "claude",
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "claude-opus-4-6",
					Reasoning:     "medium",
				},
			},
		}, nil
	case TemplateProviderProfileGemini:
		return "gemini-cli", "default", ProviderConfig{
			Type:    ProviderTypeCLI,
			CLIType: "gemini",
			Command: "gemini",
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "gemini-3.1-pro-preview",
					Reasoning:     "medium",
				},
			},
		}, nil
	case TemplateProviderProfileAntigravity:
		return "antigravity-cli", "default", ProviderConfig{
			Type:    ProviderTypeCLI,
			CLIType: CLITypeAntigravity,
			Command: "agy",
			Models: map[string]ProviderModelConfig{
				"default": {
					ProviderModel: "Gemini 3.5 Flash (High)",
					Reasoning:     "medium",
				},
			},
		}, nil
	default:
		return "", "", ProviderConfig{}, fmt.Errorf("unsupported provider profile: %s", profile)
	}
}

func buildTemplateAgents(mode consensus.WorkflowMode, providerID string, modelID string) ([]AgentConfig, RolesConfig) {
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		return []AgentConfig{
				{ID: "participant-a", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "participant-b", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "participant-c", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "reporter-a", Provider: providerID, Model: modelID, Role: "reporter"},
				{ID: "actor-a", Provider: providerID, Model: modelID, Role: "actor"},
			}, RolesConfig{
				Participants: []string{"participant-a", "participant-b", "participant-c"},
				Reporter:     "reporter-a",
				Actor:        "actor-a",
			}
	case consensus.WorkflowModeDelphi:
		return []AgentConfig{
				{ID: "participant-a", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "participant-b", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "participant-c", Provider: providerID, Model: modelID, Role: "participant"},
				{ID: "facilitator-a", Provider: providerID, Model: modelID, Role: "facilitator"},
				{ID: "reporter-a", Provider: providerID, Model: modelID, Role: "reporter"},
			}, RolesConfig{
				Participants: []string{"participant-a", "participant-b", "participant-c"},
				Facilitator:  "facilitator-a",
				Reporter:     "reporter-a",
			}
	default:
		return []AgentConfig{
				{ID: "proposer-a", Provider: providerID, Model: modelID, Role: "proposer"},
				{ID: "challenger-a", Provider: providerID, Model: modelID, Role: "challenger"},
				{ID: "arbiter-a", Provider: providerID, Model: modelID, Role: "arbiter"},
				{ID: "verifier-a", Provider: providerID, Model: modelID, Role: "semantic-verifier"},
				{ID: "reporter-a", Provider: providerID, Model: modelID, Role: "reporter"},
				{ID: "actor-a", Provider: providerID, Model: modelID, Role: "actor"},
			}, RolesConfig{
				Proposers:        []string{"proposer-a"},
				Challengers:      []string{"challenger-a"},
				Arbiter:          "arbiter-a",
				SemanticVerifier: "verifier-a",
				Reporter:         "reporter-a",
				Actor:            "actor-a",
			}
	}
}

func resolvePresetAlias(preset string) (TemplateSelection, error) {
	switch normalizePreset(preset) {
	case "", TemplatePresetQuickstart:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileMock,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetQuickstart,
		}, nil
	case TemplatePresetOpenAI:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileOpenAI,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetOpenAI,
		}, nil
	case TemplatePresetCoding:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileMock,
			TaskProfile:     TemplateTaskProfileCoding,
			Alias:           TemplatePresetCoding,
		}, nil
	case TemplatePresetDebate:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeFreeDebate,
			ProviderProfile: TemplateProviderProfileMock,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetDebate,
		}, nil
	case TemplatePresetDelphi:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeDelphi,
			ProviderProfile: TemplateProviderProfileMock,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetDelphi,
		}, nil
	case TemplatePresetGeneric:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileGeneric,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetGeneric,
		}, nil
	case TemplatePresetCodex:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileCodex,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetCodex,
		}, nil
	case TemplatePresetClaude:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileClaude,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetClaude,
		}, nil
	case TemplatePresetGemini:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileGemini,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetGemini,
		}, nil
	case TemplatePresetAntigravity:
		return TemplateSelection{
			Mode:            consensus.WorkflowModeAdjudication,
			ProviderProfile: TemplateProviderProfileAntigravity,
			TaskProfile:     TemplateTaskProfileGeneral,
			Alias:           TemplatePresetAntigravity,
		}, nil
	default:
		return TemplateSelection{}, fmt.Errorf("unsupported config preset: %s", preset)
	}
}

func canonicalTemplateAlias(selection TemplateSelection) string {
	switch {
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileMock && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetQuickstart
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileOpenAI && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetOpenAI
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileMock && selection.TaskProfile == TemplateTaskProfileCoding:
		return TemplatePresetCoding
	case selection.Mode == consensus.WorkflowModeFreeDebate && selection.ProviderProfile == TemplateProviderProfileMock && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetDebate
	case selection.Mode == consensus.WorkflowModeDelphi && selection.ProviderProfile == TemplateProviderProfileMock && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetDelphi
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileGeneric && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetGeneric
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileCodex && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetCodex
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileClaude && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetClaude
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileGemini && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetGemini
	case selection.Mode == consensus.WorkflowModeAdjudication && selection.ProviderProfile == TemplateProviderProfileAntigravity && selection.TaskProfile == TemplateTaskProfileGeneral:
		return TemplatePresetAntigravity
	default:
		return ""
	}
}

func normalizeTemplateMode(mode string) (consensus.WorkflowMode, error) {
	value := strings.TrimSpace(strings.ToLower(mode))
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "":
		return "", nil
	case string(consensus.WorkflowModeAdjudication):
		return consensus.WorkflowModeAdjudication, nil
	case string(consensus.WorkflowModeFreeDebate):
		return consensus.WorkflowModeFreeDebate, nil
	case string(consensus.WorkflowModeDelphi):
		return consensus.WorkflowModeDelphi, nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

func normalizeProviderProfile(profile string) (string, error) {
	value := strings.TrimSpace(strings.ToLower(profile))
	switch value {
	case "":
		return "", nil
	case TemplateProviderProfileMock,
		TemplateProviderProfileOpenAI,
		TemplateProviderProfileGeneric,
		TemplateProviderProfileCodex,
		TemplateProviderProfileClaude,
		TemplateProviderProfileGemini,
		TemplateProviderProfileAntigravity:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported provider profile: %s", profile)
	}
}

func normalizeTaskProfile(profile string) (string, error) {
	value := strings.TrimSpace(strings.ToLower(profile))
	switch value {
	case "":
		return "", nil
	case TemplateTaskProfileGeneral, TemplateTaskProfileCoding:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported task profile: %s", profile)
	}
}
