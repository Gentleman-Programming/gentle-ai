package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/state"
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
// Note: reExec and goOS were removed in task 4.6 — restartAfterGentleAIUpgrade
// now always prints the restart message and returns; no re-exec on any OS.
type selfUpdateStubs struct {
	checkCalled   int
	upgradeCalled int
}

func swapSelfUpdateDeps(t *testing.T, checkResult []update.UpdateResult, upgradeReport upgrade.UpgradeReport) *selfUpdateStubs {
	t.Helper()

	stubs := &selfUpdateStubs{}

	origCheck := updateCheckFiltered
	origUpgrade := upgradeExecute
	origHomeDir := selfUpdateHomeDirFn
	origNow := selfUpdateNowFn

	// Use a temp dir for cooldown state so the gate always reads "never checked"
	// (no state.json present) and calls the injected updateCheckFiltered stub.
	tmpHome := t.TempDir()

	t.Cleanup(func() {
		updateCheckFiltered = origCheck
		upgradeExecute = origUpgrade
		selfUpdateHomeDirFn = origHomeDir
		selfUpdateNowFn = origNow
	})

	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }
	// Use a fixed "now" far in the future so any stale state would still trigger.
	selfUpdateNowFn = func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }

	updateCheckFiltered = func(_ context.Context, _ string, _ system.PlatformProfile, _ []string) []update.UpdateResult {
		stubs.checkCalled++
		return checkResult
	}

	upgradeExecute = func(_ context.Context, _ []update.UpdateResult, _ system.PlatformProfile, _ string, _ bool, _ ...io.Writer) upgrade.UpgradeReport {
		stubs.upgradeCalled++
		return upgradeReport
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

func TestSelfUpdate_UpdateAvailable_CallsUpgradeAndRestart(t *testing.T) {
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

	// Output must contain the restart guidance message (print-and-return path, no re-exec).
	out := buf.String()
	if !containsSubstring(out, "restart") {
		t.Errorf("output = %q, want it to contain restart guidance", out)
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

	swapSelfUpdateDeps(t, checkResults, upgradeReport)

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate should return nil on upgrade error, got: %v", err)
	}
}

// TestSelfUpdate_PrintsRestartMessage verifies that after a successful upgrade
// on any OS, restartAfterGentleAIUpgrade prints a restart-guidance message and
// does NOT re-exec (converged behavior — task 4.6).
func TestSelfUpdate_PrintsRestartMessage(t *testing.T) {
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

	// restartAfterGentleAIUpgrade is OS-agnostic after task 4.6 — prints and returns.
	// No goOS swap needed; the behavior is identical on all platforms.
	for _, osName := range []string{"darwin", "windows", "linux"} {
		t.Run("os="+osName, func(t *testing.T) {
			stubs := swapSelfUpdateDeps(t, checkResults, upgradeReport)

			var buf bytes.Buffer
			err := selfUpdate(context.Background(), "1.7.0", stubProfile(), &buf)
			if err != nil {
				t.Fatalf("selfUpdate returned error: %v", err)
			}
			if stubs.upgradeCalled != 1 {
				t.Errorf("upgradeCalled = %d, want 1", stubs.upgradeCalled)
			}

			out := buf.String()
			if !containsSubstring(out, "restart") {
				t.Errorf("output = %q, want it to contain restart guidance", out)
			}
		})
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
	origHomeDir := selfUpdateHomeDirFn
	origNow := selfUpdateNowFn
	tmpHome := t.TempDir()
	t.Cleanup(func() {
		updateCheckFiltered = origCheck
		upgradeExecute = origUpgrade
		selfUpdateHomeDirFn = origHomeDir
		selfUpdateNowFn = origNow
	})

	// Use a temp home with no state.json so the cooldown gate never fires.
	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }
	selfUpdateNowFn = func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }

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

	// wantReExec removed: restartAfterGentleAIUpgrade always prints and returns (task 4.6).
	tests := []struct {
		name            string
		confirmEnv      string // "" means unset
		promptReply     bool
		wantUpgrade     int
		wantPromptCalls int
	}{
		{
			name:            "env unset → auto-apply (no prompt)",
			confirmEnv:      "",
			promptReply:     false,
			wantUpgrade:     1,
			wantPromptCalls: 0,
		},
		{
			name:            "env set + accept → upgrade runs",
			confirmEnv:      "1",
			promptReply:     true,
			wantUpgrade:     1,
			wantPromptCalls: 1,
		},
		{
			name:            "env set + decline → upgrade skipped",
			confirmEnv:      "1",
			promptReply:     false,
			wantUpgrade:     0,
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
		})
	}
}

