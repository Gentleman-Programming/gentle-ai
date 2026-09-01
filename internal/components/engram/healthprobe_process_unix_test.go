//go:build !windows

package engram

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func assertTestProcessExited(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("check descendant process %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant process %d still exists", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
