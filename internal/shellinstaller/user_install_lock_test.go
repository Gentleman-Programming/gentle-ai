//go:build linux || darwin || windows

package shellinstaller

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const userLockHelperEnv = "GENTLE_SHELL_LOCK_HELPER_DESTINATION"

func userLockedMessage(destination string) string {
	return "another Gentle Shell operation is in progress for " + destination + "; wait for it to finish and retry"
}

func userLockDestination(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "install")
}

func requireUserLockContention(t *testing.T, err error, destination string) {
	t.Helper()
	if !isOperationInProgress(err) || err.Error() != userLockedMessage(destination) {
		t.Fatalf("expected lock contention for %s, got %v", destination, err)
	}
}

func TestUserOperationLockRejectsASecondHolderUntilReleased(t *testing.T) {
	destination := userLockDestination(t)
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatal(err)
	}
	_, err = userAcquireOperationLock(destination)
	requireUserLockContention(t, err, destination)
	// A different destination in the same parent is independent.
	other, err := userAcquireOperationLock(filepath.Join(filepath.Dir(destination), "other"))
	if err != nil {
		t.Fatalf("an unrelated destination must not contend: %v", err)
	}
	if err := other(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("release must be idempotent: %v", err)
	}
	again, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatalf("lock must be free after release: %v", err)
	}
	defer again()
	info, err := os.Lstat(userOperationLockPath(destination))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("a held lock must be a regular file beside the destination: %v", err)
	}
	if filepath.Base(userOperationLockPath(destination)) != ".install.gentle-shell.lock" {
		t.Fatalf("unexpected lock name %q", userOperationLockPath(destination))
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("locking must never create the destination")
	}
}

func userParentEntries(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestUserOperationLockLeavesNoEntryBesideTheDestination(t *testing.T) {
	destination := userLockDestination(t)
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	before := userParentEntries(t, parent)
	for round := 0; round < 2; round++ {
		release, err := userAcquireOperationLock(destination)
		if err != nil {
			t.Fatal(err)
		}
		if err := release(); err != nil {
			t.Fatal(err)
		}
		if after := userParentEntries(t, parent); strings.Join(after, "\n") != strings.Join(before, "\n") {
			t.Fatalf("round %d: the parent gained entries: before %v after %v", round, before, after)
		}
	}
	// A failed operation releases the same way.
	if _, err := userWithOperationLock(destination, func() (int, error) { return 0, os.ErrInvalid }); err != os.ErrInvalid {
		t.Fatalf("operation error must keep its identity, got %v", err)
	}
	if after := userParentEntries(t, parent); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("a failed operation left entries: before %v after %v", before, after)
	}
}

func TestUserOperationLockRefusesRelativeDestination(t *testing.T) {
	if _, err := userAcquireOperationLock("install"); err == nil || isOperationInProgress(err) {
		t.Fatalf("relative destination must be refused, got %v", err)
	}
}

func TestUserWithOperationLockHoldsTheLockForTheWholeOperation(t *testing.T) {
	destination := userLockDestination(t)
	value, err := userWithOperationLock(destination, func() (string, error) {
		_, inner := userAcquireOperationLock(destination)
		requireUserLockContention(t, inner, destination)
		return "done", nil
	})
	if err != nil || value != "done" {
		t.Fatalf("operation under lock failed: %q %v", value, err)
	}
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatalf("lock must be released after the operation: %v", err)
	}
	_ = release()
}

