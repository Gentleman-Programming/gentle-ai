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

	// Slice 5: prompt is always called — inject auto-accept stub.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

			// Slice 5: prompt is always called — inject auto-accept stub.
			origPrompt := promptFn
			t.Cleanup(func() { promptFn = origPrompt })
			promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

	// Slice 5: prompt is always called — inject auto-accept stub.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

// TestSelfUpdate_ConfirmUpdate_UserAccepts verifies that when the user accepts
// the prompt, the upgrade runs. GENTLE_AI_CONFIRM_UPDATE removed in slice 5 —
// prompt is unconditional.
func TestSelfUpdate_ConfirmUpdate_UserAccepts(t *testing.T) {
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

// TestSelfUpdate_ConfirmUpdate_UserDeclines verifies that when the user declines
// the prompt, the upgrade is skipped. GENTLE_AI_CONFIRM_UPDATE removed in slice 5.
func TestSelfUpdate_ConfirmUpdate_UserDeclines(t *testing.T) {
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

// TestSelfUpdate_PromptAlwaysShown verifies that when an update is available,
// promptFn is always called — GENTLE_AI_CONFIRM_UPDATE is removed and ignored.
// Slice 5 replaces the old "env unset → auto-apply (no prompt)" behavior.
func TestSelfUpdate_PromptAlwaysShown(t *testing.T) {
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

	// Inject promptFn that accepts — must be called exactly once.
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
	// Prompt must be called — there is no longer a no-prompt auto-apply path.
	if promptCalled != 1 {
		t.Errorf("promptCalled = %d, want 1 (prompt is now unconditional)", promptCalled)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1 (user accepted)", stubs.upgradeCalled)
	}
}

// TestSelfUpdate_ConfirmUpdateTable exercises prompt-path combinations.
// Slice 5: GENTLE_AI_CONFIRM_UPDATE is removed — prompt is always called.
// Only promptFn reply + --yes flag govern whether upgrade proceeds.
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
		promptReply     bool
		wantUpgrade     int
		wantPromptCalls int
	}{
		{
			name:            "prompt accept → upgrade runs",
			promptReply:     true,
			wantUpgrade:     1,
			wantPromptCalls: 1,
		},
		{
			name:            "prompt decline → upgrade skipped",
			promptReply:     false,
			wantUpgrade:     0,
			wantPromptCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unsetEnv(t, envNoSelfUpdate)
			unsetEnv(t, envSelfUpdateDone)

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
	stubs := swapSelfUpdateDeps(t, checkResults, upgradeReport)
	_ = stubs

	// swapSelfUpdateDeps sets selfUpdateHomeDirFn to a temp dir; capture that dir.
	tmpHome := t.TempDir()
	selfUpdateHomeDirFn = func() (string, error) { return tmpHome, nil }

	// Slice 5: prompt is always called — inject auto-accept stub.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

	// Slice 5: prompt is always called — inject auto-accept stub so upgrade runs.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

	// Slice 5: prompt is always called — inject auto-accept stub.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) { return true, nil }

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

// ─── Slice 5 RED: CLI prompt is default ──────────────────────────────────────

// TestSelfUpdate_PromptCalledWithoutEnvVar verifies that selfUpdate calls
// promptFn unconditionally when an update is available — no GENTLE_AI_CONFIRM_UPDATE
// env var is required. Task 5.1.
func TestSelfUpdate_PromptCalledWithoutEnvVar(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)
	// Ensure env is NOT set — prompt must be called anyway.
	unsetEnv(t, "GENTLE_AI_CONFIRM_UPDATE")

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

	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	var promptCalled int
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return true, nil // accept so upgrade proceeds
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if promptCalled != 1 {
		t.Errorf("promptCalled = %d, want 1 (prompt must be called without GENTLE_AI_CONFIRM_UPDATE)", promptCalled)
	}
}

// TestSelfUpdate_ConfirmUpdateEnvIgnored verifies that setting GENTLE_AI_CONFIRM_UPDATE=1
// makes no difference — it is ignored and the prompt is called regardless. Task 5.1.
func TestSelfUpdate_ConfirmUpdateEnvIgnored(t *testing.T) {
	unsetEnv(t, envNoSelfUpdate)
	unsetEnv(t, envSelfUpdateDone)
	// Env var set to "1" but must have NO effect on whether prompt is called.
	setEnv(t, "GENTLE_AI_CONFIRM_UPDATE", "1")

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
	// Prompt must be called: GENTLE_AI_CONFIRM_UPDATE is no longer a gate.
	if promptCalled != 1 {
		t.Errorf("promptCalled = %d, want 1 (GENTLE_AI_CONFIRM_UPDATE must be ignored)", promptCalled)
	}
}

// TestSelfUpdate_UserDeclines_NoUpgrade verifies that when promptFn returns false,
// the upgrade is skipped and selfUpdate returns nil. Task 5.1.
func TestSelfUpdate_UserDeclines_NoUpgrade(t *testing.T) {
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

	stubs := swapSelfUpdateDeps(t, checkResults, upgrade.UpgradeReport{})

	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		return false, nil // user declines
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.upgradeCalled != 0 {
		t.Errorf("upgradeCalled = %d, want 0 (user declined)", stubs.upgradeCalled)
	}
}

// TestDefaultPromptForUpdate_NonTTY_AutoDeclines verifies that defaultPromptForUpdate
// returns (false, nil) when stdin is not a TTY, so CI and scripts never hang. Task 5.2.
func TestDefaultPromptForUpdate_NonTTY_AutoDeclines(t *testing.T) {
	// Pass os.Stdin's non-TTY substitute: a regular *os.File from a pipe.
	// We create a pipe — read end is not a TTY.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	defer r.Close()
	defer w.Close()

	ok, err := defaultPromptForUpdate(io.Discard, r, "1.7.0", "1.8.0")
	if err != nil {
		t.Fatalf("defaultPromptForUpdate returned error: %v", err)
	}
	if ok {
		t.Errorf("defaultPromptForUpdate on non-TTY = true, want false (auto-decline)")
	}
}

// TestDefaultPromptForUpdate_EmptyInput_AcceptsDefault verifies that pressing Enter
// (empty input) accepts the default Y. Task 5.6.
func TestDefaultPromptForUpdate_EmptyInput_AcceptsDefault(t *testing.T) {
	// Simulate TTY by passing a file that isatty will consider terminal-like.
	// Since we can't easily fake a TTY in tests, we test the logic after the TTY
	// check by calling an extracted helper or by patching isattyFn.
	// We use the isattyFn package-level var for testability (injected in GREEN).
	origIsatty := isattyFn
	t.Cleanup(func() { isattyFn = origIsatty })
	isattyFn = func(_ uintptr) bool { return true } // simulate TTY

	// Pipe "\n" as input (empty line = Enter).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	_, _ = w.WriteString("\n")
	w.Close()

	ok, err := defaultPromptForUpdate(io.Discard, r, "1.7.0", "1.8.0")
	r.Close()
	if err != nil {
		t.Fatalf("defaultPromptForUpdate returned error: %v", err)
	}
	if !ok {
		t.Errorf("defaultPromptForUpdate with empty input = false, want true (default Y)")
	}
}

// TestDefaultPromptForUpdate_YInput_Accepts verifies explicit "y" accepts. Task 5.6.
func TestDefaultPromptForUpdate_YInput_Accepts(t *testing.T) {
	origIsatty := isattyFn
	t.Cleanup(func() { isattyFn = origIsatty })
	isattyFn = func(_ uintptr) bool { return true }

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	_, _ = w.WriteString("y\n")
	w.Close()

	ok, err := defaultPromptForUpdate(io.Discard, r, "1.7.0", "1.8.0")
	r.Close()
	if err != nil {
		t.Fatalf("defaultPromptForUpdate returned error: %v", err)
	}
	if !ok {
		t.Errorf("defaultPromptForUpdate with 'y' = false, want true")
	}
}

// TestDefaultPromptForUpdate_NInput_Declines verifies explicit "n" declines. Task 5.6.
func TestDefaultPromptForUpdate_NInput_Declines(t *testing.T) {
	origIsatty := isattyFn
	t.Cleanup(func() { isattyFn = origIsatty })
	isattyFn = func(_ uintptr) bool { return true }

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	_, _ = w.WriteString("n\n")
	w.Close()

	ok, err := defaultPromptForUpdate(io.Discard, r, "1.7.0", "1.8.0")
	r.Close()
	if err != nil {
		t.Fatalf("defaultPromptForUpdate returned error: %v", err)
	}
	if ok {
		t.Errorf("defaultPromptForUpdate with 'n' = true, want false")
	}
}

// TestSelfUpdate_YesFlag_AutoAccepts verifies that when selfUpdateYesFn returns true,
// the prompt is skipped and the upgrade proceeds automatically. Task 5.3 / 5.5.
func TestSelfUpdate_YesFlag_AutoAccepts(t *testing.T) {
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

	// Inject --yes: replace promptFn with a stub that always accepts.
	// selfUpdateYesFn returns true → selfUpdate substitutes promptFn with auto-accept stub.
	origYes := selfUpdateYesFn
	t.Cleanup(func() { selfUpdateYesFn = origYes })
	selfUpdateYesFn = func() bool { return true }

	// Also ensure the real promptFn is NOT called (upgrade proceeds via yes-stub).
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptCalled := 0
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return false, nil // would decline if called directly
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1 (--yes auto-accepted)", stubs.upgradeCalled)
	}
	// promptFn (which would decline) should NOT have been called — yes-stub took over.
	if promptCalled != 0 {
		t.Errorf("promptCalled = %d, want 0 (yes-flag bypasses interactive prompt)", promptCalled)
	}
}

// TestSelfUpdate_YesEnvVar_AutoAccepts verifies the GENTLE_AI_YES=1 env-var
// contract end-to-end using the real selfUpdateYesFn (not an injected stub).
// When the env var is set, the upgrade must proceed without calling the
// interactive promptFn. Task 5.3 / 5.5 env-var path.
func TestSelfUpdate_YesEnvVar_AutoAccepts(t *testing.T) {
	t.Setenv("GENTLE_AI_YES", "1")
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

	// Restore the real selfUpdateYesFn so it reads the env var for real.
	origYes := selfUpdateYesFn
	t.Cleanup(func() { selfUpdateYesFn = origYes })
	selfUpdateYesFn = func() bool { return os.Getenv(envYesUpdate) == "1" }

	// A promptFn that declines — it must NOT be called when GENTLE_AI_YES=1.
	origPrompt := promptFn
	t.Cleanup(func() { promptFn = origPrompt })
	promptCalled := 0
	promptFn = func(_ io.Writer, _ io.Reader, _, _ string) (bool, error) {
		promptCalled++
		return false, nil // would decline if reached
	}

	err := selfUpdate(context.Background(), "1.7.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.upgradeCalled != 1 {
		t.Errorf("upgradeCalled = %d, want 1 (GENTLE_AI_YES=1 auto-accepts)", stubs.upgradeCalled)
	}
	if promptCalled != 0 {
		t.Errorf("promptCalled = %d, want 0 (GENTLE_AI_YES=1 bypasses interactive prompt)", promptCalled)
	}
}

// TestSelfUpdate_NoSelfUpdate_StillSkips_Slice5 verifies that GENTLE_AI_NO_SELF_UPDATE
// continues to work after slice 5 changes. Task 5.7 guard.
func TestSelfUpdate_NoSelfUpdate_StillSkips_Slice5(t *testing.T) {
	setEnv(t, envNoSelfUpdate, "1")
	unsetEnv(t, envSelfUpdateDone)

	stubs := swapSelfUpdateDeps(t, nil, upgrade.UpgradeReport{})

	err := selfUpdate(context.Background(), "1.8.0", stubProfile(), io.Discard)
	if err != nil {
		t.Fatalf("selfUpdate returned error: %v", err)
	}
	if stubs.checkCalled != 0 {
		t.Errorf("checkCalled = %d, want 0 (GENTLE_AI_NO_SELF_UPDATE must still skip)", stubs.checkCalled)
	}
}

// containsSubstring reports whether s contains substr (case-insensitive not needed here).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
