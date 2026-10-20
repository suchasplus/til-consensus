package codexserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/runtime/cli"
)

func TestCodexServerHelperProcess(t *testing.T) {
	if os.Getenv("TCVIEW_CODEX_HELPER") != "1" {
		return
	}
	if path := os.Getenv("TCVIEW_CODEX_HELPER_PID"); path != "" {
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(5)
		}
	}
	var requests *os.File
	if path := os.Getenv("TCVIEW_CODEX_HELPER_REQUESTS"); path != "" {
		var err error
		requests, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(6)
		}
		defer requests.Close()
	}
	scenario := os.Getenv("TCVIEW_CODEX_HELPER_SCENARIO")
	scanner := bufio.NewScanner(os.Stdin)
	send := func(v any) {
		if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
			os.Exit(2)
		}
	}
	for scanner.Scan() {
		var request map[string]any
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(3)
		}
		if requests != nil {
			if _, err := requests.Write(append(append([]byte(nil), scanner.Bytes()...), '\n')); err != nil {
				os.Exit(7)
			}
		}
		switch request["method"] {
		case "initialize":
			send(map[string]any{"id": 1, "result": map[string]any{"userAgent": "fixture"}})
		case "thread/start":
			params := object(request["params"])
			if params["ephemeral"] != true || params["sandbox"] != "read-only" || params["approvalPolicy"] != "never" {
				os.Exit(4)
			}
			response := map[string]any{"thread": map[string]any{"id": "test-thread"}, "model": "unit-fixture"}
			if reasoning := os.Getenv("TCVIEW_CODEX_HELPER_REASONING"); reasoning != "" {
				response["reasoningEffort"] = reasoning
			}
			send(map[string]any{"id": 2, "result": response})
		case "turn/start":
			send(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]any{"id": "test-turn"}}})
			if scenario == "rerouted" {
				send(map[string]any{"method": "model/rerouted", "params": map[string]any{"fromModel": "unit-fixture", "toModel": "other-fixture", "reason": "fixture capacity"}})
			}
			send(map[string]any{"method": "item/reasoning/summaryTextDelta", "params": map[string]any{"delta": "private reasoning fixture"}})
			send(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": `{"answer":`}})
			if os.Getenv("TCVIEW_CODEX_HELPER_BLOCK") == "1" {
				continue
			}
			send(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": `"公开结果"}`}})
			send(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"type": "agentMessage", "text": `{"answer":"公开结果"}`}}})
			send(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 20, "outputTokens": 7}}}})
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "completed"}}})
		case "turn/interrupt":
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "interrupted"}}})
		case "model/list":
			if scenario == "catalog-block" {
				continue
			}
			if scenario == "catalog-request-error" {
				send(map[string]any{"id": request["id"], "error": map[string]any{"code": -32602, "message": "fixture catalog unavailable"}})
				continue
			}
			if scenario == "catalog-invalid-json" {
				_, _ = os.Stdout.WriteString("{invalid fixture JSON\n")
				continue
			}
			if scenario == "catalog-exit" {
				os.Exit(0)
			}
			if scenario == "catalog-empty" {
				send(map[string]any{"id": request["id"], "result": map[string]any{"data": []any{}, "nextCursor": nil}})
				continue
			}
			params := object(request["params"])
			if params["cursor"] == nil {
				send(map[string]any{"id": request["id"], "result": map[string]any{"data": []any{
					map[string]any{"id": "internal-a", "model": "fixture-a", "displayName": "公开模型 A", "defaultReasoningEffort": "high", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}, map[string]any{"reasoningEffort": "high"}, map[string]any{"reasoningEffort": "low"}, map[string]any{"reasoningEffort": ""}}, "privateField": "must not be exposed"},
					map[string]any{"id": "hidden-id", "model": "hidden-fixture", "hidden": true},
					map[string]any{"id": "fallback-fixture", "model": "", "displayName": "回退 ID", "supportedReasoningEfforts": []any{}},
					map[string]any{"id": "", "model": " "},
				}, "nextCursor": "page-two"}})
			} else if scenario == "catalog-second-page-error" {
				send(map[string]any{"id": request["id"], "error": map[string]any{"code": 429, "message": "fixture pagination limited"}})
			} else {
				nextCursor := ""
				if scenario == "catalog-repeated-cursor" {
					nextCursor = "page-two"
				}
				send(map[string]any{"id": request["id"], "result": map[string]any{"data": []any{
					map[string]any{"id": "another-internal-a", "model": "fixture-a", "displayName": "重复项不可覆盖"},
					map[string]any{"id": "internal-b", "model": "fixture-b", "displayName": "公开模型 B", "defaultReasoningEffort": "future-effort", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "future-effort"}}},
				}, "nextCursor": nextCursor}})
			}
		}
	}
	os.Exit(0)
}

func helperOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "codex-protocol-fixture")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec \"$TCVIEW_CODEX_HELPER_BINARY\" -test.run=TestCodexServerHelperProcess\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return Options{Command: script, Cwd: dir, Environment: append(os.Environ(), "TCVIEW_CODEX_HELPER=1", "TCVIEW_CODEX_HELPER_BINARY="+os.Args[0])}
}

func TestCodexServerPublicStreamAndStrictTerminal(t *testing.T) {
	options := helperOptions(t)
	var text strings.Builder
	options.OnEvent = func(event cli.CLIEvent) error {
		if strings.Contains(event.Text, "private reasoning") {
			t.Error("reasoning leaked")
		}
		if event.Type == "text_delta" {
			text.WriteString(event.Text)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := Run(ctx, options, "协议传输测试", map[string]any{"type": "object"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != `{"answer":"公开结果"}` || text.String() != result.Output || result.Model != "unit-fixture" || result.Usage["outputTokens"] != float64(7) {
		t.Fatalf("invalid result: %+v text=%q", result, text.String())
	}
}

func TestCodexServerCancellationIsNotCompletion(t *testing.T) {
	options := helperOptions(t)
	options.Environment = append(options.Environment, "TCVIEW_CODEX_HELPER_BLOCK=1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	options.OnEvent = func(event cli.CLIEvent) error {
		if event.Type == "text_delta" {
			cancel()
		}
		return nil
	}
	started := time.Now()
	_, err := Run(ctx, options, "取消测试", map[string]any{"type": "object"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if time.Since(started) > 4*time.Second {
		t.Fatal("cancel left the process running")
	}
}

func TestLiveCodexServerSchema(t *testing.T) {
	if os.Getenv("TCVIEW_LIVE_CODEX") != "1" {
		t.Skip("仅显式真实验收时调用模型")
	}
	root := filepath.Join("..", "..", "..", "..")
	material, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Name string            `json:"name"`
		Dev  map[string]string `json:"devDependencies"`
	}
	if err = json.Unmarshal(material, &pkg); err != nil {
		t.Fatal(err)
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"project", "electron"}, "properties": map[string]any{"project": map[string]any{"type": "string"}, "electron": map[string]any{"type": "string"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	events := 0
	options := Options{Cwd: t.TempDir(), Model: "gpt-6.1-sol", Environment: os.Environ(), OnEvent: func(e cli.CLIEvent) error {
		if e.Type == "text_delta" {
			events++
		}
		return nil
	}}
	result, err := Run(ctx, options, "根据以下真实package.json摘取project=name与electron=devDependencies.electron；不要调用工具，只返回JSON。\n"+string(material), schema)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err = json.Unmarshal([]byte(result.Output), &value); err != nil {
		t.Fatal(err)
	}
	if value["project"] != pkg.Name || value["electron"] != pkg.Dev["electron"] || events == 0 {
		t.Fatalf("real result mismatch: %+v events=%d", value, events)
	}
	artifact := filepath.Join(root, ".tcview", "live-validation", "codex-app-server.json")
	if err = os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		t.Fatal(err)
	}
	body, _ := json.MarshalIndent(map[string]any{"model": result.Model, "output": value, "publicDeltaCount": events, "usage": result.Usage, "mock": false}, "", "  ")
	if err = os.WriteFile(artifact, body, 0600); err != nil {
		t.Fatal(err)
	}
}
