package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestExitCodeForAppError(t *testing.T) {
	err := appError(ExitArtifactNotFound, "missing", "hint", nil)
	if got := ExitCodeForError(err); got != ExitArtifactNotFound {
		t.Fatalf("unexpected exit code: %d", got)
	}
	if got := FormatError(err, false); !strings.Contains(got, "missing") || !strings.Contains(got, "hint: hint") {
		t.Fatalf("unexpected formatted error: %s", got)
	}
}

func TestRunDryRunDoesNotCreateArtifacts(t *testing.T) {
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
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}
	cmd := newRunCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"run", "--config", configPath, "--task", "dry run task", "--dry-run", "--format", "json"}); err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	var payload dryRunPlan
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run json: %v\n%s", err, stdout.String())
	}
	if payload.Mode == "" || len(payload.Agents) == 0 || payload.Output.ResultPath == "" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if _, err := os.Stat(filepath.Join(tmp, "out")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create out dir, stat err=%v", err)
	}
}

func TestConfigRenderAndExplain(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}
	renderCmd := newConfigRenderCommand()
	var renderOut bytes.Buffer
	renderCmd.Writer = &renderOut
	if err := renderCmd.Run(context.Background(), []string{"render", "--config", configPath, "--format", "json"}); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(renderOut.String(), `"providers"`) {
		t.Fatalf("unexpected render output: %s", renderOut.String())
	}

	explainCmd := newConfigExplainCommand()
	var explainOut bytes.Buffer
	explainCmd.Writer = &explainOut
	if err := explainCmd.Run(context.Background(), []string{"explain", "--config", configPath}); err != nil {
		t.Fatalf("explain failed: %v", err)
	}
	for _, needle := range []string{"Providers", "Agents", "Roles", "Output"} {
		if !strings.Contains(explainOut.String(), needle) {
			t.Fatalf("explain output missing %q:\n%s", needle, explainOut.String())
		}
	}
}

func TestArtifactListAndShow(t *testing.T) {
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
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}
	runCmd := newRunCommand()
	var runOut bytes.Buffer
	runCmd.Writer = &runOut
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--task", "artifact task"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	resultPath := tryExtractResultPath(runOut.String())
	if resultPath == "" {
		t.Fatalf("missing result path:\n%s", runOut.String())
	}

	listCmd := newArtifactListCommand()
	var listOut bytes.Buffer
	listCmd.Writer = &listOut
	if err := listCmd.Run(context.Background(), []string{"list", "--result", resultPath, "--type", "telemetry"}); err != nil {
		t.Fatalf("artifact list failed: %v", err)
	}
	if !strings.Contains(listOut.String(), "run-telemetry.json") {
		t.Fatalf("unexpected artifact list:\n%s", listOut.String())
	}

	showCmd := newArtifactShowCommand()
	var showOut bytes.Buffer
	showCmd.Writer = &showOut
	if err := showCmd.Run(context.Background(), []string{"show", "--result", resultPath, "--path", "artifacts/run-telemetry.json"}); err != nil {
		t.Fatalf("artifact show failed: %v", err)
	}
	if !strings.Contains(showOut.String(), `"requestId"`) {
		t.Fatalf("unexpected artifact show:\n%s", showOut.String())
	}
}

func TestDoctorMockConfig(t *testing.T) {
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
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}
	cmd := newDoctorCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"doctor", "--config", configPath, "--format", "json"}); err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor output: %v\n%s", err, stdout.String())
	}
	if report.Summary.Fail != 0 || report.Summary.OK == 0 {
		t.Fatalf("unexpected doctor report: %#v", report.Summary)
	}
}

