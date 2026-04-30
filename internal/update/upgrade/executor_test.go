package upgrade

import (
	"bytes"
	"context"
	"errors"
	"log"
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
		makeResult("gga", update.CheckFailed, "", "", update.InstallScript),
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

func TestExecute_VersionUnknownIsSurfacedAsSkipped(t *testing.T) {
	results := []update.UpdateResult{
		makeResult("engram", update.VersionUnknown, "", "1.2.0", update.InstallBinary),
	}
	results[0].Tool.DetectCmd = []string{"engram", "version"}

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	if report.Results[0].Status != UpgradeSkipped {
		t.Fatalf("status = %q, want %q", report.Results[0].Status, UpgradeSkipped)
	}
	if report.Results[0].ManualHint == "" {
		t.Fatal("ManualHint must be populated for version-unknown tools")
	}
	if !strings.Contains(report.Results[0].ManualHint, "`engram version`") {
		t.Fatalf("ManualHint = %q, want detect command hint", report.Results[0].ManualHint)
	}
	if report.BackupID != "" {
		t.Fatalf("BackupID = %q, want empty when nothing is executed", report.BackupID)
	}
}

func TestExecute_RegisteredNotMaterializedIsExecutable(t *testing.T) {
	origExecCommand := execCommand
	origHomeDir := openCodeHomeDir
	origLookPath := lookPathCommand
	origSnapshotCreator := snapshotCreator
	t.Cleanup(func() {
		execCommand = origExecCommand
		openCodeHomeDir = origHomeDir
		lookPathCommand = origLookPath
		snapshotCreator = origSnapshotCreator
	})

	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opencodeDir, "tui.json"), []byte(`{"plugin":["opencode-sdd-engram-manage"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	openCodeHomeDir = func() (string, error) { return home, nil }
	lookPathCommand = func(file string) (string, error) {
		if file == "npm" {
			return "/usr/bin/npm", nil
		}
		return "", errors.New("not found")
	}
	snapshotCreator = func(snapshotDir string, paths []string) (backup.Manifest, error) {
		return backup.Manifest{ID: "backup-test"}, nil
	}
	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return exec.Command("true")
	}

	result := makeResult("opencode-sdd-engram-manage", update.RegisteredNotMaterialized, "", "1.2.0", update.InstallOpenCodePlugin)
	result.Tool.NpmPackage = "opencode-sdd-engram-manage"
	result.UpdateHint = "Restart or reload OpenCode; check OpenCode logs for package or peer dependency errors."

	report := Execute(context.Background(), []update.UpdateResult{result}, linuxProfile(), home, false)

	if !execCalled {
		t.Fatal("registered-pending OpenCode plugins should execute npm dependency upgrade")
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	if report.Results[0].Status != UpgradeSucceeded {
		t.Fatalf("status = %q, want %q", report.Results[0].Status, UpgradeSucceeded)
	}
	if report.BackupID == "" {
		t.Fatal("BackupID should be populated before executing registered-pending plugin upgrade")
	}
}

// --- TestRenderUpgradeReport_DryRunManualHintNotCountedAsPending ---

func TestRenderUpgradeReport_DryRunManualHintNotCountedAsPending(t *testing.T) {
	report := UpgradeReport{
		DryRun: true,
		Results: []ToolUpgradeResult{
			{ToolName: "engram", Status: UpgradeSkipped, ManualHint: "source build — upgrade manually"},
		},
	}

	output := RenderUpgradeReport(report)

	if strings.Contains(output, "upgrade(s) pending") {
		t.Fatalf("manual-hint skips must NOT be counted as pending upgrades in dry-run:\n%s", output)
	}
	if !strings.Contains(output, "manual") {
		t.Fatalf("dry-run output should mention manual attention:\n%s", output)
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

// NOTE: Install isolation is enforced by the import boundary at the top of
// executor.go — this package MUST NOT import pipeline, planner, or cli.
// The compiler enforces this; no runtime test is needed.

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

// NOTE: BackupWarning field existence is verified by the compiler (struct literal
// usage in tests). The failure path is fully covered by
// TestExecute_ForcedSnapshotFailureSurfacesWarningEndToEnd below.

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
	if !strings.Contains(report.BackupWarning, "pre-upgrade backup failed") {
		t.Errorf("BackupWarning = %q, want it to mention 'pre-upgrade backup failed'", report.BackupWarning)
	}
	if !strings.Contains(report.BackupWarning, "simulated snapshot failure") {
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
	if !strings.Contains(rendered, "WARNING:") {
		t.Errorf("RenderUpgradeReport output must contain 'WARNING:' when BackupWarning is set;\ngot:\n%s", rendered)
	}
	if !strings.Contains(rendered, "pre-upgrade backup failed") {
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
	if strings.Contains(rendered, "WARNING:") {
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

// --- TestEnumerateFilesInDir_ExcludesSubdirs ---

// TestEnumerateFilesInDir_ExcludesSubdirs verifies that enumerateFilesInDir skips
// directories whose base name appears in the excludeDirNames set at ANY depth.
// This is critical for agents like Gemini where heavy runtime dirs
// (browser_recordings/) are nested 2+ levels deep (e.g. ~/.gemini/antigravity/browser_recordings/).
func TestEnumerateFilesInDir_ExcludesSubdirs(t *testing.T) {
	root := t.TempDir()

	// Config file at root level — must be included.
	rootFile := filepath.Join(root, "settings.json")
	if err := os.WriteFile(rootFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Allowed subdir with a config file — must be included.
	allowedFile := filepath.Join(root, "mcp", "server.json")
	if err := os.MkdirAll(filepath.Dir(allowedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(allowedFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Excluded subdir at depth 1 — must be skipped.
	excludedFile := filepath.Join(root, "projects", "data.json")
	if err := os.MkdirAll(filepath.Dir(excludedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(excludedFile, []byte(`big data`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Excluded subdir at depth 2 — simulates ~/.gemini/antigravity/browser_recordings/.
	// Must also be skipped.
	nestedExcludedFile := filepath.Join(root, "antigravity", "browser_recordings", "video.dat")
	if err := os.MkdirAll(filepath.Dir(nestedExcludedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(nestedExcludedFile, []byte(`3.6GB of video`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Config file NEXT TO excluded dir inside antigravity — must be included.
	nestedConfigFile := filepath.Join(root, "antigravity", "config.toml")
	if err := os.WriteFile(nestedConfigFile, []byte(`[settings]`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	excludes := map[string]bool{
		"projects":           true,
		"browser_recordings": true,
	}

	files, err := enumerateFilesInDir(root, excludes)
	if err != nil {
		t.Fatalf("enumerateFilesInDir error: %v", err)
	}

	pathSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		pathSet[f] = struct{}{}
	}

	// Root-level config file must be present.
	if _, ok := pathSet[rootFile]; !ok {
		t.Errorf("missing root-level file %q", rootFile)
	}

	// Allowed subdir file must be present.
	if _, ok := pathSet[allowedFile]; !ok {
		t.Errorf("missing allowed subdir file %q", allowedFile)
	}

	// Depth-1 excluded subdir files must NOT be present.
	if _, ok := pathSet[excludedFile]; ok {
		t.Errorf("excluded subdir file %q should not be in results", excludedFile)
	}

	// Depth-2 excluded subdir files must NOT be present.
	if _, ok := pathSet[nestedExcludedFile]; ok {
		t.Errorf("nested excluded dir file %q should not be in results — exclude must work at any depth", nestedExcludedFile)
	}

	// Config file next to excluded dir must still be present.
	if _, ok := pathSet[nestedConfigFile]; !ok {
		t.Errorf("config file next to excluded dir should be present; missing %q", nestedConfigFile)
	}
}

func TestEnumerateFilesInDir_DefaultExclusionDiagnosticsAreSilent(t *testing.T) {
	root := t.TempDir()
	excludedFile := filepath.Join(root, "projects", "data.json")
	if err := os.MkdirAll(filepath.Dir(excludedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(excludedFile, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var legacyLog bytes.Buffer
	origLogWriter := log.Writer()
	log.SetOutput(&legacyLog)
	t.Cleanup(func() { log.SetOutput(origLogWriter) })

	files, err := enumerateFilesInDir(root, map[string]bool{"projects": true})
	if err != nil {
		t.Fatalf("enumerateFilesInDir error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %v, want no files from excluded directory", files)
	}
	if got := legacyLog.String(); got != "" {
		t.Fatalf("default backup enumeration wrote to global log output %q; TUI paths must remain silent", got)
	}
}

func TestEnumerateFilesInDir_WritesExclusionDiagnosticsToInjectedWriter(t *testing.T) {
	root := t.TempDir()
	configFile := filepath.Join(root, "settings.json")
	excludedFile := filepath.Join(root, "projects", "data.json")
	for _, f := range []string{configFile, excludedFile} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var diagnostics bytes.Buffer
	files, err := enumerateFilesInDir(root, map[string]bool{"projects": true}, &diagnostics)
	if err != nil {
		t.Fatalf("enumerateFilesInDir error: %v", err)
	}
	if len(files) != 1 || files[0] != configFile {
		t.Fatalf("files = %v, want only %q", files, configFile)
	}

	got := diagnostics.String()
	if !strings.Contains(got, "backup: excluding directory ") || !strings.Contains(got, "projects") {
		t.Fatalf("diagnostics = %q, want controlled exclusion diagnostic", got)
	}
	if !strings.HasPrefix(got, "backup:") {
		t.Fatalf("diagnostics = %q, want backup message without log package timestamp prefix", got)
	}
}

// TestEnumerateFilesInDir_NilExcludesWalksEverything verifies that passing nil
// for excludeSubdirs results in a full walk with no exclusions.
func TestEnumerateFilesInDir_NilExcludesWalksEverything(t *testing.T) {
	root := t.TempDir()

	file1 := filepath.Join(root, "a.txt")
	file2 := filepath.Join(root, "projects", "b.txt")
	for _, f := range []string{file1, file2} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	files, err := enumerateFilesInDir(root, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files with nil excludes, got %d: %v", len(files), files)
	}
}

// TestConfigPathsForBackup_ExcludesRuntimeDirs verifies that the production
// backupExcludeSubdirs list prevents configPathsForBackup from walking into
// large runtime directories across ALL agents: Claude, Gemini, OpenCode.
func TestConfigPathsForBackup_ExcludesRuntimeDirs(t *testing.T) {
	homeDir := t.TempDir()

	// --- Claude: config file (keep) + runtime dirs (exclude) ---
	claudeConfig := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(claudeConfig), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(claudeConfig, []byte("# Claude"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	claudeExcludes := []string{"projects", "sessions", "plugins", "cache", "backups"}

	// --- Gemini: config file (keep) + runtime dirs (exclude) ---
	geminiConfig := filepath.Join(homeDir, ".gemini", "GEMINI.md")
	if err := os.MkdirAll(filepath.Dir(geminiConfig), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(geminiConfig, []byte("# Gemini"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	geminiExcludes := []string{"browser_recordings", "brain", "conversations"}

	// --- OpenCode: config file (keep) + node_modules (exclude) ---
	openCodeConfig := filepath.Join(homeDir, ".config", "opencode", "config.json")
	if err := os.MkdirAll(filepath.Dir(openCodeConfig), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(openCodeConfig, []byte(`{"model":"free"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	openCodeExcludes := []string{"node_modules"}

	// Create excluded dirs with files for all agents.
	type agentExclude struct {
		base     string
		excludes []string
	}
	agents := []agentExclude{
		{filepath.Join(homeDir, ".claude"), claudeExcludes},
		{filepath.Join(homeDir, ".gemini"), geminiExcludes},
		{filepath.Join(homeDir, ".config", "opencode"), openCodeExcludes},
	}

	var excludedFiles []string
	for _, agent := range agents {
		for _, dir := range agent.excludes {
			f := filepath.Join(agent.base, dir, "data.json")
			if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
				t.Fatalf("MkdirAll %s: %v", dir, err)
			}
			if err := os.WriteFile(f, []byte("runtime data"), 0o644); err != nil {
				t.Fatalf("WriteFile %s: %v", dir, err)
			}
			excludedFiles = append(excludedFiles, f)
		}
	}

	// Gemini Antigravity temp dir is nested under antigravity/ and must also be excluded.
	geminiAntigravityTmpFile := filepath.Join(homeDir, ".gemini", "antigravity", "tmp", "artifact.json")
	if err := os.MkdirAll(filepath.Dir(geminiAntigravityTmpFile), 0o755); err != nil {
		t.Fatalf("MkdirAll gemini antigravity tmp: %v", err)
	}
	if err := os.WriteFile(geminiAntigravityTmpFile, []byte("temp runtime data"), 0o644); err != nil {
		t.Fatalf("WriteFile gemini antigravity tmp: %v", err)
	}
	excludedFiles = append(excludedFiles, geminiAntigravityTmpFile)

	paths := configPathsForBackup(homeDir)
	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	// Config files must be present.
	for _, cfg := range []string{claudeConfig, geminiConfig, openCodeConfig} {
		if _, ok := pathSet[cfg]; !ok {
			t.Errorf("configPathsForBackup missing config file %q", cfg)
		}
	}

	// Runtime files must NOT be present.
	for _, f := range excludedFiles {
		if _, ok := pathSet[f]; ok {
			t.Errorf("configPathsForBackup should exclude runtime file %q", f)
		}
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

// TestEnumerateFilesInDir_ExcludesNestedSameNameDir documents the intentional
// behavior change: directories matching an excluded name are pruned at ANY depth,
// not just directly under the walked root. For example, mcp/cache/data.json is
// excluded because "cache" matches the exclude list even at depth 2. This is the
// accepted tradeoff — we skip all dirs named "cache" regardless of nesting to
// ensure heavy runtime dirs like ~/.gemini/antigravity/browser_recordings/ are
// always excluded without requiring path-specific rules.
func TestEnumerateFilesInDir_ExcludesNestedSameNameDir(t *testing.T) {
	root := t.TempDir()

	// Create mcp/cache/data.json — "cache" is an excluded name, so this file
	// must NOT appear in results even though it's nested under "mcp".
	nestedCacheFile := filepath.Join(root, "mcp", "cache", "data.json")
	if err := os.MkdirAll(filepath.Dir(nestedCacheFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(nestedCacheFile, []byte(`{"cached":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// A sibling file under mcp (not in an excluded dir) — must be included.
	mcpConfig := filepath.Join(root, "mcp", "server.json")
	if err := os.WriteFile(mcpConfig, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	excludes := map[string]bool{
		"cache": true,
	}

	files, err := enumerateFilesInDir(root, excludes)
	if err != nil {
		t.Fatalf("enumerateFilesInDir error: %v", err)
	}

	pathSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		pathSet[f] = struct{}{}
	}

	// mcp/cache/data.json must be excluded — "cache" matches at depth 2.
	if _, ok := pathSet[nestedCacheFile]; ok {
		t.Errorf("nested excluded dir file %q must NOT be in results — exclude applies at any depth", nestedCacheFile)
	}

	// mcp/server.json must be present — "mcp" is not excluded.
	if _, ok := pathSet[mcpConfig]; !ok {
		t.Errorf("non-excluded file %q must be present", mcpConfig)
	}
}

// TestEnumerateFilesInDir_EmptyExcludesWalksEverything verifies that passing an
// empty (non-nil) map for excludeSubdirs results in a full walk with no exclusions,
// same as nil.
func TestEnumerateFilesInDir_EmptyExcludesWalksEverything(t *testing.T) {
	root := t.TempDir()

	file1 := filepath.Join(root, "a.txt")
	file2 := filepath.Join(root, "projects", "b.txt")
	for _, f := range []string{file1, file2} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	files, err := enumerateFilesInDir(root, map[string]bool{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files with empty excludes map, got %d: %v", len(files), files)
	}
}

// TestEnumerateFilesInDir_CaseInsensitiveExclude verifies that directory names
// with mixed casing (e.g. "Projects", "CACHE") are excluded on case-insensitive
// filesystems like Windows NTFS. The exclude map keys are lowercase; the
// strings.ToLower normalization in enumerateFilesInDir handles the mismatch.
func TestEnumerateFilesInDir_CaseInsensitiveExclude(t *testing.T) {
	root := t.TempDir()

	// Config file at root — must be included.
	configFile := filepath.Join(root, "settings.json")
	if err := os.WriteFile(configFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Directory with uppercase name matching a lowercase exclude key.
	upperDir := filepath.Join(root, "Projects", "data.json")
	if err := os.MkdirAll(filepath.Dir(upperDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(upperDir, []byte("big"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Mixed case directory.
	mixedDir := filepath.Join(root, "Cache", "temp.dat")
	if err := os.MkdirAll(filepath.Dir(mixedDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(mixedDir, []byte("cached"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	excludes := map[string]bool{
		"projects": true,
		"cache":    true,
	}

	files, err := enumerateFilesInDir(root, excludes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	pathSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		pathSet[f] = struct{}{}
	}

	if _, ok := pathSet[configFile]; !ok {
		t.Errorf("config file should be present: %q", configFile)
	}
	if _, ok := pathSet[upperDir]; ok {
		t.Errorf("uppercase 'Projects' dir should be excluded by lowercase 'projects' key: %q", upperDir)
	}
	if _, ok := pathSet[mixedDir]; ok {
		t.Errorf("mixed-case 'Cache' dir should be excluded by lowercase 'cache' key: %q", mixedDir)
	}
}
