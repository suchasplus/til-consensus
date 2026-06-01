package app

import (
	"context"

	"github.com/suchasplus/til-consensus/internal/buildinfo"
	"github.com/urfave/cli/v3"
)

func New() *cli.Command {
	return &cli.Command{
		Name:    "til-consensus",
		Usage:   "裁决式多 agent CLI",
		Version: buildinfo.Short(),
		ExtraInfo: func() map[string]string {
			return buildinfo.Info()
		},
		Commands: []*cli.Command{
			newRunCommand(),
			newFollowUpCommand(),
			newConfigCommand(),
			newProfileCommand(),
			newTelemetryCommand(),
			newActCommand(),
			newSessionCommand(),
			newViewCommand(),
			newVersionCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowAppHelp(cmd)
		},
	}
}