// TestUserOperationLockHelperProcess is the child half of the cross-process
// test: it holds the lock until its stdin closes or it is killed.
func TestUserOperationLockHelperProcess(t *testing.T) {
	destination := os.Getenv(userLockHelperEnv)
	if destination == "" {
		t.Skip("helper process only")
	}
	if _, err := userAcquireOperationLock(destination); err != nil {
		os.Stdout.WriteString("error " + err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(0)
}

func TestUserOperationLockContendsAcrossProcessesAndReleasesOnExit(t *testing.T) {
	destination := userLockDestination(t)
	child := exec.Command(os.Args[0], "-test.run=^TestUserOperationLockHelperProcess$", "-test.count=1")
	child.Env = append(os.Environ(), userLockHelperEnv+"="+destination)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = child.Process.Kill(); _ = child.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("helper did not take the lock: %q %v", line, err)
	}
	_, err = userAcquireOperationLock(destination)
	requireUserLockContention(t, err, destination)
	// The owner exits without releasing: the operating system drops the lock.
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		release, err := userAcquireOperationLock(destination)
		if err == nil {
			_ = release()
			break
		}
		if !isOperationInProgress(err) || time.Now().After(deadline) {
			t.Fatalf("lock must be released when its owner exits: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A lock file a killed owner left behind is removed by the next release.
	if _, err := os.Lstat(userOperationLockPath(destination)); !os.IsNotExist(err) {
		t.Fatalf("the next release must remove the lock file: %v", err)
	}
}

func TestUninstallUserShellInstallFailsFastWhileAnotherOperationHoldsTheLock(t *testing.T) {
	destination := userLockDestination(t)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "marker")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: destination, Mode: "separate"})
	requireUserLockContention(t, err, destination)
	if backup != "" {
		t.Fatalf("contended uninstall must not report a backup, got %q", backup)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "x" {
		t.Fatalf("contended uninstall must leave the target untouched: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".removed-") {
			t.Fatalf("contended uninstall created a backup %q", entry.Name())
		}
	}
}

// TestUserOperationLockInstallHelperProcess is the child half of the install
// contention test: the direct installer sets no-new-privileges on its own
// process, so it runs isolated from the parent test binary.
func TestUserOperationLockInstallHelperProcess(t *testing.T) {
	destination := os.Getenv(userLockInstallHelperEnv)
	if destination == "" {
		t.Skip("helper process only")
	}
	req := UserInstallRequest{Destination: destination, Mode: "separate"}
	confirmation, err := InspectUserInstall(req)
	if err != nil {
		os.Stdout.WriteString("inspect " + err.Error() + "\n")
		os.Exit(2)
	}
	// The bound keeps a missing lock from turning into a real installation.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = RunUserEntry(ctx, os.Args[0], []string{"internal-install-direct", destination, "separate", "", "", confirmation}, nil, io.Discard, io.Discard)
	if isOperationInProgress(err) {
		os.Stdout.WriteString("contended " + err.Error() + "\n")
		os.Exit(0)
	}
	os.Stdout.WriteString(fmt.Sprintf("unexpected %v\n", err))
	os.Exit(3)
}

const userLockInstallHelperEnv = "GENTLE_SHELL_LOCK_INSTALL_HELPER_DESTINATION"

func TestUserInstallFailsFastWhileAnotherOperationHoldsTheLock(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" || os.Getuid() == 0 {
		// Windows installs only inside the supervisor's Job Object worker and
		// darwin has no runner here; both share the same acquire call site.
		t.Skip("the direct installer entry is Linux amd64 non-root only")
	}
	destination := userLockDestination(t)
	if err := os.Chmod(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	child := exec.Command(os.Args[0], "-test.run=^TestUserOperationLockInstallHelperProcess$", "-test.count=1")
	child.Env = append(os.Environ(), userLockInstallHelperEnv+"="+destination)
	output, err := child.Output()
	want := "contended " + userLockedMessage(destination) + "\n"
	if err != nil || !strings.HasPrefix(string(output), want) {
		t.Fatalf("second install must fail fast with the typed lock error: %v %q", err, output)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".install.gentle-shell.lock" {
		t.Fatalf("contended install must leave the parent untouched, found %v", entries)
	}
}

func TestApplyUserExperiencePreferencesFailsFastWhileAnotherOperationHoldsTheLock(t *testing.T) {
	destination := userLockDestination(t)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	err = ApplyUserExperiencePreferences(UserShellInstall{Target: destination, Mode: "separate"}, DefaultUserExperience())
	requireUserLockContention(t, err, destination)
	if _, err := os.Lstat(filepath.Join(destination, "config")); !os.IsNotExist(err) {
		t.Fatal("contended preference apply must not write the target")
	}
}

// isOperationInProgress reports whether err is a lock contention.
func isOperationInProgress(err error) bool {
	var locked *UserShellOperationInProgressError
	return errors.As(err, &locked)
}
