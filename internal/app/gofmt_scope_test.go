package app

import (
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGofmtFilesScriptUsesGitAwareScope(t *testing.T) {
	root := repoRoot(t)
	repo := t.TempDir()
	runGofmtScopeCommand(t, repo, "git", "init", "-q")

	trackedSource := "package fixture\n\nfunc Tracked() {}\n"
	unformattedSource := "package fixture\nfunc Unformatted( ){ }\n"
	writeGofmtScopeFile(t, repo, ".gitignore", "ignored/\n")
	writeGofmtScopeFile(t, repo, "tracked.go", trackedSource)
	writeGofmtScopeFile(t, repo, "deleted.go", trackedSource)
	runGofmtScopeCommand(t, repo, "git", "add", ".gitignore", "tracked.go", "deleted.go")
	if err := os.Remove(filepath.Join(repo, "deleted.go")); err != nil {
		t.Fatalf("remove deleted fixture: %v", err)
	}
	writeGofmtScopeFile(t, repo, "untracked.go", unformattedSource)
	writeGofmtScopeFile(t, repo, "ignored/ignored.go", unformattedSource)

	script := filepath.Join(root, "scripts", "gofmt-files.sh")
	listed := runGofmtScopeCommand(t, repo, "bash", script, "list")
	selected := make(map[string]bool)
	for _, path := range strings.Split(strings.TrimSuffix(string(listed), "\x00"), "\x00") {
		if path != "" {
			selected[path] = true
		}
	}
	for _, path := range []string{"tracked.go", "untracked.go"} {
		if !selected[path] {
			t.Errorf("expected %s to be selected", path)
		}
	}
	for _, path := range []string{"deleted.go", "ignored/ignored.go"} {
		if selected[path] {
			t.Errorf("expected %s to be excluded", path)
		}
	}

	unformatted := string(runGofmtScopeCommand(t, repo, "bash", script, "check"))
	if unformatted != "untracked.go\n" {
		t.Fatalf("unexpected check output: %q", unformatted)
	}
	runGofmtScopeCommand(t, repo, "bash", script, "write")
	formatted, err := format.Source([]byte(unformattedSource))
	if err != nil {
		t.Fatalf("format fixture source: %v", err)
	}
	assertGofmtScopeFileContent(t, filepath.Join(repo, "untracked.go"), string(formatted))
	assertGofmtScopeFileContent(t, filepath.Join(repo, "ignored", "ignored.go"), unformattedSource)
	if output := runGofmtScopeCommand(t, repo, "bash", script, "check"); len(output) != 0 {
		t.Fatalf("expected clean check output, got %q", output)
	}
	runGofmtScopeCommand(
		t,
		repo,
		"make",
		"-f", filepath.Join(root, "Makefile"),
		"fmt-check",
		"GOFMT_FILES_SCRIPT="+script,
	)
}

func runGofmtScopeCommand(t *testing.T, dir string, name string, args ...string) []byte {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return output
}

func writeGofmtScopeFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", rel, err)
	}
}

func assertGofmtScopeFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != expected {
		t.Errorf("unexpected content in %s:\n%s", path, content)
	}
}
