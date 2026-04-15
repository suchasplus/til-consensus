package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
)

var newHTTPClient = func() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func RunTask(ctx context.Context, prompt string, systemPrompt string, provider config.ProviderConfig, providerModel string, schema map[string]any) (string, error) {
	baseURL := provider.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := os.Getenv(provider.APIKeyEnv)
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("openai api key env %s is not set", provider.APIKeyEnv)
	}
	model := providerModel
	if model == "" {
		return "", fmt.Errorf("provider model is required")
	}
	requestBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if systemPrompt != "" {
		requestBody["messages"] = []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		}
	}
	requestBody["response_format"] = map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "til_consensus_task_output",
			"strict": true,
			"schema": schema,
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := newHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai request failed: status=%d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response contains no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
