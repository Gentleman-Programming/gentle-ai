package reviewtransaction

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunGitTypesProcessStartFailure(t *testing.T) {
	originalCommand := gitCommandContext
	t.Cleanup(func() { gitCommandContext = originalCommand })
	missing := filepath.Join(t.TempDir(), "missing-git-binary")
	gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, missing)
	}

	_, err := runGit(context.Background(), t.TempDir(), nil, nil, "status", "--short")
	var control *GitProcessControlError
	if !errors.As(err, &control) {
		t.Fatalf("runGit start failure = %T %v", err, err)
	}
	if strings.Join(control.Args, " ") != "status --short" {
		t.Fatalf("process control args = %#v", control.Args)
	}
	// Unix exec start failures wrap ENOENT (fs.ErrNotExist); Windows lookup
	// failures wrap exec.ErrNotFound. Either proves the cause chain survived.
	if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("process control cause lost: %v", err)
	}
	if message := err.Error(); !strings.Contains(message, "status --short") || !strings.Contains(message, filepath.Base(missing)) {
		t.Fatalf("process control message = %q", message)
	}
}

func TestRunGitExpiredContextWinsOverProcessTreeStartFailure(t *testing.T) {
	originalStarter := gitProcessTreeStarter
	t.Cleanup(func() { gitProcessTreeStarter = originalStarter })
	gitProcessTreeStarter = func(*exec.Cmd) (func() error, error) {
		return nil, errors.New("job object creation rejected")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := runGit(ctx, t.TempDir(), nil, nil, "status")
	var timeout *GitCommandTimeoutError
	if !errors.As(err, &timeout) || !timeout.Aggregate || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired-context start failure = %T %v", err, err)
	}
	var control *GitProcessControlError
	if errors.As(err, &control) {
		t.Fatalf("expired context misclassified as process control: %v", err)
	}
}

func TestRunGitTypesProcessTreeControlFailureAndReleasesTree(t *testing.T) {
	originalStarter := gitProcessTreeStarter
	t.Cleanup(func() { gitProcessTreeStarter = originalStarter })
	released := false
	cause := errors.New("job object assignment denied (0xC0000022)")
	gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		return func() error {
			released = true
			return command.Process.Kill()
		}, cause
	}

	_, err := runGit(context.Background(), t.TempDir(), nil, nil, "rev-parse", "HEAD")
	var control *GitProcessControlError
	if !errors.As(err, &control) {
		t.Fatalf("runGit process-tree failure = %T %v", err, err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("process control cause lost: %v", err)
	}
	if !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("process control message = %q", err.Error())
	}
	if !released {
		t.Fatal("failed process tree was not released")
	}
}
