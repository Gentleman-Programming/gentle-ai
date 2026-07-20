//go:build !windows

package sysproc

import (
	"os/exec"
	"testing"
)

func TestHideConsole(t *testing.T) {
	cmd := exec.Command("echo", "test")
	HideConsole(cmd)

	if cmd.SysProcAttr != nil {
		t.Fatal("expected SysProcAttr to remain nil on non-Windows platforms")
	}
}
