package securefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileUsesPrivatePermissionsAndRejectsSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(root, "artifact.json")
	if err := WriteFile(path, []byte("first")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	assertPermission(t, root, 0o700)
	assertPermission(t, path, 0o600)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod fixture: %v", err)
	}
	if err := WriteFile(path, []byte("second")); err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}
	assertPermission(t, path, 0o600)

	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := WriteFile(link, []byte("overwrite")); err == nil {
		t.Fatal("expected symlink write to fail")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(body) != "unchanged" {
		t.Fatalf("symlink target was modified: %q", body)
	}
}

func TestEnsureContainedRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if err := EnsureContained(root, filepath.Join(root, "nested", "file")); err != nil {
		t.Fatalf("contained path rejected: %v", err)
	}
	if err := EnsureContained(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Fatal("expected escaped path to fail")
	}
}

func assertPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got=%#o want=%#o", path, got, want)
	}
}
