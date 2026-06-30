package app

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/internal/buildinfo"
	"github.com/urfave/cli/v3"
)

func init() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		_, _ = fmt.Fprint(cmd.Root().Writer, buildinfo.Format())
	}
}

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
			newAskCommand(),
			newDebateCommand(),
			newDelphiCommand(),
			newClassifyCommand(),
			newSetupCommand(),
			newFollowUpCommand(),
			newConfigCommand(),
			newProfileCommand(),
			newDoctorCommand(),
			newArtifactCommand(),
			newLastCommand(),
			newInspectCommand(),
			newLogsCommand(),
			newOpenCommand(),
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
