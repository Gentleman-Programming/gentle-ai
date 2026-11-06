//go:build windows

package opencode

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// catalogDescendantPIDEnv names the environment variable the generated helper
// reads to record its grandchild's PID. Reaping that grandchild before
// t.TempDir removes the helper image is what keeps the Windows cleanup from
// failing with "Access is denied" on an executable that is still running
// (#4843).
const catalogDescendantPIDEnv = "GENTLE_AI_CATALOG_DESCENDANT_PID_FILE"

// reapDescendantProcess registers a cleanup that terminates and reaps the
// grandchild whose PID the helper records in pidPath. On Windows an escaping
// descendant keeps the test-built helper .exe mapped, so t.TempDir's RemoveAll
// fails until the process is gone. Cleanups run LIFO and t.TempDir registers
// its removal first, so a cleanup registered after it runs before the removal.
// The pattern mirrors the descendant-pid reaping in
// internal/components/engram/healthprobe_test.go.
func reapDescendantProcess(t *testing.T, pidPath string) {
	t.Helper()
	t.Cleanup(func() {
		pid, err := waitForDescendantPID(pidPath, time.Second)
		if err != nil {
			return
		}
		terminateAndReapProcess(t, pid)
	})
}

// waitForDescendantPID polls pidPath until the helper has written a positive
// PID. The helper writes it before printing, but a bounded wait keeps the
// cleanup robust without racing the spawn.
func waitForDescendantPID(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			switch {
			case parseErr == nil && pid > 0:
				return pid, nil
			case parseErr != nil:
				lastErr = parseErr
			default:
				lastErr = errors.New("descendant pid must be positive")
			}
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			return 0, lastErr
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// terminateAndReapProcess kills pid and waits for the process object to signal
// so the executable image stops being mapped before t.TempDir removes it.
func terminateAndReapProcess(t *testing.T, pid int) {
	t.Helper()
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
		_ = process.Release()
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return
	}
	if err != nil {
		t.Errorf("open descendant process %d: %v", pid, err)
		return
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, uint32(2*time.Second/time.Millisecond))
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Errorf("wait for descendant process %d: status=%d error=%v", pid, status, err)
	}
}

// TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindows is the
// Windows counterpart of TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendant:
// a grandchild that inherits stdout and stderr must not extend discovery past
// the context deadline. Cancellation terminates the whole Job Object tree, so
// the pipes close as soon as the deadline fires instead of staying open for
// the grandchild's 30s sleep.
func TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindows(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "descendant-helper.exe")
	source := filepath.Join(dir, "main.go")
	pidPath := filepath.Join(dir, "descendant.pid")
	t.Setenv(catalogDescendantPIDEnv, pidPath)
	reapDescendantProcess(t, pidPath)
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "grandchild" {
		time.Sleep(30 * time.Second)
		return
	}
	grandchild := exec.Command(os.Args[0], "grandchild")
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	_ = grandchild.Start()
	if path := os.Getenv("GENTLE_AI_CATALOG_DESCENDANT_PID_FILE"); path != "" && grandchild.Process != nil {
		_ = os.WriteFile(path, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
	}
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
	time.Sleep(25 * time.Millisecond)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	started := time.Now()
	_, _ = io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("discovery stayed blocked for %v; want bounded by the 300ms deadline", elapsed)
	}
}

// TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindowsFallback verifies
// that when Job Object creation fails, cmd.WaitDelay is still configured and bounds
// discovery shutdown even though the grandchild cannot be assigned to a Job Object.
func TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindowsFallback(t *testing.T) {
	oldHook := testHookCreateJobObject
	t.Cleanup(func() { testHookCreateJobObject = oldHook })
	testHookCreateJobObject = func(*windows.SecurityAttributes, *uint16) (windows.Handle, error) {
		return 0, errors.New("simulated job object creation failure")
	}

	dir := t.TempDir()
	helper := filepath.Join(dir, "descendant-fallback-helper.exe")
	source := filepath.Join(dir, "main.go")
	pidPath := filepath.Join(dir, "descendant.pid")
	t.Setenv(catalogDescendantPIDEnv, pidPath)
	reapDescendantProcess(t, pidPath)
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "grandchild" {
		time.Sleep(30 * time.Second)
		return
	}
	grandchild := exec.Command(os.Args[0], "grandchild")
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	_ = grandchild.Start()
	if path := os.Getenv("GENTLE_AI_CATALOG_DESCENDANT_PID_FILE"); path != "" && grandchild.Process != nil {
		_ = os.WriteFile(path, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
	}
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
	time.Sleep(25 * time.Millisecond)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	started := time.Now()
	_, _ = io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("discovery stayed blocked for %v; want bounded by the 300ms deadline + WaitDelay fallback", elapsed)
	}
}

// TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindowsAssignmentFailureFallback verifies
// that when AssignProcessToJobObject fails (e.g. nested job object restriction), cmd.WaitDelay bounds
// discovery shutdown even though the grandchild was not added to the job.
func TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendantWindowsAssignmentFailureFallback(t *testing.T) {
	oldHook := testHookAssignProcessToJobObject
	t.Cleanup(func() { testHookAssignProcessToJobObject = oldHook })
	testHookAssignProcessToJobObject = func(windows.Handle, windows.Handle) error {
		return errors.New("simulated job object assignment failure")
	}

	dir := t.TempDir()
	helper := filepath.Join(dir, "descendant-assign-fallback-helper.exe")
	source := filepath.Join(dir, "main.go")
	pidPath := filepath.Join(dir, "descendant.pid")
	t.Setenv(catalogDescendantPIDEnv, pidPath)
	reapDescendantProcess(t, pidPath)
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "grandchild" {
		time.Sleep(30 * time.Second)
		return
	}
	grandchild := exec.Command(os.Args[0], "grandchild")
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	_ = grandchild.Start()
	if path := os.Getenv("GENTLE_AI_CATALOG_DESCENDANT_PID_FILE"); path != "" && grandchild.Process != nil {
		_ = os.WriteFile(path, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
	}
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
	time.Sleep(25 * time.Millisecond)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	started := time.Now()
	_, _ = io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("discovery stayed blocked for %v; want bounded by the 300ms deadline + WaitDelay fallback", elapsed)
	}
}
