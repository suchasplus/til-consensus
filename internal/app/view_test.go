package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewCommandRejectsSchemaMismatch(t *testing.T) {
	tmp := t.TempDir()
	resultPath := filepath.Join(tmp, "result.json")
	if err := os.WriteFile(resultPath, []byte(`{"schemaVersion":2}`), 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	cmd := newViewCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout

	err := cmd.Run(context.Background(), []string{"view", "--result", resultPath})
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "unsupported result schema version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestViewCommandColorizesTextWhenForced(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	resultPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "view", "sample-run", "result.json"))
	if err != nil {
		t.Fatalf("resolve result path: %v", err)
	}

	cmd := newViewCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout

	if err := cmd.Run(context.Background(), []string{"view", "--result", resultPath, "--format", "text"}); err != nil {
		t.Fatalf("view command failed: %v", err)
	}

	text := stdout.String()
	for _, needle := range []string{
		"\x1b[36m运行头部\x1b[0m",
		"\x1b[34m关键 Claims\x1b[0m",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected colored view output to contain %q, got:\n%s", needle, text)
		}
	}
}

func TestViewCommandDoesNotColorizeNonTextFormats(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	resultPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "view", "sample-run", "result.json"))
	if err != nil {
		t.Fatalf("resolve result path: %v", err)
	}

	cmd := newViewCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout

	if err := cmd.Run(context.Background(), []string{"view", "--result", resultPath, "--format", "markdown"}); err != nil {
		t.Fatalf("view command failed: %v", err)
	}

	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("expected markdown output to stay plain, got:\n%s", stdout.String())
	}
}
