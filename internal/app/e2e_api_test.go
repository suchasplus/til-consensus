package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestE2EFreeDebateAPIProvidersSmoke(t *testing.T) {
	openAIHits := 0
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected openai-compatible path: %s", r.URL.Path)
		}
		openAIHits++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode openai-compatible request: %v", err)
		}
		if _, ok := body["response_format"].(map[string]any); !ok {
			t.Fatalf("expected response_format json_schema in openai-compatible request, got %#v", body)
		}
		prompt := extractUserPrompt(t, body["messages"])
		response := freeDebateTestResponse(prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": response},
			}},
		})
	}))
	defer openAIServer.Close()

	openAIResponsesHits := 0
	openAIResponsesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected openai-responses path: %s", r.URL.Path)
		}
		openAIResponsesHits++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode openai-responses request: %v", err)
		}
		if _, ok := body["text"].(map[string]any); !ok {
			t.Fatalf("expected text.format json_schema in openai-responses request, got %#v", body)
		}
		prompt := extractResponsesPrompt(t, body["input"])
		response := freeDebateTestResponse(prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "resp_test",
			"object":     "response",
			"created_at": 0,
			"model":      "responses-test",
			"status":     "completed",
			"output": []map[string]any{{
				"type":   "message",
				"id":     "msg_test",
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]any{{
					"type":        "output_text",
					"text":        response,
					"annotations": []any{},
				}},
			}},
		})
	}))
	defer openAIResponsesServer.Close()

	anthropicHits := 0
	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected anthropic-compatible path: %s", r.URL.Path)
		}
		anthropicHits++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode anthropic-compatible request: %v", err)
		}
		prompt := extractAnthropicPrompt(t, body["messages"])
		response := freeDebateTestResponse(prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": response},
			},
		})
	}))
	defer anthropicServer.Close()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	inputPath := filepath.Join(tmp, "free-debate.run.yaml")
	writeE2EAPIConfig(t, configPath, filepath.Join(tmp, "out", "{requestId}"), openAIServer.URL, openAIResponsesServer.URL, anthropicServer.URL)
	writeFile(t, inputPath, `request_id: debate-api-smoke-001
mode: free_debate
task_spec:
  goal: 评估未来 12 个月内，当前微服务体系更适合维持 polyrepo，还是逐步收敛到 monorepo
  task_type: strategy
  success_criteria:
    - 必须明确主要支持理由和主要反对理由
    - 必须保留尚未解决的分歧点
debate_policy:
  min_rounds: 2
  max_rounds: 2
  vote_threshold: 0.67
  enable_early_stop: true
  peer_context_mode: summary+active_claims
`)

	runCmd := newRunCommand()
	runStdout, runStderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--input", inputPath})
	if err != nil {
		t.Fatalf("free_debate api run failed: %v\nstderr=%s", err, runStderr)
	}
	resultPath := extractResultPath(t, runStdout)
	result := loadRunResult(t, resultPath)
	if result.Mode != consensus.WorkflowModeFreeDebate || result.FreeDebate == nil {
		t.Fatalf("expected free_debate result, got %#v", result)
	}
	if len(result.FreeDebate.Rounds) < 2 {
		t.Fatalf("expected at least initial + debate rounds, got %#v", result.FreeDebate.Rounds)
	}
	if len(result.FreeDebate.Claims) == 0 {
		t.Fatalf("expected debate claims, got %#v", result.FreeDebate)
	}
	if len(result.FreeDebate.Votes) == 0 {
		t.Fatalf("expected final votes, got %#v", result.FreeDebate)
	}
	if openAIHits == 0 || openAIResponsesHits == 0 || anthropicHits == 0 {
		t.Fatalf("expected all api providers to be exercised, openai=%d openai-responses=%d anthropic=%d", openAIHits, openAIResponsesHits, anthropicHits)
	}

	viewCmd := newViewCommand()
	viewStdout, viewStderr, err := runCLICommand(context.Background(), viewCmd, []string{"view", "--result", resultPath, "--section", "rounds", "--section", "votes"})
	if err != nil {
		t.Fatalf("free_debate api view failed: %v\nstderr=%s", err, viewStderr)
	}
	for _, fragment := range []string{"Rounds", "Votes", "participant-anthropic | accept"} {
		if !strings.Contains(viewStdout, fragment) {
			t.Fatalf("expected %q in view output\n%s", fragment, viewStdout)
		}
	}
}

