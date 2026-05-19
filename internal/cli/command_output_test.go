package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/internal/components/uninstall"
)

func TestExecuteCommandQuietModeIncludesCapturedOutputOnFailure(t *testing.T) {
	restore := SetCommandOutputStreaming(false)
	defer restore()
	t.Setenv("GO_WANT_EXECUTE_COMMAND_HELPER_PROCESS", "1")

	err := executeCommand(os.Args[0], "-test.run=TestExecuteCommandHelperProcess", "--")
	if err == nil {
		t.Fatal("executeCommand() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("executeCommand() error = %q, want captured output", err.Error())
	}
}

func TestExecuteCommandHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EXECUTE_COMMAND_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "boom")
	os.Exit(1)
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
