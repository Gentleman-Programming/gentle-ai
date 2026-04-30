package cli

import (
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/internal/components/uninstall"
)

func TestExecuteCommandQuietModeIncludesCapturedOutputOnFailure(t *testing.T) {
	restore := SetCommandOutputStreaming(false)
	defer restore()

	err := executeCommand("bash", "-c", "echo boom && exit 1")
	if err == nil {
		t.Fatal("executeCommand() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("executeCommand() error = %q, want captured output", err.Error())
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
