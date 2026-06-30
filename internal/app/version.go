package app

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/internal/buildinfo"
	"github.com/urfave/cli/v3"
)

func newVersionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "显示版本信息",
		Action: func(_ context.Context, cmd *cli.Command) error {
			_, _ = fmt.Fprint(cmd.Writer, buildinfo.Format())
			return nil
		},
	}
}
