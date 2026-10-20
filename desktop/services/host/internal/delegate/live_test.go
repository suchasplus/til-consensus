package delegate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	tcruntime "github.com/suchasplus/til-consensus/runtime"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("../../../../fixtures/cli", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func replayCommand(t *testing.T, fixture string, failed bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "replay-cli")
	// Both fixture and destination paths are generated locally by the test.
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '%s\\n' 'recorded-fixture-replay'; exit 0; fi\ncat " + quote(fixture) + "\n"
	if failed {
		script += "printf '%s\\n' 'recorded CLI failure' >&2\nexit 1\n"
	}
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Replay the exact sanitized responses captured during the real four-CLI
// audit. These checks do not make model requests or synthesize model answers.
func TestRecordedCLIResponsesPassNativeNormalization(t *testing.T) {
	fixtures := []struct{ id, cliType, file string }{{"codex", config.CLITypeCodex, "codex/exec-schema-success.jsonl"}, {"claude", config.CLITypeClaude, "claude/stream-success.jsonl"}, {"grok", config.CLITypeGrok, "grok/stream-success.jsonl"}, {"agy", config.CLITypeAntigravity, "agy/stream-schema-success.jsonl"}}
	for _, fixture := range fixtures {
		t.Run(fixture.id, func(t *testing.T) {
			command := replayCommand(t, fixturePath(t, fixture.file), false)
			provider := config.ProviderConfig{CLIType: fixture.cliType, Command: command, Workdir: t.TempDir(), OutputFormat: "stream-json", PermissionMode: "read-only"}
			runner := clirunner.NewRunnerWithOptions(provider, clirunner.RunnerOptions{OnEvent: func(e clirunner.CLIEvent) error {
				if e.Type == "thinking" || e.Type == "reasoning" {
					t.Fatal("private reasoning escaped the CLI adapter")
				}
				return nil
			}})
			task := consensus.ActionTask{TaskMeta: consensus.TaskMeta{AgentID: fixture.id}, Prompt: "回放真实接口审计产物"}
			result, err := runner.RunTaskDetailed(context.Background(), task, task.Prompt, fixture.id, "reviewer", "", "", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var output map[string]any
			if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
				t.Fatal(err)
			}
			// 历史真实 CLI 录制中的项目名保持原样，不作为应用标识或兼容入口。
			if output["project_name"] != "newtc" {
				t.Fatalf("lost recorded project data: %s", result.Output)
			}
		})
	}
}

func TestLiveFailureNeverFallsBackToMock(t *testing.T) {
	command := replayCommand(t, fixturePath(t, "grok/auth-error.json"), true)
	var messages []Message
	d := New(Options{ArtifactDir: t.TempDir(), Providers: []providers.Config{{ID: "grok", Command: command}}, OnMessage: func(message Message) error { messages = append(messages, message); return nil }})
	defer d.Close()
	if d.options.Mode != "cli" {
		t.Fatal("real CLI must be the default")
	}
	task := testTasks()[1].(consensus.InitialProposalTask)
	task.AgentID = "grok"
	receipt, err := d.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	result, err := d.Await(context.Background(), receipt.TaskID, time.Second*5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !strings.Contains(result.Error, "grok CLI") || len(messages) != 0 {
		t.Fatalf("failed CLI became a success: %#v %#v", result, messages)
	}
	data, err := os.ReadFile(result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact["mock"] != false || artifact["complete"] != false || artifact["status"] != "failed" || !strings.Contains(artifact["rawOutput"].(string), "Not signed in") {
		t.Fatalf("lost real failure provenance: %s", data)
	}
}

func TestRealMetadataKeepsSnapshotsAndUnknownCosts(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, "grok/stream-success.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	model, usage, cost := outputMetadata(string(data))
	if model != "grok-4.7-build" || usage["input_tokens"] != float64(11947) || cost == nil || *cost <= 0 {
		t.Fatalf("recorded Grok metadata was lost: %s %#v %v", model, usage, cost)
	}
	data, err = os.ReadFile(fixturePath(t, "codex/exec-schema-success.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	model, usage, cost = outputMetadata(string(data))
	if model != "" || usage["input_tokens"] != float64(21558) || cost != nil {
		t.Fatalf("invented Codex model/cost: %s %#v %v", model, usage, cost)
	}
}

func TestStreamingOnlyPublishesNaturalSummary(t *testing.T) {
	var stream answerStream
	parts := []string{`{"schemaVersion":1,"mock":false,"invocationId":"private-id","coreOutput":{"claims":[{"statement":"先出现的主张"}],"summary":"逐`, `字\n输`, `出摘要","other":"隐藏字段"},"desktopReview":[]}`}
	var text strings.Builder
	for _, part := range parts {
		text.WriteString(stream.consume(clirunner.CLIEvent{Type: "text_delta", Text: part}))
	}
	if text.String() != "逐字\n输出摘要" {
		t.Fatalf("unexpected public preview: %q", text.String())
	}
	if delta := stream.consume(clirunner.CLIEvent{Type: "thinking", Text: "private reasoning"}); delta != "" {
		t.Fatal("reasoning reached preview")
	}
}

func TestFrozenBallotRejectsStaleHashesAndMissingVotes(t *testing.T) {
	task := consensus.FinalVoteTask{TaskMeta: consensus.TaskMeta{AgentID: "claude", Metadata: map[string]any{"proposalHash": "proposal", "ballotManifestHash": "manifest"}}, Claims: []consensus.DebateClaim{{ClaimID: "c1", Active: true}}}
	binding, err := expectedBallot(task)
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{InvocationID: "i1", ContextHash: "hash", TaskKind: string(task.Kind()), BallotBinding: binding, CoreOutput: map[string]any{"votes": []map[string]any{{"claimId": "c1"}}}}
	if err := validateInvocation(task, envelope, "i1", "hash", binding); err != nil {
		t.Fatal(err)
	}
	envelope.BallotBinding = &BallotBinding{ProposalHash: "stale", ManifestHash: "manifest"}
	if err := validateInvocation(task, envelope, "i1", "hash", binding); err == nil {
		t.Fatal("accepted stale proposal vote")
	}
	envelope.BallotBinding = binding
	envelope.CoreOutput["votes"] = []any{}
	if err := validateInvocation(task, envelope, "i1", "hash", binding); err == nil {
		t.Fatal("accepted incomplete ballot")
	}
}

func TestCodexNativeSchemaSubsetForAllTaskKinds(t *testing.T) {
	agent := tcruntime.ResolvedAgentRuntime{Provider: config.ProviderConfig{Type: config.ProviderTypeCLI, CLIType: config.CLITypeCodex}}
	unsupported := map[string]bool{"$schema": true, "uniqueItems": true, "oneOf": true, "allOf": true, "not": true, "dependentRequired": true, "dependentSchemas": true, "if": true, "then": true, "else": true}
	var check func(map[string]any)
	check = func(schema map[string]any) {
		for key := range schema {
			if unsupported[key] {
				t.Errorf("native Codex schema contains unsupported %s", key)
			}
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			if schema["additionalProperties"] != false {
				t.Error("native object is not closed")
			}
			required, ok := schema["required"].([]string)
			if !ok || len(required) != len(properties) {
				t.Errorf("native object required does not match its properties: %v", required)
			}
			for _, key := range required {
				if _, ok := properties[key]; !ok {
					t.Errorf("unexpected native required key %s", key)
				}
			}
			for _, value := range properties {
				if child, ok := value.(map[string]any); ok {
					check(child)
				}
			}
		}
		if child, ok := schema["items"].(map[string]any); ok {
			check(child)
		}
		if branches, ok := schema["anyOf"].([]any); ok {
			for _, value := range branches {
				check(value.(map[string]any))
			}
		}
	}
	for _, task := range testTasks() {
		t.Run(string(task.Kind())+"/"+phase(task), func(t *testing.T) {
			var binding *BallotBinding
			if task.Kind() == consensus.TaskKindFinalVote {
				binding = &BallotBinding{ProposalHash: "proposal", ManifestHash: "manifest"}
			}
			schema := nativeEnvelopeSchema(task, agent, "invocation", strings.Repeat("a", 64), binding)
			check(schema)
			props := schema["properties"].(map[string]any)
			if props["schemaVersion"].(map[string]any)["type"] != "integer" || props["taskKind"].(map[string]any)["type"] != "string" {
				t.Fatal("native const fields must have explicit types")
			}
		})
	}
}

func TestLocalValidationStillRejectsDuplicateExclusiveReferences(t *testing.T) {
	task := consensus.ChallengeTask{TaskMeta: consensus.TaskMeta{AgentID: "claude"}, TaskSpec: consensus.TaskSpec{Goal: "验证互斥引用"}, Claims: []consensus.ClaimNode{{ClaimID: "c1", Statement: "方案一"}, {ClaimID: "c2", Statement: "方案二"}}}
	envelope := validEnvelope(t, task)
	review := firstReview(envelope)
	review["exclusiveWith"] = []any{"c2"}
	raw, _ := json.Marshal(envelope)
	if _, _, err := ValidateEnvelope(task, raw); err != nil {
		t.Fatalf("single valid exclusive reference was rejected: %v", err)
	}
	review["exclusiveWith"] = []any{"c2", "c2"}
	raw, _ = json.Marshal(envelope)
	if _, _, err := ValidateEnvelope(task, raw); err == nil {
		t.Fatal("native uniqueItems removal weakened authoritative local validation")
	}
	local := EnvelopeSchema(task)["properties"].(map[string]any)["desktopReview"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if local["exclusiveWith"].(map[string]any)["uniqueItems"] != true {
		t.Fatal("local schema lost uniqueItems")
	}
}

func TestAllNativeProviderSchemasOmitDialectDeclarations(t *testing.T) {
	for _, cliType := range []string{config.CLITypeCodex, config.CLITypeClaude, config.CLITypeGrok, config.CLITypeAntigravity} {
		for _, task := range testTasks() {
			agent := tcruntime.ResolvedAgentRuntime{Provider: config.ProviderConfig{Type: config.ProviderTypeCLI, CLIType: cliType}}
			schema := nativeEnvelopeSchema(task, agent, "invocation", strings.Repeat("a", 64), nil)
			raw, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"$schema"`) {
				t.Fatalf("%s/%s native schema retains an unsupported dialect declaration", cliType, task.Kind())
			}
			if EnvelopeSchema(task)["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
				t.Fatal("native adaptation changed the authoritative local dialect")
			}
		}
	}
}
