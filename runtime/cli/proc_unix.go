//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func prepareCmdForKill(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func forceKillCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Signalling -pid only targets the child's own process group when the child
	// was actually started with Setpgid. Without that guard a command that never
	// went through prepareCmdForKill would deliver SIGKILL to the caller's
	// process group — i.e. til-consensus would kill itself.
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		err := cmd.Process.Kill()
		if err == nil || errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func interruptCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	return cmd.Process.Signal(os.Interrupt)
}
