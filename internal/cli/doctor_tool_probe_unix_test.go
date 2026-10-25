//go:build !windows

package cli

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func assertDoctorProbeProcessExited(t *testing.T, pid int) {
	t.Helper()
	// A killed orphan may remain a zombie until PID 1 reaps it. A zombie is not
	// executing; ps handles that distinction on Linux and macOS.
	output, err := exec.Command("/bin/ps", "-o", "stat=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("cannot inspect descendant %d: %v", pid, err)
		}
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "Z") {
		t.Fatalf("probe descendant %d is still running: %s", pid, output)
	}
}
