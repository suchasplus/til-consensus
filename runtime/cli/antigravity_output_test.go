package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readAntigravityFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "agy", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// 所有成功数据来自 2026-10-20 本机 agy 1.4.2 的真实项目提取请求。
// provenance.json 记录原始 SHA-256 与脱敏规则；没有伪造模型成功输出。
// num_turns 包含内部 schema 修复，不代表用户请求数；独立任务由调用方
// 保证全新进程、单个 stdin user、无 resume，解析器只接收唯一最终 result。
func TestNormalizeAntigravityCLIOutputRealOneShotSuccess(t *testing.T) {
	for _, fixture := range []string{"stream-schema-success.jsonl", "native-schema-result.json"} {
		t.Run(fixture, func(t *testing.T) {
			output, err := normalizeAntigravityCLIOutput(readAntigravityFixture(t, fixture))
			if err != nil {
				t.Fatal(err)
			}
			var facts map[string]json.RawMessage
			if err := json.Unmarshal([]byte(output), &facts); err != nil {
				t.Fatalf("不是项目 JSON 对象: %v", err)
			}
			for key, want := range map[string]string{
				"project_name":      "newtc",
				"electron_version":  "44.9.0",
				"react_version":     "19.3.1",
				"go_version":        "1.26.1",
				"consensus_version": "v0.0.0-20260803125524-b505773800a4",
			} {
				var got string
				if err := json.Unmarshal(facts[key], &got); err != nil || got != want {
					t.Errorf("%s = %s，期望 %q", key, facts[key], want)
				}
			}
			var scripts []string
			if err := json.Unmarshal(facts["scripts"], &scripts); err != nil {
				t.Fatal(err)
			}
			slices.Sort(scripts)
			wantScripts := []string{"build", "build:host", "check", "dev", "start", "test", "test:e2e", "test:host", "test:rpc"}
			if !slices.Equal(scripts, wantScripts) {
				t.Errorf("scripts = %v，期望 %v", scripts, wantScripts)
			}
			// 真实 response 含这些 CLI 附加字段，structured_output 已按 schema 清理。
			if _, exists := facts["toolAction"]; exists {
				t.Error("错误选择了 response，应该优先读取 structured_output")
			}
			if _, exists := facts["toolSummary"]; exists {
				t.Error("CLI 元数据不应混入业务对象")
			}
		})
	}
}

func TestNormalizeAntigravityCLIOutputRealSIGINTCancellation(t *testing.T) {
	output, err := normalizeAntigravityCLIOutput(readAntigravityFixture(t, "stream-cancel.jsonl"))
	if output != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("output=%q error=%v，期望取消且无成功输出", output, err)
	}
	var resultErr *AntigravityResultError
	if !errors.As(err, &resultErr) || resultErr.Status != "ERROR" || resultErr.Message != "interrupted" {
		t.Fatalf("丢失真实取消终态: %#v", err)
	}
}

func TestNormalizeAntigravityCLIOutputRealInternalSchemaRepairTurns(t *testing.T) {
	// 从真实捕获中只提取首个任务的完整输出片段；内部 schema 修复已产生
	// num_turns=2 和两个 user_input step，但这一任务只有一个 result。
	// 整个多任务会话仍由下一项真实回归测试明确拒绝。
	raw := readAntigravityFixture(t, "stream-schema-multiturn-stale.jsonl")
	var firstTask []string
	var result json.RawMessage
	internalInputs := 0
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		firstTask = append(firstTask, line)
		var event struct {
			Event      string          `json:"event"`
			Result     json.RawMessage `json:"result"`
			StepUpdate struct {
				StepType string `json:"step_type"`
			} `json:"step_update"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.StepUpdate.StepType == "user_input" {
			internalInputs++
		}
		if event.Event == "result" {
			result = event.Result
			break
		}
	}
	var metadata struct {
		NumTurns int `json:"num_turns"`
	}
	if err := json.Unmarshal(result, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.NumTurns <= 1 || internalInputs <= 1 {
		t.Fatal("真实 fixture 必须覆盖 schema 内部修复计数，不能假设与外部请求数相等")
	}
	for name, input := range map[string]string{
		"stream_first_task": strings.Join(firstTask, "\n") + "\n",
		"native_result":     string(result),
	} {
		t.Run(name, func(t *testing.T) {
			output, err := normalizeAntigravityCLIOutput(input)
			if err != nil {
				t.Fatalf("内部 schema 修复轮次不应导致拒绝: %v", err)
			}
			var facts struct {
				ProjectName string `json:"project_name"`
			}
			if err := json.Unmarshal([]byte(output), &facts); err != nil || facts.ProjectName != "newtc" {
				t.Fatalf("真实首任务结果错误: output=%q err=%v", output, err)
			}
		})
	}
}

func TestNormalizeAntigravityCLIOutputRejectsRealStaleMultiTurn(t *testing.T) {
	raw := readAntigravityFixture(t, "stream-schema-multiturn-stale.jsonl")
	// 确认回放确实包含两次真实 result，且第二次结构化结果沿用了首轮 risk。
	var results []struct {
		Response         string `json:"response"`
		StructuredOutput struct {
			Risk string `json:"risk"`
		} `json:"structured_output"`
	}
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		var event struct {
			Event  string          `json:"event"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Event == "result" {
			var result struct {
				Response         string `json:"response"`
				StructuredOutput struct {
					Risk string `json:"risk"`
				} `json:"structured_output"`
			}
			if err := json.Unmarshal(event.Result, &result); err != nil {
				t.Fatal(err)
			}
			results = append(results, result)
		}
	}
	if len(results) != 2 {
		t.Fatalf("真实多轮 fixture 结果数 = %d，期望 2", len(results))
	}
	var secondResponse struct {
		Risk string `json:"risk"`
	}
	if err := json.Unmarshal([]byte(results[1].Response), &secondResponse); err != nil {
		t.Fatal(err)
	}
	if secondResponse.Risk == results[1].StructuredOutput.Risk || results[0].StructuredOutput.Risk != results[1].StructuredOutput.Risk {
		t.Fatal("fixture 没有保留真实的陈旧 structured_output 缺陷")
	}
	if output, err := normalizeAntigravityCLIOutput(raw); err == nil || output != "" {
		t.Fatalf("错误接受了可能陈旧的多轮结果: output=%q err=%v", output, err)
	}
}

