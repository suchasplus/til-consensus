package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/telemetry"
	"github.com/urfave/cli/v3"
)

func newTelemetryCommand() *cli.Command {
	return &cli.Command{
		Name:  "telemetry",
		Usage: "生成 telemetry 聚合结果",
		Commands: []*cli.Command{
			newTelemetryDailyCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newTelemetryDailyCommand() *cli.Command {
	return &cli.Command{
		Name:  "daily",
		Usage: "扫描运行目录，生成每日 telemetry markdown 汇总",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径；未设置 --root 时用于推导输出目录"},
			&cli.StringFlag{Name: "root", Usage: "要扫描的运行根目录；默认从 config 推导，或使用 ./logs/out"},
			&cli.DurationFlag{Name: "since", Usage: "回溯时间窗口", Value: 24 * time.Hour},
			&cli.StringFlag{Name: "output", Usage: "输出 markdown 文件路径；未设置时打印到 stdout"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runTelemetryDailyCommand(ctx, cmd)
		},
	}
}

func runTelemetryDailyCommand(_ context.Context, cmd *cli.Command) error {
	root, err := resolveTelemetryRoot(cmd.String("config"), cmd.String("root"))
	if err != nil {
		return err
	}
	report, err := telemetry.BuildDailyReport(root, time.Now().Add(-cmd.Duration("since")), time.Now())
	if err != nil {
		return err
	}
	body := telemetry.RenderDailyMarkdown(report)
	if outputPath := strings.TrimSpace(cmd.String("output")); outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("create telemetry output dir: %w", err)
		}
		if err := os.WriteFile(outputPath, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write telemetry report: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.Writer, "telemetry report written: %s\n", outputPath)
		return nil
	}
	_, _ = fmt.Fprint(cmd.Writer, body)
	return nil
}

func resolveTelemetryRoot(configPath string, root string) (string, error) {
	if root = strings.TrimSpace(root); root != "" {
		return root, nil
	}
	if strings.TrimSpace(configPath) == "" {
		return filepath.Clean("./logs/out"), nil
	}
	resolved, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return "", err
	}
	loaded, err := config.Load(resolved)
	if err != nil {
		return "", err
	}
	template := config.ResolveResultTemplate(loaded)
	return filepath.Dir(filepath.Dir(template)), nil
}
