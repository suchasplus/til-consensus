package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func readProgressFixture(t *testing.T, provider, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", provider, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func collectProgress(t *testing.T, provider, raw string, document bool, chunkSize int) []CLIEvent {
	t.Helper()
	var events []CLIEvent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &progressWriter{
		cliType: provider, document: document, cancel: cancel,
		callback: func(event CLIEvent) error { events = append(events, event); return nil },
	}
	bytes := []byte(raw)
	for len(bytes) > 0 {
		n := min(len(bytes), chunkSize)
		if _, err := writer.Write(bytes[:n]); err != nil {
			t.Fatal(err)
		}
		bytes = bytes[n:]
	}
	if err := writer.flush(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatalf("valid fixture unexpectedly cancelled the child context: %v", ctx.Err())
	}
	return events
}

func TestProgressRealGrokStreamPreservesUTF8AndUsageSnapshots(t *testing.T) {
	raw := readProgressFixture(t, "grok", "stream-success.jsonl")
	var expectedText strings.Builder
	var expectedUsage []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var event struct {
			Type  string         `json:"type"`
			Data  string         `json:"data"`
			Usage map[string]any `json:"usage"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "text" {
			expectedText.WriteString(event.Data)
		}
		if event.Type == "usage" {
			expectedUsage = append(expectedUsage, event.Usage)
		}
	}
	// One byte per Write deliberately splits the fixture's Chinese UTF-8 code
	// points and JSON escapes across arbitrary process-pipe boundaries.
	events := collectProgress(t, "grok", raw, false, 1)
	var actualText strings.Builder
	var actualUsage []map[string]any
	terminalCount := 0
	for _, event := range events {
		if !utf8.ValidString(event.Text) {
			t.Fatalf("public progress corrupted UTF-8: %q", event.Text)
		}
		switch event.Type {
		case "text_delta":
			actualText.WriteString(event.Text)
		case "usage":
			actualUsage = append(actualUsage, event.Usage)
		case "terminal":
			terminalCount++
			if event.Status != "end_turn" {
				t.Fatalf("wrong native terminal status: %+v", event)
			}
		}
	}
	if actualText.String() != expectedText.String() {
		t.Fatal("public deltas did not reconstruct the real answer exactly")
	}
	if len(expectedUsage) == 0 || !reflect.DeepEqual(actualUsage, expectedUsage) {
		t.Fatalf("usage must preserve provider snapshots without accumulation: actual=%v expected=%v", actualUsage, expectedUsage)
	}
	if terminalCount != 1 {
		t.Fatalf("expected one public terminal, got %d", terminalCount)
	}
}

func TestProgressRealGrokPrettyDocumentBuffersUntilFlush(t *testing.T) {
	raw := readProgressFixture(t, "grok", "schema-success.json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	// Sanitized fixtures omit private reasoning. This protocol-shaped marker
	// verifies that an upstream thought field cannot enter the public event.
	envelope["thought"] = "PRIVATE_REASONING_MUST_NOT_LEAK"
	body, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var events []CLIEvent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &progressWriter{cliType: "grok", document: true, cancel: cancel, callback: func(event CLIEvent) error { events = append(events, event); return nil }}
	for _, b := range body {
		if _, err := writer.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != 0 || ctx.Err() != nil {
		t.Fatal("pretty native JSON must remain buffered until document completion")
	}
	if err := writer.flush(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "terminal" || events[0].Status != "end_turn" {
		t.Fatalf("expected one native document terminal: %+v", events)
	}
	if events[0].Text != envelope["text"] || !reflect.DeepEqual(events[0].Usage, envelope["usage"]) {
		t.Fatal("native document answer or usage snapshot was lost")
	}
	serialized, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "PRIVATE_REASONING_MUST_NOT_LEAK") {
		t.Fatal("private document thought leaked through public progress")
	}
}

func TestProgressFiltersReasoningAndPreservesRepeatedDeltas(t *testing.T) {
	const private = "PRIVATE_REASONING_MUST_NOT_LEAK"
	for _, test := range []struct{ provider, raw string }{
		{"grok", "{\"type\":\"thought\",\"data\":\"" + private + "\"}\n{\"type\":\"text\",\"data\":\"好\"}\n{\"type\":\"text\",\"data\":\"好\"}\n"},
		{"claude", "{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"" + private + "\"}}}\n{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"好\"}}}\n{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"好\"}}}\n"},
		{"codex", "{\"type\":\"item.completed\",\"item\":{\"type\":\"reasoning\",\"text\":\"" + private + "\"}}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"好\"}}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"好\"}}\n"},
		{"antigravity", "{\"event\":\"step_update\",\"step_update\":{\"step_type\":\"agent_thought\",\"text_delta\":\"" + private + "\"}}\n{\"event\":\"step_update\",\"step_update\":{\"step_type\":\"agent_response\",\"text_delta\":\"好\"}}\n{\"event\":\"step_update\",\"step_update\":{\"step_type\":\"agent_response\",\"text_delta\":\"好\"}}\n"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			events := collectProgress(t, test.provider, test.raw, false, 2)
			var text strings.Builder
			for _, event := range events {
				text.WriteString(event.Text)
			}
			if text.String() != "好好" || len(events) != 2 {
				t.Fatalf("reasoning must be filtered and repeated public text preserved: %+v", events)
			}
		})
	}
}

func TestProgressRealAgyUsageIsTerminalSnapshot(t *testing.T) {
	raw := readProgressFixture(t, "agy", "stream-schema-success.jsonl")
	events := collectProgress(t, "antigravity", raw, false, 7)
	var terminal *CLIEvent
	for i := range events {
		if events[i].Type == "terminal" {
			terminal = &events[i]
		}
	}
	if terminal == nil || terminal.Status != "SUCCESS" || terminal.Usage["total_tokens"] != float64(24961) {
		t.Fatalf("agy terminal must keep the real usage snapshot once, not add step usage: %+v", terminal)
	}
}

func TestRunCommandProgressCancellationPreservesTruncatedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX subprocess signal integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ready:
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				cancel()
			case <-ctx.Done():
			}
		case <-ctx.Done():
		}
	}()
	started := time.Now()
	_, err := runCommandWithOptions(ctx, "grok", "sh", []string{"-c", `printf '%s\n%s' '{"type":"text","data":"public"}' '{"type":"text","data":"truncated'; printf 'preserved diagnostic' >&2; sleep 30`, "fixture-cli", "--output-format", "streaming-json"}, nil, "", "", RunnerOptions{OnEvent: func(CLIEvent) error { once.Do(func() { close(ready) }); return nil }})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("truncated final event must not hide cancellation: %T %v", err, err)
	}
	var commandErr *CommandError
	if !errors.As(err, &commandErr) || !strings.Contains(commandErr.Stdout, `{"type":"text","data":"truncated`) || commandErr.Stderr != "preserved diagnostic" {
		t.Fatalf("cancellation must retain partial stdout and stderr: %+v", commandErr)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("cancellation did not stop the subprocess within its bounded grace: %s", elapsed)
	}
}

func TestRunCommandProgressCallbackFailureCancelsSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX subprocess signal integration")
	}
	callbackErr := errors.New("consumer closed progress stream")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	_, err := runCommandWithOptions(ctx, "grok", "sh", []string{"-c", `printf '%s\n' '{"type":"text","data":"public"}'; sleep 30`, "fixture-cli", "--output-format", "streaming-json"}, nil, "", "", RunnerOptions{OnEvent: func(CLIEvent) error { return callbackErr }})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("callback failure must remain inspectable: %T %v", err, err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("callback failure left the subprocess running too long: %s", elapsed)
	}
}