func TestNormalizeAntigravityCLIOutputRejectsIncompleteAndFailedProtocol(t *testing.T) {
	native := readAntigravityFixture(t, "native-schema-result.json")
	stream := readAntigravityFixture(t, "stream-schema-success.jsonl")
	mutate := func(edit func(map[string]any)) string {
		t.Helper()
		var result map[string]any
		if err := json.Unmarshal([]byte(native), &result); err != nil {
			t.Fatal(err)
		}
		edit(result)
		body, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	lines := strings.Split(strings.TrimSpace(stream), "\n")
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"business_object_is_not_terminal", `{"project_name":"newtc"}`},
		{"only_init", lines[0]},
		{"partial_response_has_no_terminal_result", strings.Join(lines[:len(lines)-1], "\n")},
		{"truncated_jsonl", stream + `{"event":`},
		{"missing_event_type", "{}\n" + stream},
		{"missing_result_payload", `{"event":"result"}`},
		{"events_after_terminal", stream + lines[0] + "\n"},
		{"multiple_results", stream + stream},
		{"missing_status", mutate(func(e map[string]any) { delete(e, "status") })},
		{"missing_num_turns", mutate(func(e map[string]any) { delete(e, "num_turns") })},
		{"zero_num_turns", mutate(func(e map[string]any) { e["num_turns"] = 0 })},
		{"negative_num_turns", mutate(func(e map[string]any) { e["num_turns"] = -1 })},
		{"waiting_not_success", mutate(func(e map[string]any) { e["status"] = "WAITING" })},
		{"running_not_success", mutate(func(e map[string]any) { e["status"] = "RUNNING" })},
		{"unknown_status", mutate(func(e map[string]any) { e["status"] = "future_status" })},
		{"error_overrides_structured_output", mutate(func(e map[string]any) { e["error"] = "permission denied" })},
		{"schema_cannot_fall_back_to_response", mutate(func(e map[string]any) { delete(e, "structured_output") })},
		{"null_schema_output", mutate(func(e map[string]any) { e["structured_output"] = nil })},
		{"empty_success", mutate(func(e map[string]any) {
			delete(e, "structured_output")
			delete(e, "json_schema")
			e["response"] = " "
		})},
		{"multiple_external_user_events", lines[0] + "\n" + `{"event":"user"}` + "\n" + `{"event":"user"}` + "\n" + strings.Join(lines[1:], "\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := normalizeAntigravityCLIOutput(test.raw)
			if err == nil || output != "" {
				t.Fatalf("接受了不完整或失败协议: output=%q error=%v", output, err)
			}
		})
	}
}

func TestNormalizeAntigravityCLIOutputCancellationCannotBeHiddenBySuccess(t *testing.T) {
	raw := readAntigravityFixture(t, "stream-cancel.jsonl") + readAntigravityFixture(t, "stream-schema-success.jsonl")
	if output, err := normalizeAntigravityCLIOutput(raw); output != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("取消被后续成功覆盖: output=%q error=%v", output, err)
	}
}

func TestNormalizeAntigravityCLIOutputTextWithoutSchema(t *testing.T) {
	// 从真实成功终态中移除 schema 字段，验证非 schema 调用的兼容路径。
	var result map[string]any
	if err := json.Unmarshal([]byte(readAntigravityFixture(t, "native-schema-result.json")), &result); err != nil {
		t.Fatal(err)
	}
	delete(result, "structured_output")
	delete(result, "json_schema")
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	output, err := normalizeAntigravityCLIOutput(string(body))
	if err != nil || output != result["response"] {
		t.Fatalf("非 schema 响应回退错误: output=%q error=%v", output, err)
	}
}

func TestAntigravityResultErrorCancellationStatuses(t *testing.T) {
	for _, status := range []string{"CANCELED", "CANCELLED", "INTERRUPTED"} {
		if !errors.Is(&AntigravityResultError{Status: status}, context.Canceled) {
			t.Errorf("%s 应暴露 context.Canceled", status)
		}
	}
	if errors.Is(&AntigravityResultError{Status: "ERROR", Message: "permission denied"}, context.Canceled) {
		t.Error("普通执行错误不应伪装成取消")
	}
}
