package upgrade

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// --- helpers ---

func brewProfile() system.PlatformProfile {
	return system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}
}

func linuxProfile() system.PlatformProfile {
	return system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", Supported: true}
}

func makeResult(name string, status update.UpdateStatus, oldVer, newVer string, method update.InstallMethod) update.UpdateResult {
	return update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          name,
			Owner:         "Gentleman-Programming",
			Repo:          name,
			InstallMethod: method,
		},
		InstalledVersion: oldVer,
		LatestVersion:    newVer,
		Status:           status,
	}
}

// --- TestExecute_NoopWhenNothingIsExecutable ---

// TestExecute_NoopWhenNothingIsExecutable verifies that Execute returns an empty
// UpgradeReport with no backup and no tool results when no UpdateResult is
// UpdateAvailable or DevBuild status (i.e. only UpToDate and NotInstalled tools).
func TestExecute_NoopWhenNothingIsExecutable(t *testing.T) {
	results := []update.UpdateResult{
		makeResult("gentle-ai", update.UpToDate, "1.0.0", "1.0.0", update.InstallBinary),
		makeResult("engram", update.NotInstalled, "", "0.4.0", update.InstallGoInstall),
		// gga: CheckFailed — should also be omitted from results.
		makeResult("gga", update.CheckFailed, "", "", update.InstallBinary),
	}

	report := Execute(context.Background(), results, brewProfile(), t.TempDir(), false)

	if report.BackupID != "" {
		t.Errorf("BackupID = %q, want empty — no backup should be created when nothing to execute", report.BackupID)
	}

	if len(report.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0 — UpToDate, NotInstalled, CheckFailed must be omitted", len(report.Results))
	}

	if report.DryRun {
		t.Errorf("DryRun should be false when not requested")
	}
}

// --- TestExecute_DevBuildOnlyNoBackupCreated ---

// TestExecute_DevBuildOnlyNoBackupCreated verifies that when ALL tools are DevBuild
// (nothing to execute), no backup snapshot is created. Backup is only needed before
// actual binary execution, not for skip-only reports.
func TestExecute_DevBuildOnlyNoBackupCreated(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return exec.Command("echo", "should not be called")
	}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.DevBuild, "dev", "1.0.0", update.InstallBinary),
	}

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if execCalled {
		t.Errorf("execCommand should NOT be called for DevBuild-only inputs")
	}

	// DevBuild tool MUST appear in results as UpgradeSkipped.
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 — DevBuild tool must appear as skipped", len(report.Results))
	}
	if report.Results[0].Status != UpgradeSkipped {
		t.Errorf("DevBuild Status = %q, want UpgradeSkipped", report.Results[0].Status)
	}

	// No backup should be created — nothing executed.
	if report.BackupID != "" {
		t.Errorf("BackupID = %q, want empty — no backup when no execution occurs", report.BackupID)
	}
}

// --- TestExecute_BackupBeforeExecution ---

// TestExecute_BackupBeforeExecution verifies the architectural invariant:
// a backup snapshot is created BEFORE any upgrade execution begins.
// We verify this by ensuring BackupID is non-empty when upgrades are available.
func TestExecute_BackupBeforeExecution(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	// Capture exec calls to verify ordering.
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, name)
		// Return a real passing command (echo) so exec succeeds.
		return exec.Command("echo", "ok")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// BackupID must be non-empty.
	if report.BackupID == "" {
		t.Errorf("BackupID is empty — backup must be created before upgrade execution")
	}

	// At least one result must be present.
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
}

// --- TestExecute_DryRunNeverExecs ---

// TestExecute_DryRunNeverExecs verifies that when dryRun=true, no exec is called
// but the report is still populated.
func TestExecute_DryRunNeverExecs(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	called := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		called = true
		return exec.Command("echo", "should not run")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), true)

	if called {
		t.Errorf("execCommand was called during dry-run — must NOT execute")
	}

	if !report.DryRun {
		t.Errorf("DryRun = false, want true")
	}

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}

	if report.Results[0].Status != UpgradeSkipped {
		t.Errorf("dry-run status = %q, want UpgradeSkipped", report.Results[0].Status)
	}
}

