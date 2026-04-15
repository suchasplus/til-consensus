package app

import (
	"context"

	"github.com/urfave/cli/v3"
)

func New() *cli.Command {
	return &cli.Command{
		Name:  "til-consensus",
		Usage: "多 agent 共识 CLI",
		Commands: []*cli.Command{
			newRunCommand(),
			newConfigCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowAppHelp(cmd)
		},
	}
}
