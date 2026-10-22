//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func context7ProofCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return cmd
}

func context7ProofOutput(cmd *exec.Cmd) ([]byte, error) {
	defer func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	return cmd.CombinedOutput()
}

func TestContext7ProofCommandBounds(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		deadline     time.Duration
		pipe, cancel bool
	}{
		{name: "normal", script: "printf canonical", deadline: 3 * time.Second},
		{name: "held-pipe", script: "/bin/sleep 4 &", deadline: 3 * time.Second, pipe: true},
		{name: "canceled-group", script: "/bin/sleep 4 & wait", deadline: 100 * time.Millisecond, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stop := context.WithTimeout(context.Background(), tc.deadline)
			defer stop()
			cmd := context7ProofCommand(ctx, "/bin/sh", "-c", tc.script)
			started := time.Now()
			output, err := context7ProofOutput(cmd)
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("command wait exceeded bound: %v, %v", elapsed, err)
			}
			if cmd.ProcessState == nil {
				t.Fatal("direct child was not waited")
			}
			switch {
			case tc.pipe:
				if !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("held pipe did not hit wait bound: %v", err)
				}
			case tc.cancel:
				if err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					t.Fatalf("group cancellation not reported: %v, %v", err, ctx.Err())
				}
			default:
				if err != nil || string(output) != "canonical" || !cmd.ProcessState.Success() {
					t.Fatalf("normal command changed: %q, %v", output, err)
				}
			}
		})
	}
}
