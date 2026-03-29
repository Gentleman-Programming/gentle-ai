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

// stubProfile returns a minimal PlatformProfile for testing.
func stubProfile() system.PlatformProfile {
	return system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
}

// setEnv is a test helper that sets an env var and registers cleanup to restore it.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	orig, existed := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

// unsetEnv is a test helper that unsets an env var and registers cleanup to restore it.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

// swapSelfUpdateDeps replaces all package-level dependency vars used by selfUpdate
// and registers cleanup to restore them. Returns pointers to track call counts.
type selfUpdateStubs struct {
	checkCalled   int
	upgradeCalled int
	reExecCalled  int
	reExecArgv0   string
	reExecEnv     []string
}

func swapSelfUpdateDeps(t *testing.T, checkResult []update.UpdateResult, upgradeReport upgrade.UpgradeReport) *selfUpdateStubs {
	t.Helper()

	stubs := &selfUpdateStubs{}

	origCheck := updateCheckFiltered
	origUpgrade := upgradeExecute
	origReExec := reExec
	origGoOS := goOS

	t.Cleanup(func() {
		updateCheckFiltered = origCheck
		upgradeExecute = origUpgrade
		reExec = origReExec
		goOS = origGoOS
	})

	updateCheckFiltered = func(_ context.Context, _ string, _ system.PlatformProfile, _ []string) []update.UpdateResult {
		stubs.checkCalled++
		return checkResult
	}

	upgradeExecute = func(_ context.Context, _ []update.UpdateResult, _ system.PlatformProfile, _ string, _ bool, _ ...io.Writer) upgrade.UpgradeReport {
		stubs.upgradeCalled++
		return upgradeReport
	}

	reExec = func(argv0 string, argv []string, envv []string) error {
		stubs.reExecCalled++
		stubs.reExecArgv0 = argv0
		stubs.reExecEnv = envv
		return nil
	}

	return stubs
}

func TestSelfUpdate_SkipWhenDevVersion(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	stubs := swapSelfUpdateDeps(t, nil, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "dev", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 0 {
		t.Errorf("expected no check call for dev version, got %d", stubs.checkCalled)
	}
}

func TestSelfUpdate_SkipWhenOptOut(t *testing.T) {
	setEnv(t, envNoSelfUpdate, "1")
	unsetEnv(t, envSelfUpdateDone)

	stubs := swapSelfUpdateDeps(t, nil, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "1.8.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 0 {
		t.Errorf("expected no check call when opt-out set, got %d", stubs.checkCalled)
	}
}

func TestSelfUpdate_SkipWhenAlreadyDone(t *testing.T) {
	setEnv(t, envSelfUpdateDone, "1")
	unsetEnv(t, envNoSelfUpdate)

	stubs := swapSelfUpdateDeps(t, nil, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "1.8.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 0 {
		t.Errorf("expected no check call when already done, got %d", stubs.checkCalled)
	}
}

func TestSelfUpdate_GuardEvaluationOrder(t *testing.T) {
	// When SELF_UPDATE_DONE is set, even if version is "dev" and opt-out is set,
	// the done-guard should fire first (no check call).
	setEnv(t, envSelfUpdateDone, "1")
	setEnv(t, envNoSelfUpdate, "1")

	stubs := swapSelfUpdateDeps(t, nil, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "dev", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 0 {
		t.Errorf("expected no check call, got %d", stubs.checkCalled)
	}
}

func TestSelfUpdate_UpdateAvailable_CallsUpgradeAndReExec(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}
	upgradeReport := upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "1.8.0"},
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgradeReport)

	var buf bytes.Buffer
	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), &buf)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 1 {
		t.Errorf("checkCalled = %d, want 1", stubs.checkCalled)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1", stubs.upgradeCalled)
	}
	if stubs.reExecCalled != 1 {
		t.Errorf("reExecCalled = %d, want 1", stubs.reExecCalled)
	}

	// Verify GENTLE_AI_SELF_UPDATE_DONE=1 is in the re-exec env.
	found := false
	for _, e := range stubs.reExecEnv {
		if e == envSelfUpdateDone+"=1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("re-exec env missing %s=1", envSelfUpdateDone)
	}
}

