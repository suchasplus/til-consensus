package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/observer"
	"github.com/suchasplus/til-consensus/internal/runtime"
	memorystore "github.com/suchasplus/til-consensus/internal/store/memory"
	"github.com/urfave/cli/v3"
)

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "运行一次 til-consensus 共识流程",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "input", Usage: "输入文件路径"},
			&cli.StringFlag{Name: "task", Usage: "任务描述"},
			&cli.StringFlag{Name: "agents", Usage: "逗号分隔的 agent 列表"},
			&cli.IntFlag{Name: "min-rounds", Usage: "最小辩论轮次"},
			&cli.IntFlag{Name: "max-rounds", Usage: "最大辩论轮次"},
			&cli.Float64Flag{Name: "threshold", Usage: "共识阈值"},
			&cli.DurationFlag{Name: "timeout", Usage: "单轮/单任务超时"},
			&cli.DurationFlag{Name: "global-deadline", Usage: "全局截止时间"},
			&cli.StringFlag{Name: "action", Usage: "共识后执行的 action"},
			&cli.BoolFlag{Name: "verbose", Usage: "输出详细事件"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runCommand(ctx, cmd)
		},
	}
}

func runCommand(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	input, err := config.LoadRunInput(cmd.String("input"))
	if err != nil {
		return err
	}
	overrides := config.RunOverrides{
		ConfigPath:     cmd.String("config"),
		InputPath:      cmd.String("input"),
		Task:           cmd.String("task"),
		Agents:         splitComma(cmd.String("agents")),
		MinRounds:      cmd.Int("min-rounds"),
		MaxRounds:      cmd.Int("max-rounds"),
		Threshold:      cmd.Float64("threshold"),
		Timeout:        cmd.Duration("timeout"),
		GlobalDeadline: cmd.Duration("global-deadline"),
		Action:         cmd.String("action"),
		Verbose:        cmd.Bool("verbose"),
	}
	plan, err := config.ResolveRunPlan(loaded, input, overrides, time.Now().UTC())
	if err != nil {
		return err
	}
	output := NewOutput(cmd.Writer, os.Stderr, plan.Verbose)
	output.RunStarted(plan.RequestID, plan.Task, plan.ParticipantIDs)

	delegate, err := runtime.NewDelegate(loaded.Config, filepath.Dir(plan.EventsPath))
	if err != nil {
		return err
	}
	eventObserver := observer.NewMulti(
		observer.NewJSONL(plan.EventsPath),
		output.EventObserver(),
	)
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: delegate,
		Observer:     eventObserver,
		SessionStore: memorystore.New(),
	})
	result, runErr := engine.Start(ctx, plan.StartRequest)
	if runErr != nil {
		_ = artifact.WriteErrorArtifact(plan.RequestID, plan.ErrorPath, runErr)
		return runErr
	}
	if err := artifact.WriteRunArtifacts(result, plan.ResultPath, plan.SummaryPath); err != nil {
		return err
	}
	output.RunCompleted(plan.ResultPath, plan.SummaryPath)
	return nil
}

func splitComma(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}
