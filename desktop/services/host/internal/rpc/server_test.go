package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/runmanager"
	"strings"
	"testing"
	"time"
)

func TestJSONRPCIDsErrorsAndEOFFlush(t *testing.T) {
	var output bytes.Buffer
	server := New(&output)
	manager, err := runmanager.New(runmanager.Options{ProviderMode: "mock", DataDir: t.TempDir(), Notify: server.Notify})
	if err != nil {
		t.Fatal(err)
	}
	server.Bind(manager)
	input := `{"jsonrpc":"2.0","id":"hello-中文","method":"hello"}
{"jsonrpc":"2.0","id":2,"method":"run.list"}
{"jsonrpc":"2.0","id":3,"method":"unknown"}
{"jsonrpc":"2.0","id":4,"method":"run.start","params":{"goal":"目标","mode":"free_debate","idempotencyKey":"test","unexpected":true}}
invalid json
`
	if err = server.Serve(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	if err = manager.Close(); err != nil {
		t.Fatal(err)
	}
	server.Close()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("EOF丢失回复: %s", output.String())
	}
	responses := make([]map[string]any, len(lines))
	for i, line := range lines {
		if err = json.Unmarshal([]byte(line), &responses[i]); err != nil {
			t.Fatal(err)
		}
	}
	if responses[0]["id"] != "hello-中文" || responses[0]["result"].(map[string]any)["protocolVersion"] != "1.4" {
		t.Fatalf("握手错误: %#v", responses[0])
	}
	if len(responses[1]["result"].([]any)) != 0 {
		t.Fatal("空列表应为[]")
	}
	for i, code := range []float64{-32601, -32602, -32700} {
		if responses[i+2]["error"].(map[string]any)["code"] != code {
			t.Fatal("错误码不匹配", responses[i+2])
		}
	}
}

type disconnectedWriter struct{}

func (disconnectedWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("mock desktop disconnected")
}
func TestDisconnectedOutputDoesNotFailEngine(t *testing.T) {
	server := New(disconnectedWriter{})
	manager, err := runmanager.New(runmanager.Options{ProviderMode: "mock", DataDir: t.TempDir(), Delay: time.Millisecond, Notify: server.Notify})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	defer manager.Close()
	server.Bind(manager)
	r, err := manager.Start(model.StartRunInput{Goal: "界面断开仍可完成", Mode: "delphi", IdempotencyKey: "disconnect"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Get(r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Status == "completed" {
			events, err := manager.Events(r.ID, 0)
			if err != nil || len(events) == 0 {
				t.Fatal("断连后事件未持久化", err)
			}
			return
		}
		if snapshot.Status == "failed" {
			t.Fatal(snapshot.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("界面断开导致Engine未完成")
}
