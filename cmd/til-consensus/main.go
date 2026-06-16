package main

import (
	"context"
	"os"
	"strings"

	"github.com/suchasplus/til-consensus/internal/app"
)

func main() {
	cmd := app.New()
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		debug := strings.TrimSpace(os.Getenv("TIL_CONSENSUS_DEBUG_ERRORS")) != ""
		_, _ = os.Stderr.WriteString(app.FormatError(err, debug) + "\n")
		os.Exit(app.ExitCodeForError(err))
	}
}
