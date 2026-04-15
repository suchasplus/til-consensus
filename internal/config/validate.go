package config

import (
	"fmt"
	"strings"
)

func Validate(cfg Config) error {
	if cfg.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version: %d", cfg.SchemaVersion)
	}
	if len(cfg.Providers) == 0 {
		return fmt.Errorf("providers must not be empty")
	}
	if len(cfg.Agents) == 0 {
		return fmt.Errorf("agents must not be empty")
	}
	for name, provider := range cfg.Providers {
		switch provider.Type {
		case "mock":
			if provider.Behavior == "" {
				cfg.Providers[name] = provider
			}
		case "openai":
			if provider.APIKeyEnv == "" {
				return fmt.Errorf("provider %s: api_key_env is required", name)
			}
		case "command":
			if strings.TrimSpace(provider.Command) == "" {
				return fmt.Errorf("provider %s: command is required", name)
			}
		default:
			return fmt.Errorf("provider %s: unsupported type %q", name, provider.Type)
		}
	}
	seen := map[string]struct{}{}
	for _, agent := range cfg.Agents {
		if strings.TrimSpace(agent.ID) == "" {
			return fmt.Errorf("agent id is required")
		}
		if _, ok := seen[agent.ID]; ok {
			return fmt.Errorf("duplicate agent id: %s", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		provider, ok := cfg.Providers[agent.Provider]
		if !ok {
			return fmt.Errorf("agent %s: unknown provider %s", agent.ID, agent.Provider)
		}
		if provider.Type == "openai" && strings.TrimSpace(agent.Model) == "" && strings.TrimSpace(provider.Model) == "" {
			return fmt.Errorf("agent %s: openai model must be set on agent or provider", agent.ID)
		}
	}
	for _, id := range cfg.Defaults.DefaultAgents {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("default_agents references unknown agent %s", id)
		}
	}
	if cfg.Defaults.MaxRounds > 0 && cfg.Defaults.MinRounds > cfg.Defaults.MaxRounds {
		return fmt.Errorf("defaults.max_rounds must be >= defaults.min_rounds")
	}
	if cfg.Defaults.Threshold < 0 || cfg.Defaults.Threshold > 1 {
		return fmt.Errorf("defaults.threshold must be in [0,1]")
	}
	return nil
}
