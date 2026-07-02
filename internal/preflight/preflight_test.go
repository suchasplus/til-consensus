package preflight

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
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

func TestPreflightSurfacesOpenAICompatibleFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason":        "content_filter",
				"native_finish_reason": "SAFETY_CHECK_TYPE_BIO",
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"refusal": nil,
				},
			}},
		})
	}))
	defer server.Close()

	sink := &captureSink{}
	entries, err := Run(context.Background(), config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"openrouter-api": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAICompatible,
				BaseURL:  server.URL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "x-ai/grok-4.3"},
				},
				Options: map[string]any{
					"max_output_tokens_field": "max_tokens",
				},
			},
		},
	}, Options{ProviderIDs: []string{"openrouter-api"}, Timeout: time.Second}, sink)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %#v", entries)
	}
	entry := entries[0]
	if entry.Ready {
		t.Fatalf("expected provider to be not ready")
	}
	for _, needle := range []string{"finish_reason=content_filter", "native_finish_reason=SAFETY_CHECK_TYPE_BIO"} {
		if !strings.Contains(entry.Error, needle) {
			t.Fatalf("error missing %q: %s", needle, entry.Error)
		}
	}
	for _, needle := range []string{`"finish_reason":"content_filter"`, `"native_finish_reason":"SAFETY_CHECK_TYPE_BIO"`, `"content":null`} {
		if !strings.Contains(sink.raw, needle) {
			t.Fatalf("raw artifact missing %q: %s", needle, sink.raw)
		}
	}
	if entry.StdoutPreview == "" {
		t.Fatalf("expected raw response preview")
	}
	if !strings.Contains(entry.StdoutFull, `"native_finish_reason":"SAFETY_CHECK_TYPE_BIO"`) {
		t.Fatalf("expected full stdout to contain complete raw response, got %s", entry.StdoutFull)
	}
	if entry.InputArtifact != "input" || entry.RawArtifact != "raw" || entry.ErrorArtifact != "error" {
		t.Fatalf("expected artifact paths to be recorded, got input=%q raw=%q error=%q", entry.InputArtifact, entry.RawArtifact, entry.ErrorArtifact)
	}
	choice, ok := entry.ResponseContext["choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected response choice context, got %#v", entry.ResponseContext)
	}
	if choice["finishReason"] != "content_filter" || choice["nativeFinishReason"] != "SAFETY_CHECK_TYPE_BIO" {
		t.Fatalf("unexpected response choice context: %#v", choice)
	}
	message, ok := choice["message"].(map[string]any)
	if !ok || message["contentState"] != "null" {
		t.Fatalf("unexpected response message context: %#v", choice)
	}
	if !strings.Contains(sink.err, "finish_reason=content_filter") {
		t.Fatalf("expected error artifact to include finish reason, got %s", sink.err)
	}
}

type captureSink struct {
	input any
	raw   string
	err   string
}

func (s *captureSink) WriteInput(_ string, payload any) string {
	s.input = payload
	return "input"
}

func (s *captureSink) WriteRaw(_ string, raw string) string {
	s.raw = raw
	return "raw"
}

func (s *captureSink) WriteError(_ string, message string) string {
	s.err = message
	return "error"
}
