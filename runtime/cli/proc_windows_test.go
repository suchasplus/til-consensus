//go:build windows

package cli

import (
	"errors"
	"os/exec"
	"testing"
)

func TestPrepareCmdForKillSetsNewProcessGroup(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "0")
	prepareCmdForKill(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&createNewProcessGroup == 0 {
		t.Fatalf("expected CREATE_NEW_PROCESS_GROUP, got %#v", cmd.SysProcAttr)
	}
}

func TestForceKillCmdUsesTaskkillAndFallsBack(t *testing.T) {
	original := runTaskkill
	t.Cleanup(func() { runTaskkill = original })

	t.Run("taskkill success", func(t *testing.T) {
		calledPID := 0
		runTaskkill = func(pid int) error {
			calledPID = pid
			return nil
		}
		cmd := exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()
		if err := forceKillCmd(cmd); err != nil {
			t.Fatalf("forceKillCmd failed: %v", err)
		}
		if calledPID != cmd.Process.Pid {
			t.Fatalf("taskkill pid: got=%d want=%d", calledPID, cmd.Process.Pid)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		runTaskkill = func(int) error { return errors.New("taskkill unavailable") }
		cmd := exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if err := forceKillCmd(cmd); err != nil {
			t.Fatalf("forceKillCmd fallback failed: %v", err)
		}
		_ = cmd.Wait()
	})
}
