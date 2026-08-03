package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
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

func TestConfigAddProviderWritesEnvironmentPassthrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "til-consensus.yaml")
	if err := config.Write(path, config.Normalize(config.InitTemplate())); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cmd := newConfigCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	err := cmd.Run(context.Background(), []string{
		"config", "add-provider",
		"--config", path,
		"--id", "pi-cli",
		"--type", "cli",
		"--cli-type", "pi",
		"--command", "pi",
		"--model-id", "default",
		"--provider-model", "gemini-3.6-flash",
		"--env-passthrough", "GEMINI_API_KEY",
	})
	if err != nil {
		t.Fatalf("config add-provider failed: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	provider := loaded.Config.Providers["pi-cli"]
	if provider.EnvPassthrough == nil || len(*provider.EnvPassthrough) != 1 || (*provider.EnvPassthrough)[0] != "GEMINI_API_KEY" {
		t.Fatalf("unexpected env passthrough: %#v", provider.EnvPassthrough)
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
