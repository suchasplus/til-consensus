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
