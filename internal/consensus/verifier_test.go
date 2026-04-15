package consensus

import (
	"context"
	"os"
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