func TestSelfUpdate_UpToDate_NoUpgradeCall(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.8.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpToDate,
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "1.8.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 1 {
		t.Errorf("checkCalled = %d, want 1", stubs.checkCalled)
	}
	if stubs.upgradeCalled != 0 {
		t.Errorf("upgradeCalled = %d, want 0 (up to date)", stubs.upgradeCalled)
	}
}

func TestSelfUpdate_CheckError_ReturnsNil(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool:   update.ToolInfo{Name: "gentle-ai"},
			Status: update.CheckFailed,
			Err:    context.DeadlineExceeded,
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate should return nil on check error, got: %v", err)
	}
	if stubs.upgradeCalled != 0 {
		t.Errorf("upgradeCalled = %d, want 0 (check failed)", stubs.upgradeCalled)
	}
}

func TestSelfUpdate_UpgradeError_ReturnsNil(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}
	upgradeReport := upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{
				ToolName: "gentle-ai",
				Status:   upgrade.UpgradeFailed,
				Err:      os.ErrPermission,
			},
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgradeReport)

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate should return nil on upgrade error, got: %v", err)
	}
	if stubs.reExecCalled != 0 {
		t.Errorf("reExecCalled = %d, want 0 (upgrade failed)", stubs.reExecCalled)
	}
}

func TestSelfUpdate_Windows_PrintsRestartMessage(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}
	upgradeReport := upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "1.8.0"},
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgradeReport)

	// Simulate Windows: re-exec should NOT be called, restart message printed instead.
	goOS = func() string { return "windows" }

	var buf bytes.Buffer
	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), &buf)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.reExecCalled != 0 {
		t.Errorf("reExecCalled = %d, want 0 on Windows", stubs.reExecCalled)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1", stubs.upgradeCalled)
	}

	out := buf.String()
	if want := "please restart"; !containsSubstring(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
}

func TestSelfUpdate_BrewInstallMethod_PassedToUpgradeExecutor(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)

	checkResults := []update.UpdateResult{
		{
			Tool: update.ToolInfo{
				Name:          "gentle-ai",
				InstallMethod: update.InstallBrew,
			},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}

	// Track what upgradeExecute receives.
	var capturedResults []update.UpdateResult
	var capturedProfile system.PlatformProfile

	origCheck := updateCheckFiltered
	origUpgrade := upgradeExecute
	origReExec := reExec
	t.Cleanup(func() {
		updateCheckFiltered = origCheck
		upgradeExecute = origUpgrade
		reExec = origReExec
	})

	updateCheckFiltered = func(_ context.Context, _ string, _ system.PlatformProfile, _ []string) []update.UpdateResult {
		return checkResults
	}

	upgradeExecute = func(_ context.Context, results []update.UpdateResult, profile system.PlatformProfile, _ string, _ bool, _ ...io.Writer) upgrade.UpgradeReport {
		capturedResults = results
		capturedProfile = profile
		return upgrade.UpgradeReport{
			Results: []upgrade.ToolUpgradeResult{
				{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "1.8.0"},
			},
		}
	}

	reExec = func(_ string, _ []string, _ []string) error { return nil }

	brewProfile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}
	err := selfUpdate(context.Background(), "1.7.0", brewProfile, io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}

	// Verify the brew install method was forwarded to the upgrade executor.
	if len(capturedResults) == 0 {
		t.Fatal("upgradeExecute was not called")
	}
	if got := capturedResults[0].Tool.InstallMethod; got != update.InstallBrew {
		t.Errorf("InstallMethod passed to upgradeExecute = %q, want %q", got, update.InstallBrew)
	}
	if capturedProfile.PackageManager != "brew" {
		t.Errorf("PackageManager passed to upgradeExecute = %q, want %q", capturedProfile.PackageManager, "brew")
	}
}

// containsSubstring reports whether s contains substr (case-insensitive not needed here).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
