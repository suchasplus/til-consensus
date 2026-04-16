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
