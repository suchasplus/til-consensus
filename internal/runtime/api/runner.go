package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
)

var newHTTPClient = func(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &http.Client{Timeout: timeout}
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
	case config.APIProtocolGemini:
		return r.runGemini(ctx, prompt, systemPrompt, providerModel, temperature, maxOutputTokens, schema)
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
		if field := r.optionString("reasoning_field", "reasoning_effort"); field != "-" && field != "" {
			body[field] = reasoning
		}
	}
	if maxOutputTokens > 0 {
		if field := r.optionString("max_output_tokens_field", "max_completion_tokens"); field != "-" && field != "" {
			body[field] = maxOutputTokens
		}
	}
	if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   r.optionString("response_format_name", "til_consensus_task_output"),
				"strict": true,
				"schema": schema,
			},
		}
	}
	body = mergeAnyMap(body, r.optionMap("extra_body"))
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal openai-compatible request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildEndpointURL(baseURL, r.optionString("endpoint_path", "/chat/completions"), providerModel), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create openai-compatible request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyAPIKey(req, r.provider, "Authorization", "Bearer ")
	for key, value := range r.provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := newHTTPClient(r.optionDuration("timeout_ms", 60*time.Second)).Do(req)
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
	body = mergeAnyMap(body, r.optionMap("extra_body"))
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal anthropic-compatible request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildEndpointURL(baseURL, r.optionString("endpoint_path", "/messages"), providerModel), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create anthropic-compatible request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if version := r.optionString("anthropic_version", "2023-06-01"); version != "-" && version != "" {
		req.Header.Set("anthropic-version", version)
	}
	applyAPIKey(req, r.provider, "x-api-key", "")
	for key, value := range r.provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := newHTTPClient(r.optionDuration("timeout_ms", 60*time.Second)).Do(req)
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

func (r *Runner) runGemini(
	ctx context.Context,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	maxOutputTokens int,
	schema map[string]any,
) (string, error) {
	baseURL := strings.TrimRight(firstNonEmpty(r.provider.BaseURL, "https://generativelanguage.googleapis.com/v1beta"), "/")
	body := map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]any{{
				"text": prompt,
			}},
		}},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		body["system_instruction"] = map[string]any{
			"parts": []map[string]any{{
				"text": systemPrompt,
			}},
		}
	}
	generationConfig := map[string]any{}
	if temperature != nil {
		generationConfig["temperature"] = *temperature
	}
	if maxOutputTokens > 0 {
		generationConfig["max_output_tokens"] = maxOutputTokens
	}
	if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
		generationConfig["response_mime_type"] = r.optionString("response_mime_type", "application/json")
		if field := r.optionString("response_schema_field", "response_json_schema"); field != "-" && field != "" {
			generationConfig[field] = schema
		}
	}
	if len(generationConfig) > 0 {
		body["generationConfig"] = generationConfig
	}
	body = mergeAnyMap(body, r.optionMap("extra_body"))
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal gemini request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildEndpointURL(baseURL, r.optionString("endpoint_path", "/models/{model}:generateContent"), providerModel), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyAPIKey(req, r.provider, "x-goog-api-key", "")
	for key, value := range r.provider.Headers {
		req.Header.Set(key, value)
	}
	resp, err := newHTTPClient(r.optionDuration("timeout_ms", 60*time.Second)).Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", buildHTTPError("api", "gemini", resp)
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode gemini response: %w", err)
	}
	if len(decoded.Candidates) == 0 {
		return "", fmt.Errorf("gemini response contains no candidates")
	}
	parts := make([]string, 0, len(decoded.Candidates[0].Content.Parts))
	for _, part := range decoded.Candidates[0].Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return "", fmt.Errorf("gemini response contains no text parts")
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

func (r *Runner) optionString(key string, fallback string) string {
	if r.provider.Options == nil {
		return fallback
	}
	value, ok := r.provider.Options[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func (r *Runner) optionMap(key string) map[string]any {
	if r.provider.Options == nil {
		return nil
	}
	raw, ok := r.provider.Options[key]
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return cloneAnyMap(typed)
	}
	return nil
}

func (r *Runner) optionDuration(key string, fallback time.Duration) time.Duration {
	if r.provider.Options == nil {
		return fallback
	}
	raw, ok := r.provider.Options[key]
	if !ok || raw == nil {
		return fallback
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return fallback
		}
		if dur, err := time.ParseDuration(text); err == nil && dur > 0 {
			return dur
		}
		if millis, err := strconv.Atoi(text); err == nil && millis > 0 {
			return time.Duration(millis) * time.Millisecond
		}
	}
	return fallback
}

func applyAPIKey(req *http.Request, provider config.ProviderConfig, defaultHeader string, defaultPrefix string) {
	apiKey, ok := resolveAPIKey(provider)
	if !ok {
		return
	}
	headerName := defaultHeader
	prefix := defaultPrefix
	queryParam := ""
	if provider.Options != nil {
		if value, ok := provider.Options["api_key_header"].(string); ok {
			headerName = value
		}
		if value, ok := provider.Options["api_key_prefix"].(string); ok {
			prefix = value
		}
		if value, ok := provider.Options["api_key_query_param"].(string); ok {
			queryParam = value
		}
	}
	if strings.TrimSpace(queryParam) != "" && strings.TrimSpace(queryParam) != "-" {
		query := req.URL.Query()
		query.Set(queryParam, apiKey)
		req.URL.RawQuery = query.Encode()
	}
	if strings.TrimSpace(headerName) != "" && strings.TrimSpace(headerName) != "-" {
		req.Header.Set(headerName, prefix+apiKey)
	}
}

func buildEndpointURL(baseURL string, pathTemplate string, providerModel string) string {
	path := strings.TrimSpace(pathTemplate)
	if path == "" {
		return baseURL
	}
	path = strings.ReplaceAll(path, "{model}", providerModel)
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func mergeAnyMap(base map[string]any, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	out := cloneAnyMap(base)
	for key, value := range extra {
		if current, ok := out[key].(map[string]any); ok {
			if next, ok := value.(map[string]any); ok {
				out[key] = mergeAnyMap(current, next)
				continue
			}
		}
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if nested, ok := value.(map[string]any); ok {
			out[key] = cloneAnyMap(nested)
			continue
		}
		out[key] = value
	}
	return out
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
