package preflight

import (
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
)

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
