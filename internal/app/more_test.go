package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	filestore "github.com/suchasplus/til-consensus/internal/store/file"
)

func TestNewBuildsCommandTree(t *testing.T) {
	cmd := New()
	if cmd.Name != "til-consensus" {
		t.Fatalf("unexpected root name: %s", cmd.Name)
	}
	if len(cmd.Commands) != 7 {
		t.Fatalf("unexpected root command count: %d", len(cmd.Commands))
	}
	if cmd.Version == "" {
		t.Fatal("expected root version to be populated")
	}
	names := []string{cmd.Commands[0].Name, cmd.Commands[1].Name, cmd.Commands[2].Name, cmd.Commands[3].Name, cmd.Commands[4].Name, cmd.Commands[5].Name, cmd.Commands[6].Name}
	if strings.Join(names, ",") != "run,followup,config,act,session,view,version" {
		t.Fatalf("unexpected command tree: %#v", names)
	}
}

func TestConfigCommandsAndActCommand(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	var initOut bytes.Buffer
	if err := runConfigInitCommand(&initOut, configPath, "quickstart", false, false); err != nil {
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
	output := NewOutput(&stdout, &stderr, false)
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

func TestFollowUpRunAndSessionCommands(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "til-consensus.yaml")
	if err := runConfigInitCommand(&bytes.Buffer{}, configPath, "quickstart", false, false); err != nil {
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
