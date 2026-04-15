package main

import (
	"context"
	"os"

	"github.com/suchasplus/til-consensus/internal/app"
)

func main() {
	cmd := app.New()
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
