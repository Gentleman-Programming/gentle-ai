//go:build windows

package sysproc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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

func TestHideConsoleFromNoConsoleParentPreservesOutputAndExitStatus(t *testing.T) {
	const roleEnv = "GENTLE_AI_NO_CONSOLE_TEST_ROLE"
	getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	consoleWindow, _, _ := getConsoleWindow.Call()

	switch os.Getenv(roleEnv) {
	case "child":
		if consoleWindow != 0 {
			t.Fatalf("child acquired console window %#x", consoleWindow)
		}
		fmt.Print("hidden-child-output")
		os.Exit(7)
	case "parent":
		if consoleWindow != 0 {
			t.Fatalf("parent acquired console window %#x", consoleWindow)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestHideConsoleFromNoConsoleParentPreservesOutputAndExitStatus$")
		cmd.Env = append(os.Environ(), roleEnv+"=child")
		HideConsole(cmd)
		output, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		if !strings.Contains(string(output), "hidden-child-output") || !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("hidden child output = %q, error = %v", output, err)
		}
		fmt.Print("no-console-parent-ok")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestHideConsoleFromNoConsoleParentPreservesOutputAndExitStatus$")
	cmd.Env = append(os.Environ(), roleEnv+"=parent")
	HideConsole(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "no-console-parent-ok") {
		t.Fatalf("no-console parent output = %q, error = %v", output, err)
	}
}