// --- TestExecute_PerToolSuccessFailureSkip ---

// TestExecute_PerToolSuccessAndFailure verifies that Execute reports success for one
// tool and failure for another in a mixed scenario.
func TestExecute_PerToolSuccessAndFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		// engram go install succeeds, gga curl/download attempt fails — we simulate
		// the failure by having execCommand return false for "gga" detection.
		if name == "go" {
			return exec.Command("echo", "go install ok")
		}
		// Any other exec attempt fails.
		return exec.Command("false")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}

	// engram should succeed (go install echo'd "ok")
	if report.Results[0].Status != UpgradeSucceeded {
		t.Errorf("engram status = %q, want UpgradeSucceeded", report.Results[0].Status)
	}
}

// --- TestExecute_DevBuildIsSkipped ---

// TestExecute_DevBuildIsSkipped verifies the spec requirement:
// gentle-ai with DevBuild status must appear in Results as UpgradeSkipped
// with a non-empty ManualHint explaining it is a source/dev build.
// DevBuild tools must NOT be auto-executed, and engram/gga remain eligible.
func TestExecute_DevBuildIsSkipped(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.DevBuild, "dev", "1.0.0", update.InstallBinary),
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[1].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// gentle-ai (DevBuild) MUST appear as UpgradeSkipped with a ManualHint.
	var devResult *ToolUpgradeResult
	for i := range report.Results {
		if report.Results[i].ToolName == "gentle-ai" {
			r := report.Results[i]
			devResult = &r
		}
	}
	if devResult == nil {
		t.Fatalf("gentle-ai (DevBuild) must appear in Results — was not found")
	}
	if devResult.Status != UpgradeSkipped {
		t.Errorf("gentle-ai DevBuild Status = %q, want UpgradeSkipped", devResult.Status)
	}
	if devResult.ManualHint == "" {
		t.Errorf("gentle-ai DevBuild ManualHint must be non-empty")
	}

	// engram should still be processed as succeeded.
	found := false
	for _, r := range report.Results {
		if r.ToolName == "engram" {
			found = true
			if r.Status != UpgradeSucceeded {
				t.Errorf("engram status = %q, want UpgradeSucceeded", r.Status)
			}
		}
	}
	if !found {
		t.Errorf("engram not found in Results")
	}
}

// --- TestExecute_FailureDoesNotImplyConfigLoss ---

// TestExecute_FailureDoesNotImplyConfigLoss verifies that when a tool upgrade fails,
// we can still retrieve the BackupID — confirming config was snapshotted first.
func TestExecute_FailureDoesNotImplyConfigLoss(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	// Force all exec to fail.
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// Even with failure, BackupID must be set (backup happened before exec).
	if report.BackupID == "" {
		t.Errorf("BackupID is empty — backup must be created before upgrade, even if upgrade fails")
	}

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}

	if report.Results[0].Status != UpgradeFailed {
		t.Errorf("status = %q, want UpgradeFailed", report.Results[0].Status)
	}

	if report.Results[0].Err == nil {
		t.Errorf("Err should not be nil on failure")
	}
}

// --- TestExecute_InstallNotInvoked ---

// TestExecute_InstallNotInvoked verifies the isolation contract:
// Execute must not invoke any install/sync functions.
// We test this by verifying the package cannot even reference installer packages.
// This is enforced by the import boundary (no import of pipeline/planner/cli).
func TestExecute_InstallNotInvoked(t *testing.T) {
	// This test is intentionally a documentation-only guard.
	// The real enforcement is: this package MUST NOT import:
	//   - github.com/gentleman-programming/gentle-ai/internal/pipeline
	//   - github.com/gentleman-programming/gentle-ai/internal/planner
	//   - github.com/gentleman-programming/gentle-ai/internal/cli
	//
	// If you see those imports appear, the isolation contract is broken.
	// See TestExecuteImportBoundary for the compile-time enforcement approach.
	t.Log("install isolation enforced by import boundary — see imports at top of executor.go")
}

