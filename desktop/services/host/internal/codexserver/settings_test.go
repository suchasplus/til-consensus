package codexserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/runtime/cli"
)

// 所有协议测试均调用当前测试二进制的显式 fixture，不读取账号或调用模型。
func recordedHelperOptions(t *testing.T) (Options, string, string) {
	t.Helper()
	options := helperOptions(t)
	requestsPath := filepath.Join(options.Cwd, "requests.jsonl")
	pidPath := filepath.Join(options.Cwd, "pid")
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_REQUESTS="+requestsPath, "TCVIEW_CODEX_HELPER_PID="+pidPath)
	return options, requestsPath, pidPath
}

func readHelperRequests(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var requests []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return requests
}

func helperRequest(t *testing.T, requests []map[string]any, method string) map[string]any {
	t.Helper()
	for _, request := range requests {
		if request["method"] == method {
			return request
		}
	}
	t.Fatalf("缺少 %s 协议请求：%+v", method, requests)
	return nil
}

func assertHelperProcessExited(t *testing.T, pidPath string) {
	t.Helper()
	body, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("fixture 子进程 %d 仍存在：%v", pid, err)
	}
}

func TestCodexServerReasoningParametersAndReportedValue(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		requested string
		reported  string
	}{
		{name: "显式设置经两个协议入口传入", model: "unit-fixture", requested: "high", reported: "high"},
		{name: "请求值不能冒充未报告的实际值", model: "unit-fixture", requested: "low"},
		{name: "CLI 默认不传覆盖且记录真实报告", reported: "medium"},
		{name: "全部使用 CLI 默认且未报告"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			options, requestsPath, pidPath := recordedHelperOptions(t)
			options.Model, options.Reasoning = test.model, test.requested
			options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_REASONING="+test.reported)
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			result, err := Run(ctx, options, "仅测试协议参数", nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.Reasoning != test.reported {
				t.Fatalf("实际推理配置应来自 thread/start 响应，得到 %q，期望 %q", result.Reasoning, test.reported)
			}
			requests := readHelperRequests(t, requestsPath)
			thread := object(helperRequest(t, requests, "thread/start")["params"])
			turn := object(helperRequest(t, requests, "turn/start")["params"])
			if test.model == "" {
				if _, exists := thread["model"]; exists {
					t.Fatalf("CLI 默认模型不应传 model：%+v", thread)
				}
			} else if thread["model"] != test.model {
				t.Fatalf("模型未传入 thread/start：%+v", thread)
			}
			if test.requested == "" {
				if _, exists := thread["config"]; exists {
					t.Fatalf("CLI 默认不应传线程推理覆盖：%+v", thread)
				}
				if _, exists := turn["effort"]; exists {
					t.Fatalf("CLI 默认不应传 turn effort：%+v", turn)
				}
			} else {
				want := map[string]any{"model_reasoning_effort": test.requested}
				if !reflect.DeepEqual(thread["config"], want) || turn["effort"] != test.requested {
					t.Fatalf("推理配置须同时传入线程与 turn：thread=%+v turn=%+v", thread, turn)
				}
			}
			assertHelperProcessExited(t, pidPath)
		})
	}
}

func TestCodexServerRejectsReportedReasoningMismatchBeforeTurn(t *testing.T) {
	options, requestsPath, pidPath := recordedHelperOptions(t)
	options.Model, options.Reasoning = "unit-fixture", "high"
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_REASONING=low")
	var events []cli.CLIEvent
	options.OnEvent = func(event cli.CLIEvent) error { events = append(events, event); return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	result, err := Run(ctx, options, "不得发送的推理请求", nil)
	if err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("请求 high、报告 low 应失败：result=%+v error=%v", result, err)
	}
	if result.Reasoning != "low" || result.Output != "" {
		t.Fatalf("须保留已报告配置且无生成结果：%+v", result)
	}
	for _, request := range readHelperRequests(t, requestsPath) {
		if request["method"] == "turn/start" {
			t.Fatal("推理不一致时已启动 turn")
		}
	}
	for _, event := range events {
		if event.Type == "terminal" || event.Type == "text_delta" {
			t.Fatalf("拒绝配置后不应报告生成成功：%+v", events)
		}
	}
	assertHelperProcessExited(t, pidPath)
}

func TestCodexServerReroutingCannotCompleteSuccessfully(t *testing.T) {
	options, _, pidPath := recordedHelperOptions(t)
	options.Model = "unit-fixture"
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_SCENARIO=rerouted")
	completed := false
	options.OnEvent = func(event cli.CLIEvent) error {
		if event.Type == "terminal" && event.Status == "completed" {
			completed = true
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	_, err := Run(ctx, options, "重路由协议 fixture", nil)
	if err == nil || !strings.Contains(err.Error(), "重路由") || !strings.Contains(err.Error(), "other-fixture") {
		t.Fatalf("重路由不能静默成功：%v", err)
	}
	if completed {
		t.Fatal("模型重路由仍发出了 completed")
	}
	assertHelperProcessExited(t, pidPath)
}
