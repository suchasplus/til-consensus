package preflight

import (
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
)

func TestBuildCandidatesSkipsDisabledProviderAndModels(t *testing.T) {
	disabled := false
	cfg := config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"enabled-api": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAICompatible,
				Models: map[string]config.ProviderModelConfig{
					"enabled":  {ProviderModel: "enabled-model"},
					"disabled": {Enabled: &disabled, ProviderModel: "disabled-model"},
				},
			},
			"disabled-api": {
				Enabled:  &disabled,
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAICompatible,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "disabled-provider-model"},
				},
			},
		},
	})

	candidates, err := buildCandidates(cfg, Options{All: true})
	if err != nil {
		t.Fatalf("buildCandidates failed: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ProviderID != "enabled-api" || candidates[0].ModelID != "enabled" {
		t.Fatalf("expected only enabled provider/model candidate, got %#v", candidates)
	}

	_, err = buildCandidates(cfg, Options{ProviderIDs: []string{"disabled-api"}})
	if err == nil || !strings.Contains(err.Error(), "provider disabled-api is disabled") {
		t.Fatalf("expected explicit disabled provider error, got %v", err)
	}
}

func TestEffectiveMaxOutputTokensUsesDefaultBudget(t *testing.T) {
	got := effectiveMaxOutputTokens(candidate{
		ProviderType: config.ProviderTypeAPI,
		Protocol:     config.APIProtocolGemini,
	})
	if got != 2048 {
		t.Fatalf("expected default budget 2048, got %d", got)
	}
}

func TestEffectiveMaxOutputTokensUsesConfiguredLowerBudget(t *testing.T) {
	got := effectiveMaxOutputTokens(candidate{
		ProviderType: config.ProviderTypeAPI,
		Protocol:     config.APIProtocolGemini,
		ModelConfig:  config.ProviderModelConfig{MaxOutputTokens: 512},
	})
	if got != 512 {
		t.Fatalf("expected configured lower Gemini API budget 512, got %d", got)
	}
}

func TestAnnotatePreflightBudgetShowsConfiguredCap(t *testing.T) {
	ctx := map[string]any{
		"generation": map[string]any{
			"maxOutputTokens": 2048,
		},
	}
	annotatePreflightBudget(ctx, 65535, 2048)
	generation := ctx["generation"].(map[string]any)
	if generation["configuredMaxOutputTokens"] != 65535 {
		t.Fatalf("expected configured budget annotation, got %#v", generation)
	}
	if generation["budgetPolicy"] != "preflight_cap" {
		t.Fatalf("expected preflight cap annotation, got %#v", generation)
	}
}
