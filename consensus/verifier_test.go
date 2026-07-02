package consensus

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
					AllowedPaths: []string{"consensus"},
				},
			},
		},
		SessionID: "session-1",
		Claim: ClaimNode{
			ClaimID: "claim-1",
			Metadata: map[string]any{
				"touchedPaths": []string{"consensus/engine.go", "cmd/til-consensus/main.go"},
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

type semanticStubDelegate struct {
	output      TaskResult
	outputs     []TaskResult
	awaitErr    error
	awaitErrs   []error
	dispatchErr error
	awaited     bool
}

func (d *semanticStubDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	if d.dispatchErr != nil {
		return DispatchReceipt{}, d.dispatchErr
	}
	return DispatchReceipt{TaskID: "task-1", AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *semanticStubDelegate) Await(_ context.Context, _ string, _ time.Duration) (AwaitedTask, error) {
	d.awaited = true
	if len(d.awaitErrs) > 0 {
		err := d.awaitErrs[0]
		d.awaitErrs = d.awaitErrs[1:]
		if err != nil {
			return AwaitedTask{}, err
		}
	}
	if d.awaitErr != nil {
		return AwaitedTask{}, d.awaitErr
	}
	if len(d.outputs) > 0 {
		output := d.outputs[0]
		d.outputs = d.outputs[1:]
		return AwaitedTask{
			OK:       true,
			Output:   output,
			Artifact: &ArtifactRef{Path: "artifact.log"},
		}, nil
	}
	return AwaitedTask{
		OK:       true,
		Output:   d.output,
		Artifact: &ArtifactRef{Path: "artifact.log"},
	}, nil
}

func (d *semanticStubDelegate) Cancel(_ context.Context, _ string) error { return nil }

func TestVerifierRunIncludesUnsupportedAndSemanticChecks(t *testing.T) {
	delegate := &semanticStubDelegate{
		output: SemanticVerificationTaskResult{Output: SemanticVerificationOutput{
			Results: []SemanticVerificationFinding{{
				ClaimID:    "claim-1",
				Verdict:    ClaimVerdictSupported,
				Confidence: 0.8,
				Rationale:  "semantic support",
			}},
		}},
	}
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		TaskDelegate:   delegate,
		IDFactory:      &deterministicIDs{},
		PerTaskTimeout: time.Second,
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec:  TaskSpec{Goal: "verify"},
			Roles: RoleAssignments{
				SemanticVerifier: "verifier-a",
			},
			VerificationPolicy: VerificationPolicy{
				AllowSemanticVerifier: true,
				RequiredChecks: []VerificationCheck{
					{Name: "unknown", Kind: "unknown"},
				},
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	results, err := verifier.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two verification results, got %#v", results)
	}
	if !strings.Contains(results[0].Summary, "unsupported verification check kind") || results[1].VerdictSuggestion != ClaimVerdictSupported || !delegate.awaited {
		t.Fatalf("unexpected verification results: %#v", results)
	}
}

func TestRunSemanticVerificationErrorPaths(t *testing.T) {
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec:  TaskSpec{Goal: "verify"},
			Roles: RoleAssignments{
				SemanticVerifier: "verifier-a",
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	tests := []struct {
		name        string
		delegate    *semanticStubDelegate
		failureCode string
	}{
		{
			name: "dispatch failed",
			delegate: &semanticStubDelegate{
				dispatchErr: errors.New("dispatch boom"),
			},
			failureCode: "semantic_dispatch_failed",
		},
		{
			name: "await failed",
			delegate: &semanticStubDelegate{
				awaitErr: errors.New("await boom"),
			},
			failureCode: "semantic_await_failed",
		},
		{
			name: "type mismatch",
			delegate: &semanticStubDelegate{
				output: ProposalTaskResult{},
			},
			failureCode: "semantic_type_mismatch",
		},
		{
			name: "empty result",
			delegate: &semanticStubDelegate{
				output: SemanticVerificationTaskResult{Output: SemanticVerificationOutput{}},
			},
			failureCode: "semantic_empty_result",
		},
		{
			name: "claim mismatch",
			delegate: &semanticStubDelegate{
				output: SemanticVerificationTaskResult{Output: SemanticVerificationOutput{
					Results: []SemanticVerificationFinding{{
						ClaimID:   "claim-else",
						Verdict:   ClaimVerdictSupported,
						Rationale: "wrong claim",
					}},
				}},
			},
			failureCode: "semantic_claim_mismatch",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifier := NewCompositeVerifier(CompositeVerifierDeps{
				TaskDelegate:   tc.delegate,
				IDFactory:      &deterministicIDs{},
				PerTaskTimeout: time.Second,
			})
			results := verifier.runSemanticVerification(context.Background(), req)
			if len(results) != 1 || results[0].FailureCode != tc.failureCode {
				t.Fatalf("unexpected semantic verification result: %#v", results)
			}
		})
	}
}

func TestRunSemanticVerificationRetriesOnce(t *testing.T) {
	delegate := &semanticStubDelegate{
		awaitErrs: []error{errors.New("temporary failure"), nil},
		outputs: []TaskResult{
			SemanticVerificationTaskResult{Output: SemanticVerificationOutput{
				Results: []SemanticVerificationFinding{{
					ClaimID:    "claim-1",
					Verdict:    ClaimVerdictSupported,
					Rationale:  "retry succeeded",
					Confidence: 0.9,
				}},
			}},
		},
	}
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		TaskDelegate:   delegate,
		IDFactory:      &deterministicIDs{},
		PerTaskTimeout: time.Second,
		RetryAttempts:  1,
	})
	results := verifier.runSemanticVerification(context.Background(), VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec:  TaskSpec{Goal: "verify"},
			Roles: RoleAssignments{
				SemanticVerifier: "verifier-a",
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	})
	if len(results) != 1 || results[0].Status != VerificationStatusPassed {
		t.Fatalf("expected retry to succeed, got %#v", results)
	}
}

func TestVerifierHelperFunctions(t *testing.T) {
	verifier := NewCompositeVerifier(CompositeVerifierDeps{
		IDFactory: &deterministicIDs{},
	})
	req := VerificationRequest{
		Request: StartRequest{
			RequestID: "req-1",
			TaskSpec: TaskSpec{
				Goal: "verify",
				WorkspaceSnapshot: &WorkspaceSnapshot{
					Root: "/tmp/work",
				},
			},
		},
		SessionID: "session-1",
		Claim:     ClaimNode{ClaimID: "claim-1"},
	}
	result := verifier.runCheck(context.Background(), req, VerificationCheck{Name: "unknown", Kind: "unknown"})
	if result.Status != VerificationStatusInconclusive {
		t.Fatalf("expected unsupported check to be inconclusive, got %#v", result)
	}

	if got := classifyCommandFailure(&exec.ExitError{}); got != "command_exit_nonzero" {
		t.Fatalf("unexpected exit error classification: %s", got)
	}
	if got := classifyCommandFailure(errors.New("boom")); got != "command_exec_failed" {
		t.Fatalf("unexpected exec error classification: %s", got)
	}

	meta := MarshalVerificationMetadata(struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{"bench", 2})
	if meta["name"] != "bench" || meta["count"] != float64(2) {
		t.Fatalf("unexpected marshaled metadata: %#v", meta)
	}

	if _, err := readArtifactBody(nil); err == nil {
		t.Fatal("expected nil artifact to fail")
	}

	env := renderVerificationEnv(req, map[string]string{"EXTRA": "1"})
	joined := strings.Join(env, "|")
	if !strings.Contains(joined, "TIL_CONSENSUS_WORKSPACE_ROOT=/tmp/work") || !strings.Contains(joined, "EXTRA=1") {
		t.Fatalf("unexpected rendered env: %v", env)
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
