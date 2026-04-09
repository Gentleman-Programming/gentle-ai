package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
)

// TestListBackupsNewestFirst verifies that ListBackups returns manifests sorted
// newest-first by CreatedAt timestamp, matching the spec "newest first" ordering.
func TestListBackupsNewestFirst(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	older := backup.Manifest{
		ID:        "older",
		CreatedAt: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC),
		RootDir:   filepath.Join(backupRoot, "older"),
		Entries:   []backup.ManifestEntry{},
	}
	newer := backup.Manifest{
		ID:        "newer",
		CreatedAt: time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
		RootDir:   filepath.Join(backupRoot, "newer"),
		Entries:   []backup.ManifestEntry{},
	}

	// Write older backup first.
	for _, m := range []backup.Manifest{older, newer} {
		dir := filepath.Join(backupRoot, m.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := backup.WriteManifest(filepath.Join(dir, backup.ManifestFilename), m); err != nil {
			t.Fatalf("WriteManifest: %v", err)
		}
	}

	// Temporarily override home dir resolution for ListBackups.
	origHomeDir := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHomeDir) })
	os.Setenv("HOME", home)

	manifests := ListBackups()

	if len(manifests) != 2 {
		t.Fatalf("ListBackups() returned %d manifests, want 2", len(manifests))
	}

	// Newest must be first.
	if manifests[0].ID != "newer" {
		t.Errorf("ListBackups()[0].ID = %q, want %q (newest first)", manifests[0].ID, "newer")
	}
	if manifests[1].ID != "older" {
		t.Errorf("ListBackups()[1].ID = %q, want %q", manifests[1].ID, "older")
	}
}

