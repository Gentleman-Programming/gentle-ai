//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// Restart Manager cannot see working-directory handles, so a named holder
	// does not rule out a second, invisible one: the hint is kept.
	if !strings.Contains(message, fmt.Sprintf("a program may also be running with its working directory inside %s", root)) {
		t.Fatalf("busy error with named holders must keep the working-directory hint: %q", message)
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

// A real second process holding a file under the root is named in the error of
// a real rename, and the diagnostics never stop it.
func TestUserWindowsRenameBusyNamesRealHolderProcess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install")
	held := filepath.Join(root, "home", ".engram", "held.log")
	if err := os.MkdirAll(filepath.Dir(held), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(held, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ready := held + ".ready"
	child := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"$f=[IO.File]::Open('"+held+"','Open','ReadWrite','None'); New-Item -ItemType File '"+ready+"' | Out-Null; Start-Sleep 60")
	if err := child.Start(); err != nil {
		t.Skipf("powershell.exe unavailable: %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("holder process never opened the file")
		}
		time.Sleep(50 * time.Millisecond)
	}
	previousSleep := userWindowsBusySleep
	userWindowsBusySleep = func(time.Duration) {}
	t.Cleanup(func() { userWindowsBusySleep = previousSleep })

	err := userWindowsRenameBusy(root, root, root+".replaced")
	if err == nil {
		t.Fatal("rename of a root with a held file unexpectedly succeeded")
	}
	if !userWindowsBusyCause(err) {
		t.Fatalf("busy error lost its Windows cause: %v", err)
	}
	message := err.Error()
	if want := fmt.Sprintf("(PID %d)", child.Process.Pid); !strings.Contains(message, want) || !strings.Contains(strings.ToLower(message), "powershell.exe") {
		t.Fatalf("busy error %q does not name powershell.exe %s", message, want)
	}
	process, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(child.Process.Pid))
	if openErr != nil {
		t.Fatalf("holder process is gone after diagnostics: %v", openErr)
	}
	defer windows.CloseHandle(process)
	if event, _ := windows.WaitForSingleObject(process, 0); event != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("diagnostics must never stop the holder process (wait result %d)", event)
	}
}

// Junctions inside the root are not followed when listing candidate files.
func TestUserWindowsLockHolderCandidatesDoNotFollowJunctions(t *testing.T) {
	base := t.TempDir()
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(root, "plain"), filepath.Join(outside, "x")} {
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	junction := filepath.Join(root, "j")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %v %s", err, out)
	}
	files := userWindowsLockHolderCandidates(root, 100, 100)
	if len(files) != 1 || files[0] != filepath.Join(root, "plain") {
		t.Fatalf("candidates = %v, want only %s (junction %s must not be followed)", files, filepath.Join(root, "plain"), junction)
	}
}