func writeE2EAPIConfig(t *testing.T, path string, outputDir string, openAIBaseURL string, openAIResponsesBaseURL string, anthropicBaseURL string) {
	t.Helper()
	cfg := config.Normalize(config.Config{
		SchemaVersion: 1,
		Defaults: config.DefaultsConfig{
			Mode:              consensus.WorkflowModeFreeDebate,
			PerTaskTimeout:    config.Duration{},
			TaskRetryAttempts: 0,
			ProposalPolicy: config.ProposalPolicyConfig{
				MaxPasses:          1,
				MaxClaimsPerWorker: 1,
			},
			DebatePolicy: config.DebatePolicyConfig{
				MinRounds:       2,
				MaxRounds:       2,
				VoteThreshold:   0.67,
				EnableEarlyStop: true,
				PeerContextMode: "summary+active_claims",
			},
		},
		Output: config.OutputConfig{
			Directory: outputDir,
		},
		Providers: map[string]config.ProviderConfig{
			"openai-test": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAICompatible,
				BaseURL:  openAIBaseURL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "gpt-test"},
				},
			},
			"anthropic-test": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolAnthropicCompatible,
				BaseURL:  anthropicBaseURL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "claude-test"},
				},
			},
			"openai-responses-test": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAIResponses,
				BaseURL:  openAIResponsesBaseURL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "responses-test"},
				},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "participant-openai-a", Provider: "openai-test", Model: "default", Role: "participant"},
			{ID: "participant-anthropic", Provider: "anthropic-test", Model: "default", Role: "participant"},
			{ID: "participant-openai-b", Provider: "openai-responses-test", Model: "default", Role: "participant"},
			{ID: "reporter-openai", Provider: "openai-test", Model: "default", Role: "reporter"},
		},
		Roles: config.RolesConfig{
			Participants: []string{"participant-openai-a", "participant-anthropic", "participant-openai-b"},
			Reporter:     "reporter-openai",
		},
	})
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("write e2e api config failed: %v", err)
	}
}

func extractUserPrompt(t *testing.T, raw any) string {
	t.Helper()
	messages, ok := raw.([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("unexpected openai-compatible messages payload: %#v", raw)
	}
	last, ok := messages[len(messages)-1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected openai-compatible message shape: %#v", messages[len(messages)-1])
	}
	content, ok := last["content"].(string)
	if !ok {
		t.Fatalf("unexpected openai-compatible content: %#v", last)
	}
	return content
}

func extractAnthropicPrompt(t *testing.T, raw any) string {
	t.Helper()
	messages, ok := raw.([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("unexpected anthropic-compatible messages payload: %#v", raw)
	}
	first, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected anthropic-compatible message shape: %#v", messages[0])
	}
	content, ok := first["content"].(string)
	if !ok {
		t.Fatalf("unexpected anthropic-compatible content: %#v", first)
	}
	return content
}

func extractResponsesPrompt(t *testing.T, raw any) string {
	t.Helper()
	content, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected openai-responses input payload: %#v", raw)
	}
	return content
}

func extractGeminiPrompt(t *testing.T, raw any) string {
	t.Helper()
	contents, ok := raw.([]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("unexpected gemini contents payload: %#v", raw)
	}
	first, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected gemini content shape: %#v", contents[0])
	}
	parts, ok := first["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("unexpected gemini parts payload: %#v", first)
	}
	part, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected gemini part shape: %#v", parts[0])
	}
	content, ok := part["text"].(string)
	if !ok {
		t.Fatalf("unexpected gemini text payload: %#v", part)
	}
	return content
}

func freeDebateTestResponse(prompt string) string {
	switch {
	case strings.Contains(prompt, `"maxClaims"`):
		return `{"summary":"提出一条主张，强调需要平衡共享改动成本与构建复杂度。","claims":[{"statement":"如果未来一年跨服务重构和共享库演进仍然频繁，monorepo 值得优先评估，但前提是先补齐增量构建与权限边界。","claimType":"recommendation","confidence":0.72}]}`
	case strings.Contains(prompt, `"selfClaims"`) || strings.Contains(prompt, `"peerClaims"`):
		judgements := make([]string, 0)
		for _, claimID := range extractClaimIDs(prompt) {
			judgements = append(judgements, fmt.Sprintf(`{"claimId":"%s","judgement":"agree","rationale":"当前轮次暂不引入新的反驳，先保留并继续比较边界条件。"}`, claimID))
		}
		if len(judgements) == 0 {
			return `{"summary":"本轮没有新增异议。","judgements":[]}`
		}
		return `{"summary":"本轮先保留现有主张，继续比较边界条件。","judgements":[` + strings.Join(judgements, ",") + `]}`
	case strings.Contains(prompt, `"votes"`) && strings.Contains(prompt, `"claimId"`):
		votes := make([]string, 0)
		for _, claimID := range extractClaimIDs(prompt) {
			votes = append(votes, fmt.Sprintf(`{"claimId":"%s","vote":"accept","rationale":"当前主张已覆盖主要约束，可以进入最终汇总。"}`, claimID))
		}
		if len(votes) == 0 {
			return `{"summary":"当前没有可投票主张。","votes":[]}`
		}
		return `{"summary":"对当前保留主张给出接受票。","votes":[` + strings.Join(votes, ",") + `]}`
	default:
		return `{"summary":"自由辩论已完成。当前共识倾向是：只有在补齐增量构建、缓存和权限边界后，monorepo 才值得推进；否则继续维持 polyrepo 并降低版本漂移更稳妥。"}`
	}
}

func extractClaimIDs(prompt string) []string {
	re := regexp.MustCompile(`"claimId"\s*:\s*"([^"]+)"`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		claimID := strings.TrimSpace(match[1])
		if claimID == "" {
			continue
		}
		if _, ok := seen[claimID]; ok {
			continue
		}
		seen[claimID] = struct{}{}
		out = append(out, claimID)
	}
	return out
}
