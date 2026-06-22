package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/urfave/cli/v3"
)

func newConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "配置管理",
		Commands: []*cli.Command{
			newConfigInitCommand(),
			newConfigValidateCommand(),
			newConfigWizardCommand(),
			newConfigRenderCommand(),
			newConfigExplainCommand(),
			newConfigAddProviderCommand(),
			newConfigAddAgentCommand(),
		},
	}
}

func newConfigInitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "写入首用友好的示例配置",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "preset", Usage: "兼容别名(quickstart|openai|coding|debate|delphi|generic|codex|claude|gemini|antigravity)"},
			&cli.StringFlag{Name: "mode", Usage: "workflow 模式(adjudication|free-debate|delphi)"},
			&cli.StringFlag{Name: "provider-profile", Usage: "provider profile(mock|openai|generic|codex|claude|gemini|antigravity)"},
			&cli.StringFlag{Name: "task-profile", Usage: "task profile(general|coding)", Value: config.TemplateTaskProfileGeneral},
			&cli.BoolFlag{Name: "stdout", Usage: "只打印模板，不写入文件"},
			&cli.BoolFlag{Name: "force", Usage: "允许覆盖已存在的配置文件"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runConfigInitCommand(
				cmd.Writer,
				cmd.String("config"),
				cmd.String("preset"),
				cmd.String("mode"),
				cmd.String("provider-profile"),
				cmd.String("task-profile"),
				cmd.Bool("stdout"),
				cmd.Bool("force"),
			)
		},
	}
}

func newConfigValidateCommand() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "校验配置",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			if _, err := config.LoadWithProfile(path, cmd.String("profile")); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.Writer, "config is valid: %s\n", path)
			return nil
		},
	}
}

func newConfigAddProviderCommand() *cli.Command {
	return &cli.Command{
		Name:  "add-provider",
		Usage: "在模板配置基础上增量新增 provider",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "id", Usage: "provider id", Required: true},
			&cli.StringFlag{Name: "type", Usage: "provider 类型(api|cli|sdk|mock)", Required: true},
			&cli.StringFlag{Name: "model-id", Usage: "模型 id"},
			&cli.StringFlag{Name: "provider-model", Usage: "provider 实际模型名"},
			&cli.IntFlag{Name: "context-window", Usage: "provider model 上下文窗口"},
			&cli.IntFlag{Name: "max-output-tokens", Usage: "provider model 默认最大输出 token"},
			&cli.StringFlag{Name: "protocol", Usage: "api 协议(openai-compatible|anthropic-compatible|gemini-api)"},
			&cli.StringFlag{Name: "base-url", Usage: "api base url"},
			&cli.StringFlag{Name: "api-key-env", Usage: "api key 环境变量名"},
			&cli.StringSliceFlag{Name: "header", Usage: "重复传入 KEY=VALUE"},
			&cli.StringFlag{Name: "cli-type", Usage: "cli 适配类型"},
			&cli.StringFlag{Name: "command", Usage: "命令名"},
			&cli.StringSliceFlag{Name: "arg", Usage: "重复传入额外参数"},
			&cli.StringSliceFlag{Name: "env", Usage: "重复传入 KEY=VALUE"},
			&cli.StringFlag{Name: "adapter", Usage: "sdk adapter 可执行文件"},
			&cli.StringSliceFlag{Name: "option", Usage: "重复传入 KEY=VALUE，VALUE 优先按 JSON 解析"},
			&cli.StringFlag{Name: "behavior", Usage: "mock 行为"},
			&cli.DurationFlag{Name: "delay", Usage: "mock 延迟"},
			&cli.StringFlag{Name: "error", Usage: "mock 错误信息"},
			&cli.Float64Flag{Name: "temperature", Usage: "provider model 默认 temperature"},
			&cli.StringFlag{Name: "reasoning", Usage: "provider model 默认 reasoning"},
			&cli.StringFlag{Name: "agent", Usage: "可选：同步创建一个 agent id"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.Load(path)
			if err != nil {
				return err
			}
			headers, err := parseStringAssignments(cmd.StringSlice("header"))
			if err != nil {
				return err
			}
			env, err := parseStringAssignments(cmd.StringSlice("env"))
			if err != nil {
				return err
			}
			options, err := parseAnyAssignments(cmd.StringSlice("option"))
			if err != nil {
				return err
			}
			input := config.AddProviderInput{
				ID:              cmd.String("id"),
				Type:            cmd.String("type"),
				ModelID:         cmd.String("model-id"),
				ProviderModel:   cmd.String("provider-model"),
				ContextWindow:   cmd.Int("context-window"),
				MaxOutputTokens: cmd.Int("max-output-tokens"),
				Protocol:        cmd.String("protocol"),
				BaseURL:         cmd.String("base-url"),
				APIKeyEnv:       cmd.String("api-key-env"),
				Headers:         headers,
				CLIType:         cmd.String("cli-type"),
				Command:         cmd.String("command"),
				Args:            cmd.StringSlice("arg"),
				Env:             env,
				Adapter:         cmd.String("adapter"),
				Options:         options,
				Behavior:        cmd.String("behavior"),
				Delay:           config.Duration{Duration: cmd.Duration("delay")},
				Error:           cmd.String("error"),
				Reasoning:       cmd.String("reasoning"),
				AgentID:         cmd.String("agent"),
			}
			if cmd.IsSet("temperature") {
				value := cmd.Float64("temperature")
				input.Temperature = &value
			}
			next, err := config.ApplyAddProvider(loaded.Config, input)
			if err != nil {
				return err
			}
			if err := config.Write(path, next); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.Writer, "provider added: %s\n", input.ID)
			if input.AgentID != "" {
				_, _ = fmt.Fprintf(cmd.Writer, "agent added: %s\n", input.AgentID)
			}
			return nil
		},
	}
}

