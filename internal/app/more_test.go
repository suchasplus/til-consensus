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
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	filestore "github.com/suchasplus/til-consensus/store/file"
	"github.com/urfave/cli/v3"
)

func TestNewBuildsCommandTree(t *testing.T) {
	cmd := New()
	if cmd.Name != "til-consensus" {
		t.Fatalf("unexpected root name: %s", cmd.Name)
	}
	if len(cmd.Commands) != 20 {
		t.Fatalf("unexpected root command count: %d", len(cmd.Commands))
	}
	if cmd.Version == "" {
		t.Fatal("expected root version to be populated")
	}
	names := make([]string, 0, len(cmd.Commands))
	for _, child := range cmd.Commands {
		names = append(names, child.Name)
	}
	if strings.Join(names, ",") != "run,ask,debate,delphi,classify,setup,followup,config,profile,doctor,artifact,last,inspect,logs,open,telemetry,act,session,view,version" {
		t.Fatalf("unexpected command tree: %#v", names)
	}
}

func TestVersionFlagPrintsFullBuildInfo(t *testing.T) {
	cmd := New()
	var flagOut bytes.Buffer
	cmd.Writer = &flagOut
	cli.ShowVersion(cmd)
	assertFullVersionOutput(t, "--version", flagOut.String())

	versionCmd := newVersionCommand()
	var subcommandOut bytes.Buffer
	versionCmd.Writer = &subcommandOut
	if err := versionCmd.Run(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	assertFullVersionOutput(t, "version", subcommandOut.String())
}

func assertFullVersionOutput(t *testing.T, label string, output string) {
	t.Helper()
	for _, needle := range []string{
		"version:",
		"commit:",
		"build time:",
		"build machine:",
		"dirty:",
		"go version:",
		"goos:",
		"goarch:",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("version output for %s missing %q:\n%s", label, needle, output)
		}
	}
}

