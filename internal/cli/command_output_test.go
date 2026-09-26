package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/v3/internal/components/uninstall"
)

func TestExecuteCommandQuietModeIncludesCapturedOutputOnFailure(t *testing.T) {
	restore := SetCommandOutputStreaming(false)
	defer restore()

	shell := "bash"
	args := []string{"-c", "echo boom && exit 1"}
	if runtime.GOOS == "windows" {
		shell = "cmd"
		args = []string{"/c", "echo boom && exit 1"}
	}

	err := executeCommand(shell, args...)
	if err == nil {
		t.Fatal("executeCommand() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("executeCommand() error = %q, want captured output", err.Error())
	}
}

// TestExecuteCommandSetsHomebrewNoAutoUpdateEnvForBrewCommands proves the
// HOMEBREW_NO_AUTO_UPDATE/HOMEBREW_NO_INSTALL_CLEANUP variables commandEnv
// computes actually reach a brew child process spawned through
// executeCommand, using a fake "brew" script on the same pattern as
// TestExecuteCommandInheritsPiCodingAgentDirForChildProcesses.
func TestExecuteCommandSetsHomebrewNoAutoUpdateEnvForBrewCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake brew script below is a POSIX shell script")
	}
	restoreStreaming := SetCommandOutputStreaming(false)
	t.Cleanup(restoreStreaming)

	fakeBrew := filepath.Join(t.TempDir(), "brew")
	script := "#!/bin/sh\nprintf 'HOMEBREW_NO_AUTO_UPDATE=%s\\nHOMEBREW_NO_INSTALL_CLEANUP=%s\\n' \"$HOMEBREW_NO_AUTO_UPDATE\" \"$HOMEBREW_NO_INSTALL_CLEANUP\" > \"$3\"\n"
	if err := os.WriteFile(fakeBrew, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(t.TempDir(), "observed-env")
	if err := executeCommand(fakeBrew, "install", "engram", outFile); err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile(observed env) error = %v", err)
	}
	want := "HOMEBREW_NO_AUTO_UPDATE=1\nHOMEBREW_NO_INSTALL_CLEANUP=1\n"
	if string(got) != want {
		t.Fatalf("observed brew env = %q, want %q", got, want)
	}
}

func TestSetCommandOutputStreamingRestore(t *testing.T) {
	streamCommandOutput = true
	restore := SetCommandOutputStreaming(false)

	if streamCommandOutput {
		t.Fatal("streamCommandOutput should be false after SetCommandOutputStreaming(false)")
	}

	restore()
	if !streamCommandOutput {
		t.Fatal("restore should reset streamCommandOutput to previous value")
	}
}

func TestRenderUninstallReportIncludesManualCleanup(t *testing.T) {
	report := RenderUninstallReport(componentuninstall.Result{
		RemovedDirectories: []string{"/tmp/agent-skills"},
		ManualActions: []string{
			"Remove manually if no longer needed: /tmp/skills (directory still contains non-managed files)",
		},
	})

	if !strings.Contains(report, "Manual cleanup required") {
		t.Fatalf("RenderUninstallReport() should include manual cleanup heading; got:\n%s", report)
	}
	if !strings.Contains(report, "/tmp/skills") {
		t.Fatalf("RenderUninstallReport() should include manual cleanup item; got:\n%s", report)
	}
}
