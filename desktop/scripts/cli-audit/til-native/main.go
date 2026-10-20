// 通过真实 til-consensus Runner 调用 CLI，不包含 mock provider。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/runtime/cli"
)

type auditTask struct{ consensus.TaskMeta }

func (auditTask) Kind() consensus.TaskKind   { return "interface_audit" }
func (t auditTask) Meta() consensus.TaskMeta { return t.TaskMeta }

func main() {
	provider := flag.String("provider", "", "真实 CLI 类型")
	input := flag.String("prompt", "", "真实输入文件")
	schemaPath := flag.String("schema", "", "JSON Schema 文件")
	directory := flag.String("output", "", "本次调用的私有目录")
	format := flag.String("format", "json", "json 或 stream-json")
	flag.Parse()
	if *provider == "" || *directory == "" {
		panic("缺少 provider/output")
	}
	if err := os.MkdirAll(*directory, 0700); err != nil {
		panic(err)
	}
	bytes, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	schemaBytes, err := os.ReadFile(*schemaPath)
	if err != nil {
		panic(err)
	}
	var schema map[string]any
	if err = json.Unmarshal(schemaBytes, &schema); err != nil {
		panic(err)
	}
	keys := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		for _, prefix := range []string{"OPENAI_", "CODEX_", "ANTHROPIC_", "XAI_", "GROK_", "GEMINI_"} {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
				break
			}
		}
	}
	command := *provider
	if command == config.CLITypeAntigravity {
		command = "agy"
	}
	binaryPath, _ := exec.LookPath(command)
	versionBytes, versionErr := exec.Command(command, "--version").CombinedOutput()
	cliVersion := strings.TrimSpace(string(versionBytes))
	if versionErr != nil {
		panic(fmt.Errorf("读取真实CLI版本: %w", versionErr))
	}
	started := time.Now()
	events := []cli.CLIEvent{}
	runner := cli.NewRunnerWithOptions(config.ProviderConfig{Type: config.ProviderTypeCLI, CLIType: *provider, Command: command, Workdir: *directory, PermissionMode: "read-only", OutputFormat: *format, EnvPassthrough: &keys}, cli.RunnerOptions{OnEvent: func(event cli.CLIEvent) error { events = append(events, event); return nil }})
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()
	result, runErr := runner.RunTaskDetailed(ctx, auditTask{consensus.TaskMeta{RequestID: "live-cli-audit", SessionID: "live-cli-audit", AgentID: *provider}}, string(bytes), *provider, "reviewer", "", "", nil, schema)
	if result.RawOutput != "" {
		if err := os.WriteFile(filepath.Join(*directory, "raw-stdout.txt"), []byte(result.RawOutput), 0600); err != nil {
			panic(err)
		}
	}
	var value any
	parseErr := json.Unmarshal([]byte(result.Output), &value)
	output := map[string]any{"provider": *provider, "cliVersion": cliVersion, "binaryPath": binaryPath, "elapsedMs": time.Since(started).Milliseconds(), "command": result.Command, "eventCount": len(events), "output": value, "ok": runErr == nil && parseErr == nil}
	if runErr != nil {
		output["error"] = runErr.Error()
	}
	if parseErr != nil {
		output["parseError"] = parseErr.Error()
	}
	for name, value := range map[string]any{"result.json": output, "public-events.json": events} {
		body, _ := json.MarshalIndent(value, "", "  ")
		if err = os.WriteFile(filepath.Join(*directory, name), body, 0600); err != nil {
			panic(err)
		}
	}
	body, _ := json.Marshal(output)
	fmt.Println(string(body))
	if runErr != nil || parseErr != nil {
		os.Exit(1)
	}
}
