package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestReadmeQuickstartCommands(t *testing.T) {
	tmp := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	configPath := filepath.Join(tmp, "til-consensus.yaml")

	var initOut bytes.Buffer
	if err := runConfigInitCommand(&initOut, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("runConfigInitCommand failed: %v", err)
	}

	runSubcommand := func(cmdFactory func() *cli.Command, args ...string) string {
		t.Helper()
		cmd := cmdFactory()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Writer = &stdout
		cmd.ErrWriter = &stderr
		if err := cmd.Run(context.Background(), args); err != nil {
			t.Fatalf("command failed (%v): %v\nstderr:\n%s", args, err, stderr.String())
		}
		return stdout.String()
	}

	runSubcommand(newRunCommand, "run", "--config", configPath, "--task", "判断这个 patch 是否真正修复了竞态问题")
	output := runSubcommand(newViewCommand, "view", "--config", configPath)

	if !strings.Contains(output, "运行头部") || !strings.Contains(output, "关键 Claims") {
		t.Fatalf("unexpected view output:\n%s", output)
	}
}

func TestReadmeAndDocsIndexLinks(t *testing.T) {
	root := repoRoot(t)
	readmePath := filepath.Join(root, "README.md")
	indexPath := filepath.Join(root, "docs", "index.md")

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read docs/index.md: %v", err)
	}

	for _, needle := range []string{
		"result.json",
		"ledger.jsonl",
		"summary.md",
		"artifacts/",
		"til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml",
		"til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml",
		"til-consensus config init --mode free-debate --provider-profile mock --stdout",
		"--task-profile coding",
		"til-consensus run \\",
		"--mode free-debate",
		"--mode delphi",
		"--participants debater-a,debater-b,debater-c",
		"--convergence-threshold 0.8",
		"til-consensus view --config ./til-consensus.yaml",
		"make ci",
		"make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false",
		"docs/release.md",
		"docs/examples.md",
		"docs/examples/generic.config.yaml",
		"docs/examples/codex.config.yaml",
		"docs/examples/claude.config.yaml",
		"docs/examples/gemini.config.yaml",
		"docs/examples/document-refinement.run.yaml",
		"docs/examples/coding-review.run.yaml",
	} {
		if !strings.Contains(string(readme), needle) {
			t.Fatalf("README missing %q", needle)
		}
	}

	for _, rel := range []string{
		"docs/config.md",
		"docs/examples.md",
		"docs/output.md",
		"docs/release.md",
		"docs/view.md",
		"docs/viewer.md",
		"docs/rewrite.md",
		"docs/examples/generic.config.yaml",
		"docs/examples/codex.config.yaml",
		"docs/examples/claude.config.yaml",
		"docs/examples/gemini.config.yaml",
		"docs/examples/generic.run.yaml",
		"docs/examples/codex.run.yaml",
		"docs/examples/claude.run.yaml",
		"docs/examples/gemini.run.yaml",
		"docs/examples/document-refinement.run.yaml",
		"docs/examples/architecture-decision.run.yaml",
		"docs/examples/coding-review.run.yaml",
		"docs/examples/factual-conflict.run.yaml",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing linked doc %s: %v", rel, err)
		}
		if strings.HasPrefix(rel, "docs/examples/") {
			continue
		}
		if !strings.Contains(string(index), filepath.Base(rel)) {
			t.Fatalf("docs/index.md missing link to %s", rel)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}
