package delegate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/runtime/mock"
)

// Explicit native-stream test substitute: a shell replays a synthetic, schema-valid Claude P0 envelope. No model or real CLI is called.
func TestCLINativeP0UsesFrozenClaudeModelAndEffort(t *testing.T) {
	goal := "明确脚本替身：停用 Codex 后由 Claude 生成 P0"
	task := consensus.ProposalTask{TaskMeta: consensus.TaskMeta{AgentID: "claude", Role: "proposer"}, TaskSpec: consensus.TaskSpec{Goal: goal}, MaxClaims: 2}
	body, _ := json.Marshal(task)
	hash := sha256.Sum256(body)
	raw, err := mock.RunTask(context.Background(), task, config.AgentConfig{ID: "claude"}, config.ProviderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	core, err := localizeCore(task, raw)
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{SchemaVersion: 1, Mock: false, InvocationID: "native-p0-inv-0001", TaskKind: string(task.Kind()), ContextHash: hex.EncodeToString(hash[:]), CoreOutput: core, DesktopReview: []Review{}}
	serialized, _ := json.Marshal(envelope)
	init, _ := json.Marshal(map[string]any{"type": "system", "subtype": "init", "model": "claude-native-p0-fixture"})
	result, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "num_turns": 1, "result": string(serialized), "structured_output": envelope})
	dir := t.TempDir()
	fixture := filepath.Join(dir, "claude-p0.jsonl")
	argsPath := filepath.Join(dir, "args.txt")
	if err = os.WriteFile(fixture, append(append(init, '\n'), append(result, '\n')...), 0600); err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	command := filepath.Join(dir, "claude-fixture")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '%s\\n' 'explicit-native-p0-fixture'; exit 0; fi\nprintf '%s\\n' \"$@\" > " + quote(argsPath) + "\ncat " + quote(fixture) + "\n"
	if err = os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	d := New(Options{Mode: "cli", AttemptID: "native-p0", ArtifactDir: dir, Timeout: 5 * time.Second, Providers: []providers.Config{{ID: "claude", Command: command, Model: "claude-request-selector-fixture", Reasoning: "low"}}})
	defer d.Close()
	content, err := d.ProposeP0For(context.Background(), "claude", goal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, goal) {
		t.Fatal("P0 result lost selected task")
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--model\nclaude-request-selector-fixture\n") || !strings.Contains(string(args), "--effort\nlow\n") {
		t.Fatalf("lost frozen native arguments: %s", args)
	}
	if _, err = d.Dispatch(context.Background(), consensus.ProposalTask{TaskMeta: consensus.TaskMeta{AgentID: "codex"}, TaskSpec: consensus.TaskSpec{Goal: goal}, MaxClaims: 2}); err == nil {
		t.Fatal("unconfigured Codex dispatch was allowed")
	}
}
