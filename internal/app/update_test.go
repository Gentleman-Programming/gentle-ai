package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

func TestRunUpdate_ReturnsErrorWhenChecksFail(t *testing.T) {
	origCheckAll := updateCheckAll
	t.Cleanup(func() {
		updateCheckAll = origCheckAll
	})

	updateCheckAll = func(context.Context, string, system.PlatformProfile) []update.UpdateResult {
		return []update.UpdateResult{{
			Tool:   update.ToolInfo{Name: "engram"},
			Status: update.CheckFailed,
		}}
	}

	var buf bytes.Buffer
	err := runUpdate(context.Background(), "1.0.0", system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, &buf)
	if err == nil {
		t.Fatal("runUpdate() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "update check failed for: engram") {
		t.Fatalf("runUpdate() error = %v, want update check failure", err)
	}

	out := buf.String()
	if strings.Contains(out, "All tools are up to date!") {
		t.Fatalf("runUpdate() output incorrectly claimed tools are up to date:\n%s", out)
	}
	if !strings.Contains(out, "Update check incomplete") {
		t.Fatalf("runUpdate() output missing incomplete check warning:\n%s", out)
	}
}

func TestRunUpgrade_ReturnsErrorBeforeExecutingWhenChecksFail(t *testing.T) {
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecute := upgradeExecute
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecute = origUpgradeExecute
	})

	called := false
	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{
			{
				Tool:   update.ToolInfo{Name: "engram"},
				Status: update.CheckFailed,
			},
			{
				Tool:             update.ToolInfo{Name: "gga"},
				InstalledVersion: "1.0.0",
				LatestVersion:    "2.0.0",
				Status:           update.UpdateAvailable,
			},
		}
	}
	upgradeExecute = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, ...io.Writer) upgrade.UpgradeReport {
		called = true
		return upgrade.UpgradeReport{}
	}

	var buf bytes.Buffer
	err := runUpgrade(context.Background(), []string{"--no-backup"}, system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, &buf)
	if err == nil {
		t.Fatal("runUpgrade() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "update check failed for: engram") {
		t.Fatalf("runUpgrade() error = %v, want update check failure", err)
	}
	if called {
		t.Fatal("runUpgrade() executed upgrades despite failed checks")
	}

	out := buf.String()
	if !strings.Contains(out, "Update Check") {
		t.Fatalf("runUpgrade() output missing check report:\n%s", out)
	}
	if strings.Contains(out, "All tools are up to date!") {
		t.Fatalf("runUpgrade() output incorrectly claimed tools are up to date:\n%s", out)
	}
	if strings.Contains(out, "Upgrade\n") {
		t.Fatalf("runUpgrade() should stop before rendering upgrade report:\n%s", out)
	}
}

// TestRunUpgrade_RestartsAfterGentleAIUpgrade verifies that `gentle-ai upgrade`
// prints the restart guidance message after a successful gentle-ai upgrade.
// After task 4.6, no re-exec occurs on any OS — the message is always printed.
func TestRunUpgrade_RestartsAfterGentleAIUpgrade(t *testing.T) {
	unsetEnv(t, envSelfUpdateDone)

	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
	})

	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{{
			Tool:             update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
			InstalledVersion: "1.36.1",
			LatestVersion:    "1.36.2",
			Status:           update.UpdateAvailable,
		}}
	}
	upgradeExecuteWithOptions = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, upgrade.ExecuteOptions) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{{
			ToolName:   "gentle-ai",
			OldVersion: "1.36.1",
			NewVersion: "1.36.2",
			Status:     upgrade.UpgradeSucceeded,
		}}}
	}

	var buf bytes.Buffer
	err := runUpgrade(context.Background(), []string{"--no-backup"}, system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, &buf)
	if err != nil {
		t.Fatalf("runUpgrade() error = %v", err)
	}
	// After task 4.6: restart message printed, no re-exec.
	if !strings.Contains(buf.String(), "restart gentle-ai") {
		t.Fatalf("runUpgrade() output missing restart notice:\n%s", buf.String())
	}
}

// TestRestartAfterGentleAIUpgrade_PrintsRestartGuidance verifies that
// restartAfterGentleAIUpgrade (converged in task 4.6) always prints
// the restart guidance message and never re-execs, on any OS.
func TestRestartAfterGentleAIUpgrade_PrintsRestartGuidance(t *testing.T) {
	unsetEnv(t, envSelfUpdateDone)

	var buf bytes.Buffer
	err := restartAfterGentleAIUpgrade("1.36.2", &buf)
	if err != nil {
		t.Fatalf("restartAfterGentleAIUpgrade() error = %v", err)
	}
	out := buf.String()
	// Must contain version and "restart" guidance.
	if !strings.Contains(out, "1.36.2") {
		t.Errorf("output = %q, want version 1.36.2 mentioned", out)
	}
	if !strings.Contains(strings.ToLower(out), "restart") {
		t.Errorf("output = %q, want restart guidance", out)
	}
}

func envContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

// TestRunUpgrade_DryRunDoesNotRestartAfterGentleAIUpgrade verifies that a dry-run
// upgrade does not trigger the restart-guidance message (no actual upgrade occurred).
// reExec was removed in task 4.6; restartAfterGentleAIUpgrade now prints+returns.
// Dry-run skips that path entirely, so no restart message should appear.
func TestRunUpgrade_DryRunDoesNotRestartAfterGentleAIUpgrade(t *testing.T) {
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
	})

	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{{Tool: update.ToolInfo{Name: "gentle-ai"}, Status: update.UpdateAvailable}}
	}
	upgradeExecuteWithOptions = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, upgrade.ExecuteOptions) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{DryRun: true, Results: []upgrade.ToolUpgradeResult{{ToolName: "gentle-ai", NewVersion: "1.36.2", Status: upgrade.UpgradeSucceeded}}}
	}

	var buf bytes.Buffer
	err := runUpgrade(context.Background(), []string{"--dry-run"}, system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{OS: "darwin", Supported: true}}}, &buf)
	if err != nil {
		t.Fatalf("runUpgrade() error = %v", err)
	}
	if strings.Contains(buf.String(), "restarting") {
		t.Fatalf("dry-run output should not mention restart:\n%s", buf.String())
	}
}

// TestTUIUpgrade_DoesNotRestartBeforeModelCanRenderReport verifies that the
// TUI upgrade path returns the UpgradeReport without triggering any side-effects
// (e.g. exit or restart) before the UI has a chance to render the result.
// reExec was removed in task 4.6; tuiUpgrade must still return the report intact.
func TestTUIUpgrade_DoesNotRestartBeforeModelCanRenderReport(t *testing.T) {
	origUpgradeExecute := upgradeExecute
	t.Cleanup(func() {
		upgradeExecute = origUpgradeExecute
	})

	upgradeExecute = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, ...io.Writer) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{{ToolName: "gentle-ai", NewVersion: "1.36.2", Status: upgrade.UpgradeSucceeded}}}
	}

	report := tuiUpgrade(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, os.TempDir())(context.Background(), nil)
	if len(report.Results) != 1 || report.Results[0].ToolName != "gentle-ai" {
		t.Fatalf("tuiUpgrade() report = %#v", report)
	}
}
