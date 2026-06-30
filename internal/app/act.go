package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/runtime"
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
	actorID := cmd.String("agent")
	if actorID == "" {
		mode := loaded.Config.Defaults.Mode
		if mode == "" {
			mode = consensus.WorkflowModeAdjudication
		}
		actorID = config.RoleAssignmentsForMode(loaded.Config.Roles, mode).Actor
	}
	if actorID == "" {
		return fmt.Errorf("missing actor agent id")
	}
	delegate, err := runtime.NewDelegate(loaded.Config, filepath.Join(filepath.Dir(resultPath), "artifacts"))
	if err != nil {
		return err
	}
	actionTask := consensus.ActionTask{
		TaskMeta: consensus.TaskMeta{
			SessionID: result.SessionID,
			RequestID: result.RequestID,
			AgentID:   actorID,
			Role:      "actor",
		},
		Prompt: cmd.String("task"),
		Input:  result,
	}
	timeout := cmd.Duration("timeout")
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	_, awaited, _, err := consensus.ExecuteTaskWithRetry(ctx, delegate, actionTask, timeout, consensus.DefaultTaskRetryAttempts, consensus.TaskRetryHooks{})
	if err != nil {
		return err
	}
	if !awaited.OK {
		return fmt.Errorf("action failed: %s", awaited.Error)
	}
	typed, ok := awaited.Output.(consensus.ActionTaskResult)
	if !ok {
		return fmt.Errorf("unexpected action result type")
	}
	_, _ = fmt.Fprintln(cmd.Writer, typed.Output.FullResponse)
	return nil
}
