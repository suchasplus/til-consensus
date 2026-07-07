package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
)

func TestExecutorClassifyUsesProviderOnlyConfig(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"recommendation\":\"free_debate\",\"confidence\":0.86,\"summary\":\"需要多方比较方案取舍。\",\"why\":[\"问题是开放式架构取舍\",\"需要暴露不同立场\"],\"missingInformation\":[],\"estimatedModeAfterClarification\":\"\",\"estimatedModeReason\":\"\",\"suggestedTask\":\"比较 monorepo 和 polyrepo 在当前微服务团队中的取舍。\"}"}}]}`))
	}))
	defer server.Close()
	t.Setenv("TEST_CLASSIFY_KEY", "test-key")

	loaded := config.LoadedConfig{
		Config: config.Normalize(config.Config{
			SchemaVersion: 1,
			Providers: map[string]config.ProviderConfig{
				"gemini-api": {
					Type:      config.ProviderTypeAPI,
					Protocol:  config.APIProtocolOpenAICompatible,
					BaseURL:   server.URL,
					APIKeyEnv: "TEST_CLASSIFY_KEY",
					Models: map[string]config.ProviderModelConfig{
						"default": {
							ProviderModel:   "fake-classifier",
							MaxOutputTokens: 512,
						},
					},
				},
			},
		}),
	}
	executor := NewExecutor(loaded)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := executor.Classify(ctx, ClassifyInput{
		Task: "monorepo 和 polyrepo 如何取舍？",
	})
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}
	if result.Recommendation != "free_debate" || result.Confidence != 0.86 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Raw == "" {
		t.Fatal("expected raw classify response")
	}
	if requestBody["response_format"] == nil {
		t.Fatalf("expected json_schema response_format in request: %#v", requestBody)
	}
}

func TestDecodeClassifyOutputRejectsInvalidRecommendation(t *testing.T) {
	_, err := DecodeClassifyOutput(`{"recommendation":"other","confidence":0.5,"summary":"x","why":["x"],"missingInformation":[],"estimatedModeAfterClarification":"","estimatedModeReason":"","suggestedTask":"x"}`)
	if err == nil {
		t.Fatal("expected invalid recommendation to fail")
	}
}

func TestClassifyMaxOutputTokensCapsLargeModelBudget(t *testing.T) {
	if got := classifyMaxOutputTokens(config.ProviderModelConfig{MaxOutputTokens: 65536}); got != 32768 {
		t.Fatalf("expected classify cap 32768, got %d", got)
	}
	if got := classifyMaxOutputTokens(config.ProviderModelConfig{MaxOutputTokens: 512}); got != 512 {
		t.Fatalf("expected classify to respect smaller model budget, got %d", got)
	}
}
