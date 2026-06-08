package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFilename = "default.yaml"
	LegacyConfigFilename  = "config.yaml"
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
	defaultGlobalPath := filepath.Join(configHome, "til-consensus", DefaultConfigFilename)
	if _, err := os.Stat(defaultGlobalPath); err == nil {
		return defaultGlobalPath, nil
	}
	legacyGlobalPath := filepath.Join(configHome, "til-consensus", LegacyConfigFilename)
	if _, err := os.Stat(legacyGlobalPath); err == nil {
		return legacyGlobalPath, nil
	}
	return "", fmt.Errorf("cannot find config file, tried %s, %s and %s", projectPath, defaultGlobalPath, legacyGlobalPath)
}

func Load(path string) (LoadedConfig, error) {
	path = toAbs(path, mustGetwd())
	cfg, err := loadConfigWithIncludes(path, nil)
	if err != nil {
		return LoadedConfig{}, err
	}
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return LoadedConfig{}, err
	}
	return LoadedConfig{
		Path:      path,
		ConfigDir: filepath.Dir(path),
		Config:    cfg,
	}, nil
}

func LoadProfiles(path string) (LoadedConfig, error) {
	path = toAbs(path, mustGetwd())
	cfg, err := loadConfigWithIncludes(path, nil)
	if err != nil {
		return LoadedConfig{}, err
	}
	cfg = Normalize(cfg)
	if err := ValidateProfiles(cfg); err != nil {
		return LoadedConfig{}, err
	}
	return LoadedConfig{
		Path:      path,
		ConfigDir: filepath.Dir(path),
		Config:    cfg,
	}, nil
}

func loadConfigWithIncludes(path string, stack []string) (Config, error) {
	path = toAbs(path, mustGetwd())
	key, err := canonicalConfigPath(path)
	if err != nil {
		return Config{}, err
	}
	for _, active := range stack {
		if active == key {
			return Config{}, fmt.Errorf("config include cycle detected: %s", strings.Join(append(stack, key), " -> "))
		}
	}
	cfg, err := decodeConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	var merged Config
	nextStack := append(stack, key)
	for _, includePath := range cfg.Include {
		includePath = strings.TrimSpace(includePath)
		if includePath == "" {
			return Config{}, fmt.Errorf("empty config include in %s", path)
		}
		resolvedInclude := toAbs(includePath, filepath.Dir(path))
		included, err := loadConfigWithIncludes(resolvedInclude, nextStack)
		if err != nil {
			return Config{}, fmt.Errorf("load include %s from %s: %w", includePath, path, err)
		}
		merged = mergeConfig(merged, included)
	}
	cfg.Include = nil
	return mergeConfig(merged, cfg), nil
}

func decodeConfigFile(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode yaml config: %w", err)
	}
	return cfg, nil
}

func canonicalConfigPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if _, statErr := os.Stat(abs); statErr != nil {
		return "", fmt.Errorf("config file not found: %s", abs)
	}
	return filepath.Clean(abs), nil
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

func DefaultConfigPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "til-consensus", DefaultConfigFilename), nil
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
