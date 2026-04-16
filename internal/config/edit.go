package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type AddProviderInput struct {
	ID            string
	Type          string
	ModelID       string
	ProviderModel string
	Protocol      string
	BaseURL       string
	APIKeyEnv     string
	Headers       map[string]string
	CLIType       string
	Command       string
	Args          []string
	Env           map[string]string
	Adapter       string
	Options       map[string]any
	Behavior      string
	Delay         Duration
	Error         string
	Temperature   *float64
	Reasoning     string
	AgentID       string
}

type AddAgentInput struct {
	ID           string
	Provider     string
	Model        string
	Role         string
	SystemPrompt string
	Timeout      Duration
	Temperature  *float64
	Reasoning    string
	Assigns      []string
}

func Write(path string, cfg Config) error {
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ApplyAddProvider(cfg Config, input AddProviderInput) (Config, error) {
	if strings.TrimSpace(input.ID) == "" {
		return Config{}, fmt.Errorf("provider id is required")
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if _, exists := cfg.Providers[input.ID]; exists {
		return Config{}, fmt.Errorf("provider id already exists: %s", input.ID)
	}
	provider, err := BuildProvider(input)
	if err != nil {
		return Config{}, err
	}
	cfg.Providers[input.ID] = provider
	if strings.TrimSpace(input.AgentID) != "" {
		next, err := ApplyAddAgent(cfg, AddAgentInput{
			ID:       input.AgentID,
			Provider: input.ID,
			Model:    defaultModelIDForAgent(provider, input.ModelID),
		})
		if err != nil {
			return Config{}, err
		}
		cfg = next
	}
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ApplyAddAgent(cfg Config, input AddAgentInput) (Config, error) {
	if strings.TrimSpace(input.ID) == "" {
		return Config{}, fmt.Errorf("agent id is required")
	}
	for _, agent := range cfg.Agents {
		if agent.ID == input.ID {
			return Config{}, fmt.Errorf("agent id already exists: %s", input.ID)
		}
	}
	provider, ok := cfg.Providers[input.Provider]
	if !ok {
		return Config{}, fmt.Errorf("unknown provider: %s", input.Provider)
	}
	modelID := strings.TrimSpace(input.Model)
	if modelID == "" {
		if inferred, ok := singleModelID(provider); ok {
			modelID = inferred
		}
	}
	cfg.Agents = append(cfg.Agents, AgentConfig{
		ID:           input.ID,
		Provider:     input.Provider,
		Model:        modelID,
		Role:         input.Role,
		SystemPrompt: input.SystemPrompt,
		Timeout:      input.Timeout,
		Temperature:  input.Temperature,
		Reasoning:    input.Reasoning,
	})
	for _, assign := range input.Assigns {
		switch strings.TrimSpace(assign) {
		case "proposer":
			cfg.Roles.Proposers = append(cfg.Roles.Proposers, input.ID)
		case "challenger":
			cfg.Roles.Challengers = append(cfg.Roles.Challengers, input.ID)
		case "participant":
			cfg.Roles.Participants = append(cfg.Roles.Participants, input.ID)
		case "arbiter":
			cfg.Roles.Arbiter = input.ID
		case "semantic-verifier":
			cfg.Roles.SemanticVerifier = input.ID
		case "facilitator":
			cfg.Roles.Facilitator = input.ID
		case "reporter":
			cfg.Roles.Reporter = input.ID
		case "actor":
			cfg.Roles.Actor = input.ID
		default:
			return Config{}, fmt.Errorf("unsupported role assignment: %s", assign)
		}
	}
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func BuildProvider(input AddProviderInput) (ProviderConfig, error) {
	providerType := strings.TrimSpace(input.Type)
	if providerType == "" {
		return ProviderConfig{}, fmt.Errorf("provider type is required")
	}
	provider := ProviderConfig{
		Type:      providerType,
		Protocol:  input.Protocol,
		CLIType:   input.CLIType,
		BaseURL:   input.BaseURL,
		APIKeyEnv: input.APIKeyEnv,
		Headers:   cloneStringMap(input.Headers),
		Command:   input.Command,
		Args:      append([]string(nil), input.Args...),
		Env:       cloneStringMap(input.Env),
		Adapter:   input.Adapter,
		Options:   cloneAnyMap(input.Options),
		Behavior:  input.Behavior,
		Delay:     input.Delay,
		Error:     input.Error,
	}
	modelID := strings.TrimSpace(input.ModelID)
	if providerType == ProviderTypeMock && modelID == "" {
		modelID = "default"
	}
	if modelID != "" {
		model := ProviderModelConfig{
			ProviderModel: input.ProviderModel,
			Temperature:   input.Temperature,
			Reasoning:     input.Reasoning,
		}
		if strings.TrimSpace(model.ProviderModel) == "" {
			model.ProviderModel = modelID
		}
		provider.Models = map[string]ProviderModelConfig{
			modelID: model,
		}
	}
	provider = normalizeProvider(provider)
	if err := Validate(Config{
		SchemaVersion: 1,
		Providers:     map[string]ProviderConfig{input.ID: provider},
		Agents:        []AgentConfig{{ID: "probe", Provider: input.ID, Model: defaultModelIDForAgent(provider, modelID)}},
		Roles: RolesConfig{
			Proposers:   []string{"probe"},
			Challengers: []string{"probe"},
		},
	}); err != nil {
		if provider.Type == ProviderTypeMock && strings.Contains(err.Error(), "agent probe") {
			return provider, nil
		}
		return ProviderConfig{}, err
	}
	return provider, nil
}

func defaultModelIDForAgent(provider ProviderConfig, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return requested
	}
	if inferred, ok := singleModelID(provider); ok {
		return inferred
	}
	return ""
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
