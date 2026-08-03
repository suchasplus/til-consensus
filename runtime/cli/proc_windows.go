//go:build windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const createNewProcessGroup = 0x00000200

var runTaskkill = func(pid int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

func prepareCmdForKill(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup
}

func forceKillCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	taskkillErr := runTaskkill(cmd.Process.Pid)
	if taskkillErr == nil {
		return nil
	}
	killErr := cmd.Process.Kill()
	if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
		return nil
	}
	return fmt.Errorf("taskkill process tree: %v; kill direct process: %w", taskkillErr, killErr)
}
