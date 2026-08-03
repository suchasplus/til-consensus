package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TestPreparePromptTransportAntigravityUsesPrivateFile(t *testing.T) {
	prompt := "private and very long task prompt"
	transport, err := preparePromptTransport("antigravity", "Gemini 3.5 Flash (High)", prompt, "medium")
	if err != nil {
		t.Fatalf("preparePromptTransport failed: %v", err)
	}
	dir := argumentAfter(transport.Args, "--add-dir")
	if dir == "" {
		t.Fatalf("missing --add-dir in %#v", transport.Args)
	}
	if transport.Stdin != "" {
		t.Fatalf("expected empty stdin, got %q", transport.Stdin)
	}
	joined := strings.Join(transport.Args, " ")
	if strings.Contains(joined, prompt) {
		t.Fatalf("prompt leaked into argv: %#v", transport.Args)
	}
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Fatalf("missing automatic approval flag: %#v", transport.Args)
	}
	path := filepath.Join(dir, "prompt.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	if string(body) != prompt {
		t.Fatalf("prompt file mismatch: %q", string(body))
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("prompt dir mode: info=%v err=%v", info, err)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("prompt file mode: info=%v err=%v", info, err)
		}
	}
	if err := transport.Cleanup(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if err := transport.Cleanup(); err != nil {
		t.Fatalf("repeated cleanup failed: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected prompt directory removed, stat err=%v", err)
	}
}

func TestPreviewPromptTransportAntigravityDoesNotCreateFile(t *testing.T) {
	transport, err := previewPromptTransport("antigravity", "model", "secret prompt", "")
	if err != nil {
		t.Fatalf("previewPromptTransport failed: %v", err)
	}
	joined := strings.Join(transport.Args, " ")
	if !strings.Contains(joined, "<prompt-dir>") || !strings.Contains(joined, "<prompt-file>") {
		t.Fatalf("expected prompt placeholders, got %#v", transport.Args)
	}
	if strings.Contains(joined, "secret prompt") {
		t.Fatalf("preview leaked prompt into argv: %#v", transport.Args)
	}
}

func TestPromptTransportSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		wantErr bool
	}{
		{name: "below", prompt: strings.Repeat("a", maxCLIPromptBytes-1)},
		{name: "boundary", prompt: strings.Repeat("a", maxCLIPromptBytes)},
		{name: "above", prompt: strings.Repeat("a", maxCLIPromptBytes+1), wantErr: true},
		{name: "utf8 above", prompt: strings.Repeat("界", maxCLIPromptBytes/3+1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := preparePromptTransport("generic", "model", tt.prompt, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error mismatch: err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil {
				_ = transport.Cleanup()
			}
		})
	}
}

func TestPromptTransportCleanupReportsFailure(t *testing.T) {
	original := removePromptDir
	t.Cleanup(func() { removePromptDir = original })
	removePromptDir = func(string) error { return errors.New("forced cleanup failure") }
	transport, err := preparePromptTransport("antigravity", "model", "prompt", "")
	if err != nil {
		t.Fatalf("preparePromptTransport failed: %v", err)
	}
	if err := transport.Cleanup(); err == nil || !strings.Contains(err.Error(), "forced cleanup failure") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
	removePromptDir = original
	_ = transport.Cleanup()
}

func TestRunTaskDetailedCleansAntigravityPromptAfterExecutionFailure(t *testing.T) {
	runner := NewRunner(config.ProviderConfig{
		Type:    config.ProviderTypeCLI,
		CLIType: config.CLITypeAntigravity,
		Command: "til-consensus-missing-agy-for-cleanup-test",
	})
	result, err := runner.RunTaskDetailed(context.Background(), realPromptTransportTestTask{
		TaskMeta: consensus.TaskMeta{RequestID: "req", SessionID: "session", AgentID: "agent"},
	}, "private prompt", "agent", "role", "model", "", nil, nil)
	if err == nil {
		t.Fatal("expected missing command to fail")
	}
	dir := argumentAfter(result.Command, "--add-dir")
	if dir == "" {
		t.Fatalf("missing prompt directory in command: %#v", result.Command)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("expected prompt directory cleanup, stat err=%v", statErr)
	}
}

type realPromptTransportTestTask struct {
	consensus.TaskMeta
}

func (realPromptTransportTestTask) Kind() consensus.TaskKind {
	return consensus.TaskKind("transport-test")
}
func (t realPromptTransportTestTask) Meta() consensus.TaskMeta {
	return t.TaskMeta
}

func argumentAfter(args []string, name string) string {
	for idx := 0; idx < len(args)-1; idx++ {
		if args[idx] == name {
			return args[idx+1]
		}
	}
	return ""
}
