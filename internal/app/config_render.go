package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	"github.com/urfave/cli/v3"
)

func newConfigRenderCommand() *cli.Command {
	return &cli.Command{
		Name:  "render",
		Usage: "渲染 include/overlay 后的最终配置",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(yaml|json)", Value: "yaml"},
			&cli.BoolFlag{Name: "profiles-only", Usage: "只校验 provider / agent profile，不校验完整 workflow roles"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.LoadWithTraceAndProfile(path, cmd.Bool("profiles-only"), cmd.String("profile"))
			if err != nil {
				return err
			}
			switch strings.TrimSpace(cmd.String("format")) {
			case "", "yaml":
				body, err := config.RenderYAML(loaded.Config)
				if err != nil {
					return fmt.Errorf("marshal rendered config: %w", err)
				}
				_, _ = cmd.Writer.Write(body)
			case "json":
				body, err := config.RenderJSON(loaded.Config)
				if err != nil {
					return fmt.Errorf("marshal rendered config: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.Writer, string(body))
			default:
				return appError(ExitUsageError, "unsupported config render format: "+cmd.String("format"), "使用 --format yaml 或 --format json", nil)
			}
			return nil
		},
	}
}

func newConfigExplainCommand() *cli.Command {
	return &cli.Command{
		Name:  "explain",
		Usage: "解释最终生效的 provider、agent、roles 和输出路径",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.StringFlag{Name: "provider", Usage: "只展示指定 provider"},
			&cli.StringFlag{Name: "agent", Usage: "只展示指定 agent"},
			&cli.BoolFlag{Name: "profiles-only", Usage: "只校验 provider / agent profile，不校验完整 workflow roles"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.LoadWithTraceAndProfile(path, cmd.Bool("profiles-only"), cmd.String("profile"))
			if err != nil {
				return err
			}
			report := config.BuildExplainReport(loaded, config.ExplainOptions{
				ProviderFilter: cmd.String("provider"),
				AgentFilter:    cmd.String("agent"),
			})
			switch strings.TrimSpace(cmd.String("format")) {
			case "", "text":
				_, _ = fmt.Fprint(cmd.Writer, config.RenderExplainText(report))
			case "json":
				body, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal config explain report: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.Writer, string(body))
			default:
				return appError(ExitUsageError, "unsupported config explain format: "+cmd.String("format"), "使用 --format text 或 --format json", nil)
			}
			return nil
		},
	}
}