func TestShortcutRunCommandsDryRun(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}

	cases := []struct {
		name string
		cmd  *cli.Command
		args []string
		mode string
	}{
		{name: "ask", cmd: newAskCommand(), args: []string{"ask", "--config", configPath, "--dry-run", "--format", "json", "shortcut ask"}, mode: "adjudication"},
		{name: "debate", cmd: newDebateCommand(), args: []string{"debate", "--config", configPath, "--dry-run", "--format", "json", "shortcut debate"}, mode: "free_debate"},
		{name: "delphi", cmd: newDelphiCommand(), args: []string{"delphi", "--config", configPath, "--dry-run", "--format", "json", "shortcut delphi"}, mode: "delphi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			tc.cmd.Writer = &stdout
			if err := tc.cmd.Run(context.Background(), tc.args); err != nil {
				t.Fatalf("%s failed: %v\n%s", tc.name, err, stdout.String())
			}
			var payload dryRunPlan
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("decode dry-run json: %v\n%s", err, stdout.String())
			}
			if string(payload.Mode) != tc.mode {
				t.Fatalf("unexpected mode: %s", payload.Mode)
			}
			if tc.mode != "adjudication" && len(payload.Roles.Participants) < 2 {
				t.Fatalf("expected inferred participants: %#v", payload.Roles)
			}
		})
	}
}

func TestSetupGeneratesSplitConfig(t *testing.T) {
	tmp := t.TempDir()
	cmd := newSetupCommand()
	var stdout bytes.Buffer
	cmd.Writer = &stdout
	if err := cmd.Run(context.Background(), []string{"setup", "--dir", tmp, "--mode", "delphi", "--force"}); err != nil {
		t.Fatalf("setup failed: %v\n%s", err, stdout.String())
	}
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	for _, path := range []string{configPath, filepath.Join(tmp, "conf", "providers.yaml"), filepath.Join(tmp, "conf", "profiles.yaml")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected setup file %s: %v", path, err)
		}
	}
	validateCmd := newConfigValidateCommand()
	var validateOut bytes.Buffer
	validateCmd.Writer = &validateOut
	if err := validateCmd.Run(context.Background(), []string{"validate", "--config", configPath}); err != nil {
		t.Fatalf("generated config should validate: %v\n%s", err, validateOut.String())
	}
	explainCmd := newConfigExplainCommand()
	var explainOut bytes.Buffer
	explainCmd.Writer = &explainOut
	if err := explainCmd.Run(context.Background(), []string{"explain", "--config", configPath}); err != nil {
		t.Fatalf("explain generated config: %v", err)
	}
	if !strings.Contains(explainOut.String(), "profile: default") || !strings.Contains(explainOut.String(), "mode: delphi") {
		t.Fatalf("unexpected generated explain:\n%s", explainOut.String())
	}
}

func TestInspectAndLogsShortcuts(t *testing.T) {
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
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("init config: %v", err)
	}
	runCmd := newRunCommand()
	var runOut bytes.Buffer
	runCmd.Writer = &runOut
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--task", "inspect task"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	resultPath := tryExtractResultPath(runOut.String())
	if resultPath == "" {
		t.Fatalf("missing result path:\n%s", runOut.String())
	}

	inspectCmd := newInspectCommand()
	var inspectOut bytes.Buffer
	inspectCmd.Writer = &inspectOut
	if err := inspectCmd.Run(context.Background(), []string{"inspect", "--result", resultPath, "--section", "overview"}); err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "requestId:") {
		t.Fatalf("unexpected inspect output:\n%s", inspectOut.String())
	}

	lastCmd := newLastCommand()
	var lastOut bytes.Buffer
	lastCmd.Writer = &lastOut
	if err := lastCmd.Run(context.Background(), []string{"last", "--config", configPath, "--section", "overview"}); err != nil {
		t.Fatalf("last failed: %v", err)
	}
	if !strings.Contains(lastOut.String(), "requestId:") {
		t.Fatalf("unexpected last output:\n%s", lastOut.String())
	}

	logsCmd := newLogsCommand()
	var logsOut bytes.Buffer
	logsCmd.Writer = &logsOut
	if err := logsCmd.Run(context.Background(), []string{"logs", "--result", resultPath, "--type", "telemetry"}); err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	if !strings.Contains(logsOut.String(), "run-telemetry.json") {
		t.Fatalf("unexpected logs output:\n%s", logsOut.String())
	}
}