func TestConfigCommandsAndActCommand(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	var initOut bytes.Buffer
	if err := runConfigInitCommand(&initOut, configPath, "quickstart", "", "", "", false, false); err != nil {
		t.Fatalf("runConfigInitCommand failed: %v", err)
	}

	validateCmd := newConfigValidateCommand()
	var validateOut bytes.Buffer
	validateCmd.Writer = &validateOut
	if err := validateCmd.Run(context.Background(), []string{"validate", "--config", configPath}); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !strings.Contains(validateOut.String(), "config is valid") {
		t.Fatalf("unexpected validate output: %s", validateOut.String())
	}

	addProviderCmd := newConfigAddProviderCommand()
	var providerOut bytes.Buffer
	addProviderCmd.Writer = &providerOut
	if err := addProviderCmd.Run(context.Background(), []string{
		"add-provider",
		"--config", configPath,
		"--id", "api1",
		"--type", "api",
		"--model-id", "general",
		"--provider-model", "gpt-5",
		"--context-window", "128000",
		"--max-output-tokens", "4096",
		"--protocol", "openai-compatible",
		"--base-url", "https://example.com/v1",
		"--api-key-env", "OPENAI_API_KEY",
		"--header", "X-Test=1",
		"--env", "DEBUG=true",
		"--option", "retries=3",
		"--temperature", "0.2",
		"--reasoning", "medium",
		"--agent", "api-agent",
	}); err != nil {
		t.Fatalf("add-provider failed: %v", err)
	}

	addAgentCmd := newConfigAddAgentCommand()
	var agentOut bytes.Buffer
	addAgentCmd.Writer = &agentOut
	if err := addAgentCmd.Run(context.Background(), []string{
		"add-agent",
		"--config", configPath,
		"--id", "actor-b",
		"--provider", "mock",
		"--assign", "actor",
		"--assign", "reporter",
		"--temperature", "0.5",
		"--reasoning", "high",
	}); err != nil {
		t.Fatalf("add-agent failed: %v", err)
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if _, ok := loaded.Config.Providers["api1"]; !ok {
		t.Fatalf("expected api1 provider in config: %#v", loaded.Config.Providers)
	}
	if loaded.Config.Providers["api1"].Models["general"].ContextWindow != 128000 ||
		loaded.Config.Providers["api1"].Models["general"].MaxOutputTokens != 4096 {
		t.Fatalf("expected provider model sizing to be written: %#v", loaded.Config.Providers["api1"].Models["general"])
	}
	if loaded.Config.Roles.Actor != "actor-b" || loaded.Config.Roles.Reporter != "actor-b" {
		t.Fatalf("unexpected role assignment: %#v", loaded.Config.Roles)
	}

	resultPath := filepath.Join(tmp, "result.json")
	body, err := json.Marshal(consensus.AdjudicationResult{
		SchemaVersion: 1,
		RequestID:     "req-1",
		SessionID:     "session-1",
		TaskSpec:      consensus.TaskSpec{Goal: "generate next steps"},
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := os.WriteFile(resultPath, body, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}
	actCmd := newActCommand()
	var actOut bytes.Buffer
	actCmd.Writer = &actOut
	if err := actCmd.Run(context.Background(), []string{
		"act",
		"--config", configPath,
		"--result", resultPath,
		"--task", "给出下一步修复计划",
		"--agent", "actor-b",
	}); err != nil {
		t.Fatalf("act failed: %v", err)
	}
	if !strings.Contains(actOut.String(), "Action completed by actor-b") {
		t.Fatalf("unexpected act output: %s", actOut.String())
	}
}

func TestParseHelpersAndOutputHelpers(t *testing.T) {
	assignments, err := parseStringAssignments([]string{"A=1", "B=two"})
	if err != nil {
		t.Fatalf("parseStringAssignments failed: %v", err)
	}
	if assignments["A"] != "1" || assignments["B"] != "two" {
		t.Fatalf("unexpected assignments: %#v", assignments)
	}
	if _, err := parseStringAssignments([]string{"broken"}); err == nil {
		t.Fatal("expected invalid string assignment to fail")
	}

	values, err := parseAnyAssignments([]string{`obj={"a":1}`, "flag=true", "count=3", "ratio=1.5", "raw=text"})
	if err != nil {
		t.Fatalf("parseAnyAssignments failed: %v", err)
	}
	if values["flag"] != true || values["count"] != int64(3) {
		t.Fatalf("unexpected parsed values: %#v", values)
	}
	if values["ratio"] != 1.5 || values["raw"] != "text" {
		t.Fatalf("unexpected scalar parsing: %#v", values)
	}
	if _, err := parseAnyAssignments([]string{"broken"}); err == nil {
		t.Fatal("expected invalid any assignment to fail")
	}

	if !isSupportedClaimVerdict(string(consensus.ClaimVerdictSupported)) || isSupportedClaimVerdict("bad") {
		t.Fatal("unexpected claim verdict filter behavior")
	}
	if !isSupportedViewFormat("json") || isSupportedViewFormat("bad") {
		t.Fatal("unexpected view format behavior")
	}
	if !isSupportedViewSection("artifacts") || isSupportedViewSection("bad") {
		t.Fatal("unexpected view section behavior")
	}
	if got := splitComma("a, b , ,c"); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("unexpected splitComma result: %#v", got)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	output := NewOutput(&stdout, &stderr, false, false, "")
	output.Errorf("boom: %d", 1)
	if !strings.Contains(stderr.String(), "boom: 1") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if got := compactPayload(map[string]any{"a": 1, "b": "two"}); !strings.Contains(got, "a=1") || !strings.Contains(got, "b=two") {
		t.Fatalf("unexpected compact payload: %s", got)
	}
	if got := formatPhaseChanged(consensus.SessionPhaseVerify); got != "phase: verify (running verification)" {
		t.Fatalf("unexpected phase text: %s", got)
	}
	if err := output.EventObserver().OnEvent(context.Background(), consensus.RunEvent{
		Type:  consensus.RunEventPhaseChanged,
		Phase: consensus.SessionPhaseChallenge,
	}); err != nil {
		t.Fatalf("OnEvent phase change failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "phase: challenge (collecting challenges)") {
		t.Fatalf("unexpected stdout for phase change: %s", stdout.String())
	}
	if err := output.EventObserver().OnEvent(context.Background(), consensus.RunEvent{
		Type: consensus.RunEventTaskDispatched,
		Payload: map[string]any{
			"agentId":     "proposer-codex",
			"taskKind":    "propose",
			"attempt":     1,
			"maxAttempts": 2,
		},
	}); err != nil {
		t.Fatalf("OnEvent task dispatched failed: %v", err)
	}
	if err := output.EventObserver().OnEvent(context.Background(), consensus.RunEvent{
		Type: consensus.RunEventTaskRetrying,
		Payload: map[string]any{
			"agentId":     "challenger-claude",
			"taskKind":    "challenge",
			"error":       "__timeout__",
			"attempt":     2,
			"maxAttempts": 2,
		},
	}); err != nil {
		t.Fatalf("OnEvent task retrying failed: %v", err)
	}
	if err := output.EventObserver().OnEvent(context.Background(), consensus.RunEvent{
		Type: consensus.RunEventTaskFailed,
		Payload: map[string]any{
			"agentId":     "challenger-claude",
			"taskKind":    "challenge",
			"error":       "__timeout__",
			"attempt":     1,
			"maxAttempts": 2,
		},
	}); err != nil {
		t.Fatalf("OnEvent task failed failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "task dispatched: proposer-codex -> propose (collecting claims) attempt=1/2") {
		t.Fatalf("unexpected stdout for task dispatch: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "task retrying: challenger-claude -> challenge (collecting challenges) attempt=2/2 reason=__timeout__") {
		t.Fatalf("unexpected stdout for task retry: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "task failed: challenger-claude -> challenge (collecting challenges) attempt=1/2 error=__timeout__") {
		t.Fatalf("unexpected stdout for task failure: %s", stdout.String())
	}
}

func TestResolveTaskOverride(t *testing.T) {
	tmp := t.TempDir()
	taskFile := filepath.Join(tmp, "task.txt")
	const body = "第一行任务描述\n\n第二行补充背景\n"
	if err := os.WriteFile(taskFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	task, err := resolveTaskOverride("", taskFile)
	if err != nil {
		t.Fatalf("resolveTaskOverride failed: %v", err)
	}
	if task != body {
		t.Fatalf("expected full task file contents, got %q", task)
	}

	if _, err := resolveTaskOverride("inline", taskFile); err == nil || !strings.Contains(err.Error(), "--task 不能与 --task-file 同时使用") {
		t.Fatalf("expected task/task-file conflict, got %v", err)
	}

	emptyFile := filepath.Join(tmp, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte(" \n\t "), 0o644); err != nil {
		t.Fatalf("write empty task file: %v", err)
	}
	if _, err := resolveTaskOverride("", emptyFile); err == nil || !strings.Contains(err.Error(), "task file is empty") {
		t.Fatalf("expected empty task file error, got %v", err)
	}
}

func TestParseModeEmptyDoesNotForceAdjudication(t *testing.T) {
	if got := parseMode(""); got != "" {
		t.Fatalf("expected empty mode when flag is absent, got %q", got)
	}
	if got := parseMode("adjudication"); got != consensus.WorkflowModeAdjudication {
		t.Fatalf("unexpected adjudication parse result: %q", got)
	}
	if got := parseMode("free-debate"); got != consensus.WorkflowModeFreeDebate {
		t.Fatalf("unexpected free-debate parse result: %q", got)
	}
	if got := parseMode("delphi"); got != consensus.WorkflowModeDelphi {
		t.Fatalf("unexpected delphi parse result: %q", got)
	}
}

func TestRunCommandDoesNotOverrideDelphiModeWhenFlagAbsent(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-delphi.yaml")
	inputPath := filepath.Join(tmp, "delphi.run.yaml")
	writeFile(t, configPath, fmt.Sprintf(`schema_version: 1
defaults:
  mode: delphi
  per_task_timeout: 1s
  task_retry_attempts: 0
  delphi_policy:
    min_rounds: 2
    max_rounds: 2
output:
  directory: %q
providers:
  mock:
    type: mock
    models:
      default:
        provider_model: mock
agents:
  - id: participant-a
    provider: mock
    model: default
    role: participant
  - id: participant-b
    provider: mock
    model: default
    role: participant
  - id: facilitator-a
    provider: mock
    model: default
    role: facilitator
  - id: reporter-a
    provider: mock
    model: default
    role: reporter
roles:
  delphi:
    participants: [participant-a, participant-b]
    facilitator: facilitator-a
    reporter: reporter-a
`, filepath.Join(tmp, "out", "{requestId}")))
	writeFile(t, inputPath, `request_id: delphi-run-001
mode: delphi
task_spec:
  goal: 评估是否应迁移 CI 平台
roles:
  delphi:
    participants: [participant-a, participant-b]
    facilitator: facilitator-a
    reporter: reporter-a
delphi_policy:
  min_rounds: 2
  max_rounds: 2
`)

	runCmd := newRunCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runCmd.Writer = &stdout
	runCmd.ErrWriter = &stderr
	if err := runCmd.Run(context.Background(), []string{
		"run",
		"--config", configPath,
		"--input", inputPath,
	}); err != nil {
		t.Fatalf("expected delphi run to succeed without --mode override, got %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mode: delphi") {
		t.Fatalf("expected delphi mode in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "phase: delphi_questionnaire") {
		t.Fatalf("expected delphi workflow phases in output, got %s", stdout.String())
	}
}

func TestRunCommandTaskFileOverridesInputGoal(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	taskFile := filepath.Join(tmp, "task.txt")
	inputPath := filepath.Join(tmp, "run.yaml")

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
	writeFile(t, taskFile, "来自 task-file 的完整任务\n第二行也要保留\n")
	writeFile(t, inputPath, `task_spec:
  goal: from-input
`)

	runCmd := newRunCommand()
	stdout, stderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--input", inputPath, "--task-file", taskFile})
	if err != nil {
		t.Fatalf("run failed: %v\nstderr=%s", err, stderr)
	}
	resultPath := extractResultPath(t, stdout)
	data, readErr := os.ReadFile(resultPath)
	if readErr != nil {
		t.Fatalf("read result: %v", readErr)
	}
	var result struct {
		TaskSpec struct {
			Goal string `json:"goal"`
		} `json:"taskSpec"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.TaskSpec.Goal != "来自 task-file 的完整任务\n第二行也要保留" {
		t.Fatalf("unexpected goal: %q", result.TaskSpec.Goal)
	}
}

func TestVerboseOutputShowsDurationsAndPhaseSummaries(t *testing.T) {
	var stdout bytes.Buffer
	output := NewOutput(&stdout, &bytes.Buffer{}, true, false, "")
	observer := output.EventObserver()
	base := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	mustEvent := func(event consensus.RunEvent) {
		t.Helper()
		if err := observer.OnEvent(context.Background(), event); err != nil {
			t.Fatalf("OnEvent failed: %v", err)
		}
	}

	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventPhaseChanged,
		Phase: consensus.SessionPhaseRevise,
		At:    base.Format(time.RFC3339Nano),
	})
	mustEvent(consensus.RunEvent{
		Type: consensus.RunEventTaskDispatched,
		At:   base.Add(time.Second).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"agentId":     "proposer-a",
			"taskKind":    "revise",
			"attempt":     1,
			"maxAttempts": 2,
		},
	})
	mustEvent(consensus.RunEvent{
		Type: consensus.RunEventTaskCompleted,
		At:   base.Add(3 * time.Second).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"agentId":     "proposer-a",
			"taskKind":    "revise",
			"attempt":     1,
			"maxAttempts": 2,
		},
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventClaimRevised,
		Phase: consensus.SessionPhaseRevise,
		At:    base.Add(3200 * time.Millisecond).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"claimId":         "claim_1",
			"action":          "downgrade_confidence",
			"confidenceDelta": -0.2,
			"reason":          "Need more evidence",
		},
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventPhaseChanged,
		Phase: consensus.SessionPhaseAdjudicate,
		At:    base.Add(4 * time.Second).Format(time.RFC3339Nano),
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventClaimAdjudicated,
		Phase: consensus.SessionPhaseAdjudicate,
		At:    base.Add(4500 * time.Millisecond).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"claimId":         "claim_1",
			"disposition":     "keep_with_caveat",
			"verdict":         "supported",
			"finalConfidence": 0.62,
			"reason":          "evidence is mixed",
		},
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventPhaseChanged,
		Phase: consensus.SessionPhaseObserve,
		At:    base.Add(5 * time.Second).Format(time.RFC3339Nano),
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventObservationAdded,
		Phase: consensus.SessionPhaseObserve,
		At:    base.Add(5500 * time.Millisecond).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"observationId":  "observe_1",
			"outcome":        "contradicted",
			"reopen":         true,
			"followUpCaseId": "case_followup",
			"summary":        "new evidence contradicts retained claim",
		},
	})
	mustEvent(consensus.RunEvent{
		Type:  consensus.RunEventSessionFinalized,
		Phase: consensus.SessionPhaseObserve,
		At:    base.Add(6 * time.Second).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"mode":        "adjudication",
			"taskVerdict": "undetermined",
		},
	})

	text := stdout.String()
	if !strings.Contains(text, "task completed: proposer-a -> revise attempt=1/2 duration=2s") {
		t.Fatalf("expected task duration in verbose output, got: %s", text)
	}
	if !strings.Contains(text, "claim revised: claim_1 action=downgrade_confidence confidenceDelta=-0.2 reason=Need more evidence") {
		t.Fatalf("expected claim revised line, got: %s", text)
	}
	if !strings.Contains(text, "phase completed: revise duration=4s tasks(d=1 c=1 f=0 r=0) revisions=1 actions=downgrade_confidence=1") {
		t.Fatalf("expected revise phase summary, got: %s", text)
	}
	if !strings.Contains(text, "claim adjudicated: claim_1 disposition=keep_with_caveat verdict=supported confidence=0.62 reason=evidence is mixed") {
		t.Fatalf("expected claim adjudicated line, got: %s", text)
	}
	if !strings.Contains(text, "observation recorded: contradicted reopen=true followUpCaseId=case_followup summary=new evidence contradicts retained claim") {
		t.Fatalf("expected observation line, got: %s", text)
	}
	if !strings.Contains(text, "phase completed: adjudicate duration=1s adjudications=1 dispositions=keep_with_caveat=1") {
		t.Fatalf("expected adjudicate phase summary, got: %s", text)
	}
	if !strings.Contains(text, "phase completed: observe duration=1s observations=1") {
		t.Fatalf("expected observe phase summary, got: %s", text)
	}
}

func TestDebugOutputShowsPayloadAndArtifactPaths(t *testing.T) {
	var stdout bytes.Buffer
	output := NewOutput(&stdout, &bytes.Buffer{}, false, true, "/tmp/til-consensus-artifacts")
	observer := output.EventObserver()

	if err := observer.OnEvent(context.Background(), consensus.RunEvent{
		Type: consensus.RunEventTaskDispatched,
		At:   time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Payload: map[string]any{
			"agentId":     "verifier-codex",
			"taskKind":    "semantic_verify",
			"attempt":     1,
			"maxAttempts": 2,
		},
	}); err != nil {
		t.Fatalf("OnEvent debug dispatch failed: %v", err)
	}

	text := stdout.String()
	if !strings.Contains(text, "[til-consensus][debug] task_dispatched payload=") {
		t.Fatalf("expected debug payload line, got: %s", text)
	}
	if !strings.Contains(text, "provider artifacts input=/tmp/til-consensus-artifacts/input-verifier-codex-semantic_verify-<taskID>.json") {
		t.Fatalf("expected debug artifact path, got: %s", text)
	}
}

func TestDebugOutputColorizesPayloadJSONWhenForced(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	output := NewOutput(&stdout, &bytes.Buffer{}, false, true, "")
	observer := output.EventObserver()

	err := observer.OnEvent(context.Background(), consensus.RunEvent{
		Type: consensus.RunEventTaskDispatched,
		Payload: map[string]any{
			"agentId":     "verifier-codex",
			"attempt":     1,
			"enabled":     true,
			"taskKind":    "semantic_verify",
			"maxAttempts": 2,
		},
	})
	if err != nil {
		t.Fatalf("OnEvent debug dispatch failed: %v", err)
	}

	text := stdout.String()
	for _, needle := range []string{
		"\x1b[34m\"agentId\"\x1b[0m",
		"\x1b[32m\"verifier-codex\"\x1b[0m",
		"\x1b[33m1\x1b[0m",
		"\x1b[35mtrue\x1b[0m",
		"\x1b[36m{\x1b[0m",
		"\x1b[36m:\x1b[0m",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected colored debug payload to contain %q, got: %q", needle, text)
		}
	}
}

func TestOutputColorizesKeywordsWhenForced(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	output := NewOutput(&stdout, &bytes.Buffer{}, true, false, "")
	output.Printf("[til-consensus] task failed: verifier-a -> semantic_verify error=boom supported undetermined\n")

	text := stdout.String()
	for _, needle := range []string{
		"\x1b[36m[til-consensus]\x1b[0m",
		"\x1b[31mtask failed:\x1b[0m",
		"\x1b[31merror=\x1b[0m",
		"\x1b[32msupported\x1b[0m",
		"\x1b[33mundetermined\x1b[0m",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected colored output to contain %q, got: %q", needle, text)
		}
	}
}

func TestFollowUpRunAndSessionCommands(t *testing.T) {
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
		t.Fatalf("runConfigInitCommand failed: %v", err)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	request, err := consensus.NormalizeStartRequest(consensus.StartRequest{
		RequestID: "followup-request-1",
		Lineage: &consensus.RunLineage{
			ParentRequestID: "parent-request-1",
			ParentSessionID: "parent-session-1",
			ParentCaseID:    "parent-case-1",
			Trigger:         "observe_contradiction",
		},
		TaskSpec: consensus.TaskSpec{
			Goal:            "复核上一轮裁决是否被新的观测证据推翻",
			SuccessCriteria: []string{"必须输出 claim 级裁决"},
		},
		Roles: consensus.RoleAssignments{
			Proposers:        loaded.Config.Roles.Proposers,
			Challengers:      loaded.Config.Roles.Challengers,
			Arbiter:          loaded.Config.Roles.Arbiter,
			SemanticVerifier: loaded.Config.Roles.SemanticVerifier,
			Reporter:         loaded.Config.Roles.Reporter,
		},
		ProposalPolicy: consensus.ProposalPolicy{MaxPasses: 1, MaxClaimsPerWorker: 2},
		VerificationPolicy: consensus.VerificationPolicy{
			AllowSemanticVerifier: true,
			MaxParallelChecks:     1,
		},
		ArbiterPolicy: consensus.ArbiterPolicy{AllowUndetermined: true, BlindReview: true},
		ReportPolicy:  consensus.ReportPolicy{Style: "builtin"},
		WaitingPolicy: consensus.WaitingPolicy{PerTaskTimeout: consensus.DefaultPerTaskTimeout, RetryAttempts: 1},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest failed: %v", err)
	}
	artifactPath := filepath.Join(tmp, "followup.json")
	body, err := json.Marshal(consensus.FollowUpCaseArtifact{
		SchemaVersion:   consensus.SchemaVersion,
		CaseID:          "child-case-1",
		RequestID:       request.RequestID,
		ParentRequestID: "parent-request-1",
		ParentSessionID: "parent-session-1",
		ParentCaseID:    "parent-case-1",
		Trigger:         "observe_contradiction",
		CreatedAt:       "2026-04-16T10:00:00Z",
		Request:         request,
	})
	if err != nil {
		t.Fatalf("marshal followup artifact failed: %v", err)
	}
	if err := os.WriteFile(artifactPath, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write followup artifact failed: %v", err)
	}

	followupCmd := newFollowUpCommand()
	var followupOut bytes.Buffer
	followupCmd.Writer = &followupOut
	if err := followupCmd.Run(context.Background(), []string{"followup", "run", "--config", configPath, "--artifact", artifactPath}); err != nil {
		t.Fatalf("followup run failed: %v", err)
	}
	paths := config.ResolveRunArtifacts(loaded, request.RequestID)
	body, err = os.ReadFile(paths.ResultPath)
	if err != nil {
		t.Fatalf("read result failed: %v", err)
	}
	result, err := consensus.DecodeRunResult(body)
	if err != nil {
		t.Fatalf("DecodeRunResult failed: %v", err)
	}
	if result.Lineage == nil || result.Lineage.ParentRequestID != "parent-request-1" || result.Lineage.ParentSessionID != "parent-session-1" {
		t.Fatalf("unexpected lineage: %#v", result.Lineage)
	}

	sessionListCmd := newSessionCommand().Commands[0]
	var listOut bytes.Buffer
	sessionListCmd.Writer = &listOut
	if err := sessionListCmd.Run(context.Background(), []string{"list", "--config", configPath, "--request-id", request.RequestID}); err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	var snapshots []consensus.SessionSnapshot
	if err := json.Unmarshal(listOut.Bytes(), &snapshots); err != nil {
		t.Fatalf("decode session list failed: %v", err)
	}
	if len(snapshots) == 0 || snapshots[0].SessionID == "" {
		t.Fatalf("unexpected session list: %#v", snapshots)
	}
	sessionShowCmd := newSessionCommand().Commands[1]
	var showOut bytes.Buffer
	sessionShowCmd.Writer = &showOut
	if err := sessionShowCmd.Run(context.Background(), []string{"show", "--config", configPath, "--session-id", snapshots[0].SessionID}); err != nil {
		t.Fatalf("session show failed: %v", err)
	}
	var shown consensus.SessionSnapshot
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("decode session show failed: %v", err)
	}
	if shown.SessionID != snapshots[0].SessionID || shown.Request == nil {
		t.Fatalf("unexpected session show payload: %#v", shown)
	}

	runCmd := newRunCommand()
	var replayOut bytes.Buffer
	runCmd.Writer = &replayOut
	if err := runCmd.Run(context.Background(), []string{"run", "--config", configPath, "--replay-session", snapshots[0].SessionID}); err != nil {
		t.Fatalf("run --replay-session failed: %v", err)
	}
	allSnapshots, err := filestore.New(config.ResolveSessionStoreDir(loaded)).List(context.Background())
	if err != nil {
		t.Fatalf("list persisted sessions failed: %v", err)
	}
	foundReplay := false
	for _, snapshot := range allSnapshots {
		if snapshot.Request != nil && snapshot.Request.Lineage != nil && snapshot.Request.Lineage.ParentSessionID == snapshots[0].SessionID && snapshot.Request.Lineage.Trigger == "session_replay" {
			foundReplay = true
			break
		}
	}
	if !foundReplay {
		t.Fatalf("expected replay session in store, got %#v", allSnapshots)
	}
}
