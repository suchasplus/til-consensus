package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// The fixture constants below are sanitized excerpts from real Claude Code
// 2.1.303 requests on 2026-10-20. Sources and original SHA-256 digests are saved
// in newtc/fixtures/cli/claude/provenance.json. Session identifiers, paths,
// plugins, thinking, and signatures were removed; no model response was invented.
func TestNormalizeClaudeCLIOutputRealSuccess(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"native_schema", realClaudeNativeSuccess},
		{"stream_complete", realClaudeStreamSuccess},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := normalizeClaudeCLIOutput(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			var facts map[string]any
			if err := json.Unmarshal([]byte(output), &facts); err != nil {
				t.Fatalf("output is not the project's JSON object: %v", err)
			}
			for key, want := range map[string]string{
				"project_name":      "newtc",
				"electron_version":  "44.9.0",
				"react_version":     "19.3.1",
				"go_version":        "1.26.1",
				"consensus_version": "v0.0.0-20260803125524-b505773800a4",
			} {
				if facts[key] != want {
					t.Errorf("%s = %v, want %s", key, facts[key], want)
				}
			}
		})
	}
}

func TestNormalizeClaudeCLIOutputRealCancellationWithZeroProcessExit(t *testing.T) {
	// The captured process exited 0 with an empty stderr. Its result envelope is
	// authoritative and says aborted_streaming rather than successful completion.
	output, err := normalizeClaudeCLIOutput(realClaudeStreamCancellation)
	if output != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("got output=%q error=%v; want canceled with no successful output", output, err)
	}
	var resultErr *ClaudeResultError
	if !errors.As(err, &resultErr) || resultErr.Subtype != "error_during_execution" || resultErr.TerminalReason != "aborted_streaming" {
		t.Fatalf("missing real protocol failure metadata: %#v", err)
	}
}

func TestNormalizeClaudeCLIOutputPrefersStructuredOutput(t *testing.T) {
	var event map[string]any
	if err := json.Unmarshal([]byte(realClaudeNativeSuccess), &event); err != nil {
		t.Fatal(err)
	}
	event["result"] = "text must not replace structured_output"
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	output, err := normalizeClaudeCLIOutput(string(body))
	if err != nil || !strings.Contains(output, `"project_name":"newtc"`) || strings.Contains(output, "text must not") {
		t.Fatalf("did not prefer structured output: output=%q err=%v", output, err)
	}
	event["structured_output"] = nil
	event["result"] = "fallback text"
	body, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	output, err = normalizeClaudeCLIOutput(string(body))
	if err != nil || output != "fallback text" {
		t.Fatalf("did not fall back to result: output=%q err=%v", output, err)
	}
}

func TestNormalizeClaudeCLIOutputRejectsFailureAndIncompleteProtocol(t *testing.T) {
	mutate := func(edit func(map[string]any)) string {
		t.Helper()
		var event map[string]any
		if err := json.Unmarshal([]byte(realClaudeNativeSuccess), &event); err != nil {
			t.Fatal(err)
		}
		edit(event)
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"business_object_has_no_result", `{"project_name":"newtc"}`},
		{"partial_stream", realClaudeStreamSuccess[:strings.LastIndex(strings.TrimSpace(realClaudeStreamSuccess), "\n")]},
		{"truncated_jsonl", realClaudeStreamSuccess + "{\"type\":"},
		{"failed_result_cannot_be_hidden_by_later_success", realClaudeStreamCancellation + realClaudeStreamSuccess},
		{"missing_is_error", mutate(func(e map[string]any) { delete(e, "is_error") })},
		{"missing_subtype", mutate(func(e map[string]any) { delete(e, "subtype") })},
		{"is_error_overrides_output", mutate(func(e map[string]any) { e["is_error"] = true })},
		{"error_subtype_overrides_output", mutate(func(e map[string]any) { e["subtype"] = "error_max_turns" })},
		{"unknown_subtype", mutate(func(e map[string]any) { e["subtype"] = "unknown_future_status" })},
		{"unknown_terminal_reason", mutate(func(e map[string]any) { e["terminal_reason"] = "pending" })},
		{"errors_overrides_output", mutate(func(e map[string]any) { e["errors"] = []string{"failure"} })},
		{"error_overrides_output", mutate(func(e map[string]any) { e["error"] = "failure" })},
		{"empty_success", mutate(func(e map[string]any) { e["structured_output"] = nil; e["result"] = "  " })},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := normalizeClaudeCLIOutput(test.raw)
			if err == nil || output != "" {
				t.Fatalf("accepted invalid terminal protocol: output=%q err=%v", output, err)
			}
		})
	}
}