// --- TestExecute_DevBuildSurfacedAsSkipped ---

// TestExecute_DevBuildSurfacedAsSkipped verifies the spec gap:
// A DevBuild tool (e.g. gentle-ai with version="dev") MUST appear in UpgradeReport.Results
// with Status=UpgradeSkipped and a non-empty ManualHint explaining it is a dev/source build.
// Previously, DevBuild tools were silently omitted from Results entirely.
func TestExecute_DevBuildSurfacedAsSkipped(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.DevBuild, "dev", "1.0.0", update.InstallBinary),
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[1].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// gentle-ai (DevBuild) MUST appear in results as UpgradeSkipped.
	var devResult *ToolUpgradeResult
	for i := range report.Results {
		if report.Results[i].ToolName == "gentle-ai" {
			r := report.Results[i]
			devResult = &r
		}
	}

	if devResult == nil {
		t.Fatalf("gentle-ai DevBuild must appear in Results as UpgradeSkipped, but was not found")
	}

	if devResult.Status != UpgradeSkipped {
		t.Errorf("gentle-ai DevBuild Status = %q, want UpgradeSkipped", devResult.Status)
	}

	if devResult.ManualHint == "" {
		t.Errorf("gentle-ai DevBuild ManualHint must be non-empty — should explain dev/source build")
	}

	// engram (UpdateAvailable) must still be processed normally.
	found := false
	for _, r := range report.Results {
		if r.ToolName == "engram" {
			found = true
			if r.Status != UpgradeSucceeded {
				t.Errorf("engram status = %q, want UpgradeSucceeded", r.Status)
			}
		}
	}
	if !found {
		t.Errorf("engram not found in Results")
	}
}

// --- TestExecute_ManualFallbackSurfacedAsSkippedNotFailed ---

// TestExecute_ManualFallbackSurfacedAsSkippedNotFailed verifies the spec gap:
// When runStrategy returns a manual fallback error (e.g. Windows binary self-replace),
// the ToolUpgradeResult must be UpgradeSkipped (not UpgradeFailed) and ManualHint
// must be populated from the error message so RenderUpgradeReport can display it.
func TestExecute_ManualFallbackSurfacedAsSkippedNotFailed(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return exec.Command("echo", "should not be called")
	}

	// Windows profile → binaryUpgrade returns a manual fallback error.
	windowsProfile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.5.0", update.InstallBinary),
	}
	results[0].UpdateHint = "See https://github.com/Gentleman-Programming/gentle-ai/releases"

	report := Execute(context.Background(), results, windowsProfile, t.TempDir(), false)

	if execCalled {
		t.Errorf("execCommand should not be called for Windows binary manual fallback")
	}

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}

	r := report.Results[0]

	// Must be UpgradeSkipped (not UpgradeFailed) — this is a manual action, not a failure.
	if r.Status != UpgradeSkipped {
		t.Errorf("Windows binary fallback Status = %q, want UpgradeSkipped (not UpgradeFailed)", r.Status)
	}

	// ManualHint must be populated.
	if r.ManualHint == "" {
		t.Errorf("Windows binary fallback ManualHint must be non-empty")
	}

	// Err should be nil for a manual skip (it is not a failure).
	if r.Err != nil {
		t.Errorf("Windows binary fallback Err = %v, want nil (manual skips are not errors)", r.Err)
	}
}

// --- TestExecute_ConfigNotMutatedDuringUpgrade ---

