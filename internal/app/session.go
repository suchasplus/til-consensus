package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	filestore "github.com/suchasplus/til-consensus/store/file"
	"github.com/urfave/cli/v3"
)

func newSessionCommand() *cli.Command {
	return &cli.Command{
		Name:  "session",
		Usage: "查看持久化 session store 中的历史状态",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "列出 sessions，可按 request_id 过滤",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
					&cli.StringFlag{Name: "request-id", Usage: "按 request id 过滤"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runSessionList(ctx, cmd)
				},
			},
			{
				Name:  "show",
				Usage: "查看某个 session snapshot",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
					&cli.StringFlag{Name: "session-id", Usage: "session id", Required: true},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runSessionShow(ctx, cmd)
				},
			},
		},
	}
}

func runSessionList(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	store := filestore.New(config.ResolveSessionStoreDir(loaded))
	requestID := strings.TrimSpace(cmd.String("request-id"))
	var snapshots any
	if requestID == "" {
		snapshots, err = store.List(ctx)
	} else {
		snapshots, err = store.ListByRequestID(ctx, requestID)
	}
	if err != nil {
		return err
	}
	body, err := marshalPretty(snapshots)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.Writer, body)
	return nil
}

func runSessionShow(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	store := filestore.New(config.ResolveSessionStoreDir(loaded))
	snapshot, err := store.Load(ctx, strings.TrimSpace(cmd.String("session-id")))
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("session not found")
	}
	body, err := marshalPretty(snapshot)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.Writer, body)
	return nil
}
