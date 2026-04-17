package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
)

var newHTTPClient = func() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

type Runner struct {
	provider config.ProviderConfig
}

func NewRunner(provider config.ProviderConfig) *Runner {
	return &Runner{provider: provider}
}

func (r *Runner) RunTask(
	ctx context.Context,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	reasoning string,
	maxOutputTokens int,
	schema map[string]any,
) (string, error) {
	switch r.provider.Protocol {
	case config.APIProtocolAnthropicCompatible:
		return r.runAnthropic(ctx, prompt, systemPrompt, providerModel, temperature, maxOutputTokens)
	default:
		return r.runOpenAI(ctx, prompt, systemPrompt, providerModel, temperature, reasoning, maxOutputTokens, schema)
	}
}

func (r *Runner) runOpenAI(
	ctx context.Context,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	reasoning string,
	maxOutputTokens int,
	schema map[string]any,
) (string, error) {
	baseURL := strings.TrimRight(firstNonEmpty(r.provider.BaseURL, "https://api.openai.com/v1"), "/")
	messages := []map[string]string{}
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})
	body := map[string]any{
		"model":    providerModel,
		"messages": messages,
	}
	if temperature != nil {
		body["temperature"] = *temperature
	}
	if reasoning != "" {
		body["reasoning_effort"] = reasoning
	}
	if maxOutputTokens > 0 {
		body["max_completion_tokens"] = maxOutputTokens
	}
	if len(schema) > 0 {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "til_consensus_task_output",
				"strict": true,
				"schema": schema,
			},
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal openai-compatible request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create openai-compatible request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey, ok := resolveAPIKey(r.provider); ok {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for key, value := range r.provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("openai-compatible request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", buildHTTPError("api", "openai-compatible", resp)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible response contains no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func (r *Runner) runAnthropic(
	ctx context.Context,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	maxOutputTokens int,
) (string, error) {
	baseURL := strings.TrimRight(firstNonEmpty(r.provider.BaseURL, "https://api.anthropic.com/v1"), "/")
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1024
	}
	body := map[string]any{
		"model":      providerModel,
		"max_tokens": maxOutputTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		body["system"] = systemPrompt
	}
	if temperature != nil {
		body["temperature"] = *temperature
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal anthropic-compatible request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create anthropic-compatible request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if apiKey, ok := resolveAPIKey(r.provider); ok {
		req.Header.Set("x-api-key", apiKey)
	}
	for key, value := range r.provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic-compatible request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", buildHTTPError("api", "anthropic-compatible", resp)
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode anthropic-compatible response: %w", err)
	}
	parts := make([]string, 0, len(decoded.Content))
	for _, block := range decoded.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return "", fmt.Errorf("anthropic-compatible response contains no text blocks")
	}
	return text, nil
}

func resolveAPIKey(provider config.ProviderConfig) (string, bool) {
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		return "", false
	}
	value := strings.TrimSpace(os.Getenv(provider.APIKeyEnv))
	return value, value != ""
}

func buildHTTPError(provider string, label string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("%s:%s request failed: status=%d", provider, label, resp.StatusCode)
	}
	return fmt.Errorf("%s:%s request failed: status=%d body=%s", provider, label, resp.StatusCode, text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
