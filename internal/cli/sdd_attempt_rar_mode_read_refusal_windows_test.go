//go:build windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReviewModeUnsafeFileRepairCommandRunsOnWindowsPowerShell(t *testing.T) {
	target := filepath.Join(t.TempDir(), "unsafe file")
	if err := os.WriteFile(target, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	windowsRunPowerShell(t, (&reviewModeUnsafePathError{Path: target}).repairCommand())
}

func windowsRunPowerShell(t *testing.T, script string) {
	t.Helper()
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell script failed: %v\n%s", err, output)
	}
}
