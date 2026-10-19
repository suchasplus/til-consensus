package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readGrokFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "grok", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertGrokProjectFacts(t *testing.T, output string) {
	t.Helper()
	var value struct {
		ProjectName      string   `json:"project_name"`
		ElectronVersion  string   `json:"electron_version"`
		ReactVersion     string   `json:"react_version"`
		GoVersion        string   `json:"go_version"`
		ConsensusVersion string   `json:"consensus_version"`
		Scripts          []string `json:"scripts"`
		Risk             string   `json:"risk"`
	}
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatalf("output is not the project's JSON object: %v", err)
	}
	if value.ProjectName != "newtc" || value.ElectronVersion != "44.9.0" || value.ReactVersion != "19.3.1" || value.GoVersion != "1.26.1" || value.ConsensusVersion != "v0.0.0-20260803125524-b505773800a4" || len(value.Scripts) != 9 || value.Risk == "" {
		t.Fatalf("real project facts were lost or duplicated: %+v", value)
	}
}

func TestNormalizeGrokRealSchemaAndStream(t *testing.T) {
	for _, name := range []string{"schema-success.json", "stream-success.jsonl"} {
		t.Run(name, func(t *testing.T) {
			output, err := normalizeGrokCLIOutput(readGrokFixture(t, name))
			if err != nil {
				t.Fatal(err)
			}
			assertGrokProjectFacts(t, output)
		})
	}
}

func TestNormalizeGrokRealTextFallback(t *testing.T) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readGrokFixture(t, "schema-success.json")), &envelope); err != nil {
		t.Fatal(err)
	}
	delete(envelope, "structuredOutput")
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	output, err := normalizeGrokCLIOutput(string(body))
	if err != nil {
		t.Fatal(err)
	}
	assertGrokProjectFacts(t, output)
}

func TestNormalizeGrokRealAuthenticationFailure(t *testing.T) {
	output, err := normalizeGrokCLIOutput(readGrokFixture(t, "auth-error.json"))
	var resultErr *GrokResultError
	if output != "" || !errors.As(err, &resultErr) || resultErr.EventType != "error" || !strings.Contains(resultErr.Message, "Not signed in") {
		t.Fatalf("expected native authentication error, got output=%q error=%v", output, err)
	}
}

func TestNormalizeGrokRealInterruptedStreamIsIncomplete(t *testing.T) {
	output, err := normalizeGrokCLIOutput(readGrokFixture(t, "stream-cancelled.jsonl"))
	if output != "" || err == nil || !strings.Contains(err.Error(), "no native end envelope") && !strings.Contains(err.Error(), "no end envelope") {
		t.Fatalf("interrupted stdout must not become a successful response: output=%q error=%v", output, err)
	}
	// The real CLI exits 130 without a cancelled end event. The host must use
	// its cancellation context/exit status; stdout alone cannot prove why it ended.
	if errors.Is(err, context.Canceled) {
		t.Fatalf("incomplete stdout alone must not invent a cancellation cause: %v", err)
	}
}

func TestNormalizeGrokProtocolEdges(t *testing.T) {
	tests := []struct {
		name, raw, want string
		failed          bool
		canceled        bool
	}{
		{name: "structured output wins", raw: `{"text":"stale","structuredOutput":{"answer":42},"stopReason":"end_turn"}`, want: `{"answer":42}`},
		{name: "null structure uses text", raw: `{"text":"answer","structuredOutput":null,"stopReason":"end_turn"}`, want: "answer"},
		{name: "terminal snapshot is used once", raw: "{\"type\":\"text\",\"data\":\"answer\"}\n{\"type\":\"end\",\"text\":\"answer\",\"stopReason\":\"end_turn\"}", want: "answer"},
		{name: "repeated deltas preserved", raw: "{\"type\":\"text\",\"data\":\"ha\"}\n{\"type\":\"text\",\"data\":\"ha\"}\n{\"type\":\"end\",\"stopReason\":\"end_turn\"}", want: "haha"},
		{name: "cancelled terminal", raw: `{"type":"end","stopReason":"cancelled"}`, failed: true, canceled: true},
		{name: "cancelled error", raw: `{"type":"error","message":"Request cancelled","stopReason":"cancelled"}`, failed: true, canceled: true},
		{name: "truncation", raw: `{"text":"partial","stopReason":"max_tokens"}`, failed: true},
		{name: "turn limit", raw: `{"text":"partial","stopReason":"max_turn_requests"}`, failed: true},
		{name: "refusal", raw: `{"text":"refused","stopReason":"refusal"}`, failed: true},
		{name: "unknown terminal", raw: `{"text":"answer","stopReason":"success"}`, failed: true},
		{name: "missing terminal reason", raw: `{"text":"answer"}`, failed: true},
		{name: "empty success", raw: `{"stopReason":"end_turn"}`, failed: true},
		{name: "partial text without end", raw: `{"type":"text","data":"partial"}`, failed: true},
		{name: "wrong messages protocol", raw: `{"type":"result","subtype":"success","is_error":false,"result":"answer"}`, failed: true},
		{name: "malformed event", raw: "{\"type\":\"text\",\"data\":\"partial\"}\nnot-json", failed: true},
		{name: "event after end", raw: "{\"type\":\"end\",\"text\":\"answer\",\"stopReason\":\"end_turn\"}\n{\"type\":\"text\",\"data\":\"late\"}", failed: true},
		{name: "error after text", raw: "{\"type\":\"text\",\"data\":\"partial\"}\n{\"type\":\"error\",\"message\":\"backend failed\"}", failed: true},
		{name: "missing delta data", raw: "{\"type\":\"text\"}\n{\"type\":\"end\",\"stopReason\":\"end_turn\"}", failed: true},
		{name: "unknown event tolerated", raw: "{\"type\":\"future_metadata\",\"data\":\"ignored\"}\n{\"type\":\"end\",\"text\":\"answer\",\"stopReason\":\"end_turn\"}", want: "answer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := normalizeGrokCLIOutput(test.raw)
			if (err != nil) != test.failed || output != test.want {
				t.Fatalf("output=%q error=%v; want=%q failed=%v", output, err, test.want, test.failed)
			}
			if errors.Is(err, context.Canceled) != test.canceled {
				t.Fatalf("cancellation classification=%v; want=%v", err, test.canceled)
			}
		})
	}
}
