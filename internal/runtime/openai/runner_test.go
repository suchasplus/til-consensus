package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
)

func TestRunTask(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	oldFactory := newHTTPClient
	t.Cleanup(func() { newHTTPClient = oldFactory })
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/chat/completions" {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != "gpt-test" {
					t.Fatalf("unexpected model: %#v", body["model"])
				}
				if _, ok := body["response_format"]; !ok {
					t.Fatal("expected response_format")
				}
				payload, _ := json.Marshal(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": `{"fullResponse":"ok","summary":"ok"}`}},
					},
				})
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(payload)),
					Header:     make(http.Header),
				}, nil
			}),
		}
	}
	got, err := RunTask(context.Background(), "prompt", "system", config.ProviderConfig{
		Type:      "openai",
		BaseURL:   "https://example.test/v1",
		APIKeyEnv: "OPENAI_API_KEY",
	}, "gpt-test", map[string]any{"type": "object"})
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected content")
	}
}

func TestRunTaskRequiresAPIKey(t *testing.T) {
	_ = os.Unsetenv("OPENAI_API_KEY")
	if _, err := RunTask(context.Background(), "prompt", "", config.ProviderConfig{
		Type:      "openai",
		BaseURL:   "http://example.com",
		APIKeyEnv: "OPENAI_API_KEY",
	}, "gpt-test", map[string]any{"type": "object"}); err == nil {
		t.Fatal("expected api key error")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
