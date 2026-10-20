package preparation

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
)

// Native protocol fixtures intentionally use a synthetic analysis document.
// They exercise real subprocess/stdio adapters without contacting any model.
func TestNativePreparationHelperProcess(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-3] != "--" {
		return
	}
	fixture, cliType := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	raw, err := os.ReadFile(fixture)
	if err != nil {
		os.Exit(3)
	}
	var output any
	if json.Unmarshal(raw, &output) != nil {
		os.Exit(4)
	}
	send := func(value any) {
		if json.NewEncoder(os.Stdout).Encode(value) != nil {
			os.Exit(5)
		}
	}
	usage := map[string]any{"input_tokens": 100, "output_tokens": 20}
	switch cliType {
	case "claude":
		send(map[string]any{"type": "system", "subtype": "init", "model": "claude-unit-fixture"})
		send(map[string]any{"type": "stream_event", "event": map[string]any{"delta": map[string]any{"type": "thinking_delta", "thinking": "PRIVATE-NATIVE-REASONING"}}})
		send(map[string]any{"type": "result", "subtype": "success", "is_error": false, "terminal_reason": "completed", "structured_output": output, "usage": usage})
	case "grok":
		send(map[string]any{"type": "text", "data": string(raw)})
		send(map[string]any{"type": "usage", "usage": usage})
		send(map[string]any{"type": "usage", "usage": usage})
		send(map[string]any{"type": "end", "stopReason": "end_turn", "usage": usage, "modelUsage": map[string]any{"grok-4.7-build": map[string]any{"modelCalls": 1}}, "total_cost_usd": 0.004})
	case "agy":
		send(map[string]any{"event": "init", "init": map[string]any{"model": "gemini-unit-fixture", "json_schema": map[string]any{"type": "object"}}})
		send(map[string]any{"event": "step_update", "step_update": map[string]any{"step_type": "user_input", "state": "DONE"}})
		send(map[string]any{"event": "step_update", "step_update": map[string]any{"step_type": "agent_response", "state": "DONE", "text_delta": string(raw)}})
		send(map[string]any{"event": "result", "result": map[string]any{"status": "SUCCESS", "num_turns": 1, "structured_output": output, "usage": usage}})
	case "codex":
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 65536), 1024*1024)
		for scanner.Scan() {
			var request map[string]any
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				os.Exit(6)
			}
			switch request["method"] {
			case "initialize":
				send(map[string]any{"id": 1, "result": map[string]any{"userAgent": "preparation-fixture"}})
			case "thread/start":
				params, _ := request["params"].(map[string]any)
				if params["ephemeral"] != true || params["sandbox"] != "read-only" || params["approvalPolicy"] != "never" {
					os.Exit(7)
				}
				send(map[string]any{"id": 2, "result": map[string]any{"thread": map[string]any{"id": "private-native-thread"}, "model": "gpt-unit-fixture"}})
			case "turn/start":
				params, _ := request["params"].(map[string]any)
				schema, _ := json.Marshal(params["outputSchema"])
				if strings.Contains(string(schema), `"$schema"`) || strings.Contains(string(schema), `"uniqueItems"`) {
					os.Exit(8)
				}
				send(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]any{"id": "private-native-turn"}}})
				send(map[string]any{"method": "item/reasoning/summaryTextDelta", "params": map[string]any{"delta": "PRIVATE-NATIVE-REASONING"}})
				send(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": string(raw)}})
				send(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"type": "agentMessage", "text": string(raw)}}})
				send(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 100, "outputTokens": 20}}}})
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "completed"}}})
			}
		}
	default:
		os.Exit(9)
	}
	os.Exit(0)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func nativeCommand(t *testing.T, cliType string) string {
	t.Helper()
	directory := t.TempDir()
	fixture, err := filepath.Abs("testdata/needs.json")
	if err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(directory, "native-protocol-fixture")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '%s\\n' 'native-protocol-fixture-v1'; exit 0; fi\nexec " + shellQuote(os.Args[0]) + " -test.run='^TestNativePreparationHelperProcess$' -- " + shellQuote(fixture) + " " + shellQuote(cliType) + "\n"
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return command
}

func TestPreparationNativeProtocolSubprocesses(t *testing.T) {
	for _, id := range []string{"codex", "claude", "grok", "agy"} {
		t.Run(id, func(t *testing.T) {
			r := recordFixture(t)
			out, err := Execute(context.Background(), r, providers.Config{ID: id, Command: nativeCommand(t, id), Model: "unit-selector"}, "cli", 0)
			if err != nil {
				t.Fatal(err)
			}
			if out.Classification == nil || out.Classification.Recommendation != "needs_clarification" || out.Source.Mock || out.Source.ModelSource != "cli-report" || out.Source.ActualModel == "" || out.Source.CLIVersion != "native-protocol-fixture-v1" {
				t.Fatalf("原生协议结果未严格解码或来源不实：%#v", out.Source)
			}
			if out.Usage.InputTokens != 100 || out.Usage.OutputTokens != 20 || out.Usage.Calls != 1 {
				t.Fatal("真实传输usage快照统计错误")
			}
			if id != "grok" && out.CostUSD != nil {
				t.Fatal("未知费用被伪报")
			}
			encoded, _ := json.Marshal(out)
			if strings.Contains(string(encoded), "PRIVATE-NATIVE-REASONING") || strings.Contains(string(encoded), "private-native-thread") {
				t.Fatal("推理或原生会话标识进入RPC")
			}
		})
	}
}
