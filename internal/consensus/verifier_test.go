package consensus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceSnapshotCheckUsesConfiguredHash(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := computePathsHash(root, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		IDFactory:   &deterministicIDs{},
		ArtifactDir: filepath.Join(root, "artifacts"),
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec: TaskSpec{
				Goal: "verify",
				WorkspaceSnapshot: &WorkspaceSnapshot{
					Root:  root,
					Paths: []string{"a.txt"},
					Hash:  hash,
				},
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	result := verifier.runWorkspaceSnapshotCheck(req, VerificationCheck{Name: "snapshot", Kind: "workspace_snapshot"})
	if result.Status != VerificationStatusPassed {
		t.Fatalf("expected passed snapshot check, got %#v", result)
	}

	req.Request.TaskSpec.WorkspaceSnapshot.Hash = "deadbeef"
	result = verifier.runWorkspaceSnapshotCheck(req, VerificationCheck{Name: "snapshot", Kind: "workspace_snapshot"})
	if result.Status != VerificationStatusFailed {
		t.Fatalf("expected failed snapshot check, got %#v", result)
	}
}

func TestAllowedPathsCheckRejectsOutOfScopePath(t *testing.T) {
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		IDFactory: &deterministicIDs{},
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec: TaskSpec{
				Goal: "verify",
				Constraints: TaskConstraints{
					AllowedPaths: []string{"internal/consensus"},
				},
			},
		},
		SessionID: "session-1",
		Claim: ClaimNode{
			ClaimID: "claim-1",
			Metadata: map[string]any{
				"touchedPaths": []string{"internal/consensus/engine.go", "cmd/til-consensus/main.go"},
			},
		},
	}
	result := verifier.runAllowedPathsCheck(req, VerificationCheck{Name: "allowed", Kind: "allowed_paths"})
	if result.Status != VerificationStatusFailed {
		t.Fatalf("expected failed allowed paths check, got %#v", result)
	}
	if result.FailureCode != "path_out_of_scope" {
		t.Fatalf("unexpected failure code: %#v", result)
	}
}

func TestCommandCheckWritesArtifactAndInjectsContextEnv(t *testing.T) {
	root := t.TempDir()
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		IDFactory:   &deterministicIDs{},
		ArtifactDir: filepath.Join(root, "artifacts"),
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-123",
			TaskSpec: TaskSpec{
				Goal: "verify",
				WorkspaceSnapshot: &WorkspaceSnapshot{
					Root: root,
				},
			},
		},
		SessionID: "session-456",
		Claim:     ClaimNode{ClaimID: "claim-789"},
	}
	result := verifier.runCommandCheck(context.Background(), req, VerificationCheck{
		Name:    "env",
		Kind:    "command",
		Command: "sh",
		Args:    []string{"-c", `printf "%s|%s|%s" "$TIL_CONSENSUS_REQUEST_ID" "$TIL_CONSENSUS_SESSION_ID" "$TIL_CONSENSUS_CLAIM_ID"`},
		Workdir: root,
	})
	if result.Status != VerificationStatusPassed {
		t.Fatalf("expected passed command check, got %#v", result)
	}
	if result.Artifact == nil {
		t.Fatal("expected command artifact to be written")
	}
	body, err := os.ReadFile(result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "req-123|session-456|claim-789") {
		t.Fatalf("artifact missing injected env values: %s", string(body))
	}
}

func TestGitDiffPathsCheckRejectsChangedFileOutsideScope(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	mkdirAll(t, filepath.Join(root, "allowed"))
	mkdirAll(t, filepath.Join(root, "blocked"))
	writeFile(t, filepath.Join(root, "allowed", "a.txt"), "a")
	writeFile(t, filepath.Join(root, "blocked", "b.txt"), "b")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	baseRevision := strings.TrimSpace(string(runGit(t, root, "rev-parse", "HEAD")))
	writeFile(t, filepath.Join(root, "blocked", "b.txt"), "changed")

	verifier := NewCompositeVerifier(CompositeVerifierDeps{IDFactory: &deterministicIDs{}})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec: TaskSpec{
				Goal: "verify",
				WorkspaceSnapshot: &WorkspaceSnapshot{
					Root: root,
				},
				Constraints: TaskConstraints{
					AllowedPaths: []string{"allowed"},
				},
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	result := verifier.runGitDiffPathsCheck(context.Background(), req, VerificationCheck{
		Name:         "git-diff",
		Kind:         "git_diff_paths",
		BaseRevision: baseRevision,
	})
	if result.Status != VerificationStatusFailed || result.FailureCode != "git_diff_path_out_of_scope" {
		t.Fatalf("expected git diff out-of-scope failure, got %#v", result)
	}
}

func TestBenchmarkThresholdCheckPassesAndFails(t *testing.T) {
	root := t.TempDir()
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		IDFactory:   &deterministicIDs{},
		ArtifactDir: filepath.Join(root, "artifacts"),
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec: TaskSpec{
				Goal: "benchmark",
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	pass := verifier.runBenchmarkThresholdCheck(context.Background(), req, VerificationCheck{
		Name:          "bench",
		Kind:          "benchmark_threshold",
		Command:       "sh",
		Args:          []string{"-c", `printf 'p95_ms=42.5\n'`},
		Pattern:       `p95_ms=([0-9.]+)`,
		Threshold:     50,
		ThresholdMode: "max",
		Workdir:       root,
	})
	if pass.Status != VerificationStatusPassed {
		t.Fatalf("expected passing benchmark, got %#v", pass)
	}
	fail := verifier.runBenchmarkThresholdCheck(context.Background(), req, VerificationCheck{
		Name:          "bench",
		Kind:          "benchmark_threshold",
		Command:       "sh",
		Args:          []string{"-c", `printf 'p95_ms=71.2\n'`},
		Pattern:       `p95_ms=([0-9.]+)`,
		Threshold:     50,
		ThresholdMode: "max",
		Workdir:       root,
	})
	if fail.Status != VerificationStatusFailed || fail.FailureCode != "benchmark_threshold_exceeded" {
		t.Fatalf("expected failed benchmark threshold, got %#v", fail)
	}
}

func runGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return output
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
