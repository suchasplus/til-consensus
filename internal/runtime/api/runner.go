package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/suchasplus/til-consensus/internal/config"
	genai "google.golang.org/genai"
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

type ProviderResponseError struct {
	Message            string
	RawResponse        string
	FinishReason       string
	NativeFinishReason string
	Refusal            string
}

func (e *ProviderResponseError) Error() string {
	parts := []string{e.Message}
	if e.FinishReason != "" {
		parts = append(parts, "finish_reason="+e.FinishReason)
	}
	if e.NativeFinishReason != "" {
		parts = append(parts, "native_finish_reason="+e.NativeFinishReason)
	}
	if e.Refusal != "" {
		parts = append(parts, "refusal="+e.Refusal)
	}
	return strings.Join(parts, ": ")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func NewRunner(provider config.ProviderConfig) *Runner {
	return &Runner{provider: provider}
}

func PreviewRequestContext(
	provider config.ProviderConfig,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	reasoning string,
	maxOutputTokens int,
	schema map[string]any,
) (map[string]any, error) {
	r := Runner{provider: provider}
	timeout := r.optionDuration("timeout_ms", 60*time.Second)
	out := map[string]any{
		"method":     http.MethodPost,
		"model":      providerModel,
		"timeout":    timeout.String(),
		"timeoutMs":  timeout.Milliseconds(),
		"system":     requestPreviewText(systemPrompt, 160),
		"prompt":     requestPreviewText(prompt, 220),
		"headers":    sortedHeaderKeys(provider.Headers),
		"extraBody":  flattenMapKeys(r.optionMap("extra_body")),
		"schema":     schemaPreview(schema),
		"generation": map[string]any{},
		"auth":       "",
		"transport":  "",
		"endpoint":   "",
	}
	generation := out["generation"].(map[string]any)
	if temperature != nil {
		generation["temperature"] = *temperature
	}
	if maxOutputTokens > 0 {
		generation["maxOutputTokens"] = maxOutputTokens
	}
	switch provider.Protocol {
	case config.APIProtocolGemini:
		endpointPath := r.optionString("endpoint_path", "/models/{model}:generateContent")
		if err := validateGeminiEndpointPath(endpointPath); err != nil {
			return nil, err
		}
		apiVersionHint := firstNonEmpty(
			r.optionString("api_version", ""),
			geminiAPIVersionFromEndpointPath(endpointPath),
		)
		baseURL, apiVersion, err := geminiSDKEndpoint(provider.BaseURL, apiVersionHint)
		if err != nil {
			return nil, err
		}
		out["transport"] = "google.golang.org/genai Models.GenerateContent"
		out["endpoint"] = strings.TrimRight(baseURL, "/") + "/" + apiVersion + "/models/" + providerModel + ":generateContent"
		out["auth"] = requestAuthSummary(provider, "x-goog-api-key", "")
		out["apiVersion"] = apiVersion
		if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
			generation["responseMimeType"] = r.optionString("response_mime_type", "application/json")
			switch field := r.optionString("response_schema_field", "response_json_schema"); field {
			case "", "-", "none":
			case "response_json_schema", "responseJsonSchema":
				generation["responseJsonSchema"] = "enabled"
			default:
				return nil, fmt.Errorf("gemini sdk runner supports response_schema_field=response_json_schema only, got %q", field)
			}
		}
		if strings.TrimSpace(reasoning) != "" {
			generation["reasoning"] = strings.TrimSpace(reasoning)
			if geminiExtraBodyHasThinkingConfig(r.optionMap("extra_body")) {
				generation["thinkingLevel"] = "explicit extra_body.generationConfig.thinkingConfig"
			} else {
				level, err := geminiThinkingLevel(reasoning)
				if err != nil {
					return nil, err
				}
				generation["thinkingLevel"] = string(level)
			}
		}
	case config.APIProtocolAnthropicCompatible:
		baseURL := strings.TrimRight(firstNonEmpty(provider.BaseURL, "https://api.anthropic.com/v1"), "/")
		out["transport"] = "raw HTTP anthropic-compatible messages"
		out["endpoint"] = buildEndpointURL(baseURL, r.optionString("endpoint_path", "/messages"), providerModel)
		out["auth"] = requestAuthSummary(provider, "x-api-key", "")
		generation["maxTokensField"] = "max_tokens"
		if maxOutputTokens <= 0 {
			generation["maxOutputTokens"] = 1024
		}
	case config.APIProtocolOpenAIResponses:
		endpointPath := r.optionString("endpoint_path", "/responses")
		if err := validateOpenAIResponsesEndpointPath(endpointPath); err != nil {
			return nil, err
		}
		baseURL := strings.TrimRight(firstNonEmpty(provider.BaseURL, "https://api.openai.com/v1"), "/")
		out["transport"] = "github.com/openai/openai-go/v3 Responses.New"
		out["endpoint"] = strings.TrimRight(baseURL, "/") + "/responses"
		out["auth"] = requestAuthSummary(provider, "Authorization", "Bearer ")
		generation["maxOutputTokensField"] = "max_output_tokens"
		if strings.TrimSpace(reasoning) != "" {
			generation["reasoningField"] = "reasoning.effort"
			generation["reasoning"] = strings.TrimSpace(reasoning)
		}
		if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
			mode := r.optionString("structured_output_mode", "json_schema")
			if mode == "json_object" {
				generation["responseFormat"] = "text.format=json_object"
			} else {
				generation["responseFormat"] = "text.format=json_schema"
				generation["responseFormatName"] = r.optionString("response_format_name", "til_consensus_task_output")
			}
		}
	default:
		baseURL := strings.TrimRight(firstNonEmpty(provider.BaseURL, "https://api.openai.com/v1"), "/")
		out["transport"] = "raw HTTP openai-compatible chat/completions"
		out["endpoint"] = buildEndpointURL(baseURL, r.optionString("endpoint_path", "/chat/completions"), providerModel)
		out["auth"] = requestAuthSummary(provider, "Authorization", "Bearer ")
		if maxOutputTokens > 0 {
			field := r.optionString("max_output_tokens_field", "max_completion_tokens")
			if field != "-" && field != "" {
				generation["maxOutputTokensField"] = field
			}
		}
		if strings.TrimSpace(reasoning) != "" {
			field := r.optionString("reasoning_field", "reasoning_effort")
			if field != "-" && field != "" {
				generation["reasoningField"] = field
				generation["reasoning"] = strings.TrimSpace(reasoning)
			}
		}
		if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
			generation["responseFormat"] = "json_schema"
			generation["responseFormatName"] = r.optionString("response_format_name", "til_consensus_task_output")
		}
	}
	return out, nil
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
		return r.runGemini(ctx, prompt, systemPrompt, providerModel, temperature, reasoning, maxOutputTokens, schema)
	case config.APIProtocolOpenAIResponses:
		return r.runOpenAIResponses(ctx, prompt, systemPrompt, providerModel, temperature, reasoning, maxOutputTokens, schema)
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
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read openai-compatible response: %w", err)
	}
	var decoded struct {
		Choices []struct {
			FinishReason       string `json:"finish_reason"`
			NativeFinishReason string `json:"native_finish_reason"`
			Message            struct {
				Content *string `json:"content"`
				Refusal any     `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &decoded); err != nil {
		return "", fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", &ProviderResponseError{
			Message:     "openai-compatible response contains no choices",
			RawResponse: string(rawBody),
		}
	}
	choice := decoded.Choices[0]
	if choice.Message.Content == nil || strings.TrimSpace(*choice.Message.Content) == "" {
		return "", &ProviderResponseError{
			Message:            "openai-compatible response contains no message content",
			RawResponse:        string(rawBody),
			FinishReason:       choice.FinishReason,
			NativeFinishReason: choice.NativeFinishReason,
			Refusal:            stringifyResponseField(choice.Message.Refusal),
		}
	}
	return *choice.Message.Content, nil
}

func (r *Runner) runOpenAIResponses(
	ctx context.Context,
	prompt string,
	systemPrompt string,
	providerModel string,
	temperature *float64,
	reasoning string,
	maxOutputTokens int,
	schema map[string]any,
) (string, error) {
	if err := validateOpenAIResponsesEndpointPath(r.optionString("endpoint_path", "/responses")); err != nil {
		return "", err
	}
	timeout := r.optionDuration("timeout_ms", 60*time.Second)
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(firstNonEmpty(r.provider.BaseURL, "https://api.openai.com/v1"), "/")),
		option.WithHTTPClient(newHTTPClient(timeout)),
		option.WithRequestTimeout(timeout),
	}
	if apiKey, ok := resolveAPIKey(r.provider); ok {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	for key, value := range r.provider.Headers {
		opts = append(opts, option.WithHeader(key, value))
	}
	opts = append(opts, openAIResponsesExtraBodyOptions(r.optionMap("extra_body"))...)

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(providerModel),
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		params.Instructions = openai.String(systemPrompt)
	}
	if temperature != nil {
		params.Temperature = openai.Float(*temperature)
	}
	if maxOutputTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(maxOutputTokens))
	}
	if strings.TrimSpace(reasoning) != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(strings.TrimSpace(reasoning))}
	}
	if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
		params.Text = responses.ResponseTextConfigParam{
			Format: openAIResponsesTextFormat(r.optionString("structured_output_mode", "json_schema"), r.optionString("response_format_name", "til_consensus_task_output"), schema),
		}
	}

	client := openai.NewClient(opts...)
	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("openai-responses request failed: %w", err)
	}
	text := strings.TrimSpace(resp.OutputText())
	if text == "" {
		text = strings.TrimSpace(openAIResponsesOutputTextFallback(resp.RawJSON()))
	}
	if text == "" {
		return "", fmt.Errorf("openai-responses response contains no output text")
	}
	return text, nil
}

func openAIResponsesTextFormat(mode string, name string, schema map[string]any) responses.ResponseFormatTextConfigUnionParam {
	if mode == "json_object" {
		format := shared.NewResponseFormatJSONObjectParam()
		return responses.ResponseFormatTextConfigUnionParam{OfJSONObject: &format}
	}
	return responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
		Name:   name,
		Schema: schema,
		Strict: openai.Bool(true),
	}}
}

func openAIResponsesExtraBodyOptions(extra map[string]any) []option.RequestOption {
	if len(extra) == 0 {
		return nil
	}
	opts := []option.RequestOption{}
	var walk func(string, map[string]any)
	walk = func(prefix string, values map[string]any) {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if nested, ok := values[key].(map[string]any); ok {
				walk(path, nested)
				continue
			}
			opts = append(opts, option.WithJSONSet(path, values[key]))
		}
	}
	walk("", extra)
	return opts
}

func openAIResponsesOutputTextFallback(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var decoded struct {
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil && strings.TrimSpace(decoded.OutputText) != "" {
		return decoded.OutputText
	}
	return ""
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
	reasoning string,
	maxOutputTokens int,
	schema map[string]any,
) (string, error) {
	if err := validateGeminiEndpointPath(r.optionString("endpoint_path", "/models/{model}:generateContent")); err != nil {
		return "", err
	}
	apiVersionHint := firstNonEmpty(
		r.optionString("api_version", ""),
		geminiAPIVersionFromEndpointPath(r.optionString("endpoint_path", "")),
	)
	baseURL, apiVersion, err := geminiSDKEndpoint(r.provider.BaseURL, apiVersionHint)
	if err != nil {
		return "", err
	}
	apiKey, _ := resolveAPIKey(r.provider)
	timeout := r.optionDuration("timeout_ms", 60*time.Second)
	headers := http.Header{}
	for key, value := range r.provider.Headers {
		headers.Set(key, value)
	}
	httpClient := newHTTPClient(timeout)
	httpClient.Transport = geminiAPIKeyTransport(r.provider, http.DefaultTransport)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    baseURL,
			APIVersion: apiVersion,
			Headers:    headers,
			Timeout:    &timeout,
			ExtraBody:  r.optionMap("extra_body"),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create gemini client: %w", err)
	}
	generationConfig := &genai.GenerateContentConfig{}
	if strings.TrimSpace(systemPrompt) != "" {
		generationConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		}
	}
	if temperature != nil {
		value := float32(*temperature)
		generationConfig.Temperature = &value
	}
	if maxOutputTokens > 0 {
		if maxOutputTokens > int(^uint32(0)>>1) {
			return "", fmt.Errorf("gemini max_output_tokens exceeds int32: %d", maxOutputTokens)
		}
		generationConfig.MaxOutputTokens = int32(maxOutputTokens)
	}
	if len(schema) > 0 && r.optionString("structured_output_mode", "json_schema") != "none" {
		generationConfig.ResponseMIMEType = r.optionString("response_mime_type", "application/json")
		switch field := r.optionString("response_schema_field", "response_json_schema"); field {
		case "", "-", "none":
		case "response_json_schema", "responseJsonSchema":
			generationConfig.ResponseJsonSchema = schema
		default:
			return "", fmt.Errorf("gemini sdk runner supports response_schema_field=response_json_schema only, got %q", field)
		}
	}
	if strings.TrimSpace(reasoning) != "" && !geminiExtraBodyHasThinkingConfig(r.optionMap("extra_body")) {
		level, err := geminiThinkingLevel(reasoning)
		if err != nil {
			return "", err
		}
		generationConfig.ThinkingConfig = &genai.ThinkingConfig{ThinkingLevel: level}
	}
	contents := []*genai.Content{{
		Role: genai.RoleUser,
		Parts: []*genai.Part{{
			Text: prompt,
		}},
	}}
	resp, err := client.Models.GenerateContent(ctx, providerModel, contents, generationConfig)
	if err != nil {
		return "", fmt.Errorf("gemini generateContent failed: %w", err)
	}
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf(
			"gemini response contains no candidates%s",
			geminiSDKResponseDiagnostics(resp, ""),
		)
	}
	text := geminiSDKResponseText(resp)
	if text == "" {
		return "", fmt.Errorf(
			"gemini response contains no text parts%s",
			geminiSDKResponseDiagnostics(resp, string(resp.Candidates[0].FinishReason)),
		)
	}
	return text, nil
}

func geminiSDKResponseText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return ""
	}
	parts := make([]string, 0, len(resp.Candidates[0].Content.Parts))
	for _, part := range resp.Candidates[0].Content.Parts {
		if part != nil && !part.Thought && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func geminiSDKResponseDiagnostics(resp *genai.GenerateContentResponse, finishReason string) string {
	var blockReason string
	var promptTokenCount int
	var candidatesTokenCount int
	var thoughtsTokenCount int
	var totalTokenCount int
	if resp != nil {
		if resp.PromptFeedback != nil {
			blockReason = string(resp.PromptFeedback.BlockReason)
		}
		if resp.UsageMetadata != nil {
			promptTokenCount = int(resp.UsageMetadata.PromptTokenCount)
			candidatesTokenCount = int(resp.UsageMetadata.CandidatesTokenCount)
			thoughtsTokenCount = int(resp.UsageMetadata.ThoughtsTokenCount)
			totalTokenCount = int(resp.UsageMetadata.TotalTokenCount)
		}
	}
	return geminiResponseDiagnostics(
		finishReason,
		blockReason,
		promptTokenCount,
		candidatesTokenCount,
		thoughtsTokenCount,
		totalTokenCount,
	)
}

func geminiThinkingLevel(reasoning string) (genai.ThinkingLevel, error) {
	switch strings.ToLower(strings.TrimSpace(reasoning)) {
	case "minimal":
		return genai.ThinkingLevelMinimal, nil
	case "low":
		return genai.ThinkingLevelLow, nil
	case "medium":
		return genai.ThinkingLevelMedium, nil
	case "high":
		return genai.ThinkingLevelHigh, nil
	default:
		return "", fmt.Errorf("unsupported gemini reasoning level %q; allowed: minimal, low, medium, high", reasoning)
	}
}

func geminiExtraBodyHasThinkingConfig(extra map[string]any) bool {
	generationConfig, ok := extra["generationConfig"].(map[string]any)
	if !ok {
		return false
	}
	if _, ok := generationConfig["thinkingConfig"]; ok {
		return true
	}
	if _, ok := generationConfig["thinking_config"]; ok {
		return true
	}
	return false
}

func validateOpenAIResponsesEndpointPath(endpointPath string) error {
	trimmed := strings.Trim(strings.TrimSpace(endpointPath), "/")
	if trimmed == "" || trimmed == "responses" {
		return nil
	}
	return fmt.Errorf("openai-responses sdk runner does not support custom endpoint_path %q; put proxy prefixes in base_url instead", endpointPath)
}

func validateGeminiEndpointPath(endpointPath string) error {
	trimmed := strings.Trim(strings.TrimSpace(endpointPath), "/")
	if trimmed == "" || trimmed == "models/{model}:generateContent" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 3 && isGeminiAPIVersion(parts[0]) && parts[1] == "models" && parts[2] == "{model}:generateContent" {
		return nil
	}
	return fmt.Errorf("gemini sdk runner does not support custom endpoint_path %q; use base_url/options.api_version for API version overrides", endpointPath)
}

func geminiAPIVersionFromEndpointPath(endpointPath string) string {
	trimmed := strings.Trim(strings.TrimSpace(endpointPath), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 3 && isGeminiAPIVersion(parts[0]) && parts[1] == "models" && parts[2] == "{model}:generateContent" {
		return parts[0]
	}
	return ""
}

func geminiSDKEndpoint(rawBaseURL string, apiVersionHint string) (string, string, error) {
	baseURL := strings.TrimSpace(rawBaseURL)
	apiVersion := strings.Trim(strings.TrimSpace(apiVersionHint), "/")
	if baseURL == "" {
		return "https://generativelanguage.googleapis.com/", firstNonEmpty(apiVersion, "v1beta"), nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", "", fmt.Errorf("parse gemini base_url: %w", err)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	segments := splitURLPathSegments(parsed.Path)
	if apiVersion == "" && len(segments) > 0 && isGeminiAPIVersion(segments[len(segments)-1]) {
		apiVersion = segments[len(segments)-1]
		segments = segments[:len(segments)-1]
	}
	if apiVersion == "" {
		apiVersion = "v1beta"
	}
	if len(segments) == 0 {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + strings.Join(segments, "/")
	}
	return strings.TrimRight(parsed.String(), "/") + "/", apiVersion, nil
}

func splitURLPathSegments(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, segment := range raw {
		if segment != "" {
			out = append(out, segment)
		}
	}
	return out
}

func isGeminiAPIVersion(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	for i := 1; i < len(segment); i++ {
		if segment[i] >= '0' && segment[i] <= '9' {
			return true
		}
	}
	return false
}

func geminiAPIKeyTransport(provider config.ProviderConfig, base http.RoundTripper) http.RoundTripper {
	headerName := "x-goog-api-key"
	prefix := ""
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
	if base == nil {
		base = http.DefaultTransport
	}
	if strings.TrimSpace(headerName) == "x-goog-api-key" && prefix == "" && strings.TrimSpace(queryParam) == "" {
		return base
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		next := req.Clone(req.Context())
		next.Header = req.Header.Clone()
		key := next.Header.Get("x-goog-api-key")
		if key != "" {
			if strings.TrimSpace(queryParam) != "" && strings.TrimSpace(queryParam) != "-" {
				u := *next.URL
				query := u.Query()
				query.Set(queryParam, key)
				u.RawQuery = query.Encode()
				next.URL = &u
			}
			if strings.TrimSpace(headerName) == "" || strings.TrimSpace(headerName) == "-" {
				next.Header.Del("x-goog-api-key")
			} else {
				next.Header.Del("x-goog-api-key")
				next.Header.Set(headerName, prefix+key)
			}
		}
		return base.RoundTrip(next)
	})
}

func geminiResponseDiagnostics(
	finishReason string,
	blockReason string,
	promptTokenCount int,
	candidatesTokenCount int,
	thoughtsTokenCount int,
	totalTokenCount int,
) string {
	details := []string{}
	if strings.TrimSpace(finishReason) != "" {
		details = append(details, "finishReason="+strings.TrimSpace(finishReason))
	}
	if strings.TrimSpace(blockReason) != "" {
		details = append(details, "promptBlockReason="+strings.TrimSpace(blockReason))
	}
	if promptTokenCount > 0 || candidatesTokenCount > 0 || thoughtsTokenCount > 0 || totalTokenCount > 0 {
		details = append(
			details,
			fmt.Sprintf("promptTokenCount=%d", promptTokenCount),
			fmt.Sprintf("candidatesTokenCount=%d", candidatesTokenCount),
			fmt.Sprintf("thoughtsTokenCount=%d", thoughtsTokenCount),
			fmt.Sprintf("totalTokenCount=%d", totalTokenCount),
		)
	}
	if len(details) == 0 {
		return ""
	}
	return " " + strings.Join(details, " ")
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

func requestAuthSummary(provider config.ProviderConfig, defaultHeader string, defaultPrefix string) string {
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		return "not configured"
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
		return fmt.Sprintf("query %s <- %s", strings.TrimSpace(queryParam), provider.APIKeyEnv)
	}
	if strings.TrimSpace(headerName) == "" || strings.TrimSpace(headerName) == "-" {
		return "disabled"
	}
	if prefix != "" {
		return fmt.Sprintf("header %s: %s$%s", strings.TrimSpace(headerName), prefix, provider.APIKeyEnv)
	}
	return fmt.Sprintf("header %s <- %s", strings.TrimSpace(headerName), provider.APIKeyEnv)
}

func requestPreviewText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	if limit <= 1 {
		return trimmed[:limit]
	}
	return trimmed[:limit-1] + "…"
}

func schemaPreview(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	out := map[string]any{"enabled": true}
	if schemaType, ok := schema["type"].(string); ok && strings.TrimSpace(schemaType) != "" {
		out["type"] = schemaType
	}
	if additional, ok := schema["additionalProperties"].(bool); ok {
		out["additionalProperties"] = additional
	}
	switch required := schema["required"].(type) {
	case []string:
		out["required"] = append([]string(nil), required...)
	case []any:
		values := make([]string, 0, len(required))
		for _, value := range required {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
		if len(values) > 0 {
			out["required"] = values
		}
	}
	return out
}

func sortedHeaderKeys(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func flattenMapKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	out := []string{}
	var walk func(prefix string, current map[string]any)
	walk = func(prefix string, current map[string]any) {
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			full := key
			if prefix != "" {
				full = prefix + "." + key
			}
			if nested, ok := current[key].(map[string]any); ok {
				walk(full, nested)
				continue
			}
			out = append(out, full)
		}
	}
	walk("", values)
	return out
}

func stringifyResponseField(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		body, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(body)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
