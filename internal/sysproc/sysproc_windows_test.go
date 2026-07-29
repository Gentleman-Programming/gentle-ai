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

func TestHideBackgroundConsoleAndPreserveConsoleScoping(t *testing.T) {
	t.Run("background subprocess receives CREATE_NO_WINDOW", func(t *testing.T) {
		cmd := exec.Command("cmd.exe", "/c", "echo", "bg")
		HideBackgroundConsole(cmd)
		if cmd.SysProcAttr == nil || (cmd.SysProcAttr.CreationFlags&CREATE_NO_WINDOW) == 0 {
			t.Fatal("expected HideBackgroundConsole to set CREATE_NO_WINDOW flag on Windows")
		}
	})

	t.Run("explicit preserve console clears CREATE_NO_WINDOW flag", func(t *testing.T) {
		cmd := exec.Command("cmd.exe", "/c", "echo", "interactive")
		HideBackgroundConsole(cmd)
		PreserveConsole(cmd)
		if cmd.SysProcAttr != nil && (cmd.SysProcAttr.CreationFlags&CREATE_NO_WINDOW) != 0 {
			t.Fatal("expected PreserveConsole to clear CREATE_NO_WINDOW flag on Windows")
		}
	})
}
