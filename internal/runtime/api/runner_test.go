package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