// TestListBackupsWithSourceMetadata verifies that ListBackups returns manifests
// with Source metadata intact, so display labels can use the source field.
func TestListBackupsWithSourceMetadata(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	m := backup.Manifest{
		ID:          "test-with-source",
		CreatedAt:   time.Now().UTC(),
		RootDir:     filepath.Join(backupRoot, "test-with-source"),
		Source:      backup.BackupSourceInstall,
		Description: "pre-install snapshot",
		Entries:     []backup.ManifestEntry{},
	}

	dir := filepath.Join(backupRoot, m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := backup.WriteManifest(filepath.Join(dir, backup.ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", home)

	manifests := ListBackups()

	if len(manifests) != 1 {
		t.Fatalf("ListBackups() returned %d manifests, want 1", len(manifests))
	}

	got := manifests[0]
	if got.Source != backup.BackupSourceInstall {
		t.Errorf("Source = %q, want %q", got.Source, backup.BackupSourceInstall)
	}
	if got.Description != "pre-install snapshot" {
		t.Errorf("Description = %q, want %q", got.Description, "pre-install snapshot")
	}
}

// TestRunArgsRestoreListIsDispatched verifies that `gentle-ai restore --list`
// is correctly dispatched through RunArgs and produces a meaningful response
// (either a backup list or a "no backups" message — never "unknown command").
func TestRunArgsRestoreListIsDispatched(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", home)

	var buf bytes.Buffer
	err := RunArgs([]string{"restore", "--list"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(restore --list) error = %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatalf("restore --list produced no output")
	}

	// Must not produce "unknown command".
	if strings.Contains(out, "unknown command") {
		t.Errorf("restore is not registered in RunArgs; got: %s", out)
	}
}

// TestRunArgsRestoreByIDWithYes verifies end-to-end wiring of `restore <id> --yes`
// through app.RunArgs.
func TestRunArgsRestoreByIDWithYes(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	// Create a backup with a real file entry so restore can succeed.
	sourceFile := filepath.Join(home, "config.md")
	if err := os.WriteFile(sourceFile, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	snapshotDir := filepath.Join(backupRoot, "test-backup-001")
	snapshotFile := filepath.Join(snapshotDir, "files", "config.md")
	if err := os.MkdirAll(filepath.Dir(snapshotFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(snapshotFile, []byte("backup-content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile snapshot: %v", err)
	}

	m := backup.Manifest{
		ID:        "test-backup-001",
		CreatedAt: time.Now().UTC(),
		RootDir:   snapshotDir,
		Source:    backup.BackupSourceInstall,
		Entries: []backup.ManifestEntry{
			{OriginalPath: sourceFile, SnapshotPath: snapshotFile, Existed: true, Mode: 0o644},
		},
	}
	if err := backup.WriteManifest(filepath.Join(snapshotDir, backup.ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", home)

	var buf bytes.Buffer
	err := RunArgs([]string{"restore", "test-backup-001", "--yes"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(restore test-backup-001 --yes) error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(strings.ToLower(out), "restor") {
		t.Errorf("restore output should confirm restoration; got:\n%s", out)
	}
}

// TestRunArgsRestoreUnknownIDReturnsError verifies that an unknown backup ID
// is surfaced as an error from RunArgs.
func TestRunArgsRestoreUnknownIDReturnsError(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", home)

	var buf bytes.Buffer
	err := RunArgs([]string{"restore", "no-such-backup", "--yes"}, &buf)
	if err == nil {
		t.Fatalf("RunArgs(restore no-such-backup) expected error")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Errorf("restore returned 'unknown command' — not dispatched: %v", err)
	}
}

// TestListBackupsFallsBackGracefullyForOldManifests verifies that old manifests
// without Source/Description are still returned (not skipped) and can be displayed
// via DisplayLabel without panicking.
func TestListBackupsFallsBackGracefullyForOldManifests(t *testing.T) {
	_ = fmt.Sprintf // Ensure fmt is used.
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	// Write a manifest with no Source/Description.
	m := backup.Manifest{
		ID:        "old-backup",
		CreatedAt: time.Now().UTC(),
		RootDir:   filepath.Join(backupRoot, "old-backup"),
		Entries:   []backup.ManifestEntry{},
		// Source and Description intentionally omitted — simulates old manifest.
	}

	dir := filepath.Join(backupRoot, m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := backup.WriteManifest(filepath.Join(dir, backup.ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", home)

	manifests := ListBackups()

	if len(manifests) != 1 {
		t.Fatalf("ListBackups() returned %d manifests, want 1", len(manifests))
	}

	// Must not panic — DisplayLabel should handle empty Source gracefully.
	label := manifests[0].DisplayLabel()
	if label == "" {
		t.Errorf("DisplayLabel() returned empty string, want non-empty fallback label")
	}
}

// ─── BUG 3: SyncOverrides.StrictTDD never read in tuiSync ───────────────────

// TestTuiSyncAppliesStrictTDDOverride verifies that applyOverrides correctly
// merges SyncOverrides.StrictTDD into the selection.
// Previously, the field was declared on SyncOverrides but never applied.
func TestTuiSyncAppliesStrictTDDOverride(t *testing.T) {
	sel := boolPtr(true)
	overrides := &model.SyncOverrides{StrictTDD: sel}

	selection := model.Selection{StrictTDD: false}
	applyOverrides(&selection, overrides)

	if !selection.StrictTDD {
		t.Fatalf("Selection.StrictTDD = false after applyOverrides with StrictTDD=true override; field is not being applied")
	}
}

// TestTuiSyncAppliesStrictTDDOverrideFalse verifies the override correctly sets
// StrictTDD to false when the pointer points to false.
func TestTuiSyncAppliesStrictTDDOverrideFalse(t *testing.T) {
	sel := boolPtr(false)
	overrides := &model.SyncOverrides{StrictTDD: sel}

	selection := model.Selection{StrictTDD: true}
	applyOverrides(&selection, overrides)

	if selection.StrictTDD {
		t.Fatalf("Selection.StrictTDD = true after applyOverrides with StrictTDD=false override")
	}
}

// TestTuiSyncStrictTDDNilOverrideNoChange verifies that when StrictTDD override
// is nil, the selection's existing value is preserved.
func TestTuiSyncStrictTDDNilOverrideNoChange(t *testing.T) {
	overrides := &model.SyncOverrides{StrictTDD: nil}

	selection := model.Selection{StrictTDD: true}
	applyOverrides(&selection, overrides)

	if !selection.StrictTDD {
		t.Fatalf("Selection.StrictTDD changed unexpectedly; nil override should not modify the field")
	}
}

func boolPtr(b bool) *bool { return &b }

// ─── Persist model assignments (TUI path) ───────────────────────────────────

// TestLoadPersistedAssignmentsPopulatesEmptySelection verifies that when
// state.json has model assignments and the selection maps are empty, they
// get populated from persisted state.
func TestLoadPersistedAssignmentsPopulatesEmptySelection(t *testing.T) {
	home := t.TempDir()

	// Seed state with assignments.
	err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode"},
		ClaudeModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
		},
		ModelAssignments: map[string]state.ModelAssignmentState{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	})
	if err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	selection := model.Selection{}
	loadPersistedAssignments(home, &selection)

	if got := selection.ClaudeModelAssignments["orchestrator"]; got != "opus" {
		t.Errorf("ClaudeModelAssignments[orchestrator] = %q, want %q", got, "opus")
	}
	if got := selection.ClaudeModelAssignments["sdd-apply"]; got != "sonnet" {
		t.Errorf("ClaudeModelAssignments[sdd-apply] = %q, want %q", got, "sonnet")
	}
	ma := selection.ModelAssignments["sdd-init"]
	if ma.ProviderID != "anthropic" || ma.ModelID != "claude-sonnet-4" {
		t.Errorf("ModelAssignments[sdd-init] = %+v, want anthropic/claude-sonnet-4", ma)
	}
}

// TestLoadPersistedAssignmentsDoesNotOverrideExisting verifies that when the
// selection already has assignments (e.g. from TUI overrides), persisted
// state does NOT clobber them.
func TestLoadPersistedAssignmentsDoesNotOverrideExisting(t *testing.T) {
	home := t.TempDir()

	// Seed state with "old" assignments.
	err := state.Write(home, state.InstallState{
		ClaudeModelAssignments: map[string]string{"orchestrator": "haiku"},
		ModelAssignments: map[string]state.ModelAssignmentState{
			"sdd-init": {ProviderID: "google", ModelID: "gemini-pro"},
		},
	})
	if err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	// Selection already has assignments from the TUI configure flow.
	selection := model.Selection{
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"orchestrator": "opus",
		},
		ModelAssignments: map[string]model.ModelAssignment{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}
	loadPersistedAssignments(home, &selection)

	// Existing values must be preserved, NOT overwritten.
	if got := selection.ClaudeModelAssignments["orchestrator"]; got != "opus" {
		t.Errorf("ClaudeModelAssignments[orchestrator] = %q, want %q (should not be overwritten)", got, "opus")
	}
	ma := selection.ModelAssignments["sdd-init"]
	if ma.ProviderID != "anthropic" {
		t.Errorf("ModelAssignments[sdd-init].ProviderID = %q, want %q (should not be overwritten)", ma.ProviderID, "anthropic")
	}
}

// TestPersistAssignmentsPreservesInstalledAgents verifies the read-merge-write
// pattern: persisting assignments must NOT lose the InstalledAgents list.
func TestPersistAssignmentsPreservesInstalledAgents(t *testing.T) {
	home := t.TempDir()

	// Pre-existing state with agents.
	err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"claude-code", "opencode"},
	})
	if err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	selection := model.Selection{
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"orchestrator": "opus",
		},
	}
	persistAssignments(home, selection)

	// Read back and verify agents are still there.
	got, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if len(got.InstalledAgents) != 2 {
		t.Fatalf("InstalledAgents = %v, want [claude-code opencode]", got.InstalledAgents)
	}
	if got.ClaudeModelAssignments["orchestrator"] != "opus" {
		t.Errorf("ClaudeModelAssignments[orchestrator] = %q, want %q", got.ClaudeModelAssignments["orchestrator"], "opus")
	}
}

// TestPersistAssignmentsNoOpWhenEmpty verifies that persistAssignments does
// not write to state.json when the selection has no assignments.
func TestPersistAssignmentsNoOpWhenEmpty(t *testing.T) {
	home := t.TempDir()

	// Write initial state.
	err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode"},
	})
	if err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	statePath := filepath.Join(home, ".gentle-ai", "state.json")
	infoBefore, _ := os.Stat(statePath)

	selection := model.Selection{} // empty assignments
	persistAssignments(home, selection)

	infoAfter, _ := os.Stat(statePath)
	if infoAfter.ModTime() != infoBefore.ModTime() {
		t.Errorf("persistAssignments() modified state.json when selection had no assignments")
	}
}

// TestVersionBeforeSystemGuards verifies that `gentle-ai version` returns the
// version string without going through system detection or platform guards.
func TestVersionBeforeSystemGuards(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"version"}, &buf)
	if err != nil {
		t.Fatalf("version should not fail: %v", err)
	}
	if !strings.Contains(buf.String(), "gentle-ai") {
		t.Error("version output should contain 'gentle-ai'")
	}
}

// TestHelpCommand verifies that help, --help, and -h all print USAGE and COMMANDS
// without triggering system detection or platform guards.
func TestHelpCommand(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var buf bytes.Buffer
			err := RunArgs([]string{arg}, &buf)
			if err != nil {
				t.Fatalf("help should not fail: %v", err)
			}
			if !strings.Contains(buf.String(), "USAGE") {
				t.Errorf("help output for %q should contain USAGE", arg)
			}
			if !strings.Contains(buf.String(), "COMMANDS") {
				t.Errorf("help output for %q should contain COMMANDS", arg)
			}
		})
	}
}

// TestUnknownCommandSuggestsHelp verifies that an unrecognised command returns
// an error whose message suggests running 'gentle-ai help'.
func TestUnknownCommandSuggestsHelp(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"notacommand"}, &buf)
	if err == nil {
		t.Fatal("unknown command should return error")
	}
	if !strings.Contains(err.Error(), "gentle-ai help") {
		t.Error("unknown command error should suggest 'gentle-ai help'")
	}
}