func newConfigAddAgentCommand() *cli.Command {
	return &cli.Command{
		Name:  "add-agent",
		Usage: "在模板配置基础上增量新增 agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "id", Usage: "agent id", Required: true},
			&cli.StringFlag{Name: "provider", Usage: "provider id", Required: true},
			&cli.StringFlag{Name: "model", Usage: "模型 id"},
			&cli.StringFlag{Name: "role", Usage: "agent role"},
			&cli.StringFlag{Name: "system-prompt", Usage: "system prompt"},
			&cli.DurationFlag{Name: "timeout", Usage: "agent 超时"},
			&cli.Float64Flag{Name: "temperature", Usage: "agent temperature"},
			&cli.StringFlag{Name: "reasoning", Usage: "agent reasoning"},
			&cli.StringSliceFlag{Name: "assign", Usage: "分配到 proposer|challenger|participant|arbiter|semantic-verifier|facilitator|reporter|actor"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.Load(path)
			if err != nil {
				return err
			}
			input := config.AddAgentInput{
				ID:           cmd.String("id"),
				Provider:     cmd.String("provider"),
				Model:        cmd.String("model"),
				Role:         cmd.String("role"),
				SystemPrompt: cmd.String("system-prompt"),
				Timeout:      config.Duration{Duration: cmd.Duration("timeout")},
				Reasoning:    cmd.String("reasoning"),
				Assigns:      cmd.StringSlice("assign"),
			}
			if cmd.IsSet("temperature") {
				value := cmd.Float64("temperature")
				input.Temperature = &value
			}
			next, err := config.ApplyAddAgent(loaded.Config, input)
			if err != nil {
				return err
			}
			if err := config.Write(path, next); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.Writer, "agent added: %s\n", input.ID)
			return nil
		},
	}
}

func runConfigInitCommand(writer io.Writer, explicitPath string, preset string, mode string, providerProfile string, taskProfile string, stdout bool, force bool) error {
	body, selection, err := config.RenderTemplateRequest(preset, mode, providerProfile, taskProfile)
	if err != nil {
		return err
	}
	if stdout {
		_, _ = io.WriteString(writer, body)
		return nil
	}
	path := explicitPath
	if path == "" {
		defaultPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		path = defaultPath
	}
	if err := config.WriteTemplateSelection(path, selection, force); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(
		writer,
		"config template written: %s (mode=%s provider_profile=%s task_profile=%s)\n",
		path,
		selection.Mode,
		selection.ProviderProfile,
		selection.TaskProfile,
	)
	return nil
}

func parseStringAssignments(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid assignment: %s", item)
		}
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}

func parseAnyAssignments(items []string) (map[string]any, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(items))
	for _, item := range items {
		key, raw, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid assignment: %s", item)
		}
		value, err := parseScalarOrJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("parse option %s: %w", key, err)
		}
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}

func parseScalarOrJSON(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	var decoded any
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) ||
		(strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"")) {
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return decoded, nil
		}
	}
	if value, err := strconv.ParseBool(trimmed); err == nil {
		return value, nil
	}
	if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return value, nil
	}
	if value, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return value, nil
	}
	return raw, nil
}
