package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	tilrunner "github.com/suchasplus/til-consensus/runner"
	"github.com/urfave/cli/v3"
)

func newFollowUpCommand() *cli.Command {
	return &cli.Command{
		Name:  "followup",
		Usage: "执行或检查 follow-up case artifact",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "执行 follow-up case artifact",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
					&cli.StringFlag{Name: "artifact", Usage: "follow-up case artifact 路径", Required: true},
					&cli.BoolFlag{Name: "verbose", Usage: "输出详细事件"},
					&cli.BoolFlag{Name: "debug", Usage: "输出完整事件 payload 以及 provider 输入/输出 artifact 路径"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runFollowUpCommand(ctx, cmd)
				},
			},
		},
	}
}

func runFollowUpCommand(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	artifactPath := strings.TrimSpace(cmd.String("artifact"))
	if artifactPath == "" {
		return fmt.Errorf("artifact path is required")
	}
	followup, err := consensus.LoadFollowUpCaseArtifact(artifactPath)
	if err != nil {
		return err
	}
	executor := tilrunner.NewExecutor(loaded)
	plan, err := executor.ResolveRequest(followup.Request, cmd.Bool("verbose"), cmd.Bool("debug"))
	if err != nil {
		return err
	}
	return executeResolvedPlan(ctx, executor, plan, cmd.Writer)
}
