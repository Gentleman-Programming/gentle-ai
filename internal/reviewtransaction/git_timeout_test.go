package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRunGitBoundsLocalAndRemoteCommands(t *testing.T) {
	originalCommand := gitCommandContext
	originalLocal := localGitCommandTimeout
	originalRemote := remoteGitCommandTimeout
	originalWaitDelay := gitCommandWaitDelay
	t.Cleanup(func() {
		gitCommandContext = originalCommand
		localGitCommandTimeout = originalLocal
		remoteGitCommandTimeout = originalRemote
		gitCommandWaitDelay = originalWaitDelay
	})
	t.Setenv("GENTLE_AI_GIT_TIMEOUT_HELPER", "sleep")
	gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunGitTimeoutHelper$", "--")
	}
	localGitCommandTimeout = 25 * time.Millisecond
	remoteGitCommandTimeout = 35 * time.Millisecond
	gitCommandWaitDelay = 10 * time.Millisecond

	for _, tt := range []struct {
		name   string
		args   []string
		remote bool
		budget time.Duration
	}{
		{name: "local", args: []string{"status", "--short"}, budget: localGitCommandTimeout},
		{name: "remote", args: []string{"ls-remote", "origin", "HEAD"}, remote: true, budget: remoteGitCommandTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			started := time.Now()
			_, err := runGit(context.Background(), t.TempDir(), nil, nil, tt.args...)
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("bounded git command took %s", elapsed)
			}
			var timeout *GitCommandTimeoutError
			if !errors.As(err, &timeout) || !errors.Is(err, ErrGitCommandTimeout) {
				t.Fatalf("runGit error = %T %v", err, err)
			}
			if timeout.Remote != tt.remote || timeout.Timeout != tt.budget || timeout.Aggregate {
				t.Fatalf("timeout = %#v", timeout)
			}
		})
	}
	marker := t.TempDir() + "/escaped"
	t.Setenv("GENTLE_AI_GIT_TIMEOUT_HELPER", "parent-exit")
	t.Setenv("GENTLE_AI_GIT_ESCAPE_MARKER", marker)
	localGitCommandTimeout = time.Second
	_, _ = runGit(context.Background(), t.TempDir(), nil, nil, "status")
	time.Sleep(2100 * time.Millisecond)
	if payload, err := os.ReadFile(marker); !os.IsNotExist(err) {
		t.Fatalf("leader-exit descendant escaped containment: %q (%v)", payload, err)
	}
}

func TestRunGitDistinguishesAggregateDeadlineAndTypedExit(t *testing.T) {
	originalCommand := gitCommandContext
	originalLocal := localGitCommandTimeout
	t.Cleanup(func() {
		gitCommandContext = originalCommand
		localGitCommandTimeout = originalLocal
	})
	gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunGitTimeoutHelper$", "--")
	}
	localGitCommandTimeout = time.Second

	t.Setenv("GENTLE_AI_GIT_TIMEOUT_HELPER", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := runGit(ctx, t.TempDir(), nil, nil, "status")
	var timeout *GitCommandTimeoutError
	if !errors.As(err, &timeout) || !timeout.Aggregate || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("aggregate timeout = %T %#v", err, timeout)
	}

	t.Setenv("GENTLE_AI_GIT_TIMEOUT_HELPER", "exit")
	_, err = runGit(context.Background(), t.TempDir(), nil, nil, "status")
	var commandErr *GitCommandError
	if !errors.As(err, &commandErr) || commandErr.ExitCode != 7 || commandErr.Remote || errors.Is(err, ErrGitCommandTimeout) {
		t.Fatalf("typed git exit = %T %#v", err, commandErr)
	}
}

func TestRunGitTimeoutHelper(t *testing.T) {
	switch os.Getenv("GENTLE_AI_GIT_TIMEOUT_HELPER") {
	case "parent-exit":
		child := exec.Command(os.Args[0], "-test.run=^TestRunGitTimeoutHelper$", "--")
		child.Env = append(os.Environ(), "GENTLE_AI_GIT_TIMEOUT_HELPER=mark")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
	case "mark":
		time.Sleep(2 * time.Second)
		_ = os.WriteFile(os.Getenv("GENTLE_AI_GIT_ESCAPE_MARKER"), []byte("escaped"), 0o644)
	case "sleep":
		time.Sleep(10 * time.Second)
	case "exit":
		os.Exit(7)
	default:
		return
	}
}