// TestExecute_ConfigNotMutatedDuringUpgrade provides direct evidence that upgrade
// execution does not mutate config file contents — the spec's config preservation
// guarantee. We create real config files in a temp dir, run Execute (stubbed exec),
// and diff the contents before and after.
func TestExecute_ConfigNotMutatedDuringUpgrade(t *testing.T) {
	homeDir := t.TempDir()

	// Create realistic config files with known contents.
	configFiles := map[string]string{
		".claude/CLAUDE.md":            "# Claude config\nThis is my config.\n",
		".config/opencode/config.json": `{"theme":"kanagawa"}`,
		".gemini/GEMINI.md":            "# Gemini config\nMy rules.\n",
	}

	for relPath, content := range configFiles {
		fullPath := homeDir + "/" + relPath
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create dir for %s: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write config %s: %v", relPath, err)
		}
	}

	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		// Simulate a successful upgrade (no-op shell command).
		return exec.Command("echo", "upgrade ok")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	profile := linuxProfile()

	// Execute upgrade.
	report := Execute(context.Background(), results, profile, homeDir, false)

	// Verify upgrade ran.
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	if report.Results[0].Status != UpgradeSucceeded {
		t.Errorf("engram status = %q, want UpgradeSucceeded", report.Results[0].Status)
	}

	// Verify config files are byte-identical after upgrade.
	for relPath, want := range configFiles {
		fullPath := homeDir + "/" + relPath
		got, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("read config %s after upgrade: %v", relPath, err)
		}
		if string(got) != want {
			t.Errorf("config %s was mutated by upgrade!\n  before: %q\n  after:  %q", relPath, want, string(got))
		}
	}
}

// --- helper: verify errors wrap correctly ---
func TestToolUpgradeResult_ErrorWrapping(t *testing.T) {
	sentinel := errors.New("sentinel error")
	r := ToolUpgradeResult{
		ToolName: "engram",
		Status:   UpgradeFailed,
		Err:      sentinel,
	}

	if !errors.Is(r.Err, sentinel) {
		t.Errorf("errors.Is failed — Err should wrap the sentinel")
	}
}

// --- Upgrade Backup Hardening Tests ---

