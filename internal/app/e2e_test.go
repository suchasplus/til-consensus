package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
)

func TestE2EResumeSessionFromCheckpoint(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	writeFile(t, configPath, fmt.Sprintf(`schema_version: 1
defaults:
  per_task_timeout: 1s
  task_retry_attempts: 0
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 1
  verification_policy:
    max_parallel_checks: 1
  arbiter_policy:
    allow_undetermined: true
    blind_review: true
output:
  directory: %q
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
    participants:
      arbiter-a:
        arbiter:
          behavior: error
          error: arbiter unavailable
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
    arbiter: arbiter-a
`, filepath.Join(tmp, "out", "{requestId}")))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runCmd := newRunCommand()
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--task", "判断补丁是否真的修复问题"})
	if err == nil {
		t.Fatal("expected initial run to fail")
	}

	sessionDir := filepath.Join(tmp, "out", "_sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("read session dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one session snapshot, got %d", len(entries))
	}
	sessionID := strings.TrimSuffix(entries[0].Name(), filepath.Ext(entries[0].Name()))

	sessionListCmd := newSessionCommand().Commands[0]
	sessionListCmd.Writer = &stdout
	sessionListCmd.ErrWriter = &stderr
	stdout.Reset()
	stderr.Reset()
	if err := sessionListCmd.Run(context.Background(), []string{"list", "--config", configPath}); err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	var listed []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode session list failed: %v\n%s", err, stdout.String())
	}
	if len(listed) != 1 {
		t.Fatalf("expected one listed session, got %#v", listed)
	}
	stdout.Reset()
	stderr.Reset()
	sessionShowCmd := newSessionCommand().Commands[1]
	sessionShowCmd.Writer = &stdout
	sessionShowCmd.ErrWriter = &stderr
	if err := sessionShowCmd.Run(context.Background(), []string{"show", "--config", configPath, "--session-id", sessionID}); err != nil {
		t.Fatalf("session show failed: %v", err)
	}
	var shown consensus.SessionSnapshot
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("decode session show failed: %v\n%s", err, stdout.String())
	}
	if shown.SessionID != sessionID {
		t.Fatalf("unexpected session show payload: %#v", shown)
	}

	writeFile(t, configPath, fmt.Sprintf(`schema_version: 1
defaults:
  per_task_timeout: 1s
  task_retry_attempts: 0
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 1
  verification_policy:
    max_parallel_checks: 1
  arbiter_policy:
    allow_undetermined: true
    blind_review: true
output:
  directory: %q
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
    arbiter: arbiter-a
`, filepath.Join(tmp, "out", "{requestId}")))

	stdout.Reset()
	stderr.Reset()
	runCmd = newRunCommand()
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--resume-session", sessionID}); err != nil {
		t.Fatalf("resume run failed: %v\nstderr=%s", err, stderr.String())
	}
	viewCmd := newViewCommand()
	viewCmd.Writer = &stdout
	viewCmd.ErrWriter = &stderr
	stdout.Reset()
	if err := viewCmd.Run(context.Background(), []string{"view", "--config", configPath}); err != nil {
		t.Fatalf("view after resume failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "关键 Claims") {
		t.Fatalf("expected rendered result after resume, got %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	runCmd = newRunCommand()
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--replay-session", sessionID}); err != nil {
		t.Fatalf("replay run failed: %v\nstderr=%s", err, stderr.String())
	}
	replayResultPath := extractResultPath(t, stdout.String())
	stdout.Reset()
	if err := viewCmd.Run(context.Background(), []string{"view", "--result", replayResultPath}); err != nil {
		t.Fatalf("view after replay failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "parent: request=") {
		t.Fatalf("expected replay lineage in view output, got %s", stdout.String())
	}
}

func TestE2EFollowupAndObservationSections(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	writeFile(t, configPath, fmt.Sprintf(`schema_version: 1
defaults:
  per_task_timeout: 1s
  task_retry_attempts: 0
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 1
  verification_policy:
    max_parallel_checks: 1
  arbiter_policy:
    allow_undetermined: true
    blind_review: true
  observe_policy:
    on_contradiction: reopen
    sources:
      - name: contradiction
        command: sh
        args:
          - -c
          - printf '{"status":{"contradicted":true},"summary":"post-action contradiction"}'
        parsing:
          mode: json
          failure_path: status.contradicted
          summary_path: summary
          required_paths:
            - status.contradicted
            - summary
output:
  directory: %q
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
`, filepath.Join(tmp, "out", "{requestId}")))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runCmd := newRunCommand()
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--task", "Should we use a monorepo or polyrepo?"}); err != nil {
		t.Fatalf("run failed: %v\nstderr=%s", err, stderr.String())
	}
	viewCmd := newViewCommand()
	viewCmd.Writer = &stdout
	viewCmd.ErrWriter = &stderr
	stdout.Reset()
	if err := viewCmd.Run(context.Background(), []string{"view", "--config", configPath, "--section", "observations", "--section", "followups", "--verbose"}); err != nil {
		t.Fatalf("view failed: %v", err)
	}
	rendered := stdout.String()
	if !strings.Contains(rendered, "Observations") || !strings.Contains(rendered, "Follow-ups") || !strings.Contains(rendered, "triggered by observation") {
		t.Fatalf("unexpected view output: %s", rendered)
	}

	artifacts, err := filepath.Glob(filepath.Join(tmp, "out", "*", "artifacts", "followups", "*.json"))
	if err != nil {
		t.Fatalf("glob followup artifacts: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("expected followup artifact to be created")
	}
	stdout.Reset()
	stderr.Reset()
	runCmd = newRunCommand()
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--followup", artifacts[0]}); err != nil {
		t.Fatalf("run --followup failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "run completed") {
		t.Fatalf("expected run --followup to complete, got %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	followupCmd := newFollowUpCommand()
	followupCmd.Writer = &stdout
	followupCmd.ErrWriter = &stderr
	if err := followupCmd.Run(context.Background(), []string{"followup", "run", "--config", configPath, "--artifact", artifacts[0]}); err != nil {
		t.Fatalf("followup run failed: %v\nstderr=%s", err, stderr.String())
	}
	viewCmd = newViewCommand()
	viewCmd.Writer = &stdout
	viewCmd.ErrWriter = &stderr
	stdout.Reset()
	if err := viewCmd.Run(context.Background(), []string{"view", "--config", configPath, "--section", "followups", "--verbose"}); err != nil {
		t.Fatalf("view followup child failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "parent request=") {
		t.Fatalf("expected child lineage in view output, got %s", stdout.String())
	}
}

func TestE2EMultiModeSmoke(t *testing.T) {
	for _, tc := range []struct {
		name   string
		preset string
		args   []string
		expect string
	}{
		{
			name:   "free debate",
			preset: "debate",
			args:   []string{"til-consensus", "run", "--config", "", "--mode", "free-debate", "--task", "Should we use a monorepo or polyrepo?"},
			expect: "Rounds",
		},
		{
			name:   "delphi",
			preset: "delphi",
			args:   []string{"til-consensus", "run", "--config", "", "--mode", "delphi", "--task", "Should we use a monorepo or polyrepo?"},
			expect: "Convergence",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			if err := runConfigInitCommand(&bytes.Buffer{}, configPath, tc.preset, "", "", "", false, false); err != nil {
				t.Fatalf("init %s config failed: %v", tc.preset, err)
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runCmd := newRunCommand()
			runCmd.Writer = &stdout
			runCmd.ErrWriter = &stderr
			args := append([]string(nil), tc.args...)
			args[3] = configPath
			args[0] = "run"
			if err := runCmd.Run(context.Background(), args); err != nil {
				t.Fatalf("run %s failed: %v\nstderr=%s", tc.name, err, stderr.String())
			}
			viewCmd := newViewCommand()
			viewCmd.Writer = &stdout
			viewCmd.ErrWriter = &stderr
			stdout.Reset()
			if err := viewCmd.Run(context.Background(), []string{"view", "--config", configPath}); err != nil {
				t.Fatalf("view %s failed: %v", tc.name, err)
			}
			if !strings.Contains(stdout.String(), tc.expect) {
				t.Fatalf("expected %q in view output, got %s", tc.expect, stdout.String())
			}
		})
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
