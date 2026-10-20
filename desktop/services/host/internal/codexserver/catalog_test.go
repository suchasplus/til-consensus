package codexserver

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCodexModelCatalogUsesPublicProtocolAndModelSpecificEfforts(t *testing.T) {
	options, requestsPath, pidPath := recordedHelperOptions(t)
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_SCENARIO=catalog-pagination")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	models, err := ListModels(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelInfo{
		{ID: "fixture-a", Label: "公开模型 A", ReasoningValues: []string{"low", "high"}, DefaultReasoning: "high"},
		{ID: "fallback-fixture", Label: "回退 ID", ReasoningValues: []string{}, DefaultReasoning: ""},
		{ID: "fixture-b", Label: "公开模型 B", ReasoningValues: []string{"future-effort"}, DefaultReasoning: "future-effort"},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("应使用模型公开能力并过滤隐藏、重复、空 ID：got=%+v want=%+v", models, want)
	}
	requests := readHelperRequests(t, requestsPath)
	var methods []string
	var lists []map[string]any
	for _, request := range requests {
		methods = append(methods, stringField(request, "method"))
		if request["method"] == "model/list" {
			lists = append(lists, request)
		}
	}
	if !reflect.DeepEqual(methods, []string{"initialize", "initialized", "model/list", "model/list"}) {
		t.Fatalf("目录只能握手并读取公开 model/list，不应创建线程或 turn：%+v", methods)
	}
	first, second := object(lists[0]["params"]), object(lists[1]["params"])
	if _, exists := first["cursor"]; exists {
		t.Fatalf("第一页不应发送 cursor：%+v", first)
	}
	if first["includeHidden"] != false || first["limit"] != float64(100) || second["cursor"] != "page-two" || second["includeHidden"] != false {
		t.Fatalf("分页参数错误：first=%+v second=%+v", first, second)
	}
	if lists[0]["id"] == lists[1]["id"] {
		t.Fatalf("分页需使用不同 request ID：%+v", lists)
	}
	assertHelperProcessExited(t, pidPath)
}

func TestCodexModelCatalogFailuresReturnNoPartialCatalogAndCleanUp(t *testing.T) {
	cases := []struct {
		scenario string
		message  string
	}{
		{scenario: "catalog-request-error", message: "fixture catalog unavailable"},
		{scenario: "catalog-second-page-error", message: "fixture pagination limited"},
		{scenario: "catalog-invalid-json", message: "协议 JSON"},
		{scenario: "catalog-empty", message: "目录为空"},
		{scenario: "catalog-repeated-cursor", message: "分页重复"},
		{scenario: "catalog-exit", message: "返回模型目录前退出"},
	}
	for _, test := range cases {
		t.Run(test.scenario, func(t *testing.T) {
			options, _, pidPath := recordedHelperOptions(t)
			options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_SCENARIO="+test.scenario)
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			models, err := ListModels(ctx, options)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("目录错误不能转为成功：models=%+v error=%v，期望 %q", models, err, test.message)
			}
			if len(models) != 0 {
				t.Fatalf("失败时不应把部分目录冒充完整结果：%+v", models)
			}
			assertHelperProcessExited(t, pidPath)
		})
	}
}

func TestCodexModelCatalogCancellationStopsHelperProcess(t *testing.T) {
	options, requestsPath, pidPath := recordedHelperOptions(t)
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_SCENARIO=catalog-block")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	type outcome struct {
		models []ModelInfo
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		models, err := ListModels(ctx, options)
		done <- outcome{models, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		body, err := os.ReadFile(requestsPath)
		if err == nil && strings.Contains(string(body), `"method":"model/list"`) {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("目录 fixture 没有收到 model/list")
		}
		time.Sleep(10 * time.Millisecond)
	}
	started := time.Now()
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) || len(result.models) != 0 {
			t.Fatalf("取消不能返回目录成功：models=%+v error=%v", result.models, result.err)
		}
		if time.Since(started) > 2*time.Second {
			t.Fatal("取消后未及时结束目录进程")
		}
		assertHelperProcessExited(t, pidPath)
	case <-time.After(3 * time.Second):
		t.Fatal("取消后目录进程没有退出")
	}
}
