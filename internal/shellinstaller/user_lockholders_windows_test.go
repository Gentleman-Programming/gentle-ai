//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// userWindowsFakeLockHolders replaces the holder enumerator for one test and
// counts how often the busy retry asks for it.
func userWindowsFakeLockHolders(t *testing.T, holders []userWindowsLockHolder, err error) *int {
	t.Helper()
	previousHolders, previousSleep := userWindowsLockHolders, userWindowsBusySleep
	calls := 0
	userWindowsLockHolders = func(string) ([]userWindowsLockHolder, error) {
		calls++
		return holders, err
	}
	userWindowsBusySleep = func(time.Duration) {}
	t.Cleanup(func() { userWindowsLockHolders, userWindowsBusySleep = previousHolders, previousSleep })
	return &calls
}

func userWindowsPersistentlyDenied() error {
	return &os.LinkError{Op: "rename", Old: "install", New: "install.replaced", Err: windows.ERROR_ACCESS_DENIED}
}

const userWindowsBusyMessage = "is still in use by another program (for example an antivirus scan or an open editor or terminal); close it and retry"

func TestUserWindowsRetryBusyNamesLockHolders(t *testing.T) {
	root := t.TempDir()
	calls := userWindowsFakeLockHolders(t, []userWindowsLockHolder{{PID: 7437, Name: "engram.exe"}, {PID: 912, Name: "node.exe"}}, nil)

	err := userWindowsRetryBusy(root, userWindowsPersistentlyDenied)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("busy error lost its Windows cause: %v", err)
	}
	message := err.Error()
	for _, want := range []string{
		fmt.Sprintf("a file under %s %s", root, userWindowsBusyMessage),
		"engram.exe (PID 7437)",
		"node.exe (PID 912)",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("busy error %q does not contain %q", message, want)
		}
	}
	if strings.Contains(message, "working directory") {
		t.Fatalf("busy error with named holders must not fall back to the working-directory hint: %q", message)
	}
	if *calls != 1 {
		t.Fatalf("holder enumeration ran %d times, want exactly once on the final failure", *calls)
	}
}

func TestUserWindowsRetryBusyHintsWorkingDirectoryWhenNoHolderIsFound(t *testing.T) {
	root := t.TempDir()
	userWindowsFakeLockHolders(t, nil, nil)

	err := userWindowsRetryBusy(root, userWindowsPersistentlyDenied)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("busy error lost its Windows cause: %v", err)
	}
	message := err.Error()
	if !strings.Contains(message, fmt.Sprintf("a file under %s %s", root, userWindowsBusyMessage)) {
		t.Fatalf("busy error lost the existing message prefix: %q", message)
	}
	want := fmt.Sprintf("a program may also be running with its working directory inside %s (for example a terminal or a WSL session)", root)
	if !strings.Contains(message, want) {
		t.Fatalf("busy error %q does not contain the working-directory hint %q", message, want)
	}
}

func TestUserWindowsRetryBusyKeepsMessageWhenEnumerationFails(t *testing.T) {
	root := t.TempDir()
	userWindowsFakeLockHolders(t, []userWindowsLockHolder{{PID: 1, Name: "ignored.exe"}}, errors.New("restart manager unavailable"))

	cause := userWindowsPersistentlyDenied()
	err := userWindowsRetryBusy(root, func() error { return cause })
	want := fmt.Sprintf("%v; a file under %s %s", cause, root, userWindowsBusyMessage)
	if err == nil || err.Error() != want {
		t.Fatalf("enumeration failure changed the busy error:\n got %v\nwant %s", err, want)
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("busy error lost its Windows cause: %v", err)
	}
}

func TestUserWindowsRetryBusySkipsEnumerationUnlessFinalFailure(t *testing.T) {
	root := t.TempDir()
	calls := userWindowsFakeLockHolders(t, []userWindowsLockHolder{{PID: 7437, Name: "engram.exe"}}, nil)

	attempts := 0
	err := userWindowsRetryBusy(root, func() error {
		attempts++
		if attempts < len(userWindowsBusyDelays) {
			return userWindowsPersistentlyDenied()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transiently busy operation failed: %v", err)
	}
	permanent := errors.New("not a lock")
	if err := userWindowsRetryBusy(root, func() error { return permanent }); err != permanent {
		t.Fatalf("non-busy error changed: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("holder enumeration ran %d times without a final busy failure", *calls)
	}
}

func TestUserWindowsLockHolderCandidatesAreCapped(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 12; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%02d", i))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files := userWindowsLockHolderCandidates(root, 5, 1000)
	if len(files) != 5 {
		t.Fatalf("candidate files = %d, want the file cap 5", len(files))
	}
	for _, file := range files {
		if info, err := os.Stat(file); err != nil || info.IsDir() {
			t.Fatalf("candidate %q is not a regular file: %v", file, err)
		}
	}
	if files := userWindowsLockHolderCandidates(root, 1000, 6); len(files) > 6 {
		t.Fatalf("candidate walk visited past its entry cap: %d files", len(files))
	}
	if files := userWindowsLockHolderCandidates(filepath.Join(root, "absent"), 5, 1000); len(files) != 0 {
		t.Fatalf("absent root produced candidates: %v", files)
	}
}

func TestUserWindowsRestartManagerReportsOwnProcess(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "home", ".engram", "engram.db")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	holders, err := userWindowsRestartManagerHolders(root)
	if err != nil {
		t.Fatalf("restart manager enumeration failed: %v", err)
	}
	pid := uint32(os.Getpid())
	for _, holder := range holders {
		if holder.PID == pid {
			if holder.Name == "" {
				t.Fatalf("own process reported without a name: %+v", holder)
			}
			return
		}
	}
	t.Fatalf("restart manager did not report the own process %d holding %s: %+v", pid, path, holders)
}