// ─── Slice 4 RED: PendingSync written on successful self-upgrade ─────────────

// TestSelfUpdate_SetsPendingSyncOnSuccess verifies that after a successful
// gentle-ai self-upgrade, PendingSync=true is written to state before the
// process exits (re-exec or print message). This is the deferred-sync flag
// that the next launch reads to run sync automatically.
func TestSelfUpdate_SetsPendingSyncOnSuccess(t *testing.T) {
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

	// swapSelfUpdateDeps sets selfUpdateHomeDirFn to a temp dir; override with our own
	// so we can read back the state after selfUpdate returns.
	swapSelfUpdateDeps(t, checkResults, upgradeReport)
	tmpHome := t.TempDir()
	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}

	s, err := state.Read(tmpHome)
	if err != nil {
		// state.json may not exist when PendingSync is not implemented yet.
		t.Fatalf("state.Read() failed — PendingSync was not written: %v", err)
	}
	if !s.PendingSync {
		t.Errorf("PendingSync = false after successful self-upgrade, want true")
	}
}

// TestSelfUpdate_DoesNotSetPendingSyncOnFailure verifies that when the
// gentle-ai upgrade fails, PendingSync is NOT set in state (no retry needed
// since sync was never deferred).
func TestSelfUpdate_DoesNotSetPendingSyncOnFailure(t *testing.T) {
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
			{ToolName: "gentle-ai", Status: upgrade.UpgradeFailed, Err: os.ErrPermission},
		},
	}

	swapSelfUpdateDeps(t, checkResults, upgradeReport)

	tmpHome := t.TempDir()
	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}

	// State may not exist at all (upgrade failed, nothing written) — that's fine.
	s, readErr := state.Read(tmpHome)
	if readErr == nil && s.PendingSync {
		t.Errorf("PendingSync = true after failed upgrade, want false")
	}
}

// TestSelfUpdate_NoClobberOnCorruptStateFile verifies that when state.Read fails
// with a non-ErrNotExist error (e.g. corrupt JSON), PendingSync is NOT written
// and the existing state file bytes are preserved unchanged.
func TestSelfUpdate_NoClobberOnCorruptStateFile(t *testing.T) {
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

	swapSelfUpdateDeps(t, checkResults, upgradeReport)
	tmpHome := t.TempDir()
	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }

	// Write a corrupt (non-missing) state file so state.Read returns a non-ErrNotExist error.
	stateDir := filepath.Join(tmpHome, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	corruptPayload := []byte("this is not valid JSON {{{")
	stateFilePath := filepath.Join(stateDir, "state.json")
	if err := os.WriteFile(stateFilePath, corruptPayload, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}

	// The state file must not have been overwritten — original bytes must be intact.
	got, readErr := os.ReadFile(stateFilePath)
	if readErr != nil {
		t.Fatalf("os.ReadFile after selfUpdate: %v", readErr)
	}
	if string(got) != string(corruptPayload) {
		t.Errorf("state file was overwritten on corrupt-read error\ngot:  %q\nwant: %q", got, corruptPayload)
	}
}

// containsSubstring reports whether s contains substr (case-insensitive not needed here).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
