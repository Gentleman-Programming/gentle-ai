//go:build windows

package opencode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveManagedLauncherRemovesOwnedLauncherOnWindows(t *testing.T) {
	home := t.TempDir()
	path := WindowsCMDPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(windowsCMDLauncher(`C:\old\opencode.exe`)), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RemoveManagedLauncher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Status, ManagedLauncherRemovalRemoved; got != want {
		t.Fatalf("removal status = %q, want %q", got, want)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned launcher after removal = %v, want absent", err)
	}
}

func TestRemoveManagedLauncherReturnsWindowsCloseError(t *testing.T) {
	home := t.TempDir()
	path := WindowsCMDPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(windowsCMDLauncher(`C:\old\opencode.exe`)), 0o644); err != nil {
		t.Fatal(err)
	}

	closeErr := errors.New("injected launcher close failure")
	originalClose := closeManagedLauncherFile
	calls := 0
	t.Cleanup(func() { closeManagedLauncherFile = originalClose })
	closeManagedLauncherFile = func(file *os.File) error {
		calls++
		if err := file.Close(); err != nil {
			return err
		}
		return closeErr
	}

	result, err := RemoveManagedLauncher(path)
	if !errors.Is(err, closeErr) {
		t.Fatalf("removal error = %v, want %v", err, closeErr)
	}
	if result.Removed() {
		t.Fatalf("removal result = %#v, want no successful removal", result)
	}
	if calls != 1 {
		t.Fatalf("launcher close calls = %d, want 1", calls)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("launcher after close error = %v, want absent", statErr)
	}
}
