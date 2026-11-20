//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// userWindowsRunFromInstallation starts a long-running copy of ping.exe whose
// image lives in dir, like a Gentle Shell process started from the
// installation, and stops it when the test ends.
func userWindowsRunFromInstallation(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(filepath.Join(system, "ping.exe"))
	if err != nil {
		t.Skipf("no ping.exe to run from the installation: %v", err)
	}
	image := filepath.Join(dir, "supervisor-test.exe")
	destination, err := os.OpenFile(image, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, source)
	source.Close()
	if err := errors.Join(copyErr, destination.Close()); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(image, "-n", "60", "127.0.0.1")
	if err := command.Start(); err != nil {
		t.Skipf("cannot start the copied process: %v", err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })

	return command
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
	if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || !strings.Contains(err.Error(), "still in use") || !strings.Contains(err.Error(), "retry") {
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

func TestUninstallWindowsNamesRunningInstalledProcess(t *testing.T) {
	previous := userWindowsBusySleep
	userWindowsBusySleep = func(time.Duration) {}
	t.Cleanup(func() { userWindowsBusySleep = previous })
	target, _ := userWindowsUninstallFixture(t)
	command := userWindowsRunFromInstallation(t, target)

	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	if err == nil {
		t.Fatalf("uninstall moved an installation with a running process: %q", backup)
	}
	want := fmt.Sprintf("supervisor-test.exe (PID %d)", command.Process.Pid)
	if backup != "" || !strings.Contains(err.Error(), "close these Gentle Shell processes and retry") || !strings.Contains(err.Error(), want) {
		t.Fatalf("busy uninstall error does not name the running process %q: %v (backup %q)", want, err, backup)
	}
	if _, err := os.Lstat(filepath.Join(target, "manifest.json")); err != nil {
		t.Fatalf("refused uninstall touched the installation: %v", err)
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
