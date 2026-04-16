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
			_, _ = fmt.Fprintf(cmd.Writer, "version: %s\n", buildinfo.Short())
			_, _ = fmt.Fprintf(cmd.Writer, "commit: %s\n", buildinfo.CommitID())
			_, _ = fmt.Fprintf(cmd.Writer, "build time: %s\n", buildinfo.BuiltAt())
			_, _ = fmt.Fprintf(cmd.Writer, "dirty: %s\n", buildinfo.IsDirty())
			return nil
		},
	}
}
