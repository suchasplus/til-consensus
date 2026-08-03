//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCommandCancellationStopsPromptly(t *testing.T) {
	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
	}{
		{
			name: "cancel",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				time.AfterFunc(50*time.Millisecond, cancel)
				return ctx, cancel
			},
		},
		{
			name: "deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.context()
			defer cancel()
			started := time.Now()
			_, err := runCommand(ctx, "generic", "sh", []string{"-c", "sleep 30"}, nil, "")
			if err == nil {
				t.Fatal("expected canceled command to fail")
			}
			if elapsed := time.Since(started); elapsed > 3*time.Second {
				t.Fatalf("canceled command took too long: %s", elapsed)
			}
		})
	}
}

func TestForceKillCmdKillsProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", `sleep 30 & child=$!; printf '%s\n' "$child"; wait "$child"`)
	prepareCmdForKill(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe failed: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = forceKillCmd(cmd)
		_ = cmd.Wait()
		t.Fatalf("read grandchild pid: %v", err)
	}
	grandchildPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		_ = forceKillCmd(cmd)
		_ = cmd.Wait()
		t.Fatalf("parse grandchild pid %q: %v", line, err)
	}
	parentPID := cmd.Process.Pid
	if err := forceKillCmd(cmd); err != nil {
		t.Fatalf("forceKillCmd failed: %v", err)
	}
	_ = cmd.Wait()
	for _, pid := range []int{parentPID, grandchildPID} {
		if !waitForProcessGone(pid, 2*time.Second) {
			t.Fatalf("process %d still exists after process-group kill", pid)
		}
	}
	if err := forceKillCmd(cmd); err != nil {
		t.Fatalf("repeated forceKillCmd should be idempotent: %v", err)
	}
}

func waitForProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestForceKillCmdWithoutSetpgidTargetsOnlyTheChild(t *testing.T) {
	// A command that never went through prepareCmdForKill must NOT be killed via
	// the negative-PID group path: -pid would resolve to the test binary's own
	// process group. Verify the child still dies and this process survives.
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		t.Fatal("test precondition failed: command must not be prepared for group kill")
	}
	childPID := cmd.Process.Pid
	if err := forceKillCmd(cmd); err != nil {
		t.Fatalf("forceKillCmd failed: %v", err)
	}
	_ = cmd.Wait()
	if !waitForProcessGone(childPID, 2*time.Second) {
		t.Fatalf("child %d still exists after kill", childPID)
	}
	// Still alive: the group signal was not sent to our own process group.
	if err := syscall.Kill(os.Getpid(), 0); err != nil {
		t.Fatalf("test process should be unaffected, got %v", err)
	}
}
