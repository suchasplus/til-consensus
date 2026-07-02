package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	tilrunner "github.com/suchasplus/til-consensus/runner"
	"github.com/urfave/cli/v3"
)

func newActCommand() *cli.Command {
	return &cli.Command{
		Name:  "act",
		Usage: "基于已有 result.json 执行后续 action",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "result", Usage: "result.json 路径", Required: true},
			&cli.StringFlag{Name: "task", Usage: "action 任务描述", Required: true},
			&cli.StringFlag{Name: "agent", Usage: "指定执行 action 的 actor"},
			&cli.DurationFlag{Name: "timeout", Usage: "等待 action 完成的超时"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runActCommand(ctx, cmd)
		},
	}
}

func runActCommand(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	resultPath := cmd.String("result")
	body, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("read result file: %w", err)
	}
	result, err := consensus.DecodeRunResult(body)
	if err != nil {
		return fmt.Errorf("decode result file: %w", err)
	}
	executor := tilrunner.NewExecutor(loaded)
	actionResult, err := executor.Act(ctx, tilrunner.ActionInput{
		Result:       result,
		Prompt:       cmd.String("task"),
		ActorID:      cmd.String("agent"),
		ArtifactsDir: filepath.Join(filepath.Dir(resultPath), "artifacts"),
		Timeout:      cmd.Duration("timeout"),
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.Writer, actionResult.Output.FullResponse)
	return nil
}
