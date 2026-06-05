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

func TestRunUpgrade_RestartsAfterGentleAIUpgrade(t *testing.T) {
	unsetEnv(t, envSelfUpdateDone)

	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	origReExec := reExec
	origLookPath := lookPathFn
	origGoOS := goOS
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
		reExec = origReExec
		lookPathFn = origLookPath
		goOS = origGoOS
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
	lookPathFn = func(string) (string, error) { return "/usr/local/bin/gentle-ai", nil }
	goOS = func() string { return "darwin" }

	var reExecCalled int
	var reExecArgv0 string
	var reExecEnv []string
	reExec = func(argv0 string, _ []string, envv []string) error {
		reExecCalled++
		reExecArgv0 = argv0
		reExecEnv = envv
		return nil
	}

	var buf bytes.Buffer
	err := runUpgrade(context.Background(), []string{"--no-backup"}, system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, &buf)
	if err != nil {
		t.Fatalf("runUpgrade() error = %v", err)
	}
	if reExecCalled != 1 {
		t.Fatalf("reExecCalled = %d, want 1", reExecCalled)
	}
	if reExecArgv0 != "/usr/local/bin/gentle-ai" {
		t.Fatalf("reExec argv0 = %q, want PATH gentle-ai", reExecArgv0)
	}
	if !envContains(reExecEnv, envSelfUpdateDone+"=1") {
		t.Fatalf("reExec env missing %s=1", envSelfUpdateDone)
	}
	if !strings.Contains(buf.String(), "Updated to v1.36.2, restarting") {
		t.Fatalf("runUpgrade() output missing restart notice:\n%s", buf.String())
	}
}

func TestRestartAfterGentleAIUpgrade_WindowsPrintsRestart(t *testing.T) {
	unsetEnv(t, envSelfUpdateDone)

	origReExec := reExec
	origGoOS := goOS
	t.Cleanup(func() {
		reExec = origReExec
		goOS = origGoOS
	})
	goOS = func() string { return "windows" }

	var reExecCalled int
	reExec = func(string, []string, []string) error {
		reExecCalled++
		return nil
	}

	var buf bytes.Buffer
	err := restartAfterGentleAIUpgrade("1.36.2", &buf)
	if err != nil {
		t.Fatalf("runUpgrade() error = %v", err)
	}
	if reExecCalled != 0 {
		t.Fatalf("reExecCalled = %d, want 0 on Windows", reExecCalled)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "please restart") {
		t.Fatalf("runUpgrade() output missing restart notice:\n%s", buf.String())
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

func TestRunUpgrade_DryRunDoesNotRestartAfterGentleAIUpgrade(t *testing.T) {
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	origReExec := reExec
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
		reExec = origReExec
	})

	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{{Tool: update.ToolInfo{Name: "gentle-ai"}, Status: update.UpdateAvailable}}
	}
	upgradeExecuteWithOptions = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, upgrade.ExecuteOptions) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{DryRun: true, Results: []upgrade.ToolUpgradeResult{{ToolName: "gentle-ai", NewVersion: "1.36.2", Status: upgrade.UpgradeSucceeded}}}
	}

	reExec = func(string, []string, []string) error {
		t.Fatal("dry-run must not re-exec")
		return nil
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

func TestTUIUpgrade_DoesNotRestartBeforeModelCanRenderReport(t *testing.T) {
	origUpgradeExecute := upgradeExecute
	origReExec := reExec
	t.Cleanup(func() {
		upgradeExecute = origUpgradeExecute
		reExec = origReExec
	})

	upgradeExecute = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, ...io.Writer) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{{ToolName: "gentle-ai", NewVersion: "1.36.2", Status: upgrade.UpgradeSucceeded}}}
	}
	reExec = func(string, []string, []string) error {
		t.Fatal("TUI upgrade must not re-exec before rendering the upgrade report")
		return nil
	}

	report := tuiUpgrade(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, os.TempDir())(context.Background(), nil)
	if len(report.Results) != 1 || report.Results[0].ToolName != "gentle-ai" {
		t.Fatalf("tuiUpgrade() report = %#v", report)
	}
}
