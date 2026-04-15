package app

import (
	"context"

	"github.com/urfave/cli/v3"
)

func New() *cli.Command {
	return &cli.Command{
		Name:  "til-consensus",
		Usage: "裁决式多 agent CLI",
		Commands: []*cli.Command{
			newRunCommand(),
			newConfigCommand(),
			newActCommand(),
			newViewCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowAppHelp(cmd)
		},
	}
}
