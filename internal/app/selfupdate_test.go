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

	goOS = func() string {
		return "darwin"
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

// TestSelfUpdate_ConfirmUpdate_UserAccepts verifies that when GENTLE_AI_CONFIRM_UPDATE=1
// and the user accepts, the upgrade runs and re-exec is called.
func TestSelfUpdate_ConfirmUpdate_UserAccepts(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)
	setEnv(t, envConfirmUpdate, "1")

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

	// Inject a promptFn that simulates user accepting.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	var promptCalled int
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return true, nil
	}

	var buf bytes.Buffer
	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), &buf)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if promptCalled != 1 {
		t.Errorf("promptCalled = %d, want 1", promptCalled)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1 (user accepted)", stubs.upgradeCalled)
	}
	if stubs.reExecCalled != 1 {
		t.Errorf("reExecCalled = %d, want 1 (user accepted)", stubs.reExecCalled)
	}
}

// TestSelfUpdate_ConfirmUpdate_UserDeclines verifies that when GENTLE_AI_CONFIRM_UPDATE=1
// and the user declines, the upgrade is skipped.
func TestSelfUpdate_ConfirmUpdate_UserDeclines(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)
	setEnv(t, envConfirmUpdate, "1")

	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}

	stubs := swapSelfUpdateDeps(t, checkResults, upgrade.UpgradeReport{})

	// Inject a promptFn that simulates user declining.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	var promptCalled int
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return false, nil
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if promptCalled != 1 {
		t.Errorf("promptCalled = %d, want 1", promptCalled)
	}
	if stubs.upgradeCalled != 0 {
		t.Errorf("upgradeCalled = %d, want 0 (user declined)", stubs.upgradeCalled)
	}
	if stubs.reExecCalled != 0 {
		t.Errorf("reExecCalled = %d, want 0 (user declined)", stubs.reExecCalled)
	}
}

// TestSelfUpdate_ConfirmUpdate_EnvUnset verifies that when GENTLE_AI_CONFIRM_UPDATE is
// not set, the existing auto-apply behaviour is preserved (no prompt shown).
func TestSelfUpdate_ConfirmUpdate_EnvUnset(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)
	unsetEnv(t, envConfirmUpdate)

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

	// Inject a promptFn that should NOT be called.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	var promptCalled int
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return true, nil
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if promptCalled != 0 {
		t.Errorf("promptCalled = %d, want 0 (auto-apply when env unset)", promptCalled)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1 (auto-apply)", stubs.upgradeCalled)
	}
	if stubs.reExecCalled != 1 {
		t.Errorf("reExecCalled = %d, want 1 (auto-apply)", stubs.reExecCalled)
	}
}

// TestSelfUpdate_ConfirmUpdateTable exercises the three confirmation paths in a
// table-driven style using the promptFn injection point.
func TestSelfUpdate_ConfirmUpdateTable(t *testing.T) {
	checkResults := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "1.7.0",
			LatestVersion:    "1.8.0",
			Status:           update.UpdateAvailable,
		},
	}
	successReport := upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "1.8.0"},
		},
	}

	tests := []struct {
		name            string
		confirmEnv      string // "" means unset
		promptReply     bool
		wantUpgrade     int
		wantReExec      int
		wantPromptCalls int
	}{
		{
			name:            "env unset → auto-apply (no prompt)",
			confirmEnv:      "",
			promptReply:     false,
			wantUpgrade:     1,
			wantReExec:      1,
			wantPromptCalls: 0,
		},
		{
			name:            "env set + accept → upgrade runs",
			confirmEnv:      "1",
			promptReply:     true,
			wantUpgrade:     1,
			wantReExec:      1,
			wantPromptCalls: 1,
		},
		{
			name:            "env set + decline → upgrade skipped",
			confirmEnv:      "1",
			promptReply:     false,
			wantUpgrade:     0,
			wantReExec:      0,
			wantPromptCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unsetEnv(t, envNoSelfUpdate)
			unsetEnv(t, envSelfUpdateDone)
			if tc.confirmEnv != "" {
				setEnv(t, envConfirmUpdate, tc.confirmEnv)
			} else {
				unsetEnv(t, envConfirmUpdate)
			}

			stubs := swapSelfUpdateDeps(t, checkResults, successReport)

			origPrompt := promptFn
			t.Cleanup(func() { promptFn = origPrompt })
			var promptCalled int
			reply := tc.promptReply
			promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
				promptCalled++
				return reply, nil
			}

			err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
			if err != nil {
				t.Fatalf("selfUpdate returned error: %v", err)
			}
			if promptCalled != tc.wantPromptCalls {
				t.Errorf("promptCalled = %d, want %d", promptCalled, tc.wantPromptCalls)
			}
			if stubs.upgradeCalled != tc.wantUpgrade {
				t.Errorf("upgradeCalled = %d, want %d", stubs.upgradeCalled, tc.wantUpgrade)
			}
			if stubs.reExecCalled != tc.wantReExec {
				t.Errorf("reExecCalled = %d, want %d", stubs.reExecCalled, tc.wantReExec)
			}
		})
	}
}

// containsSubstring reports whether s contains substr (case-insensitive not needed here).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
