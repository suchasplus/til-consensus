package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigInitCommandStdout(t *testing.T) {
	var buf bytes.Buffer
	if err := runConfigInitCommand(&buf, "", "coding", "", "", "", true, false); err != nil {
		t.Fatalf("runConfigInitCommand failed: %v", err)
	}
	if !strings.Contains(buf.String(), "mode=adjudication provider_profile=mock task_profile=coding") {
		t.Fatalf("unexpected stdout:\n%s", buf.String())
	}
}

func TestRunConfigInitCommandUsesDefaultPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	var buf bytes.Buffer
	if err := runConfigInitCommand(&buf, "", "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("runConfigInitCommand failed: %v", err)
	}
	path := filepath.Join(tmp, "til-consensus", "default.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if !strings.Contains(buf.String(), "config template written:") ||
		!strings.Contains(buf.String(), filepath.Base(path)) ||
		!strings.Contains(buf.String(), "mode=adjudication provider_profile=mock task_profile=general") {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
}
