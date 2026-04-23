package config

import (
	"path/filepath"
	"testing"
)

func TestWriteAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "til-consensus.yaml")
	cfg := Normalize(InitTemplate())
	if err := Write(path, cfg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Config.Roles.Arbiter == "" || len(loaded.Config.Agents) == 0 {
		t.Fatalf("unexpected loaded config: %#v", loaded.Config)
	}
}

func TestApplyAddProviderAndAgent(t *testing.T) {
	cfg := Normalize(InitTemplate())
	temperature := 0.3
	headers := map[string]string{"X-Test": "1"}
	env := map[string]string{"DEBUG": "true"}
	options := map[string]any{"retries": int64(3)}
	next, err := ApplyAddProvider(cfg, AddProviderInput{
		ID:              "api1",
		Type:            ProviderTypeAPI,
		ModelID:         "general",
		ProviderModel:   "gpt-5",
		ContextWindow:   128000,
		MaxOutputTokens: 4096,
		Protocol:        APIProtocolOpenAICompatible,
		BaseURL:         "https://example.com/v1",
		APIKeyEnv:       "OPENAI_API_KEY",
		Headers:         headers,
		Env:             env,
		Options:         options,
		Temperature:     &temperature,
		Reasoning:       "medium",
		AgentID:         "api-agent",
	})
	if err != nil {
		t.Fatalf("ApplyAddProvider failed: %v", err)
	}
	headers["X-Test"] = "mutated"
	env["DEBUG"] = "mutated"
	options["retries"] = int64(99)

	provider := next.Providers["api1"]
	if provider.Type != ProviderTypeAPI || provider.Models["general"].ProviderModel != "gpt-5" {
		t.Fatalf("unexpected provider: %#v", provider)
	}
	if provider.Models["general"].ContextWindow != 128000 || provider.Models["general"].MaxOutputTokens != 4096 {
		t.Fatalf("expected model sizing to be preserved: %#v", provider.Models["general"])
	}
	if provider.Headers["X-Test"] != "1" || provider.Env["DEBUG"] != "true" {
		t.Fatalf("expected provider maps to be cloned: %#v", provider)
	}
	foundAgent := false
	for _, agent := range next.Agents {
		if agent.ID == "api-agent" {
			foundAgent = true
			if agent.Provider != "api1" || agent.Model != "general" {
				t.Fatalf("unexpected generated agent: %#v", agent)
			}
		}
	}
	if !foundAgent {
		t.Fatal("expected generated agent to exist")
	}

	next, err = ApplyAddProvider(next, AddProviderInput{
		ID:            "gemini-api",
		Type:          ProviderTypeAPI,
		ModelID:       "default",
		ProviderModel: "gemini-2.5-flash",
		Protocol:      APIProtocolGemini,
		BaseURL:       "https://generativelanguage.googleapis.com/v1beta",
		APIKeyEnv:     "GEMINI_API_KEY",
	})
	if err != nil {
		t.Fatalf("ApplyAddProvider gemini failed: %v", err)
	}
	if next.Providers["gemini-api"].Protocol != APIProtocolGemini {
		t.Fatalf("expected gemini-api protocol, got %#v", next.Providers["gemini-api"])
	}

	next, err = ApplyAddAgent(next, AddAgentInput{
		ID:       "reviewer-a",
		Provider: "mock",
		Assigns:  []string{"challenger", "reporter", "actor"},
	})
	if err != nil {
		t.Fatalf("ApplyAddAgent failed: %v", err)
	}
	if next.Roles.Reporter != "reviewer-a" || next.Roles.Actor != "reviewer-a" {
		t.Fatalf("unexpected roles after add agent: %#v", next.Roles)
	}
	foundReviewer := false
	for _, agent := range next.Agents {
		if agent.ID == "reviewer-a" {
			foundReviewer = true
			if agent.Model != "default" {
				t.Fatalf("expected default model inference, got %#v", agent)
			}
		}
	}
	if !foundReviewer {
		t.Fatal("expected reviewer agent to exist")
	}
}

func TestBuildProviderVariants(t *testing.T) {
	commandProvider, err := BuildProvider(AddProviderInput{
		ID:      "cmd",
		Type:    ProviderTypeCommand,
		ModelID: "default",
		Command: "codex",
	})
	if err != nil {
		t.Fatalf("BuildProvider command failed: %v", err)
	}
	if commandProvider.Type != ProviderTypeCLI || commandProvider.CLIType != CLITypeGeneric {
		t.Fatalf("expected command provider to normalize to cli: %#v", commandProvider)
	}

	mockProvider, err := BuildProvider(AddProviderInput{
		ID:       "mock",
		Type:     ProviderTypeMock,
		Behavior: "deterministic",
	})
	if err != nil {
		t.Fatalf("BuildProvider mock failed: %v", err)
	}
	if _, ok := mockProvider.Models["default"]; !ok {
		t.Fatalf("expected mock default model, got %#v", mockProvider.Models)
	}

	if _, err := BuildProvider(AddProviderInput{ID: "bad", Type: ProviderTypeCLI, ModelID: "default"}); err == nil {
		t.Fatal("expected invalid generic cli provider to fail")
	}
}

func TestApplyAddAgentAndProviderErrors(t *testing.T) {
	cfg := Normalize(InitTemplate())
	if _, err := ApplyAddProvider(cfg, AddProviderInput{ID: "mock", Type: ProviderTypeMock}); err == nil {
		t.Fatal("expected duplicate provider to fail")
	}
	if _, err := ApplyAddAgent(cfg, AddAgentInput{ID: "proposer-a", Provider: "mock"}); err == nil {
		t.Fatal("expected duplicate agent to fail")
	}
	if _, err := ApplyAddAgent(cfg, AddAgentInput{ID: "new", Provider: "mock", Assigns: []string{"unknown"}}); err == nil {
		t.Fatal("expected unsupported role assignment to fail")
	}
}

func TestConfigHelpers(t *testing.T) {
	if got := defaultModelIDForAgent(ProviderConfig{
		Models: map[string]ProviderModelConfig{
			"only": {ProviderModel: "model"},
		},
	}, ""); got != "only" {
		t.Fatalf("unexpected default model id: %s", got)
	}
	if got := cloneStringMap(nil); got != nil {
		t.Fatalf("expected nil map clone, got %#v", got)
	}
	if got := cloneAnyMap(nil); got != nil {
		t.Fatalf("expected nil any map clone, got %#v", got)
	}
	if got := defaultModelIDForAgent(ProviderConfig{}, "explicit"); got != "explicit" {
		t.Fatalf("expected explicit model id to win, got %s", got)
	}
}
