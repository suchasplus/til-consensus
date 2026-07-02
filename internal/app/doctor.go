package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tildoctor "github.com/suchasplus/til-consensus/doctor"
	"github.com/urfave/cli/v3"
)

func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "检查配置、输出目录和 provider 可用性",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.BoolFlag{Name: "providers", Usage: "真实调用 provider 做 readiness preflight"},
			&cli.BoolFlag{Name: "all", Usage: "执行所有检查，包含 provider preflight"},
			&cli.BoolFlag{Name: "strict", Usage: "有 warning 时也返回非零 exit code"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.DurationFlag{Name: "timeout", Usage: "provider preflight 单项超时", Value: 90 * time.Second},
			&cli.BoolFlag{Name: "verbose", Usage: "展示更完整的 provider readiness 信息"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			report := tildoctor.Run(ctx, tildoctor.Options{
				ConfigPath: cmd.String("config"),
				Profile:    cmd.String("profile"),
				Providers:  cmd.Bool("providers"),
				All:        cmd.Bool("all"),
				Timeout:    cmd.Duration("timeout"),
			})
			if err := writeDoctorReport(cmd.Writer, report, cmd.String("format"), cmd.Bool("verbose")); err != nil {
				return err
			}
			if report.Summary.Fail > 0 {
				return appError(doctorExitCode(report), fmt.Sprintf("doctor failed: %d check(s) failed", report.Summary.Fail), "查看上方 fail 项并修复后重试", nil)
			}
			if cmd.Bool("strict") && report.Summary.Warn > 0 {
				return appError(ExitProviderNotReady, fmt.Sprintf("doctor warnings: %d check(s) warned", report.Summary.Warn), "去掉 --strict 可允许 warning 返回 0", nil)
			}
			return nil
		},
	}
}

func doctorExitCode(report tildoctor.Report) int {
	code := ExitInternalError
	for _, check := range report.Checks {
		if check.Status != tildoctor.StatusFail {
			continue
		}
		switch {
		case check.Name == "config.resolve":
			return ExitConfigNotFound
		case strings.HasPrefix(check.Name, "config."):
			code = ExitConfigInvalid
		case strings.HasPrefix(check.Name, "provider."):
			if code == ExitInternalError {
				code = ExitProviderNotReady
			}
		case strings.HasPrefix(check.Name, "output."):
			if code == ExitInternalError {
				code = ExitArtifactInvalid
			}
		}
	}
	return code
}

func writeDoctorReport(writer interface{ Write([]byte) (int, error) }, report tildoctor.Report, format string, verbose bool) error {
	switch strings.TrimSpace(format) {
	case "", "text":
		_, _ = fmt.Fprintf(writer, "[til-consensus] doctor ok=%d warn=%d fail=%d\n", report.Summary.OK, report.Summary.Warn, report.Summary.Fail)
		for _, check := range report.Checks {
			prefix := "[" + string(check.Status) + "]"
			_, _ = fmt.Fprintf(writer, "%-6s %s: %s\n", prefix, check.Name, check.Message)
			if verbose && check.Hint != "" {
				_, _ = fmt.Fprintf(writer, "       hint: %s\n", check.Hint)
			}
		}
	case "json":
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal doctor report: %w", err)
		}
		_, _ = fmt.Fprintln(writer, string(body))
	default:
		return appError(ExitUsageError, "unsupported doctor format: "+format, "使用 --format text 或 --format json", nil)
	}
	return nil
}
