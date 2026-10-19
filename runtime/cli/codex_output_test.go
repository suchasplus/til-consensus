package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCodexExecRealSchemaResult(t *testing.T) {
	body, err := os.ReadFile("testdata/codex/exec-schema-success.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	output, err := normalizeCodexExecOutput(string(body))
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Project string   `json:"project_name"`
		Scripts []string `json:"scripts"`
	}
	if err = json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatal(err)
	}
	if value.Project != "newtc" || len(value.Scripts) != 9 {
		t.Fatalf("unexpected real result: %s", output)
	}
	withoutTerminal := strings.Split(string(body), `{"type":"turn.completed"`)[0]
	if _, err = normalizeCodexExecOutput(withoutTerminal); err == nil {
		t.Fatal("partial message must not replace a completed turn")
	}
	if _, err = normalizeCodexExecOutput(string(body) + `{"type":"turn.failed","error":{"message":"failed"}}`); err == nil {
		t.Fatal("failed turn must not be accepted after output")
	}
}
