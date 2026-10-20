// Package providers 只检查 CLI 可执行文件和公开版本，不读取认证文件。
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/codexserver"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

type Config struct {
	// Enabled 缺失时兼容旧配置，默认参与新讨论。
	Enabled   *bool  `json:"enabled,omitempty"`
	ID        string `json:"id"`
	Command   string `json:"command,omitempty"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
}

type Info struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	CLIType         string   `json:"cliType"`
	Command         string   `json:"command"`
	Path            string   `json:"path,omitempty"`
	Version         string   `json:"version,omitempty"`
	Model           string   `json:"model,omitempty"`
	Reasoning       string   `json:"reasoning,omitempty"`
	RequestedModel  string   `json:"requestedModel,omitempty"`
	ActualModel     string   `json:"actualModel,omitempty"`
	Available       bool     `json:"available"`
	Error           string   `json:"error,omitempty"`
	EnvironmentKeys []string `json:"environmentKeys,omitempty"`
}

type CheckResult struct {
	RequestAttempted   bool           `json:"requestAttempted"`
	Provider           Info           `json:"provider"`
	OK                 bool           `json:"ok"`
	LatencyMS          int64          `json:"latencyMs"`
	Model              string         `json:"model,omitempty"`
	RequestedModel     string         `json:"requestedModel,omitempty"`
	ActualModel        string         `json:"actualModel,omitempty"`
	RequestedReasoning string         `json:"requestedReasoning,omitempty"`
	ActualReasoning    string         `json:"actualReasoning,omitempty"`
	ModelSource        string         `json:"modelSource"`
	Usage              map[string]any `json:"usage,omitempty"`
	// CLI 可能不报告价格；未知费用必须保留为 null。
	Cost  *float64 `json:"cost"`
	Error string   `json:"error,omitempty"`
}

func Defaults() []Config {
	return []Config{{ID: "codex", Command: "codex"}, {ID: "claude", Command: "claude"}, {ID: "grok", Command: "grok"}, {ID: "agy", Command: "agy"}}
}

func Resolve(id string, configured []Config) (Config, Info, error) {
	var c Config
	var info Info
	for _, item := range Defaults() {
		if item.ID == id {
			c = item
			break
		}
	}
	if c.ID == "" {
		return c, info, fmt.Errorf("unsupported CLI provider %q", id)
	}
	for _, item := range configured {
		if item.ID == id {
			if item.Command != "" {
				c.Command = item.Command
			}
			c.Model, c.Reasoning = item.Model, item.Reasoning
			c.Enabled = item.Enabled
		}
	}
	info = Info{ID: id, CLIType: id, Command: c.Command, Model: c.Model, RequestedModel: c.Model, Reasoning: c.Reasoning, Label: map[string]string{"codex": "Codex", "claude": "Claude Code", "grok": "Grok Build", "agy": "agy"}[id]}
	if id == "agy" {
		info.CLIType = config.CLITypeAntigravity
	}
	info.EnvironmentKeys = EnvironmentKeys(id)
	return c, info, nil
}

// Probe 不会调用模型，也不能据此宣称已经登录或模型可用。
func Probe(ctx context.Context, c Config) Info {
	resolved, info, err := Resolve(c.ID, []Config{c})
	if err != nil {
		return Info{ID: c.ID, Error: err.Error()}
	}
	path, err := executablePath(resolved.Command)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Path = path
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, "--version")
	task := consensus.ActionTask{TaskMeta: consensus.TaskMeta{AgentID: c.ID, Role: "version-probe"}}
	cmd.Env = clirunner.BuildEnvironment(clirunner.SnapshotHostEnvironment(os.Environ()), Runtime(resolved, info, ""), task, c.ID, task.Role, resolved.Model).Values
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if err != nil {
		info.Error = fmt.Sprintf("%s version probe: %v", c.ID, err)
		return info
	}
	info.Version = RedactSensitive(strings.TrimSpace(string(output)))
	if len(info.Version) > 512 {
		info.Version = info.Version[:512]
	}
	info.Available = info.Version != ""
	if !info.Available {
		info.Error = "CLI returned an empty version"
	}
	return info
}

func executablePath(command string) (string, error) {
	if path, err := exec.LookPath(command); err == nil {
		return filepath.Abs(path)
	}
	if strings.ContainsAny(command, `/\\`) {
		return "", fmt.Errorf("CLI executable was not found: %s", command)
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, ".grok", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		candidate := filepath.Join(dir, command)
		if path, err := exec.LookPath(candidate); err == nil {
			return filepath.Abs(path)
		}
	}
	return "", fmt.Errorf("CLI executable was not found: %s", command)
}

func Snapshot(ctx context.Context, configured []Config) []Info {
	result := make([]Info, 0, 4)
	for _, item := range Defaults() {
		c, _, _ := Resolve(item.ID, configured)
		result = append(result, Probe(ctx, c))
	}
	return result
}

func Runtime(c Config, info Info, workdir string) config.ProviderConfig {
	command := c.Command
	if info.Path != "" {
		command = info.Path
	}
	keys := EnvironmentKeys(c.ID)
	return config.ProviderConfig{Type: config.ProviderTypeCLI, CLIType: info.CLIType, Command: command, Workdir: workdir, PermissionMode: "read-only", OutputFormat: "stream-json", EnvPassthrough: &keys}
}

// EnvironmentKeys 只返回该提供方明确支持、且当前存在的变量名。
// 不把任何变量值放进配置、RPC、日志或审计记录。
func EnvironmentKeys(id string) []string {
	allowed := map[string][]string{
		"codex":  {"CODEX_HOME", "CODEX_API_KEY", "OPENAI_API_KEY", "OPENAI_BASE_URL"},
		"claude": {"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"},
		"grok":   {"XAI_API_KEY"},
		"agy":    {"GEMINI_API_KEY"},
	}[id]
	keys := make([]string, 0, len(allowed))
	for _, key := range allowed {
		if _, exists := os.LookupEnv(key); exists {
			keys = append(keys, key)
		}
	}
	return keys
}

// Check 仅由用户明确请求连通性验证时调用，会产生一次真实模型请求。
func Check(ctx context.Context, c Config) (CheckResult, error) {
	started := time.Now()
	info := Probe(ctx, c)
	result := CheckResult{Provider: info, Model: c.Model, RequestedModel: c.Model, RequestedReasoning: c.Reasoning, ModelSource: "unreported"}
	if c.Model != "" {
		result.ModelSource = "explicit-request"
	}
	if !info.Available {
		result.LatencyMS = time.Since(started).Milliseconds()
		result.Error = info.Error
		return result, errors.New(info.Error)
	}
	resolved, _, err := Resolve(c.ID, []Config{c})
	if err != nil {
		return result, err
	}
	workdir, err := os.MkdirTemp("", "tcview-provider-check-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(workdir)
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"marker"}, "properties": map[string]any{"marker": map[string]any{"type": "string", "const": "tcview-cli-connected"}}}
	progress := func(e clirunner.CLIEvent) error {
		if e.Model != "" {
			result.Model = e.Model
			result.ActualModel = e.Model
			result.ModelSource = "cli-report"
		}
		if e.Usage != nil {
			result.Usage = e.Usage
		}
		return nil
	}
	task := consensus.ActionTask{TaskMeta: consensus.TaskMeta{AgentID: c.ID, Role: "connectivity-check"}, Prompt: "检查真实 CLI 连通性"}
	prompt := "不要调用工具、不要读取任何文件，只返回 JSON：{\"marker\":\"tcview-cli-connected\"}。"
	var output clirunner.RunTaskResult
	runtimeConfig := Runtime(resolved, info, workdir)
	result.RequestAttempted = true
	if c.ID == "codex" {
		environment := clirunner.BuildEnvironment(clirunner.SnapshotHostEnvironment(os.Environ()), runtimeConfig, task, c.ID, task.Role, resolved.Model)
		native, runErr := codexserver.Run(checkCtx, codexserver.Options{Command: runtimeConfig.Command, Cwd: workdir, Model: resolved.Model, Reasoning: resolved.Reasoning, Environment: environment.Values, OnEvent: progress}, prompt, schema)
		output = clirunner.RunTaskResult{Output: native.Output, RawOutput: native.RawOutput, Command: native.Command}
		err = runErr
	} else {
		runner := clirunner.NewRunnerWithOptions(runtimeConfig, clirunner.RunnerOptions{OnEvent: progress})
		output, err = runner.RunTaskDetailed(checkCtx, task, prompt, c.ID, task.Role, resolved.Model, resolved.Reasoning, nil, schema)
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	result.ActualReasoning = ReportedReasoning(output.RawOutput)
	model, usage, cost := OutputMetadata(output.RawOutput)
	if model != "" {
		result.Model = model
		result.ActualModel = model
		result.ModelSource = "cli-report"
	}
	if usage != nil {
		result.Usage = usage
	}
	result.Cost = cost
	if err != nil {
		result.Error = SafeError(info.CLIType, err)
		return result, errors.New(result.Error)
	}
	var value map[string]any
	if json.Unmarshal([]byte(output.Output), &value) != nil || value["marker"] != "tcview-cli-connected" || len(value) != 1 {
		result.Error = info.CLIType + " CLI returned an invalid connectivity marker"
		return result, errors.New(result.Error)
	}
	if c.Reasoning != "" && result.ActualReasoning != "" && c.Reasoning != result.ActualReasoning {
		result.Error = fmt.Sprintf("%s CLI 报告推理强度 %s，与请求 %s 不一致，未将此请求认定为配置验证成功", c.ID, result.ActualReasoning, c.Reasoning)
		return result, errors.New(result.Error)
	}
	result.OK = true
	return result, nil
}

// SafeError 不把 stdout 中潜在的思考事件或整个命令行转发给界面。
func SafeError(cliType string, err error) (out string) {
	defer func() { out = RedactSensitive(out) }()
	var commandError *clirunner.CommandError
	if errors.As(err, &commandError) {
		message := fmt.Sprintf("%s CLI 执行失败（退出码 %d）", cliType, commandError.ExitCode)
		if commandError.Err != nil {
			message += ": " + commandError.Err.Error()
		}
		if stderr := strings.TrimSpace(commandError.Stderr); stderr != "" {
			if len(stderr) > 2000 {
				stderr = stderr[:2000] + "…"
			}
			message += ": " + stderr
		}
		return message
	}
	return fmt.Sprintf("%s CLI: %v", cliType, err)
}
