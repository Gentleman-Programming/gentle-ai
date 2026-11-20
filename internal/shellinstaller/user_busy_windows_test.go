//go:build windows

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// userWindowsHoldFile opens path without FILE_SHARE_DELETE, like a running
// program or an antivirus scan does, and releases it after hold.
func userWindowsHoldFile(t *testing.T, path string, hold time.Duration) <-chan struct{} {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(hold)
		file.Close()
		close(released)
	}()
	t.Cleanup(func() { <-released })
	return released
}

func TestUserWindowsRetryBusyRetriesOnlyTransientErrors(t *testing.T) {
	previous := userWindowsBusySleep
	userWindowsBusySleep = func(time.Duration) {}
	t.Cleanup(func() { userWindowsBusySleep = previous })
	root := t.TempDir()

	calls := 0
	err := userWindowsRetryBusy(root, func() error {
		calls++
		if calls < 3 {
			return &os.LinkError{Op: "rename", Old: "a", New: "b", Err: windows.ERROR_SHARING_VIOLATION}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("transient sharing violation was not retried to success: err=%v calls=%d", err, calls)
	}

	calls = 0
	permanent := errors.New("not a lock")
	if err := userWindowsRetryBusy(root, func() error { calls++; return permanent }); !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("non-transient error must fail at once unchanged: err=%v calls=%d", err, calls)
	}

	calls = 0
	locked := &os.PathError{Op: "remove", Path: "x", Err: windows.ERROR_LOCK_VIOLATION}
	err = userWindowsRetryBusy(root, func() error { calls++; return locked })
	if calls != len(userWindowsBusyDelays)+1 {
		t.Fatalf("persistent lock retried %d times, want %d", calls, len(userWindowsBusyDelays)+1)
	}
	if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || !strings.Contains(err.Error(), "in use by another program") || !strings.Contains(err.Error(), "close Gentle Shell and retry") {
		t.Fatalf("persistent lock error is not actionable or lost its cause: %v", err)
	}
}

func TestUserWindowsRemoveBusyWaitsForTransientHolder(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "codegraph.tgz")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	released := userWindowsHoldFile(t, path, 300*time.Millisecond)
	if err := userWindowsRemoveBusy(root, path); err != nil {
		t.Fatalf("transiently held archive removal failed: %v", err)
	}
	<-released
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("archive survived removal: %v", err)
	}
	// Removing an absent file stays a plain not-exist error for callers.
	if err := userWindowsRemoveBusy(root, path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent archive removal error = %v", err)
	}
}

func TestUninstallWindowsWaitsForTransientlyHeldFile(t *testing.T) {
	target, _ := userWindowsUninstallFixture(t)
	released := userWindowsHoldFile(t, filepath.Join(target, "manifest.json"), 300*time.Millisecond)
	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	<-released
	if err != nil || backup == "" {
		t.Fatalf("uninstall did not retry a transient sharing violation: %v %q", err, backup)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists after uninstall: %v", err)
	}
}

func TestUserWindowsReplaceFileKeepsOriginalWhenWriteFails(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "persona.json")
	if err := userWindowsWrite(path, []byte(`{"mode":"old"}`)); err != nil {
		t.Fatal(err)
	}
	previous := userWindowsReplaceWrite
	userWindowsReplaceWrite = func(string, []byte) error { return errors.New("injected write failure") }
	t.Cleanup(func() { userWindowsReplaceWrite = previous })
	if err := userWindowsReplaceFile(path, []byte(`{"mode":"new"}`)); err == nil {
		t.Fatal("replacement reported success after a failed write")
	}
	data, err := userWindowsRead(path, 4096)
	if err != nil || string(data) != `{"mode":"old"}` {
		t.Fatalf("failed replacement lost the original file: %q %v", data, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("failed replacement left extra entries: %v", entries)
	}
}

func TestUserWindowsReplaceFileWaitsForTransientReader(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "persona.json")
	if err := userWindowsWrite(path, []byte(`{"mode":"old"}`)); err != nil {
		t.Fatal(err)
	}
	released := userWindowsHoldFile(t, path, 300*time.Millisecond)
	err := userWindowsReplaceFile(path, []byte(`{"mode":"new"}`))
	<-released
	if err != nil {
		t.Fatalf("replacement did not wait for a transient reader: %v", err)
	}
	data, err := userWindowsRead(path, 4096)
	if err != nil || string(data) != `{"mode":"new"}` {
		t.Fatalf("replacement readback = %q %v", data, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("replacement left a temporary sibling: %v", entries)
	}
}

func TestUserWindowsMoveNoReplaceBusyRetriesAndNeverOverwrites(t *testing.T) {
	previous := userWindowsBusySleep
	userWindowsBusySleep = func(time.Duration) {}
	t.Cleanup(func() { userWindowsBusySleep = previous })
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	if err := os.WriteFile(from, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsMoveNoReplaceBusy(root, from, to); err == nil {
		t.Fatal("expected an existing destination to be refused")
	}
	if data, _ := os.ReadFile(to); string(data) != "old" {
		t.Fatalf("destination overwritten: %q", data)
	}
	if err := os.Remove(to); err != nil {
		t.Fatal(err)
	}
	held := userWindowsHoldFile(t, from, 50*time.Millisecond)
	calls := 0
	userWindowsBusySleep = func(time.Duration) {
		calls++
		<-held
	}
	if err := userWindowsMoveNoReplaceBusy(root, from, to); err != nil {
		t.Fatalf("expected the move to succeed once the hold is released: %v", err)
	}
	if calls == 0 {
		t.Fatal("expected at least one busy retry")
	}
	if data, _ := os.ReadFile(to); string(data) != "new" {
		t.Fatalf("unexpected destination content %q", data)
	}
}
