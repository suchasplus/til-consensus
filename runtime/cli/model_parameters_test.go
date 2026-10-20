package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/config"
)

// 此处使用明确的本地进程替身。验证的是 RunTaskDetailed 最终交给进程的
// 参数，而不是某个 CLI 已接受了该模型，也不是服务端已执行了该推理档位。
func TestNativeModelParametersReachProcessWithoutDefaultOverrides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture; portable argv regression is covered separately")
	}
	for _, test := range []struct {
		cliType, model, reasoning string
	}{
		{config.CLITypeClaude, "claude-opus-5-5", "low"},
		{config.CLITypeCodex, "gpt-6.1-sol", "xhigh"},
		{config.CLITypeGrok, "grok-4.7", "low"},
		{config.CLITypeAntigravity, "gemini-3.8-flash-medium", ""},
	} {
		for _, cliDefault := range []bool{false, true} {
			name := test.cliType + "/selected"
			model, effort := test.model, test.reasoning
			if cliDefault {
				name, model, effort = test.cliType+"/cli-default", "", ""
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				capture := filepath.Join(dir, "argv.bin")
				command := filepath.Join(dir, "fixture-cli")
				const script = `#!/bin/sh
printf '%s\000' "$@" > "$FIXTURE_CAPTURE"
cat >/dev/null
case "$FIXTURE_CLI" in
  claude) printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"{\"ok\":true}"}' ;;
  codex) printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"ok\":true}"}}' '{"type":"turn.completed"}' ;;
  grok) printf '%s\n' '{"type":"end","stopReason":"end_turn","text":"{\"ok\":true}"}' ;;
  antigravity) printf '%s\n' '{"event":"result","result":{"status":"SUCCESS","response":"{\"ok\":true}","num_turns":1}}' ;;
esac
`
				if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				runner := NewRunner(config.ProviderConfig{
					CLIType: test.cliType, Command: command, PermissionMode: "read-only", OutputFormat: "stream-json",
					Env: map[string]string{"FIXTURE_CAPTURE": capture, "FIXTURE_CLI": test.cliType},
				})
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				result, err := runner.RunTaskDetailed(ctx, realPromptTransportTestTask{}, "fixture prompt", "agent", "reviewer", model, effort, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if result.Output != `{"ok":true}` {
					t.Fatalf("native terminal was not normalized: %q", result.Output)
				}
				body, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				actual := strings.Split(strings.TrimSuffix(string(body), "\x00"), "\x00")
				if strings.Join(actual, "\x00") != strings.Join(result.Command[1:], "\x00") {
					t.Fatal("reported request command differs from the process argv")
				}
				assertNativeModelParameters(t, test.cliType, actual, model, effort)
			})
		}
	}
}

func TestNativeModelParametersPreserveSelectorsAndEncodedAgyEffort(t *testing.T) {
	for _, test := range []struct {
		cliType, model, effort string
	}{
		{config.CLITypeGrok, "grok-4.7", "low"},
		{config.CLITypeGrok, "grok-4.7-build-fast", ""},
		{config.CLITypeAntigravity, "gemini-3.8-flash-low", ""},
		{config.CLITypeAntigravity, "gemini-3.8-flash-medium", ""},
		{config.CLITypeAntigravity, "gemini-3.8-flash-high", ""},
		{config.CLITypeAntigravity, "claude-opus-5-5-high", ""},
		{config.CLITypeAntigravity, "gpt-oss-120b-medium", ""},
		{config.CLITypeClaude, "claude-opus-5-5", "low"},
		{config.CLITypeCodex, "gpt-6.1-sol", "xhigh"},
	} {
		t.Run(test.cliType+"/"+test.model, func(t *testing.T) {
			command, err := PreviewCommand(config.ProviderConfig{CLIType: test.cliType, Command: "fixture-cli", PermissionMode: "read-only"}, realPromptTransportTestTask{}, "fixture prompt", "agent", "reviewer", test.model, test.effort, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertNativeModelParameters(t, test.cliType, command[1:], test.model, test.effort)
		})
	}
}

func TestAgyRealModelEffortConflictIsNotSuccessfulOutput(t *testing.T) {
	raw := readProgressFixture(t, "agy", "model-effort-conflict.json")
	_, err := normalizeAntigravityCLIOutput(raw)
	if err == nil || !strings.Contains(err.Error(), "conflicts with --effort=low") {
		t.Fatalf("real zero-turn model/effort selection failure must remain a failure: %v", err)
	}
}

func TestAgyRealEffortInspectionIsNotSuccessfulModelInference(t *testing.T) {
	for _, fixture := range []string{"effort-model-medium.json", "effort-model-medium-same.json", "effort-base-model-low.json"} {
		t.Run(fixture, func(t *testing.T) {
			_, err := normalizeAntigravityCLIOutput(readProgressFixture(t, "agy", fixture))
			if err == nil || !strings.Contains(err.Error(), "positive num_turns") {
				t.Fatalf("successful metadata command is not a completed model turn: %v", err)
			}
		})
	}
}

func assertNativeModelParameters(t *testing.T, cliType string, args []string, model, effort string) {
	t.Helper()
	var actualModel, actualEffort string
	modelCount, effortCount := 0, 0
	for index, arg := range args {
		if (arg == "--model" || arg == "-m") && index+1 < len(args) {
			modelCount++
			actualModel = args[index+1]
		}
		if (arg == "--effort" || arg == "--reasoning-effort") && index+1 < len(args) {
			wantFlag := "--effort"
			if cliType == config.CLITypeGrok {
				wantFlag = "--reasoning-effort"
			}
			if cliType == config.CLITypeCodex || arg != wantFlag {
				t.Fatalf("wrong native effort parameter for %s: %v", cliType, args)
			}
			effortCount++
			actualEffort = args[index+1]
		}
		if cliType == config.CLITypeCodex && arg == "-c" && index+1 < len(args) && strings.HasPrefix(args[index+1], "model_reasoning_effort=") {
			effortCount++
			actualEffort = strings.TrimPrefix(args[index+1], "model_reasoning_effort=")
		}
	}
	if actualModel != model || modelCount != boolCount(model != "") {
		t.Fatalf("model selector changed or CLI default was overridden: %v", args)
	}
	if actualEffort != effort || effortCount != boolCount(effort != "") {
		t.Fatalf("effort changed, duplicated, or inferred from model suffix: %v", args)
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
