package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
)

func TestOpenAICompatibleRunnerUsesOneShotMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages := body["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("expected one-shot system+user messages, got %#v", messages)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": `{"summary":"ok"}`},
			}},
		})
	}))
	defer server.Close()
	runner := NewRunner(config.ProviderConfig{
		Type:     config.ProviderTypeAPI,
		Protocol: config.APIProtocolOpenAICompatible,
		BaseURL:  server.URL,
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "gpt-test", nil, "", 0, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestOpenAICompatibleRunnerSupportsGatewayOptions(t *testing.T) {
	const apiKey = "openrouter-test-key"
	if err := os.Setenv("OPENROUTER_API_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("OPENROUTER_API_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("HTTP-Referer"); got != "https://example.com" {
			t.Fatalf("unexpected referer header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := body["max_tokens"]; got != float64(512) {
			t.Fatalf("unexpected max_tokens field: %#v", got)
		}
		if _, ok := body["max_completion_tokens"]; ok {
			t.Fatalf("expected max_completion_tokens to be omitted: %#v", body)
		}
		if _, ok := body["response_format"]; ok {
			t.Fatalf("expected response_format to be omitted in prompt-only mode: %#v", body)
		}
		metadata, ok := body["metadata"].(map[string]any)
		if !ok || metadata["source"] != "til-consensus" {
			t.Fatalf("unexpected extra body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": `{"summary":"ok"}`},
			}},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolOpenAICompatible,
		BaseURL:   server.URL,
		APIKeyEnv: "OPENROUTER_API_KEY",
		Headers: map[string]string{
			"HTTP-Referer": "https://example.com",
			"X-Title":      "til-consensus",
		},
		Options: map[string]any{
			"endpoint_path":           "/api/v1/chat/completions",
			"structured_output_mode":  "none",
			"max_output_tokens_field": "max_tokens",
			"extra_body": map[string]any{
				"metadata": map[string]any{"source": "til-consensus"},
			},
		},
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "openrouter/anthropic/claude-4-sonnet", nil, "", 512, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestOpenAIResponsesRunnerUsesSDKResponsesPayload(t *testing.T) {
	const apiKey = "bailian-test-key"
	if err := os.Setenv("BAILIAN_TEST_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("BAILIAN_TEST_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := body["model"]; got != "qwen3.7-max" {
			t.Fatalf("unexpected model: %#v", got)
		}
		if got := body["input"]; got != "prompt" {
			t.Fatalf("unexpected input: %#v", got)
		}
		if got := body["instructions"]; got != "system" {
			t.Fatalf("unexpected instructions: %#v", got)
		}
		if got := body["max_output_tokens"]; got != float64(4096) {
			t.Fatalf("unexpected max_output_tokens: %#v in %#v", got, body)
		}
		if _, ok := body["max_tokens"]; ok {
			t.Fatalf("responses payload must not use chat max_tokens: %#v", body)
		}
		if got := body["enable_thinking"]; got != true {
			t.Fatalf("expected enable_thinking extra body, got %#v", body)
		}
		textConfig, ok := body["text"].(map[string]any)
		if !ok {
			t.Fatalf("expected text config: %#v", body)
		}
		format, ok := textConfig["format"].(map[string]any)
		if !ok || format["type"] != "json_schema" || format["name"] != "til_consensus_task_output" || format["strict"] != true {
			t.Fatalf("unexpected text.format: %#v", textConfig)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "resp_test",
			"object":     "response",
			"created_at": 0,
			"model":      "qwen3.7-max",
			"status":     "completed",
			"output": []map[string]any{{
				"type":   "message",
				"id":     "msg_test",
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]any{{
					"type":        "output_text",
					"text":        `{"summary":"ok"}`,
					"annotations": []any{},
				}},
			}},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolOpenAIResponses,
		BaseURL:   server.URL,
		APIKeyEnv: "BAILIAN_TEST_KEY",
		Options: map[string]any{
			"extra_body": map[string]any{"enable_thinking": true},
		},
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "qwen3.7-max", nil, "", 4096, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestPreviewRequestContextOpenAIResponses(t *testing.T) {
	ctx, err := PreviewRequestContext(
		config.ProviderConfig{
			Type:      config.ProviderTypeAPI,
			Protocol:  config.APIProtocolOpenAIResponses,
			BaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKeyEnv: "BAILIAN_API_KEY",
			Options: map[string]any{
				"extra_body": map[string]any{"enable_thinking": true},
			},
		},
		"prompt",
		"system",
		"qwen3.7-max",
		nil,
		"",
		2048,
		map[string]any{"type": "object"},
	)
	if err != nil {
		t.Fatalf("PreviewRequestContext failed: %v", err)
	}
	if got := ctx["transport"]; got != "github.com/openai/openai-go/v3 Responses.New" {
		t.Fatalf("unexpected transport: %#v", got)
	}
	if got := ctx["endpoint"]; got != "https://dashscope.aliyuncs.com/compatible-mode/v1/responses" {
		t.Fatalf("unexpected endpoint: %#v", got)
	}
	generation := ctx["generation"].(map[string]any)
	if generation["maxOutputTokensField"] != "max_output_tokens" || generation["responseFormat"] != "text.format=json_schema" {
		t.Fatalf("unexpected generation preview: %#v", generation)
	}
}

func TestAnthropicCompatibleRunnerCollectsTextBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hello"},
				{"type": "text", "text": "world"},
			},
		})
	}))
	defer server.Close()
	runner := NewRunner(config.ProviderConfig{
		Type:     config.ProviderTypeAPI,
		Protocol: config.APIProtocolAnthropicCompatible,
		BaseURL:  server.URL,
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "claude-test", nil, "", 0, nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != "hello\nworld" {
		t.Fatalf("unexpected response: %q", text)
	}
}

func TestAnthropicCompatibleRunnerSupportsCustomHeadersAndBody(t *testing.T) {
	const apiKey = "anthropic-test-key"
	if err := os.Setenv("ANTHROPIC_GATEWAY_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("ANTHROPIC_GATEWAY_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Fatalf("unexpected x-api-key: %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic-version: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := body["top_p"]; got != 0.9 {
			t.Fatalf("unexpected extra body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hello"},
				{"type": "text", "text": "world"},
			},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolAnthropicCompatible,
		BaseURL:   server.URL,
		APIKeyEnv: "ANTHROPIC_GATEWAY_KEY",
		Options: map[string]any{
			"endpoint_path": "/gateway/messages",
			"extra_body": map[string]any{
				"top_p": 0.9,
			},
		},
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "claude-test", nil, "", 0, nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != "hello\nworld" {
		t.Fatalf("unexpected response: %q", text)
	}
}

func TestGeminiRunnerUsesGenerateContentAndResponseJSONSchema(t *testing.T) {
	const apiKey = "gemini-test-key"
	if err := os.Setenv("GEMINI_API_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("GEMINI_API_KEY")
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != apiKey {
			t.Fatalf("unexpected x-goog-api-key: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		generationConfig, ok := body["generationConfig"].(map[string]any)
		if !ok {
			t.Fatalf("expected generationConfig in request, got %#v", body)
		}
		if got := generationConfig["responseMimeType"]; got != "application/json" {
			t.Fatalf("unexpected responseMimeType: %#v", got)
		}
		if _, ok := generationConfig["responseJsonSchema"].(map[string]any); !ok {
			t.Fatalf("expected responseJsonSchema in request, got %#v", generationConfig)
		}
		if got := generationConfig["maxOutputTokens"]; got != float64(4096) {
			t.Fatalf("unexpected maxOutputTokens: %#v", generationConfig)
		}
		if _, ok := generationConfig["max_output_tokens"]; ok {
			t.Fatalf("gemini generationConfig must use maxOutputTokens, got snake_case payload: %#v", generationConfig)
		}
		if _, ok := generationConfig["response_mime_type"]; ok {
			t.Fatalf("gemini generationConfig must use responseMimeType, got snake_case payload: %#v", generationConfig)
		}
		thinkingConfig, ok := generationConfig["thinkingConfig"].(map[string]any)
		if !ok || thinkingConfig["thinkingLevel"] != "HIGH" {
			t.Fatalf("expected thinkingConfig.thinkingLevel=HIGH, got %#v", generationConfig)
		}
		if _, ok := body["systemInstruction"].(map[string]any); !ok {
			t.Fatalf("expected systemInstruction in request, got %#v", body)
		}
		contents, ok := body["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("unexpected contents payload: %#v", body["contents"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{
						{"text": `{"summary":"ok"}`},
					},
				},
			}},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolGemini,
		BaseURL:   server.URL,
		APIKeyEnv: "GEMINI_API_KEY",
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "gemini-test", nil, "high", 4096, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestPreviewRequestContextGeminiUsesSDKNames(t *testing.T) {
	temperature := 0.2
	ctx, err := PreviewRequestContext(
		config.ProviderConfig{
			Type:      config.ProviderTypeAPI,
			Protocol:  config.APIProtocolGemini,
			BaseURL:   "https://generativelanguage.googleapis.com/v1beta",
			APIKeyEnv: "GEMINI_API_KEY",
		},
		"Return exactly this JSON object: {\"ok\":true}",
		"Output only a JSON object. No markdown.",
		"gemini-3.1-pro-preview",
		&temperature,
		"high",
		2048,
		map[string]any{
			"type":                 "object",
			"required":             []any{"ok"},
			"additionalProperties": false,
		},
	)
	if err != nil {
		t.Fatalf("PreviewRequestContext failed: %v", err)
	}
	if got := ctx["transport"]; got != "google.golang.org/genai Models.GenerateContent" {
		t.Fatalf("unexpected transport: %#v", got)
	}
	if got := ctx["endpoint"]; got != "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-pro-preview:generateContent" {
		t.Fatalf("unexpected endpoint: %#v", got)
	}
	generation, ok := ctx["generation"].(map[string]any)
	if !ok {
		t.Fatalf("missing generation context: %#v", ctx)
	}
	for key, want := range map[string]any{
		"maxOutputTokens":    2048,
		"temperature":        0.2,
		"responseMimeType":   "application/json",
		"responseJsonSchema": "enabled",
		"thinkingLevel":      "HIGH",
		"reasoning":          "high",
	} {
		if got := generation[key]; got != want {
			t.Fatalf("generation[%s] = %#v, want %#v; all=%#v", key, got, want, generation)
		}
	}
	if _, ok := generation["max_output_tokens"]; ok {
		t.Fatalf("preview must use Gemini SDK camelCase names, got %#v", generation)
	}
}

func TestGeminiRunnerSupportsCustomEndpointAndQueryAuth(t *testing.T) {
	const apiKey = "gemini-query-key"
	if err := os.Setenv("GEMINI_QUERY_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("GEMINI_QUERY_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v42/models/gemini-custom:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != apiKey {
			t.Fatalf("unexpected query api key: %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "" {
			t.Fatalf("expected x-goog-api-key header to be omitted, got %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		generationConfig := body["generationConfig"].(map[string]any)
		if got := generationConfig["candidateCount"]; got != float64(1) {
			t.Fatalf("unexpected nested extra body: %#v", generationConfig)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{"text": `{"summary":"ok"}`}},
				},
			}},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolGemini,
		BaseURL:   server.URL,
		APIKeyEnv: "GEMINI_QUERY_KEY",
		Options: map[string]any{
			"endpoint_path":       "/v42/models/{model}:generateContent",
			"api_key_header":      "-",
			"api_key_query_param": "key",
			"extra_body": map[string]any{
				"generationConfig": map[string]any{"candidateCount": 1},
			},
		},
	})
	text, err := runner.RunTask(context.Background(), "prompt", "system", "gemini-custom", nil, "", 0, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestGeminiRunnerReportsNoTextDiagnostics(t *testing.T) {
	const apiKey = "gemini-diagnostic-key"
	if err := os.Setenv("GEMINI_DIAGNOSTIC_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("GEMINI_DIAGNOSTIC_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"finishReason": "MAX_TOKENS",
				"content": map[string]any{
					"parts": []map[string]any{},
				},
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":     12,
				"candidatesTokenCount": 0,
				"thoughtsTokenCount":   256,
				"totalTokenCount":      268,
			},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolGemini,
		BaseURL:   server.URL,
		APIKeyEnv: "GEMINI_DIAGNOSTIC_KEY",
	})
	_, err := runner.RunTask(context.Background(), "prompt", "system", "gemini-test", nil, "", 0, map[string]any{"type": "object"})
	if err == nil {
		t.Fatalf("expected no text diagnostic error")
	}
	message := err.Error()
	for _, needle := range []string{
		"gemini response contains no text parts",
		"finishReason=MAX_TOKENS",
		"thoughtsTokenCount=256",
		"totalTokenCount=268",
	} {
		if !strings.Contains(message, needle) {
			t.Fatalf("error missing %q: %s", needle, message)
		}
	}
}

func TestGeminiRunnerReportsPromptBlockDiagnostics(t *testing.T) {
	const apiKey = "gemini-block-key"
	if err := os.Setenv("GEMINI_BLOCK_KEY", apiKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() { _ = os.Unsetenv("GEMINI_BLOCK_KEY") }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"promptFeedback": map[string]any{
				"blockReason": "SAFETY",
			},
		})
	}))
	defer server.Close()

	runner := NewRunner(config.ProviderConfig{
		Type:      config.ProviderTypeAPI,
		Protocol:  config.APIProtocolGemini,
		BaseURL:   server.URL,
		APIKeyEnv: "GEMINI_BLOCK_KEY",
	})
	_, err := runner.RunTask(context.Background(), "prompt", "system", "gemini-test", nil, "", 0, map[string]any{"type": "object"})
	if err == nil {
		t.Fatalf("expected no candidates diagnostic error")
	}
	if !strings.Contains(err.Error(), "promptBlockReason=SAFETY") {
		t.Fatalf("expected prompt block reason in error, got %s", err)
	}
}
