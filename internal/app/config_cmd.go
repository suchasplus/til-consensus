package app

import (
	"context"
	"fmt"
	"os"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/urfave/cli/v3"
)

func newConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "配置管理",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "写入示例配置",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					path := cmd.String("config")
					if path == "" {
						defaultPath, err := config.DefaultConfigPath()
						if err != nil {
							return err
						}
						path = defaultPath
					}
					if _, err := os.Stat(path); err == nil {
						return fmt.Errorf("config already exists: %s", path)
					}
					return config.WriteTemplate(path)
				},
			},
			{
				Name:  "validate",
				Usage: "校验配置",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					path, err := config.ResolveConfigPath(cmd.String("config"))
					if err != nil {
						return err
					}
					if _, err := config.Load(path); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(cmd.Writer, "config is valid: %s\n", path)
					return nil
				},
			},
		},
	}
}
