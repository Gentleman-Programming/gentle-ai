package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func assumeInteractiveTTY(t *testing.T) {
	t.Helper()
	origIsatty := isattyFn
	isattyFn = func(uintptr) bool { return true }
	t.Cleanup(func() { isattyFn = origIsatty })
}

func TestRunArgsNoArgumentRequiresInteractiveStdinAndStdout(t *testing.T) {
	stdinFD, stdoutFD := os.Stdin.Fd(), os.Stdout.Fd()
	tests := []struct {
		name    string
		isTTY   func(uintptr) bool
		wantTTY string
	}{
		{
			name:    "missing stdin",
			isTTY:   func(fd uintptr) bool { return fd == stdoutFD },
			wantTTY: "stdin",
		},
		{
			name:    "missing stdout",
			isTTY:   func(fd uintptr) bool { return fd == stdinFD },
			wantTTY: "stdout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			setupMockHome(t, home)
			if err := state.Write(home, state.InstallState{PendingSync: true}); err != nil {
				t.Fatalf("write pending state: %v", err)
			}

			origIsatty := isattyFn
			origEnsure := ensureCurrentOSSupported
			origDetect := detectSystem
			origSelfUpdate := selfUpdateFn
			origDeferredSync := deferredSyncFn
			origRunTUI := runTUI
			t.Cleanup(func() {
				isattyFn = origIsatty
				ensureCurrentOSSupported = origEnsure
				detectSystem = origDetect
				selfUpdateFn = origSelfUpdate
				deferredSyncFn = origDeferredSync
				runTUI = origRunTUI
			})

			var calls []string
			record := func(name string) {
				calls = append(calls, name)
			}
			isattyFn = tt.isTTY
			ensureCurrentOSSupported = func() error {
				record("platform validation")
				return nil
			}
			detectSystem = func(context.Context) (system.DetectionResult, error) {
				record("system detection")
				return system.DetectionResult{
					System: system.SystemInfo{
						Supported: true,
						Profile:   system.PlatformProfile{OS: "linux", Supported: true},
					},
				}, nil
			}
			selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
				record("self-update")
				return nil
			}
			deferredSyncFn = func() error {
				record("deferred sync")
				return nil
			}
			runTUI = func(tea.Model, ...tea.ProgramOption) (tea.Model, error) {
				record("Bubble Tea")
				return nil, nil
			}

			var output bytes.Buffer
			err := RunArgs(nil, &output)
			if err == nil {
				t.Fatalf("RunArgs(nil) error = nil, want non-nil terminal guidance")
			}
			for _, want := range []string{"--version", "gentle-ai update", "--help"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("RunArgs(nil) error = %q, want actionable guidance containing %q", err, want)
				}
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantTTY) {
				t.Fatalf("RunArgs(nil) error = %q, want missing %s guidance", err, tt.wantTTY)
			}
			if len(calls) != 0 {
				t.Fatalf("RunArgs(nil) reached downstream seams: %v", calls)
			}
		})
	}
}

func TestRunArgsNoArgumentAllowsInteractiveStdinAndStdout(t *testing.T) {
	home := t.TempDir()
	setupMockHome(t, home)

	origIsatty := isattyFn
	origEnsure := ensureCurrentOSSupported
	origDetect := detectSystem
	origSelfUpdate := selfUpdateFn
	origRunTUI := runTUI
	t.Cleanup(func() {
		isattyFn = origIsatty
		ensureCurrentOSSupported = origEnsure
		detectSystem = origDetect
		selfUpdateFn = origSelfUpdate
		runTUI = origRunTUI
	})

	var calls []string
	isattyCalls := 0
	isattyFn = func(uintptr) bool {
		isattyCalls++
		return true
	}
	ensureCurrentOSSupported = func() error {
		calls = append(calls, "platform validation")
		return nil
	}
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		calls = append(calls, "system detection")
		return system.DetectionResult{
			System: system.SystemInfo{
				Supported: true,
				Profile:   system.PlatformProfile{OS: "linux", Supported: true},
			},
		}, nil
	}
	selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
		calls = append(calls, "self-update")
		return nil
	}
	runTUI = func(tea.Model, ...tea.ProgramOption) (tea.Model, error) {
		calls = append(calls, "Bubble Tea")
		return nil, nil
	}

	var output bytes.Buffer
	if err := RunArgs(nil, &output); err != nil {
		t.Fatalf("RunArgs(nil) error = %v, want nil", err)
	}
	if isattyCalls != 2 {
		t.Fatalf("terminal predicate called %d times, want once for stdin and once for stdout", isattyCalls)
	}
	for _, want := range []string{"platform validation", "system detection", "Bubble Tea"} {
		if !slicesContains(calls, want) {
			t.Fatalf("RunArgs(nil) did not reach %s; calls = %v", want, calls)
		}
	}
}

func TestRunArgsExplicitCommandsBypassTTYBoundary(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "version", args: []string{"--version"}},
		{name: "help", args: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origIsatty := isattyFn
			origEnsure := ensureCurrentOSSupported
			origDetect := detectSystem
			t.Cleanup(func() {
				isattyFn = origIsatty
				ensureCurrentOSSupported = origEnsure
				detectSystem = origDetect
			})

			isattyFn = func(uintptr) bool {
				t.Fatal("explicit command evaluated the no-argument terminal boundary")
				return false
			}
			ensureCurrentOSSupported = func() error {
				t.Fatal("explicit command reached platform validation")
				return errors.New("platform validation reached")
			}
			detectSystem = func(context.Context) (system.DetectionResult, error) {
				t.Fatal("explicit command reached system detection")
				return system.DetectionResult{}, errors.New("system detection reached")
			}

			var output bytes.Buffer
			if err := RunArgs(tt.args, &output); err != nil {
				t.Fatalf("RunArgs(%v) error = %v, want nil", tt.args, err)
			}
			if output.Len() == 0 {
				t.Fatalf("RunArgs(%v) produced no output", tt.args)
			}
		})
	}
}

func TestBuiltBinaryClosedStdinRefusesBeforeBubbleTea(t *testing.T) {
	if testing.Short() {
		t.Skip("built-binary process coverage is not run in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("closed-stdin process coverage is compile-only on Windows")
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(sourceFile), "..", "..")
	binaryPath := filepath.Join(t.TempDir(), "gentle-ai")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/gentle-ai")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/gentle-ai: %v\n%s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	// A nil Cmd.Stdin is connected to the null device, proving the closed-stdin path.
	command.Stdin = nil
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("built binary did not terminate promptly: %v", ctx.Err())
	}
	if err == nil {
		t.Fatalf("built binary exited successfully; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("built binary error = %v, want a non-zero process exit", err)
	}

	combined := stdout.String() + stderr.String()
	for _, want := range []string{"--version", "gentle-ai update", "--help"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("built binary output = %q, want actionable guidance containing %q", combined, want)
		}
	}
	if strings.Contains(strings.ToLower(combined), "/dev/tty") {
		t.Fatalf("built binary reached Bubble Tea /dev/tty error: %q", combined)
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