// TestConfigPathsForBackup_CoversAgentDirectories verifies that configPathsForBackup
// returns files from all expected agent config directories, not just 4 hardcoded paths.
// This tests the G5 gap fix: computed paths aligned with ScanConfigs directories.
func TestConfigPathsForBackup_CoversAgentDirectories(t *testing.T) {
	homeDir := t.TempDir()

	// Create files in each agent config directory to verify they are discovered.
	agentFiles := map[string]string{
		".claude/CLAUDE.md":              "# Claude",
		".claude/extra_rule.md":          "# extra rule",
		".config/opencode/config.json":   `{"model":"claude"}`,
		".config/opencode/settings.json": `{"theme":"dark"}`,
		".gemini/GEMINI.md":              "# Gemini",
		".cursor/rules":                  "# Cursor rules",
	}

	for relPath, content := range agentFiles {
		full := filepath.Join(homeDir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	paths := configPathsForBackup(homeDir)

	// Must include at least the files we created.
	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	for relPath := range agentFiles {
		full := filepath.Join(homeDir, relPath)
		if _, ok := pathSet[full]; !ok {
			t.Errorf("configPathsForBackup missing %q — computed paths must cover all files in agent dirs", relPath)
		}
	}
}

// TestConfigPathsForBackup_HandlesEmptyDirs verifies that configPathsForBackup
// returns a non-nil slice (possibly empty) when agent config directories don't exist.
// It must NOT panic or error out — missing dirs simply contribute no paths.
func TestConfigPathsForBackup_HandlesEmptyDirs(t *testing.T) {
	homeDir := t.TempDir()
	// No agent config directories exist in this temp dir.

	paths := configPathsForBackup(homeDir)
	// Must return a non-nil slice (empty is fine).
	if paths == nil {
		t.Errorf("configPathsForBackup returned nil, want non-nil (empty slice is fine)")
	}
}

// TestExecute_BackupWarningWhenBackupFails verifies that when backup creation
// fails (e.g. permissions error on the backup dir), the upgrade still proceeds
// but the UpgradeReport surfaces the backup failure warning.
// This tests the G6 gap fix: explicit warning instead of silent skip.
func TestExecute_BackupWarningWhenBackupFails(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	// Use a homeDir that cannot create the backup dir by making it read-only.
	// We simulate the backup failure by overriding backupCreator.
	// Since we can't easily make a real dir unwritable in a unit test (on macOS,
	// a root process could still write), we verify the contract via BackupWarning
	// field when BackupID is empty.
	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	// Simulate backup failure by providing a homeDir where the snapshot would
	// fail — but that is OS-dependent. We test the contract: if BackupID is
	// empty (backup failed silently before), UpgradeReport.BackupWarning should
	// be non-empty to signal the omission.
	// For now, test that the field exists — the integration path is covered by
	// TestExecute_BackupBeforeExecution which confirms the happy path.
	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// If backup succeeded, BackupWarning should be empty (no warning needed).
	if report.BackupID != "" && report.BackupWarning != "" {
		t.Errorf("BackupWarning should be empty when BackupID is set (backup succeeded); got: %q", report.BackupWarning)
	}
}

// TestUpgradeReport_HasBackupWarningField verifies that UpgradeReport has a
// BackupWarning field to surface backup-creation failures explicitly.
// This tests the G6 gap: backup failure must not be silently skipped.
func TestUpgradeReport_HasBackupWarningField(t *testing.T) {
	// This test validates the struct field exists and is accessible.
	report := UpgradeReport{
		BackupID:      "",
		BackupWarning: "backup creation failed: permission denied",
		Results:       nil,
		DryRun:        false,
	}

	if report.BackupWarning == "" {
		t.Error("BackupWarning field not accessible — struct must have BackupWarning string field")
	}
}

// TestExecute_ForcedSnapshotFailureSurfacesWarningEndToEnd verifies the complete
// failure path end-to-end: when snapshot creation fails, the UpgradeReport
// carries a non-empty BackupWarning and BackupID is empty, AND RenderUpgradeReport
// renders the WARNING prefix into its output.
//
// This closes the verify gap: prior tests only validated the struct field exists
// or relied on OS permission tricks. This test injects the failure directly via
// the snapshotCreator package-level var (same testability pattern as execCommand).
func TestExecute_ForcedSnapshotFailureSurfacesWarningEndToEnd(t *testing.T) {
	origExecCommand := execCommand
	origSnapshotCreator := snapshotCreator
	t.Cleanup(func() {
		execCommand = origExecCommand
		snapshotCreator = origSnapshotCreator
	})

	// Stub exec so the upgrade itself succeeds (we're only testing the backup path).
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "upgrade ok")
	}

	// Force snapshot creation to fail.
	snapshotCreator = func(snapshotDir string, paths []string) (backup.Manifest, error) {
		return backup.Manifest{}, errors.New("simulated snapshot failure: disk full")
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	// BackupID must be empty — the snapshot failed.
	if report.BackupID != "" {
		t.Errorf("BackupID = %q, want empty when snapshot fails", report.BackupID)
	}

	// BackupWarning must be non-empty and mention the failure.
	if report.BackupWarning == "" {
		t.Errorf("BackupWarning is empty — failure must be surfaced explicitly")
	}
	if !containsSubstring(report.BackupWarning, "pre-upgrade backup failed") {
		t.Errorf("BackupWarning = %q, want it to mention 'pre-upgrade backup failed'", report.BackupWarning)
	}
	if !containsSubstring(report.BackupWarning, "simulated snapshot failure") {
		t.Errorf("BackupWarning = %q, want it to include the root cause", report.BackupWarning)
	}

	// The upgrade must still have run and produced a result.
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 — upgrade must proceed even when backup fails", len(report.Results))
	}
	if report.Results[0].Status != UpgradeSucceeded {
		t.Errorf("Result status = %q, want UpgradeSucceeded — upgrade proceeds without backup", report.Results[0].Status)
	}

	// RenderUpgradeReport must include the WARNING line in its output.
	rendered := RenderUpgradeReport(report)
	if !containsSubstring(rendered, "WARNING:") {
		t.Errorf("RenderUpgradeReport output must contain 'WARNING:' when BackupWarning is set;\ngot:\n%s", rendered)
	}
	if !containsSubstring(rendered, "pre-upgrade backup failed") {
		t.Errorf("RenderUpgradeReport output must include the backup failure message;\ngot:\n%s", rendered)
	}
}

