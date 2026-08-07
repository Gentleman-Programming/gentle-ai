//go:build windows

package filemerge

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestMoveFileReplaceReplacesARunningExecutable is the real #2319 case: the
// installed binary is the image the loader has mapped into this process. The
// loader holds it with FILE_SHARE_READ|FILE_SHARE_DELETE, which lets rename
// and replace race — replace is denied (the image is mapped), rename is
// allowed. MoveFileReplace has to take the displacement rung.
//
// The "held" image is a copy of the test binary that runs as a child and blocks
// on stdin. The parent performs the swap while the child is still executing,
// then closes the child's stdin to let it exit; killing it via t.Cleanup is the
// safety net so the swap cannot have relied on the file being briefly released.
func TestMoveFileReplaceReplacesARunningExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}
	dir := t.TempDir()
	held := filepath.Join(dir, "gentle-ai.exe")
	if err := copyFile(exe, held); err != nil {
		t.Fatalf("seed held binary: %v", err)
	}
	staged := filepath.Join(dir, "staged")
	replacement := []byte("the upgraded binary\n")
	if err := os.WriteFile(staged, replacement, 0o644); err != nil {
		t.Fatalf("write staged: %v", err)
	}

	cmd := exec.Command(held, "-test.run=^TestHelperProcessMoveBlocked$")
	cmd.Env = append(os.Environ(), "GENTLE_AI_WANT_MOVE_BLOCKED=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start held child: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	// The loader finalises the image after exec.Command.Start returns, so a
	// short sleep is the deterministic wait. A handle-count probe is not a
	// substitute: a probe open with FILE_SHARE_READ returns SHARING_VIOLATION
	// even against a file held with FILE_SHARE_READ|FILE_SHARE_DELETE, which
	// is exactly the hold a running image uses, so the probe cannot tell
	// "still loading" apart from "fully running".
	time.Sleep(300 * time.Millisecond)

	if err := os.Rename(staged, held); err == nil {
		t.Fatal("os.Rename replaced a running image; the held scenario is not what #2319 describes")
	}

	if err := MoveFileReplace(staged, held); err != nil {
		t.Fatalf("MoveFileReplace over a running image: %v", err)
	}

	onDisk, err := os.ReadFile(held)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(onDisk, replacement) {
		t.Errorf("destination = %q, want %q", onDisk, replacement)
	}
}

// TestMoveFileReplaceRefusesToLoseAnUndisplaceableDestination covers the
// failure honest: when the hold is so restrictive that even the displacement
// rename is refused, MoveFileReplace must surface that and leave the
// installation intact. The displacement rung only fires when rename is legal;
// otherwise a downgrade to "the platform would not let us move this" is the
// truth and a delay-until-reboot would be a lie.
func TestMoveFileReplaceRefusesToLoseAnUndisplaceableDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "staged")
	dst := filepath.Join(dir, "published")
	installed := []byte("the installed binary")
	staged := []byte("the staged binary")

	if err := os.WriteFile(src, staged, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, installed, 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	hold := openWithoutShareDelete(t, dst)
	t.Cleanup(func() { _ = windows.CloseHandle(hold) })

	err := MoveFileReplace(src, dst)
	if err == nil {
		t.Fatal("MoveFileReplace reported success on a destination that refused even the displacement rename")
	}
	// Windows can answer the displacement call with any of the three "file in
	// use" codes depending on what held the file; the contract is the
	// classification, not a specific errno.
	isHold := errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
	if !isHold {
		t.Errorf("error = %v, want it to wrap one of ERROR_ACCESS_DENIED/ERROR_SHARING_VIOLATION/ERROR_LOCK_VIOLATION", err)
	}

	onDisk, readErr := os.ReadFile(dst)
	if readErr != nil {
		t.Fatalf("read back dst: %v", readErr)
	}
	if !bytes.Equal(onDisk, installed) {
		t.Errorf("installed binary = %q, want the untouched %q", onDisk, installed)
	}
}

// TestDestinationIsHeldOnlyMatchesHoldCodes keeps the fallback from swallowing
// a refusal it cannot fix. Only "the file is in use" justifies displacing the
// destination; anything else has to surface unchanged.
func TestDestinationIsHeldOnlyMatchesHoldCodes(t *testing.T) {
	for _, held := range []error{
		windows.ERROR_ACCESS_DENIED,
		windows.ERROR_SHARING_VIOLATION,
		windows.ERROR_LOCK_VIOLATION,
	} {
		if !destinationIsHeld(&os.LinkError{Op: "rename", Err: held}) {
			t.Errorf("destinationIsHeld(%v) = false, want true", held)
		}
	}
	for _, other := range []error{
		windows.ERROR_FILE_NOT_FOUND,
		windows.ERROR_PATH_NOT_FOUND,
		windows.ERROR_DISK_FULL,
		os.ErrNotExist,
	} {
		if destinationIsHeld(&os.LinkError{Op: "rename", Err: other}) {
			t.Errorf("destinationIsHeld(%v) = true, want false", other)
		}
	}
}

// TestHelperProcessMoveBlocked is not a real test. The running-executable test
// re-runs the test binary as a child that holds its own image open, so this
// function exists only to satisfy -test.run. The child reads from stdin and
// exits when the parent closes the pipe.
func TestHelperProcessMoveBlocked(t *testing.T) {
	if os.Getenv("GENTLE_AI_WANT_MOVE_BLOCKED") != "1" {
		return
	}
	_, _ = os.Stdin.Read(make([]byte, 1))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// openWithoutShareDelete opens path with FILE_SHARE_READ only, which is what
// makes Windows refuse a rename over it. os.OpenFile cannot be used: Go opens
// with FILE_SHARE_DELETE, and a destination shared for delete can already be
// replaced by a plain rename, so the fallback under test would never run.
func openWithoutShareDelete(t *testing.T, path string) windows.Handle {
	t.Helper()
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("convert %q: %v", path, err)
	}
	handle, err := windows.CreateFile(
		wide,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("hold %q: %v", path, err)
	}
	return handle
}