func TestNormalizeClaudeCLIOutputOlderSuccessWithoutTerminalReason(t *testing.T) {
	var event map[string]any
	if err := json.Unmarshal([]byte(realClaudeNativeSuccess), &event); err != nil {
		t.Fatal(err)
	}
	delete(event, "terminal_reason")
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeClaudeCLIOutput(string(body)); err != nil {
		t.Fatalf("older terminal schema should remain compatible: %v", err)
	}
}

const realClaudeNativeSuccess = "{\n  \"type\": \"result\",\n  \"subtype\": \"success\",\n  \"is_error\": false,\n  \"terminal_reason\": \"completed\",\n  \"result\": \"{\\\"project_name\\\":\\\"newtc\\\",\\\"electron_version\\\":\\\"44.9.0\\\",\\\"react_version\\\":\\\"19.3.1\\\",\\\"go_version\\\":\\\"1.26.1\\\",\\\"consensus_version\\\":\\\"v0.0.0-20260803125524-b505773800a4\\\",\\\"scripts\\\":[\\\"dev\\\",\\\"start\\\",\\\"build\\\",\\\"build:host\\\",\\\"check\\\",\\\"test:host\\\",\\\"test:rpc\\\",\\\"test:e2e\\\",\\\"test\\\"],\\\"risk\\\":\\\"til-consensus 锁定的是未打标签的伪版本提交，且间接引入了 openai-go 和 genai 等 HTTP SDK，M0 接入真实 CLI 时可能误走直连 API 路径，或因上游提交变动导致 CLI 适配接口不兼容。\\\"}\",\n  \"structured_output\": {\n    \"project_name\": \"newtc\",\n    \"electron_version\": \"44.9.0\",\n    \"react_version\": \"19.3.1\",\n    \"go_version\": \"1.26.1\",\n    \"consensus_version\": \"v0.0.0-20260803125524-b505773800a4\",\n    \"scripts\": [\n      \"dev\",\n      \"start\",\n      \"build\",\n      \"build:host\",\n      \"check\",\n      \"test:host\",\n      \"test:rpc\",\n      \"test:e2e\",\n      \"test\"\n    ],\n    \"risk\": \"til-consensus 锁定的是未打标签的伪版本提交，且间接引入了 openai-go 和 genai 等 HTTP SDK，M0 接入真实 CLI 时可能误走直连 API 路径，或因上游提交变动导致 CLI 适配接口不兼容。\"\n  }\n}\n"

const realClaudeStreamSuccess = "{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"claude-opus-5-5\",\"tools\":[],\"mcp_servers\":[]}\n{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"{\"}}}\n{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"terminal_reason\":\"completed\",\"result\":\"{\\\"project_name\\\":\\\"newtc\\\",\\\"electron_version\\\":\\\"44.9.0\\\",\\\"react_version\\\":\\\"19.3.1\\\",\\\"go_version\\\":\\\"1.26.1\\\",\\\"consensus_version\\\":\\\"v0.0.0-20260803125524-b505773800a4\\\",\\\"scripts\\\":[\\\"dev\\\",\\\"start\\\",\\\"build\\\",\\\"build:host\\\",\\\"check\\\",\\\"test:host\\\",\\\"test:rpc\\\",\\\"test:e2e\\\",\\\"test\\\"],\\\"risk\\\":\\\"til-consensus 以未打 tag 的伪版本锁定某个提交，且间接依赖 openai-go 与 genai 等 HTTP SDK，说明共识层可能默认走 API 直连而非子进程 CLI；M0 接入真实 CLI 时，可能需要改动尚不稳定的上游接口，并另行处理 CLI 进程的启动、超时与输出解析。\\\"}\"}\n"

const realClaudeStreamCancellation = "{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"claude-opus-5-5\",\"tools\":[],\"mcp_servers\":[]}\n{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"{\"}}}\n{\"type\":\"result\",\"subtype\":\"error_during_execution\",\"is_error\":true,\"terminal_reason\":\"aborted_streaming\",\"errors\":[\"[ede_diagnostic] result_type=user last_content_type=n/a stop_reason=null\"]}\n"
