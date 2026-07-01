package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyCommandUsesProviderOnlyConfigAndTextInput(t *testing.T) {
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
	t.Setenv("TEST_API_KEY", "test-key")
	configPath := writeClassifyTestConfig(t, server.URL)

	cmd := newClassifyCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	err := cmd.Run(context.Background(), []string{
		"classify",
		"--config", configPath,
		"--verbose",
		"monorepo 和 polyrepo 如何取舍？",
	})
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{
		"classify started",
		"provider: gemini-api/default",
		"classify completed recommendation=free_debate confidence=0.86",
		"command: til-consensus debate",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("output missing %q:\n%s", needle, output)
		}
	}
	if requestBody["response_format"] == nil {
		t.Fatalf("expected json_schema response_format in request: %#v", requestBody)
	}
}

func TestClassifyCommandReadsFileAndOutputsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"recommendation\":\"needs_clarification\",\"confidence\":0.74,\"summary\":\"问题缺少目标和约束。\",\"why\":[\"没有说明决策目标\",\"没有评价标准\"],\"missingInformation\":[\"团队规模\",\"成功标准\"],\"estimatedModeAfterClarification\":\"free_debate\",\"estimatedModeReason\":\"补齐约束后仍是多方案取舍，适合多参与者辩论和投票。\",\"suggestedTask\":\"请补充团队规模、约束和成功标准后再运行。\"}"}}]}`))
	}))
	defer server.Close()
	t.Setenv("TEST_API_KEY", "test-key")
	configPath := writeClassifyTestConfig(t, server.URL)
	taskPath := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(taskPath, []byte("帮我看看这个方案怎么样"), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	cmd := newClassifyCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	err := cmd.Run(context.Background(), []string{
		"classify",
		"--config", configPath,
		"--file", taskPath,
		"--format", "json",
	})
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	var decoded classifyOutput
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if decoded.Recommendation != "needs_clarification" || len(decoded.MissingInformation) != 2 || decoded.EstimatedModeAfterClarification != "free_debate" {
		t.Fatalf("unexpected output: %#v", decoded)
	}
}

func TestClassifyCommandReadsStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"recommendation\":\"adjudication\",\"confidence\":0.91,\"summary\":\"适合裁决 claim 是否成立。\",\"why\":[\"目标是判断结论是否成立\"],\"missingInformation\":[],\"estimatedModeAfterClarification\":\"\",\"estimatedModeReason\":\"\",\"suggestedTask\":\"判断该 patch 是否修复竞态问题。\"}"}}]}`))
	}))
	defer server.Close()
	t.Setenv("TEST_API_KEY", "test-key")
	configPath := writeClassifyTestConfig(t, server.URL)

	cmd := newClassifyCommand()
	cmd.Reader = strings.NewReader("判断该 patch 是否修复竞态问题")
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	err := cmd.Run(context.Background(), []string{
		"classify",
		"--config", configPath,
		"--stdin",
	})
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "recommendation=adjudication") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func writeClassifyTestConfig(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "providers.yaml")
	body := `schema_version: 1
providers:
  gemini-api:
    type: api
    protocol: openai-compatible
    base_url: ` + baseURL + `
    api_key_env: TEST_API_KEY
    models:
      default:
        provider_model: fake-classifier
        max_output_tokens: 512
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
