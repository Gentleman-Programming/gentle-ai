//go:build windows

package sysproc

import (
	"os/exec"
	"testing"
)

func TestHideConsole(t *testing.T) {
	cmd := exec.Command("echo", "test")
	HideConsole(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be allocated on Windows")
	}
	if (cmd.SysProcAttr.CreationFlags & CREATE_NO_WINDOW) == 0 {
		t.Fatal("expected CREATE_NO_WINDOW flag to be set in CreationFlags on Windows")
	}
}