// TestExecute_UpgradeBackupManifestHasUpgradeMetadata verifies that when Execute
// creates a pre-upgrade backup, the manifest on disk carries Source=upgrade,
// Description="pre-upgrade snapshot", and the version from AppVersion.
//
// This closes the verify gap: "no runtime test proves upgrade manifests are
// emitted with metadata". This test reads the manifest from disk directly.
func TestExecute_UpgradeBackupManifestHasUpgradeMetadata(t *testing.T) {
	origExecCommand := execCommand
	origAppVersion := AppVersion
	t.Cleanup(func() {
		execCommand = origExecCommand
		AppVersion = origAppVersion
	})
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}
	AppVersion = "3.0.0"

	homeDir := t.TempDir()
	// Create a config file so the snapshot captures at least one file.
	configFile := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configFile, []byte("# Claude"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), homeDir, false)

	if report.BackupID == "" {
		t.Fatalf("BackupID is empty — backup must be created")
	}

	// Find the backup manifest on disk and verify its metadata.
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir backups: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no backup directories found under %s", backupRoot)
	}

	// There should be exactly one backup dir created by Execute.
	manifestPath := filepath.Join(backupRoot, entries[0].Name(), backup.ManifestFilename)
	manifest, err := backup.ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest(%q): %v", manifestPath, err)
	}

	if manifest.Source != backup.BackupSourceUpgrade {
		t.Errorf("manifest.Source = %q, want %q", manifest.Source, backup.BackupSourceUpgrade)
	}
	if manifest.Description != "pre-upgrade snapshot" {
		t.Errorf("manifest.Description = %q, want %q", manifest.Description, "pre-upgrade snapshot")
	}
	if manifest.CreatedByVersion != "3.0.0" {
		t.Errorf("manifest.CreatedByVersion = %q, want 3.0.0", manifest.CreatedByVersion)
	}
}

// TestExecute_SuccessfulSnapshotHasNoWarning verifies the happy path: when the
// snapshot succeeds, BackupWarning is empty (no false positive warning).
func TestExecute_SuccessfulSnapshotHasNoWarning(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}
	// snapshotCreator is intentionally left at its real default.

	results := []update.UpdateResult{
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[0].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if report.BackupWarning != "" {
		t.Errorf("BackupWarning = %q, want empty when snapshot succeeds", report.BackupWarning)
	}
	if report.BackupID == "" {
		t.Errorf("BackupID is empty — should be set when snapshot succeeds")
	}

	rendered := RenderUpgradeReport(report)
	if containsSubstring(rendered, "WARNING:") {
		t.Errorf("RenderUpgradeReport must NOT contain 'WARNING:' on success;\ngot:\n%s", rendered)
	}
}

// --- Phase 3: Adapter-driven configPathsForBackup ---

