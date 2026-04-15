package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

func newViewCommand() *cli.Command {
	return &cli.Command{
		Name:  "view",
		Usage: "打印某次裁决结果的可读摘要",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runViewCommand(ctx, cmd)
		},
	}
}

func runViewCommand(_ context.Context, cmd *cli.Command) error {
	resultPath := cmd.String("result")
	if resultPath == "" {
		configPath, err := config.ResolveConfigPath(cmd.String("config"))
		if err != nil {
			return err
		}
		loaded, err := config.Load(configPath)
		if err != nil {
			return err
		}
		template := config.ResolveResultTemplate(loaded)
		if requestID := cmd.String("request-id"); requestID != "" {
			resultPath = strings.ReplaceAll(template, "{requestId}", requestID)
		} else {
			latest, err := viewer.ResolveLatestRun(template)
			if err != nil {
				return err
			}
			if latest == nil {
				return fmt.Errorf("no completed runs found")
			}
			resultPath = latest.ResultPath
		}
	}
	body, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("read result file: %w", err)
	}
	var result consensus.AdjudicationResult
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode result file: %w", err)
	}
	if result.SchemaVersion != consensus.SchemaVersion {
		return fmt.Errorf("unsupported result schema version: %d", result.SchemaVersion)
	}
	_, _ = fmt.Fprint(cmd.Writer, artifact.BuildSummary(&result))
	return nil
}
