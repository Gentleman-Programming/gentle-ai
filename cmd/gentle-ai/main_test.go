package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestExitCodeFixture(t *testing.T) {
	if value := os.Getenv("GENTLE_TEST_EXIT_STATUS"); value != "" {
		status, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(status)
	}
}

func TestExitCodePreservesShellChildStatus(t *testing.T) {
	for _, status := range []int{1, 2, 130} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, self, "-test.run=^TestExitCodeFixture$")
			cmd.Env = append(os.Environ(), "GENTLE_TEST_EXIT_STATUS="+strconv.Itoa(status))
			childErr := cmd.Run()
			var child *exec.ExitError
			if !errors.As(childErr, &child) || child.ExitCode() != status {
				t.Fatalf("invalid child fixture: %v", childErr)
			}
			for _, tc := range []struct {
				name string
				args []string
				err  error
				want int
			}{
				{"direct shell", []string{"shell", "launch"}, childErr, status},
				{"delegated shell", []string{"shell", "internal-launch"}, fmt.Errorf("child: %w", childErr), status},
				{"uncertain outranks exit", []string{"shell"}, &shellinstaller.PrivateRuntimeError{Kind: "uncertain", Cause: childErr}, 1},
				{"source refusal", []string{"shell"}, &shellinstaller.PrivateRuntimeError{Kind: "source", Cause: childErr}, 1},
				{"unrelated command unchanged", []string{"install"}, childErr, 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := exitCode(tc.args, tc.err); got != tc.want {
						t.Fatalf("exit code = %d, want %d", got, tc.want)
					}
				})
			}
		})
	}
	if got := exitCode([]string{"shell"}, errors.New("clean refusal")); got != 1 {
		t.Fatalf("clean refusal exit code = %d", got)
	}
}

func TestExitCodePreservesShellSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "/bin/sh", "-c", "kill -TERM $$").Run()
	var child *exec.ExitError
	if !errors.As(err, &child) {
		t.Fatalf("invalid signal fixture: %v", err)
	}
	if got := exitCode([]string{"shell", "launch"}, err); got != 143 {
		t.Fatalf("signal exit code = %d, want 143", got)
	}
}
