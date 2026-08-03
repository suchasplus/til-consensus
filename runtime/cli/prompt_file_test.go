package cli

import (
	"context"
	"errors"
	"fmt"
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

// The preview command is what users inspect before spending money on a real
// run, so its argv structure must be identical to the executed one. Only the
// prompt directory and file path may differ.
func TestPreviewPromptTransportMatchesRealArgvStructure(t *testing.T) {
	const model = "Gemini 3.5 Flash (High)"
	// Distinctive on purpose: the agy bootstrap text itself talks about "task
	// instructions", so a natural-language prompt would false-positive the
	// leak assertion below.
	const prompt = "PROMPT-BODY-SENTINEL-9f3a"

	real, err := preparePromptTransport(config.CLITypeAntigravity, model, prompt, "medium")
	if err != nil {
		t.Fatalf("preparePromptTransport failed: %v", err)
	}
	defer func() {
		if cleanupErr := real.Cleanup(); cleanupErr != nil {
			t.Errorf("cleanup failed: %v", cleanupErr)
		}
	}()
	preview, err := previewPromptTransport(config.CLITypeAntigravity, model, prompt, "medium")
	if err != nil {
		t.Fatalf("previewPromptTransport failed: %v", err)
	}

	if len(real.Args) != len(preview.Args) {
		t.Fatalf("argv length differs: real=%d preview=%d", len(real.Args), len(preview.Args))
	}
	// Compare the flag skeleton (flag name + position) rather than values, so
	// the assertion stays meaningful without hardcoding value indices.
	skeleton := func(args []string) string {
		var parts []string
		for idx, arg := range args {
			if strings.HasPrefix(arg, "-") {
				parts = append(parts, fmt.Sprintf("%d:%s", idx, arg))
			}
		}
		return strings.Join(parts, " ")
	}
	if skeleton(real.Args) != skeleton(preview.Args) {
		t.Fatalf("argv skeleton differs:\n real=%s\n prev=%s", skeleton(real.Args), skeleton(preview.Args))
	}
	if real.Stdin != "" || preview.Stdin != "" {
		t.Fatal("antigravity must not carry the prompt on stdin")
	}
	for _, arg := range real.Args {
		if strings.Contains(arg, prompt) {
			t.Fatalf("prompt leaked into real argv: %q", arg)
		}
	}
	// Non-antigravity types must stay byte-identical between preview and real.
	for _, cliType := range []string{
		config.CLITypeClaude, config.CLITypeCodex, config.CLITypeGemini,
		config.CLITypeOpenCode, config.CLITypePi, config.CLITypeGeneric,
	} {
		realArgs, err := preparePromptTransport(cliType, "model-x", prompt, "medium")
		if err != nil {
			t.Fatalf("%s prepare failed: %v", cliType, err)
		}
		previewArgs, err := previewPromptTransport(cliType, "model-x", prompt, "medium")
		if err != nil {
			t.Fatalf("%s preview failed: %v", cliType, err)
		}
		if strings.Join(realArgs.Args, "\x00") != strings.Join(previewArgs.Args, "\x00") {
			t.Fatalf("%s argv differs between preview and real: %#v vs %#v", cliType, realArgs.Args, previewArgs.Args)
		}
		if realArgs.Stdin != previewArgs.Stdin {
			t.Fatalf("%s stdin differs between preview and real", cliType)
		}
	}
}
