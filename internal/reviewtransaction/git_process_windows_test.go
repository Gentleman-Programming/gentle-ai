//go:build windows

package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
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
