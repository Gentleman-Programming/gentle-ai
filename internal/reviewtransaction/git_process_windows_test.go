//go:build windows

package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	freeConsole      = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")
)

// TestStartGitProcessTreeSurvivesRepeatedLaunches exercises the real
// suspended-start / job-object / resume sequence many times in one process.
// The Windows crashes in #4081, #4128 and #4152 were a Go runtime
// "unknown caller pc" fault surfacing at roughly one launch in four, so a
// single successful launch proves nothing; a burst of launches under the
// garbage collector does.
func TestStartGitProcessTreeSurvivesRepeatedLaunches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	for i := 0; i < 120; i++ {
		command := exec.CommandContext(ctx, "git", "--version")
		var output bytes.Buffer
		command.Stdout = &output
		release, err := startGitProcessTree(command)
		if err != nil {
			t.Fatalf("launch %d: startGitProcessTree: %v", i, err)
		}
		if err := command.Wait(); err != nil {
			t.Fatalf("launch %d: git --version: %v", i, err)
		}
		if err := release(); err != nil {
			t.Fatalf("launch %d: release: %v", i, err)
		}
		if !bytes.Contains(output.Bytes(), []byte("git version")) {
			t.Fatalf("launch %d: unexpected output %q", i, output.String())
		}
	}
}

// A launch whose child exists but cannot be bound to its job must not leave
// that child suspended forever: the bind error is returned, no release is
// handed out, and the child has already exited when Wait returns.
func TestStartGitProcessTreeKillsUnboundChildOnFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	bindErr := errors.New("job binding refused for the test")
	previous := assignGitProcessToJob
	assignGitProcessToJob = func(windows.Handle, windows.Handle) error { return bindErr }
	t.Cleanup(func() { assignGitProcessToJob = previous })
	command := exec.Command("git", "--version")
	release, err := startGitProcessTree(command)
	if !errors.Is(err, bindErr) || release != nil {
		t.Fatalf("startGitProcessTree = (release %t, %v), want the bind error and no release", release != nil, err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("a killed suspended child must not report success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the unbound child was left suspended instead of being killed")
	}
}

// TestRunGitStartsWithoutAttachedConsole detaches the helper from the test
// console, then routes a fake Git child through production runGit. The child
// reports whether it has an attached console; CREATE_NO_WINDOW must leave its
// GetConsoleWindow result at zero. Desktop visibility remains a native manual
// smoke concern, not a claim this API-level regression makes.
func TestRunGitStartsWithoutAttachedConsole(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestRunGitConsoleWindowHelper$")
	command.Env = append(os.Environ(), "GENTLE_AI_TEST_GIT_CONSOLE_WINDOW=launcher")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("launch consoleless Git helper: %v: %s", err, output)
	}
	if err := requireNoAttachedConsole(output); err != nil {
		t.Fatal(err)
	}
}

// TestRunGitConsoleWindowHelper is invoked in two processes by
// TestRunGitStartsWithoutAttachedConsole. The launcher removes its inherited
// console; the probe is the fake Git process started by production runGit.
// Successful helpers exit directly so testing.Main cannot append PASS to the
// numeric subprocess protocol. Parent test cleanup is unaffected.
func TestRunGitConsoleWindowHelper(t *testing.T) {
	switch os.Getenv("GENTLE_AI_TEST_GIT_CONSOLE_WINDOW") {
	case "":
		return
	case "probe":
		hwnd, _, _ := getConsoleWindow.Call()
		_, _ = fmt.Fprintln(os.Stdout, hwnd)
		os.Exit(0)
	case "launcher":
		parentWindow, _, _ := getConsoleWindow.Call()
		if parentWindow != 0 {
			result, _, _ := freeConsole.Call()
			if result == 0 {
				t.Fatal("detach launcher from its visible console")
			}
		}
		originalCommand := gitCommandContext
		gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunGitConsoleWindowHelper$")
		}
		t.Setenv("GENTLE_AI_TEST_GIT_CONSOLE_WINDOW", "probe")
		output, err := runGit(context.Background(), t.TempDir(), nil, nil, "rev-parse", "--show-toplevel")
		gitCommandContext = originalCommand
		if err != nil {
			t.Fatalf("runGit fake Git child: %v", err)
		}
		_, _ = os.Stdout.Write(output)
		os.Exit(0)
	default:
		t.Fatalf("unknown console helper mode %q", os.Getenv("GENTLE_AI_TEST_GIT_CONSOLE_WINDOW"))
	}
}

func requireNoAttachedConsole(output []byte) error {
	hwnd, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return fmt.Errorf("fake Git reported an invalid attached-console value %q: %w", output, err)
	}
	if hwnd != 0 {
		return fmt.Errorf("fake Git has an attached console (GetConsoleWindow=%d)", hwnd)
	}
	return nil
}

func TestRequireNoAttachedConsoleRejectsAttachedConsole(t *testing.T) {
	if err := requireNoAttachedConsole([]byte("1\n")); err == nil {
		t.Fatal("attached console was accepted")
	}
}
