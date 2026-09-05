package statecoord

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestLockPathResolvesHomeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires privileges unavailable in standard test environments")
	}
	realHome := t.TempDir()
	linkedHome := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}

	got, err := LockPath(linkedHome)
	if err != nil {
		t.Fatalf("LockPath() error = %v", err)
	}
	resolvedHome, err := filepath.EvalSymlinks(realHome)
	if err != nil {
		t.Fatalf("EvalSymlinks(real home): %v", err)
	}
	want := state.Path(resolvedHome) + ".lock"
	if got != want {
		t.Fatalf("LockPath() = %q, want %q", got, want)
	}
}
