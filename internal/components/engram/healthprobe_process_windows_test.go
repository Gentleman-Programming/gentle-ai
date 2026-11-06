//go:build windows

package engram

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func assertTestProcessExited(t *testing.T, pid int) {
	t.Helper()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return
	}
	if err != nil {
		t.Fatalf("open descendant process %d: %v", pid, err)
	}
	defer windows.CloseHandle(process)
	status, err := windows.WaitForSingleObject(process, uint32(time.Second/time.Millisecond))
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("wait for descendant process %d: status=%d error=%v", pid, status, err)
	}
}
