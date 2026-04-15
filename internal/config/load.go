package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func ResolveConfigPath(explicitPath string) (string, error) {
	if explicitPath != "" {
		path := toAbs(explicitPath, mustGetwd())
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("config file not found: %s", path)
		}
		return path, nil
	}
	cwd := mustGetwd()
	projectPath := filepath.Join(cwd, "til-consensus.yaml")
	if _, err := os.Stat(projectPath); err == nil {
		return projectPath, nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	globalPath := filepath.Join(configHome, "til-consensus", "config.yaml")
	if _, err := os.Stat(globalPath); err == nil {
		return globalPath, nil
	}
	return "", fmt.Errorf("cannot find config file, tried %s and %s", projectPath, globalPath)
}

func Load(path string) (LoadedConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return LoadedConfig{}, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return LoadedConfig{}, fmt.Errorf("decode yaml config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return LoadedConfig{}, err
	}
	return LoadedConfig{
		Path:      path,
		ConfigDir: filepath.Dir(path),
		Config:    cfg,
	}, nil
}

func LoadRunInput(path string) (RunInput, error) {
	if strings.TrimSpace(path) == "" {
		return RunInput{}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return RunInput{}, fmt.Errorf("read run input: %w", err)
	}
	var input RunInput
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(body, &input); err != nil {
			return RunInput{}, fmt.Errorf("decode json input: %w", err)
		}
		return input, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(&input); err != nil {
		return RunInput{}, fmt.Errorf("decode yaml input: %w", err)
	}
	return input, nil
}

func InitTemplate() Config {
	return Config{
		SchemaVersion: 1,
		Defaults: DefaultsConfig{
			DefaultAgents:   []string{"mock-a", "mock-b"},
			MinRounds:       2,
			MaxRounds:       3,
			Threshold:       1.0,
			PerTaskTimeout:  Duration{Duration: 20 * time.Minute},
			PerRoundTimeout: Duration{Duration: 20 * time.Minute},
			Composer:        "builtin",
			TraceLevel:      "compact",
		},
		Output: OutputConfig{
			Directory: "./out/{requestId}",
		},
		Providers: map[string]ProviderConfig{
			"mock": {
				Type:     "mock",
				Behavior: "deterministic",
			},
		},
		Agents: []AgentConfig{
			{ID: "mock-a", Provider: "mock", Role: "reviewer-a"},
			{ID: "mock-b", Provider: "mock", Role: "reviewer-b"},
		},
	}
}

func WriteTemplate(path string) error {
	cfg := InitTemplate()
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal template config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write template config: %w", err)
	}
	return nil
}

func DefaultConfigPath() (string, error) {
	cwd := mustGetwd()
	projectPath := filepath.Join(cwd, "til-consensus.yaml")
	return projectPath, nil
}

func toAbs(path, base string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return cwd
}
