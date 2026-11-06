package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestGitCommandBudgetsAreHangGuardsNotLatencyAssertions pins the shape that
// issue #2483 demands: the per-command Git budget exists to catch a genuinely
// hung child, never to assert latency. A 15-second wall-clock budget failed
// TestSnapshotBuilder* on a loaded CI shard where a healthy git needed more
// than 15 seconds of wall time, so the ceiling must sit far above any
// plausible scheduler-starvation dilation while still being finite.
func TestGitCommandBudgetsAreHangGuardsNotLatencyAssertions(t *testing.T) {
	if LocalGitCommandTimeout < time.Minute {
		t.Fatalf("LocalGitCommandTimeout = %s; a per-command hang guard under one minute is a latency assertion that flakes under runner load (issue #2483)", LocalGitCommandTimeout)
	}
	if RemoteGitCommandTimeout < LocalGitCommandTimeout {
		t.Fatalf("RemoteGitCommandTimeout = %s is below the local budget %s; remote commands add network latency on top of local cost", RemoteGitCommandTimeout, LocalGitCommandTimeout)
	}
}

// TestGitCommandTimeoutNamesCommandAndElapsed proves the hang-guard diagnostic
// is actionable: when the budget cuts a child, the error names the exact Git
// command and the observed wall-clock time, so a timeout on a loaded runner is
// explainable instead of being a bare budget number.
func TestGitCommandTimeoutNamesCommandAndElapsed(t *testing.T) {
	originalCommand := gitCommandContext
	originalLocal := LocalGitCommandTimeout
	originalWaitDelay := gitCommandWaitDelay
	t.Cleanup(func() {
		gitCommandContext = originalCommand
		LocalGitCommandTimeout = originalLocal
		gitCommandWaitDelay = originalWaitDelay
	})
	t.Setenv("GENTLE_AI_GIT_TIMEOUT_HELPER", "sleep")
	gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunGitTimeoutHelper$", "--")
	}
	LocalGitCommandTimeout = 50 * time.Millisecond
	gitCommandWaitDelay = 10 * time.Millisecond

	_, err := runGit(context.Background(), t.TempDir(), nil, nil, "status", "--short")
	var timeout *GitCommandTimeoutError
	if !errors.As(err, &timeout) || !errors.Is(err, ErrGitCommandTimeout) {
		t.Fatalf("runGit error = %T %v, want *GitCommandTimeoutError", err, err)
	}
	if timeout.Elapsed <= 0 {
		t.Fatalf("timeout.Elapsed = %s, want the observed wall-clock duration of the cut command", timeout.Elapsed)
	}
	message := timeout.Error()
	if !strings.Contains(message, "git status --short") {
		t.Fatalf("timeout message %q does not name the command", message)
	}
	if !strings.Contains(message, timeout.Elapsed.Round(time.Millisecond).String()) {
		t.Fatalf("timeout message %q does not name the observed elapsed time %s", message, timeout.Elapsed.Round(time.Millisecond))
	}
}
