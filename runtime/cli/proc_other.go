//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package cli

import (
	"errors"
	"os"
	"os/exec"
)

func prepareCmdForKill(_ *exec.Cmd) {}

func interruptCmd(cmd *exec.Cmd) error { return forceKillCmd(cmd) }

func forceKillCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Kill()
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
