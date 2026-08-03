package cli

import (
	"fmt"
	"strings"
)

type CommandError struct {
	CLIType  string
	Command  []string
	ExitCode int
	Stderr   string
	Stdout   string
	Err      error
}

func (e *CommandError) Error() string {
	if e == nil {
		return "cli command failed"
	}
	command := strings.Join(e.Command, " ")
	if command == "" {
		command = e.CLIType
	}
	// ExitCode < 0 means the process never reported an exit status: it was
	// terminated by cancellation or timeout. Reporting "exited with code -1"
	// would be misleading, so name the actual cause instead.
	message := fmt.Sprintf("cli command %s exited with code %d", command, e.ExitCode)
	if e.ExitCode < 0 {
		message = fmt.Sprintf("cli command %s terminated before exit", command)
		if e.Err != nil {
			message += ": " + e.Err.Error()
		}
	}
	if strings.TrimSpace(e.Stderr) != "" {
		message += " stderr=" + strings.TrimSpace(e.Stderr)
	}
	if strings.TrimSpace(e.Stdout) != "" {
		message += " stdout=" + strings.TrimSpace(e.Stdout)
	}
	return message
}

func (e *CommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