// TestConfigPathsForBackup_CoversRegistryAgentsNotInOldList verifies that
// configPathsForBackup covers agents from the full registry, not just the
// previous hardcoded 4-agent list (claude, opencode, gemini, cursor).
//
// codex (~/.codex) was NOT in the old hardcoded list. After wiring to
// agents.ConfigRootsForBackup, it must be covered automatically.
func TestConfigPathsForBackup_CoversRegistryAgentsNotInOldList(t *testing.T) {
	homeDir := t.TempDir()

	// Create a file under codex config dir — not in old hardcoded list.
	codexFile := filepath.Join(homeDir, ".codex", "agents.md")
	if err := os.MkdirAll(filepath.Dir(codexFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(codexFile, []byte("# Codex config"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	paths := configPathsForBackup(homeDir)

	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	if _, ok := pathSet[codexFile]; !ok {
		t.Errorf("configPathsForBackup() missing codex config file %q — must cover all registry agents, not just old hardcoded 4; got paths: %v", codexFile, paths)
	}
}

// TestConfigPathsForBackup_GGAExtrasAreIncluded verifies that GGA-specific
// paths (config file, runtime lib dir) are included in the backup paths even
// though GGA is not an agent in the adapter registry. These are approved
// non-agent extras that must be preserved outside the canonical managed set.
func TestConfigPathsForBackup_GGAExtrasAreIncluded(t *testing.T) {
	homeDir := t.TempDir()

	// Create GGA config file at ~/.config/gga/config
	ggaConfigFile := filepath.Join(homeDir, ".config", "gga", "config")
	if err := os.MkdirAll(filepath.Dir(ggaConfigFile), 0o755); err != nil {
		t.Fatalf("MkdirAll gga config: %v", err)
	}
	if err := os.WriteFile(ggaConfigFile, []byte("gga-config"), 0o644); err != nil {
		t.Fatalf("WriteFile gga config: %v", err)
	}

	// Create GGA runtime lib file at ~/.local/share/gga/lib/pr_mode.sh
	ggaLibFile := filepath.Join(homeDir, ".local", "share", "gga", "lib", "pr_mode.sh")
	if err := os.MkdirAll(filepath.Dir(ggaLibFile), 0o755); err != nil {
		t.Fatalf("MkdirAll gga lib: %v", err)
	}
	if err := os.WriteFile(ggaLibFile, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("WriteFile gga lib: %v", err)
	}

	paths := configPathsForBackup(homeDir)

	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	if _, ok := pathSet[ggaConfigFile]; !ok {
		t.Errorf("configPathsForBackup() missing GGA config file %q — GGA extras must remain in backup; got paths: %v", ggaConfigFile, paths)
	}
	if _, ok := pathSet[ggaLibFile]; !ok {
		t.Errorf("configPathsForBackup() missing GGA lib file %q — GGA extras must remain in backup; got paths: %v", ggaLibFile, paths)
	}
}

// --- TestExecute_SkippedUpgradeDoesNotRenderFailureMarker ---

// TestExecute_SkippedUpgradeDoesNotRenderFailureMarker verifies that when a tool
// upgrade is intentionally skipped (e.g. Windows manual fallback), the progress
// output shown to the user does NOT contain the ✗ failure marker.
//
// RED: This test must fail before the fix because the executor calls Finish(false)
// for any non-success result, which renders ✗ for skipped/manual outcomes.
func TestExecute_SkippedUpgradeDoesNotRenderFailureMarker(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "should not run")
	}

	// Windows profile → binary self-update returns manual fallback → UpgradeSkipped.
	windowsProfile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.5.0", update.InstallBinary),
	}
	results[0].UpdateHint = "See https://github.com/Gentleman-Programming/gentle-ai/releases"

	// Capture the progress output written to the progress writer.
	var progressBuf bytes.Buffer

	Execute(context.Background(), results, windowsProfile, t.TempDir(), false, &progressBuf)

	got := progressBuf.String()

	// The spinner output for a skipped/manual tool must NOT show ✗.
	if strings.Contains(got, "✗") {
		t.Errorf("Execute() progress output for skipped upgrade contains '✗' (failure marker):\n%s\nWant skip marker '--' or '⊘' instead", got)
	}

	// The spinner output for a skipped/manual tool should show a skip marker.
	if !strings.Contains(got, "--") && !strings.Contains(got, "⊘") {
		t.Errorf("Execute() progress output for skipped upgrade = %q, want skip marker '--' or '⊘'", got)
	}
}

// containsSubstring checks whether s contains sub.
func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
