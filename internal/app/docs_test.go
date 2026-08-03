package app

import (
	"bufio"
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
		"til-consensus config init --mode adjudication --provider-profile pi --config ./til-consensus.yaml",
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
		"docs/examples.md",
		"docs/examples/generic.config.yaml",
		"docs/examples/codex.config.yaml",
		"docs/examples/claude.config.yaml",
		"docs/examples/gemini.config.yaml",
		"docs/examples/antigravity.config.yaml",
		"docs/examples/pi.config.yaml",
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
		"docs/examples/generic.config.yaml",
		"docs/examples/codex.config.yaml",
		"docs/examples/claude.config.yaml",
		"docs/examples/gemini.config.yaml",
		"docs/examples/antigravity.config.yaml",
		"docs/examples/pi.config.yaml",
		"docs/examples/generic.run.yaml",
		"docs/examples/codex.run.yaml",
		"docs/examples/claude.run.yaml",
		"docs/examples/gemini.run.yaml",
		"docs/examples/antigravity.run.yaml",
		"docs/examples/pi.run.yaml",
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

func TestDocumentedMakeTargetsExist(t *testing.T) {
	root := repoRoot(t)
	targets := readMakefileTargets(t, filepath.Join(root, "Makefile"))
	markdownPaths := []string{filepath.Join(root, "README.md")}
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			markdownPaths = append(markdownPaths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}

	commandCount := 0
	for _, path := range markdownPaths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(file)
		lineNumber := 0
		inFence := false
		shellFence := false
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "```") {
				if inFence {
					inFence = false
					shellFence = false
				} else {
					inFence = true
					shellFence = isShellFence(strings.TrimSpace(strings.TrimPrefix(line, "```")))
				}
				continue
			}
			if !inFence || !shellFence {
				continue
			}
			for _, target := range makeTargetsFromShellLine(line) {
				commandCount++
				if _, ok := targets[target]; !ok {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						rel = path
					}
					t.Errorf("%s:%d documents unknown make target %q", rel, lineNumber, target)
				}
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			t.Fatalf("scan %s: %v", path, scanErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", path, closeErr)
		}
	}
	if commandCount == 0 {
		t.Fatal("no documented make commands found")
	}
}

func readMakefileTargets(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open Makefile: %v", err)
	}
	targets := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 || (colon+1 < len(line) && line[colon+1] == '=') {
			continue
		}
		for _, target := range strings.Fields(line[:colon]) {
			if strings.HasPrefix(target, ".") || strings.ContainsAny(target, "$%") {
				continue
			}
			targets[target] = struct{}{}
		}
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil {
		t.Fatalf("scan Makefile: %v", scanErr)
	}
	if closeErr != nil {
		t.Fatalf("close Makefile: %v", closeErr)
	}
	return targets
}

func isShellFence(language string) bool {
	switch strings.ToLower(language) {
	case "bash", "sh", "shell", "console":
		return true
	default:
		return false
	}
}

func makeTargetsFromShellLine(line string) []string {
	fields := strings.Fields(line)
	if len(fields) > 0 && fields[0] == "$" {
		fields = fields[1:]
	}
	makeIndex := -1
	for index, field := range fields {
		if field == "make" {
			makeIndex = index
			break
		}
		if field != "env" && !isShellAssignment(field) {
			return nil
		}
	}
	if makeIndex < 0 {
		return nil
	}

	var targets []string
	for _, field := range fields[makeIndex+1:] {
		field = strings.TrimSuffix(field, "\\")
		if field == "" || strings.HasPrefix(field, "-") || isShellAssignment(field) {
			continue
		}
		if strings.HasPrefix(field, "#") || strings.ContainsAny(field, ";|&") {
			break
		}
		targets = append(targets, field)
	}
	return targets
}

func isShellAssignment(field string) bool {
	name, _, ok := strings.Cut(field, "=")
	if !ok || name == "" {
		return false
	}
	for index, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}
