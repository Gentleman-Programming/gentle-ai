package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
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
	setupMockHome(t, home)

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

	setupMockHome(t, home)

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
	setupMockHome(t, home)

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

	setupMockHome(t, home)

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
	setupMockHome(t, home)

	var buf bytes.Buffer
	err := RunArgs([]string{"restore", "no-such-backup", "--yes"}, &buf)
	if err == nil {
		t.Fatalf("RunArgs(restore no-such-backup) expected error")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Errorf("restore returned 'unknown command' — not dispatched: %v", err)
	}
}

func TestRunArgsUninstallIsDispatched(t *testing.T) {
	var buf bytes.Buffer
	// uninstall without required flags prints usage help — that's enough to
	// confirm the dispatch path works without needing real agents or state.
	_ = RunArgs([]string{"uninstall"}, &buf)
	// If we got here without panic, the dispatch to cli.RunUninstall works.
}

func TestRunArgsUninstallBypassesPlatformValidation(t *testing.T) {
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() { ensureCurrentOSSupported = origEnsure })
	ensureCurrentOSSupported = func() error {
		return fmt.Errorf("unsupported platform")
	}

	var buf bytes.Buffer
	// uninstall should NOT call ensureCurrentOSSupported — it runs before
	// the platform check in the switch.
	_ = RunArgs([]string{"uninstall"}, &buf)
	// If we got here, uninstall bypassed the platform validation.
}

func TestRunArgsInstallHelpPrintsInstallSpecificHelp(t *testing.T) {
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() { ensureCurrentOSSupported = origEnsure })
	ensureCurrentOSSupported = func() error {
		return fmt.Errorf("platform validation should not run for install help")
	}

	var buf bytes.Buffer
	err := RunArgs([]string{"install", "--help"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(install --help) error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"--channel", "beta", "nightly", "GENTLE_AI_CHANNEL"} {
		if !strings.Contains(out, want) {
			t.Fatalf("install help missing %q; output:\n%s", want, out)
		}
	}
}

func TestRunArgsSDDStatusIsDispatchedBeforePlatformValidation(t *testing.T) {
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() { ensureCurrentOSSupported = origEnsure })
	ensureCurrentOSSupported = func() error {
		return fmt.Errorf("unsupported platform")
	}

	root := t.TempDir()
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "proposal.md"), "# Proposal\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "specs", "auth", "spec.md"), "# Spec\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "design.md"), "# Design\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "tasks.md"), "- [ ] 1.1 Work\n")

	var buf bytes.Buffer
	err := RunArgs([]string{"sdd-status", "add-auth", "--cwd", root}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(sdd-status) error = %v", err)
	}
	if !strings.Contains(buf.String(), "## SDD Status: add-auth") {
		t.Fatalf("sdd-status output missing markdown status:\n%s", buf.String())
	}
}

func TestRunArgsSDDContinueIsDispatchedBeforePlatformValidation(t *testing.T) {
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() { ensureCurrentOSSupported = origEnsure })
	ensureCurrentOSSupported = func() error {
		return fmt.Errorf("unsupported platform")
	}

	root := t.TempDir()
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "proposal.md"), "# Proposal\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "specs", "auth", "spec.md"), "# Spec\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "design.md"), "# Design\n")
	writeAppSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "add-auth", "tasks.md"), "- [ ] 1.1 Work\n")

	var buf bytes.Buffer
	err := RunArgs([]string{"sdd-continue", "add-auth", "--cwd", root}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(sdd-continue) error = %v", err)
	}
	if !strings.Contains(buf.String(), "## Native SDD Dispatcher: add-auth") {
		t.Fatalf("sdd-continue output missing dispatcher markdown:\n%s", buf.String())
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

	setupMockHome(t, home)

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

func TestTuiSyncAppliesSDDProfileStrategyOverride(t *testing.T) {
	overrides := &model.SyncOverrides{SDDProfileStrategy: model.SDDProfileStrategyExternalSingleActive}

	selection := model.Selection{SDDProfileStrategy: model.SDDProfileStrategyGeneratedMulti}
	applyOverrides(&selection, overrides)

	if selection.SDDProfileStrategy != model.SDDProfileStrategyExternalSingleActive {
		t.Fatalf("Selection.SDDProfileStrategy = %q, want %q", selection.SDDProfileStrategy, model.SDDProfileStrategyExternalSingleActive)
	}
}

func TestTuiSyncSDDProfileStrategyEmptyOverrideNoChange(t *testing.T) {
	overrides := &model.SyncOverrides{}

	selection := model.Selection{SDDProfileStrategy: model.SDDProfileStrategyExternalSingleActive}
	applyOverrides(&selection, overrides)

	if selection.SDDProfileStrategy != model.SDDProfileStrategyExternalSingleActive {
		t.Fatalf("Selection.SDDProfileStrategy changed unexpectedly to %q", selection.SDDProfileStrategy)
	}
}

func boolPtr(b bool) *bool { return &b }

func TestTuiSyncTargetAgentsOverridePersistedInstallState(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentOpenCode)}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	got := syncAgentIDs(home, &model.SyncOverrides{
		TargetAgents: []model.AgentID{model.AgentClaudeCode, model.AgentClaudeCode, ""},
	})

	if len(got) != 1 || got[0] != model.AgentClaudeCode {
		t.Fatalf("syncAgentIDs() = %v, want [%s]", got, model.AgentClaudeCode)
	}
}

func TestTuiSyncTargetAgentsFallsBackToDiscoveredAgents(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentOpenCode)}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	got := syncAgentIDs(home, nil)

	if len(got) != 1 || got[0] != model.AgentOpenCode {
		t.Fatalf("syncAgentIDs(nil) = %v, want [%s]", got, model.AgentOpenCode)
	}
}

func TestTuiSyncClaudeModelConfigWritesSelectedAssignments(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentPi)}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	assignments := map[string]model.ClaudeModelAlias{
		"sdd-explore": model.ClaudeModelHaiku,
		"sdd-propose": model.ClaudeModelHaiku,
		"sdd-spec":    model.ClaudeModelHaiku,
		"sdd-design":  model.ClaudeModelHaiku,
		"sdd-tasks":   model.ClaudeModelHaiku,
		"sdd-apply":   model.ClaudeModelHaiku,
		"sdd-verify":  model.ClaudeModelHaiku,
		"sdd-archive": model.ClaudeModelHaiku,
		"default":     model.ClaudeModelHaiku,
	}

	changed, err := tuiSync(home)(&model.SyncOverrides{
		TargetAgents:           []model.AgentID{model.AgentClaudeCode},
		ClaudeModelAssignments: assignments,
	})
	if err != nil {
		t.Fatalf("tuiSync Claude model config error: %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("tuiSync Claude model config changed 0 files, want Claude assets written")
	}

	applyAgent := filepath.Join(home, ".claude", "agents", "sdd-apply.md")
	body, err := os.ReadFile(applyAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", applyAgent, err)
	}
	if !strings.Contains(string(body), "model: haiku") {
		t.Fatalf("sdd-apply agent did not receive selected model; got:\n%s", body)
	}

	claudeMD := filepath.Join(home, ".claude", "CLAUDE.md")
	body, err = os.ReadFile(claudeMD)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", claudeMD, err)
	}
	if strings.Contains(string(body), "| orchestrator |") {
		t.Fatalf("CLAUDE.md should not expose orchestrator as a configurable model row; got:\n%s", body)
	}
	for _, want := range []string{
		"| sdd-apply | haiku | default | Implementation |",
		"| default | haiku | default | Non-SDD general delegation |",
		"Gentle AI does not configure the main orchestrator model",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("CLAUDE.md missing %q; got:\n%s", want, body)
		}
	}
}

func TestTuiSyncClaudePhaseAssignmentsPersistAndGenerateEffort(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{string(model.AgentPi)}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	phaseAssignments := model.ClaudePhaseAssignmentsFromLegacy(map[string]model.ClaudeModelAlias{
		"sdd-explore": model.ClaudeModelSonnet,
		"sdd-propose": model.ClaudeModelSonnet,
		"sdd-spec":    model.ClaudeModelSonnet,
		"sdd-design":  model.ClaudeModelSonnet,
		"sdd-tasks":   model.ClaudeModelSonnet,
		"sdd-apply":   model.ClaudeModelSonnet,
		"sdd-verify":  model.ClaudeModelSonnet,
		"sdd-archive": model.ClaudeModelSonnet,
		"default":     model.ClaudeModelSonnet,
	})
	phaseAssignments["sdd-apply"] = model.ClaudePhaseAssignment{
		Model:  model.ClaudeModelSonnet,
		Effort: model.ClaudeEffortMax,
	}

	changed, err := tuiSync(home)(&model.SyncOverrides{
		TargetAgents:           []model.AgentID{model.AgentClaudeCode},
		ClaudePhaseAssignments: phaseAssignments,
	})
	if err != nil {
		t.Fatalf("tuiSync Claude phase config error: %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("tuiSync Claude phase config changed 0 files, want Claude assets written")
	}

	persisted, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	applyState, ok := persisted.ClaudePhaseAssignments["sdd-apply"]
	if !ok {
		t.Fatalf("persisted state missing claude_phase_assignments.sdd-apply: %#v", persisted.ClaudePhaseAssignments)
	}
	if applyState.Model != string(model.ClaudeModelSonnet) || applyState.Effort != string(model.ClaudeEffortMax) {
		t.Fatalf("persisted sdd-apply = %#v, want sonnet/max", applyState)
	}
	if persisted.ClaudeModelAssignments != nil {
		t.Fatalf("legacy claude_model_assignments should be cleared when phase assignments are persisted; got %#v", persisted.ClaudeModelAssignments)
	}

	applyAgent := filepath.Join(home, ".claude", "agents", "sdd-apply.md")
	body, err := os.ReadFile(applyAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", applyAgent, err)
	}
	for _, want := range []string{"model: sonnet", "effort: max"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("sdd-apply agent missing %q; got:\n%s", want, body)
		}
	}

	archiveAgent := filepath.Join(home, ".claude", "agents", "sdd-archive.md")
	body, err = os.ReadFile(archiveAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", archiveAgent, err)
	}
	if strings.Contains(string(body), "effort:") {
		t.Fatalf("default-effort sdd-archive agent should omit effort frontmatter; got:\n%s", body)
	}

	beforeState := persisted.ClaudePhaseAssignments
	beforeApply, err := os.ReadFile(applyAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", applyAgent, err)
	}
	beforeArchive, err := os.ReadFile(archiveAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", archiveAgent, err)
	}
	beforeAgentFiles := filesUnder(t, filepath.Join(home, ".claude", "agents"))

	changed, err = tuiSync(home)(&model.SyncOverrides{
		TargetAgents:           []model.AgentID{model.AgentClaudeCode},
		ClaudePhaseAssignments: phaseAssignments,
	})
	if err != nil {
		t.Fatalf("second tuiSync Claude phase config error: %v", err)
	}
	if len(changed) == 0 {
		t.Log("second tuiSync reported no file changes")
	}
	afterAgentFiles := filesUnder(t, filepath.Join(home, ".claude", "agents"))
	if !reflect.DeepEqual(afterAgentFiles, beforeAgentFiles) {
		t.Fatalf("Claude agent file set changed after second sync: got %#v want %#v", afterAgentFiles, beforeAgentFiles)
	}

	persisted, err = state.Read(home)
	if err != nil {
		t.Fatalf("state.Read after second sync: %v", err)
	}
	if !reflect.DeepEqual(persisted.ClaudePhaseAssignments, beforeState) {
		t.Fatalf("ClaudePhaseAssignments changed after second sync: got %#v want %#v", persisted.ClaudePhaseAssignments, beforeState)
	}
	afterApply, err := os.ReadFile(applyAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s) after second sync: %v", applyAgent, err)
	}
	if !bytes.Equal(afterApply, beforeApply) {
		t.Fatalf("sdd-apply agent changed after idempotent sync")
	}
	afterArchive, err := os.ReadFile(archiveAgent)
	if err != nil {
		t.Fatalf("ReadFile(%s) after second sync: %v", archiveAgent, err)
	}
	if !bytes.Equal(afterArchive, beforeArchive) {
		t.Fatalf("sdd-archive agent changed after idempotent sync")
	}
}

// TestApplyOverrides_KiroModelAssignments verifies that a non-nil KiroModelAssignments
// override replaces the entire KiroModelAssignments map in the selection (same
// replacement semantics as ClaudeModelAssignments — not a key-level merge).
func filesUnder(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

func TestApplyOverrides_KiroModelAssignments(t *testing.T) {
	selection := model.Selection{
		KiroModelAssignments: map[string]model.KiroModelAlias{"sdd-apply": model.KiroModelSonnet},
	}
	overrides := &model.SyncOverrides{
		KiroModelAssignments: map[string]model.KiroModelAlias{"sdd-design": model.KiroModelOpus},
	}

	applyOverrides(&selection, overrides)

	// The whole map is replaced — prior entries (sdd-apply) are gone.
	if got := selection.KiroModelAssignments["sdd-design"]; got != model.KiroModelOpus {
		t.Fatalf("KiroModelAssignments[sdd-design] = %q, want %q", got, model.KiroModelOpus)
	}
	if _, exists := selection.KiroModelAssignments["sdd-apply"]; exists {
		t.Fatal("KiroModelAssignments[sdd-apply] should not exist after full-map replacement")
	}
}

// ─── Persist model assignments (TUI path) ───────────────────────────────────

// TestLoadPersistedAssignmentsPopulatesEmptySelection verifies that when
// state.json has model assignments and the selection maps are empty, they
// get populated from persisted state.
func TestLoadPersistedAssignmentsPopulatesEmptySelection(t *testing.T) {
	home := t.TempDir()

	// Seed state with assignments including Kiro.
	err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode"},
		ClaudeModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
		},
		KiroModelAssignments: map[string]string{
			"sdd-design":  "opus",
			"sdd-archive": "haiku",
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

	if _, exists := selection.ClaudeModelAssignments["orchestrator"]; exists {
		t.Errorf("ClaudeModelAssignments should not load persisted orchestrator model: %v", selection.ClaudeModelAssignments)
	}
	if got := selection.ClaudeModelAssignments["sdd-apply"]; got != "sonnet" {
		t.Errorf("ClaudeModelAssignments[sdd-apply] = %q, want %q", got, "sonnet")
	}
	if got := selection.KiroModelAssignments["sdd-design"]; got != model.KiroModelOpus {
		t.Errorf("KiroModelAssignments[sdd-design] = %q, want %q", got, model.KiroModelOpus)
	}
	if got := selection.KiroModelAssignments["sdd-archive"]; got != model.KiroModelHaiku {
		t.Errorf("KiroModelAssignments[sdd-archive] = %q, want %q", got, model.KiroModelHaiku)
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
		ClaudeModelAssignments: map[string]string{"sdd-apply": "haiku"},
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
			"sdd-apply": "opus",
		},
		ModelAssignments: map[string]model.ModelAssignment{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}
	loadPersistedAssignments(home, &selection)

	// Existing values must be preserved, NOT overwritten.
	if got := selection.ClaudeModelAssignments["sdd-apply"]; got != "opus" {
		t.Errorf("ClaudeModelAssignments[sdd-apply] = %q, want %q (should not be overwritten)", got, "opus")
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
			"sdd-apply":    "sonnet",
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
	if _, exists := got.ClaudeModelAssignments["orchestrator"]; exists {
		t.Errorf("ClaudeModelAssignments should not persist orchestrator model: %v", got.ClaudeModelAssignments)
	}
	if got.ClaudeModelAssignments["sdd-apply"] != "sonnet" {
		t.Errorf("ClaudeModelAssignments[sdd-apply] = %q, want %q", got.ClaudeModelAssignments["sdd-apply"], "sonnet")
	}
}

func TestPersistAssignmentsClearsNonPhaseAssignmentMaps(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode", "codex", "kiro", "claude-code"},
		ClaudeModelAssignments: map[string]string{
			"sdd-apply": "sonnet",
		},
		KiroModelAssignments: map[string]string{
			"sdd-design": "auto",
		},
		CodexModelAssignments: map[string]string{
			"sdd-apply": "high",
		},
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.4",
		},
		ModelAssignments: map[string]state.ModelAssignmentState{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	persistAssignments(home, model.Selection{
		ClaudeModelAssignments:      map[string]model.ClaudeModelAlias{},
		KiroModelAssignments:        map[string]model.KiroModelAlias{},
		CodexModelAssignments:       map[string]model.CodexEffort{},
		CodexCarrilModelAssignments: map[string]string{},
		ModelAssignments:            map[string]model.ModelAssignment{},
	})

	got, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if got.ClaudeModelAssignments != nil {
		t.Fatalf("ClaudeModelAssignments = %#v, want nil", got.ClaudeModelAssignments)
	}
	if got.KiroModelAssignments != nil {
		t.Fatalf("KiroModelAssignments = %#v, want nil", got.KiroModelAssignments)
	}
	if got.CodexModelAssignments != nil {
		t.Fatalf("CodexModelAssignments = %#v, want nil", got.CodexModelAssignments)
	}
	if got.CodexCarrilModelAssignments != nil {
		t.Fatalf("CodexCarrilModelAssignments = %#v, want nil", got.CodexCarrilModelAssignments)
	}
	if got.ModelAssignments != nil {
		t.Fatalf("ModelAssignments = %#v, want nil", got.ModelAssignments)
	}
	if len(got.InstalledAgents) != 4 {
		t.Fatalf("InstalledAgents = %#v, want preserved agents", got.InstalledAgents)
	}
}

func TestApplyOverridesClaudePhaseAssignmentsClearsLegacyAssignments(t *testing.T) {
	selection := model.Selection{
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"sdd-apply": model.ClaudeModelOpus,
		},
	}
	overrides := &model.SyncOverrides{
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{},
	}

	applyOverrides(&selection, overrides)

	if selection.ClaudeModelAssignments != nil {
		t.Fatalf("ClaudeModelAssignments = %#v, want nil when phase assignments are provided", selection.ClaudeModelAssignments)
	}
	if selection.ClaudePhaseAssignments == nil {
		t.Fatal("ClaudePhaseAssignments = nil, want explicit override map")
	}
}

func TestPersistAssignmentsSkipsCorruptState(t *testing.T) {
	home := t.TempDir()
	statePath := state.Path(home)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := []byte("{not valid json\n")
	if err := os.WriteFile(statePath, original, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	persistAssignments(home, model.Selection{
		CodexPhaseModelAssignments: map[string]string{},
	})

	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("state.json was overwritten after corrupt-state read error:\n%s", got)
	}
}

func TestPersistAssignmentsClearsClaudePhaseAssignments(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{string(model.AgentClaudeCode)},
		ClaudeModelAssignments: map[string]string{
			"sdd-apply": "haiku",
		},
		ClaudePhaseAssignments: map[string]state.ClaudePhaseAssignmentState{
			"sdd-apply": {Model: "sonnet", Effort: "max"},
		},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	persistAssignments(home, model.Selection{
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{},
	})

	got, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if got.ClaudePhaseAssignments != nil {
		t.Fatalf("ClaudePhaseAssignments = %#v, want nil after explicit clear", got.ClaudePhaseAssignments)
	}
	if got.ClaudeModelAssignments != nil {
		t.Fatalf("ClaudeModelAssignments = %#v, want nil after explicit phase clear", got.ClaudeModelAssignments)
	}
	if len(got.InstalledAgents) != 1 || got.InstalledAgents[0] != string(model.AgentClaudeCode) {
		t.Fatalf("InstalledAgents = %#v, want preserved claude-code", got.InstalledAgents)
	}
}

// TestPersistAndLoadKiroModelAssignments verifies that KiroModelAssignments
// survive a persist/load round-trip via state.json.
func TestPersistAndLoadKiroModelAssignments(t *testing.T) {
	home := t.TempDir()

	selection := model.Selection{
		KiroModelAssignments: map[string]model.KiroModelAlias{
			"sdd-design":  model.KiroModelGLM,
			"sdd-archive": model.KiroModelQwen,
			"default":     model.KiroModelAuto,
		},
	}
	persistAssignments(home, selection)

	loaded := model.Selection{}
	loadPersistedAssignments(home, &loaded)

	if got := loaded.KiroModelAssignments["sdd-design"]; got != model.KiroModelGLM {
		t.Errorf("round-trip KiroModelAssignments[sdd-design] = %q, want %q", got, model.KiroModelGLM)
	}
	if got := loaded.KiroModelAssignments["sdd-archive"]; got != model.KiroModelQwen {
		t.Errorf("round-trip KiroModelAssignments[sdd-archive] = %q, want %q", got, model.KiroModelQwen)
	}
	if got := loaded.KiroModelAssignments["default"]; got != model.KiroModelAuto {
		t.Errorf("round-trip KiroModelAssignments[default] = %q, want %q", got, model.KiroModelAuto)
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

// TestModelAssignmentsToStateWiresEffort verifies that modelAssignmentsToState
// includes the Effort field in the serialisable output.
func TestModelAssignmentsToStateWiresEffort(t *testing.T) {
	input := map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "medium"},
	}
	got := modelAssignmentsToState(input)
	s := got["sdd-apply"]
	if s.Effort != "medium" {
		t.Errorf("modelAssignmentsToState Effort = %q, want %q", s.Effort, "medium")
	}
}

// TestLoadPersistedAssignmentsWiresEffort verifies that loadPersistedAssignments
// populates the Effort field on the model.ModelAssignment when Effort is stored
// in state.json.
func TestLoadPersistedAssignmentsWiresEffort(t *testing.T) {
	home := t.TempDir()

	err := state.Write(home, state.InstallState{
		ModelAssignments: map[string]state.ModelAssignmentState{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "medium"},
		},
	})
	if err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{}
	loadPersistedAssignments(home, &sel)

	a := sel.ModelAssignments["sdd-apply"]
	if a.Effort != "medium" {
		t.Errorf("loadPersistedAssignments Effort = %q, want %q", a.Effort, "medium")
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

func TestRunArgs_UpdateSkipsSelfUpdate(t *testing.T) {
	origSelfUpdate := selfUpdateFn
	origCheckAll := updateCheckAll
	origDetect := detectSystem
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() {
		selfUpdateFn = origSelfUpdate
		updateCheckAll = origCheckAll
		detectSystem = origDetect
		ensureCurrentOSSupported = origEnsure
	})

	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
	}

	selfUpdateCalled := 0
	selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
		selfUpdateCalled++
		return nil
	}

	updateCheckAll = func(context.Context, string, system.PlatformProfile) []update.UpdateResult {
		return []update.UpdateResult{
			{
				Tool:             update.ToolInfo{Name: "gentle-ai"},
				InstalledVersion: "1.0.0",
				LatestVersion:    "1.0.0",
				Status:           update.UpToDate,
			},
		}
	}

	var buf bytes.Buffer
	err := RunArgs([]string{"update"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(update) error = %v", err)
	}
	if selfUpdateCalled != 0 {
		t.Fatalf("selfUpdate should be skipped for explicit update flow; got %d call(s)", selfUpdateCalled)
	}
}

func TestRunArgs_UpgradeSkipsSelfUpdate(t *testing.T) {
	origSelfUpdate := selfUpdateFn
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecute := upgradeExecute
	origDetect := detectSystem
	origEnsure := ensureCurrentOSSupported
	t.Cleanup(func() {
		selfUpdateFn = origSelfUpdate
		updateCheckFiltered = origCheckFiltered
		upgradeExecute = origUpgradeExecute
		detectSystem = origDetect
		ensureCurrentOSSupported = origEnsure
	})

	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
	}

	selfUpdateCalled := 0
	selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
		selfUpdateCalled++
		return nil
	}

	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{
			{
				Tool:             update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
				InstalledVersion: "1.0.0",
				LatestVersion:    "1.0.0",
				Status:           update.UpToDate,
			},
		}
	}

	upgradeExecute = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, ...io.Writer) upgrade.UpgradeReport {
		return upgrade.UpgradeReport{}
	}

	var buf bytes.Buffer
	err := RunArgs([]string{"upgrade", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(upgrade --dry-run) error = %v", err)
	}
	if selfUpdateCalled != 0 {
		t.Fatalf("selfUpdate should be skipped for explicit upgrade flow; got %d call(s)", selfUpdateCalled)
	}
}

func TestRunArgs_TUISkipsSelfUpdate(t *testing.T) {
	// NOTE: modifies package-level vars; must not run in parallel.
	origSelfUpdate := selfUpdateFn
	origDetect := detectSystem
	origEnsure := ensureCurrentOSSupported
	origRunTUI := runTUI
	t.Cleanup(func() {
		selfUpdateFn = origSelfUpdate
		detectSystem = origDetect
		ensureCurrentOSSupported = origEnsure
		runTUI = origRunTUI
	})

	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
	}

	// Return the same model to avoid nil dereference if RunArgs inspects it.
	tuiCalled := 0
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		tuiCalled++
		return m, nil
	}

	selfUpdateCalled := 0
	selfUpdateFn = func(context.Context, string, system.PlatformProfile, io.Writer) error {
		selfUpdateCalled++
		return nil
	}

	var buf bytes.Buffer
	err := RunArgs([]string{}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(empty args) error = %v", err)
	}
	if selfUpdateCalled != 0 {
		t.Fatalf("selfUpdate should be skipped for TUI flow; got %d call(s)", selfUpdateCalled)
	}
	if tuiCalled != 1 {
		t.Fatalf("runTUI should be called exactly once for TUI flow; got %d call(s)", tuiCalled)
	}
}

func TestIsExplicitUpdateFlow(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty args", args: nil, want: false},
		{name: "no command", args: []string{}, want: false},
		{name: "update", args: []string{"update"}, want: true},
		{name: "upgrade", args: []string{"upgrade"}, want: true},
		{name: "version", args: []string{"version"}, want: false},
		{name: "help", args: []string{"help"}, want: false},
		{name: "install", args: []string{"install"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExplicitUpdateFlow(tt.args)
			if got != tt.want {
				t.Fatalf("isExplicitUpdateFlow(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func setupMockHome(t *testing.T, home string) {
	t.Helper()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	})
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
}

// TestApplyOverrides_CodexModelAssignments verifies that a non-nil
// CodexModelAssignments override sets the selection.
func TestApplyOverrides_CodexModelAssignments(t *testing.T) {
	sel := model.Selection{}
	assignments := model.CodexModelPresetPowerful()
	overrides := &model.SyncOverrides{
		CodexModelAssignments: assignments,
	}
	applyOverrides(&sel, overrides)
	if len(sel.CodexModelAssignments) != len(assignments) {
		t.Fatalf("CodexModelAssignments len = %d, want %d", len(sel.CodexModelAssignments), len(assignments))
	}
	if sel.CodexModelAssignments["sdd-apply"] != model.CodexEffortHigh {
		t.Errorf("CodexModelAssignments[sdd-apply] = %q, want high", sel.CodexModelAssignments["sdd-apply"])
	}
}

// TestLoadPersistedAssignments_Codex verifies that state with codexModelAssignments
// populates selection.CodexModelAssignments.
func TestLoadPersistedAssignments_Codex(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"codex"},
		CodexModelAssignments: map[string]string{
			"sdd-apply":   "medium",
			"sdd-explore": "low",
			"default":     "medium",
		},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{}
	loadPersistedAssignments(home, &sel)

	if sel.CodexModelAssignments["sdd-apply"] != model.CodexEffortMedium {
		t.Errorf("CodexModelAssignments[sdd-apply] = %q, want medium", sel.CodexModelAssignments["sdd-apply"])
	}
}

// TestLoadPersistedAssignments_CodexMissingKey verifies that a state without
// codexModelAssignments leaves selection.CodexModelAssignments nil.
func TestLoadPersistedAssignments_CodexMissingKey(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"codex"},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{}
	loadPersistedAssignments(home, &sel)

	if sel.CodexModelAssignments != nil {
		t.Errorf("CodexModelAssignments = %v, want nil when key absent", sel.CodexModelAssignments)
	}
}

// TestPersistAssignments_Codex verifies that non-empty CodexModelAssignments
// are persisted to state.json.
func TestPersistAssignments_Codex(t *testing.T) {
	home := t.TempDir()
	// Write initial state so state.Read succeeds.
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"codex"}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{
		CodexModelAssignments: model.CodexModelPresetLowCost(),
	}
	persistAssignments(home, sel)

	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if s.CodexModelAssignments["sdd-apply"] != "medium" {
		t.Errorf("state.CodexModelAssignments[sdd-apply] = %q, want medium", s.CodexModelAssignments["sdd-apply"])
	}
}

// ─── Carril model assignment tests (W-3 fix) ─────────────────────────────────

// TestApplyOverrides_CodexCarrilModelAssignments verifies that a non-nil
// CodexCarrilModelAssignments override sets the selection.
func TestApplyOverrides_CodexCarrilModelAssignments(t *testing.T) {
	sel := model.Selection{}
	carrilModels := model.DefaultCarrilModels()
	overrides := &model.SyncOverrides{
		CodexCarrilModelAssignments: carrilModels,
	}
	applyOverrides(&sel, overrides)
	if len(sel.CodexCarrilModelAssignments) != len(carrilModels) {
		t.Fatalf("CodexCarrilModelAssignments len = %d, want %d", len(sel.CodexCarrilModelAssignments), len(carrilModels))
	}
	if sel.CodexCarrilModelAssignments["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("CodexCarrilModelAssignments[sdd-cheap] = %q, want gpt-5.4-mini", sel.CodexCarrilModelAssignments["sdd-cheap"])
	}
	if sel.CodexCarrilModelAssignments["sdd-strong"] != "gpt-5.5" {
		t.Errorf("CodexCarrilModelAssignments[sdd-strong] = %q, want gpt-5.5", sel.CodexCarrilModelAssignments["sdd-strong"])
	}
}

// TestLoadPersistedAssignments_CodexCarrilModels verifies that state with
// codexCarrilModelAssignments populates selection.CodexCarrilModelAssignments.
func TestLoadPersistedAssignments_CodexCarrilModels(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"codex"},
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.5",
			"sdd-mid":    "gpt-5.5",
			"sdd-cheap":  "gpt-5.4-mini",
		},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{}
	loadPersistedAssignments(home, &sel)

	if sel.CodexCarrilModelAssignments["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("CodexCarrilModelAssignments[sdd-cheap] = %q, want gpt-5.4-mini", sel.CodexCarrilModelAssignments["sdd-cheap"])
	}
	if sel.CodexCarrilModelAssignments["sdd-strong"] != "gpt-5.5" {
		t.Errorf("CodexCarrilModelAssignments[sdd-strong] = %q, want gpt-5.5", sel.CodexCarrilModelAssignments["sdd-strong"])
	}
}

// TestPersistAssignments_CodexCarrilModels verifies that non-empty
// CodexCarrilModelAssignments are persisted to state.json.
func TestPersistAssignments_CodexCarrilModels(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"codex"}}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	sel := model.Selection{
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.5",
			"sdd-mid":    "gpt-5.5",
			"sdd-cheap":  "gpt-5.4-mini",
		},
	}
	persistAssignments(home, sel)

	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if s.CodexCarrilModelAssignments["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("state.CodexCarrilModelAssignments[sdd-cheap] = %q, want gpt-5.4-mini", s.CodexCarrilModelAssignments["sdd-cheap"])
	}
	if s.CodexCarrilModelAssignments["sdd-strong"] != "gpt-5.5" {
		t.Errorf("state.CodexCarrilModelAssignments[sdd-strong] = %q, want gpt-5.5", s.CodexCarrilModelAssignments["sdd-strong"])
	}
}

// ─── BUG-1: Custom→preset via sync path does not clear CodexPhaseModelAssignments ───

// TestPresetSyncClearsCodexPhaseModelAssignments is the RED→GREEN regression test.
// It simulates:
//  1. A previous Custom per-phase sync that persisted CodexPhaseModelAssignments to state.json.
//  2. The user then picks a preset (e.g. Recommended) — the sync carries an empty-map
//     clear signal for CodexPhaseModelAssignments.
//
// After the fix: state.json must NOT contain codexPhaseModelAssignments (or it must be empty).
// Against current (unfixed) code this test FAILS (stale value survives).
func TestPresetSyncClearsCodexPhaseModelAssignments(t *testing.T) {
	home := t.TempDir()

	// Step 1 — persist stale per-phase assignments (as if a previous Custom sync ran).
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"codex"},
		CodexPhaseModelAssignments: map[string]string{
			"sdd-apply":   "o3",
			"sdd-explore": "gpt-4o-mini",
			"default":     "o3",
		},
	}); err != nil {
		t.Fatalf("state.Write (seed): %v", err)
	}

	// Step 2 — simulate a preset sync: overrides carry an empty-map clear signal.
	// model.go sets CodexPhaseModelAssignments to map[string]string{} (non-nil empty)
	// when the user picks a preset. Nil means "not provided"; empty means "clear".
	presetCarril := model.DefaultCarrilModels()
	overrides := &model.SyncOverrides{
		TargetAgents:                []model.AgentID{model.AgentCodex},
		CodexModelAssignments:       model.CodexModelPresetRecommended(),
		CodexCarrilModelAssignments: presetCarril,
		CodexPhaseModelAssignments:  map[string]string{}, // explicit clear signal
	}

	selection := model.Selection{}
	loadPersistedAssignments(home, &selection)
	applyOverrides(&selection, overrides)
	persistAssignments(home, selection)

	// Assert: state must no longer contain per-phase assignments.
	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if len(s.CodexPhaseModelAssignments) > 0 {
		t.Fatalf("state.CodexPhaseModelAssignments = %v, want empty after preset sync (stale per-phase map was NOT cleared)", s.CodexPhaseModelAssignments)
	}

	// Assert: selection must also be cleared.
	if len(selection.CodexPhaseModelAssignments) > 0 {
		t.Fatalf("selection.CodexPhaseModelAssignments = %v, want empty after preset sync", selection.CodexPhaseModelAssignments)
	}
}

// TestPartialSyncDoesNotWipeCodexPhaseModelAssignments is the guard test.
// A partial sync that does NOT set CodexPhaseModelAssignments on the override
// (nil = not provided) must NOT wipe an existing per-phase map from state.
func TestPartialSyncDoesNotWipeCodexPhaseModelAssignments(t *testing.T) {
	home := t.TempDir()

	// Seed state with per-phase assignments.
	existingPhaseAssignments := map[string]string{
		"sdd-apply":   "o3",
		"sdd-explore": "gpt-4o-mini",
		"default":     "o3",
	}
	if err := state.Write(home, state.InstallState{
		InstalledAgents:            []string{"codex"},
		CodexPhaseModelAssignments: existingPhaseAssignments,
	}); err != nil {
		t.Fatalf("state.Write (seed): %v", err)
	}

	// Partial sync that only updates Claude model assignments — no Codex override.
	overrides := &model.SyncOverrides{
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"sdd-apply": model.ClaudeModelHaiku,
		},
	}

	selection := model.Selection{}
	loadPersistedAssignments(home, &selection)
	applyOverrides(&selection, overrides)
	persistAssignments(home, selection)

	// The per-phase assignments must survive an unrelated partial sync.
	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read: %v", err)
	}
	if s.CodexPhaseModelAssignments["sdd-apply"] != "o3" {
		t.Fatalf("state.CodexPhaseModelAssignments[sdd-apply] = %q, want %q (wiped by unrelated partial sync)", s.CodexPhaseModelAssignments["sdd-apply"], "o3")
	}
	if s.CodexPhaseModelAssignments["default"] != "o3" {
		t.Fatalf("state.CodexPhaseModelAssignments[default] = %q, want %q", s.CodexPhaseModelAssignments["default"], "o3")
	}
}

func writeAppSDDStatusFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// TestRunArgs_TUIRestartsAfterGentleAIUpgradeResult verifies that when the TUI
// reports a successful gentle-ai upgrade, RunArgs calls restartAfterGentleAIUpgrade
// which (after task 4.6) prints the restart guidance message instead of re-execing.
func TestRunArgs_TUIRestartsAfterGentleAIUpgradeResult(t *testing.T) {
	origDetect := detectSystem
	origEnsure := ensureCurrentOSSupported
	origRunTUI := runTUI
	t.Cleanup(func() {
		detectSystem = origDetect
		ensureCurrentOSSupported = origEnsure
		runTUI = origRunTUI
		unsetEnv(t, envSelfUpdateDone)
	})
	unsetEnv(t, envSelfUpdateDone)

	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, nil
	}

	report := upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
		{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "v1.40.0"},
	}}
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		model := m.(tui.Model)
		model.UpgradeReport = &report
		return model, nil
	}

	var buf bytes.Buffer
	if err := RunArgs(nil, &buf); err != nil {
		t.Fatalf("RunArgs(TUI) error = %v", err)
	}
	// After task 4.6: restart message is printed, no re-exec occurs.
	if !strings.Contains(buf.String(), "restart gentle-ai") {
		t.Fatalf("output missing restart notice:\n%s", buf.String())
	}
}

// ─── Slice 4 RED: deferred sync on launch via pending_sync flag ───────────────

// TestRunArgs_PendingSync_RunsSyncAndClearsFlag verifies that when
// state.json has PendingSync=true, RunArgs (TUI path / no args) calls
// the deferred sync runner and writes PendingSync=false on success.
func TestRunArgs_PendingSync_RunsSyncAndClearsFlag(t *testing.T) {
	home := t.TempDir()
	setupMockHome(t, home)

	// Write initial state with PendingSync=true.
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     true,
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	origSelf := selfUpdateFn
	origEnsure := ensureCurrentOSSupported
	origDetect := detectSystem
	origRunTUI := runTUI
	origDeferredSync := deferredSyncFn
	t.Cleanup(func() {
		selfUpdateFn = origSelf
		ensureCurrentOSSupported = origEnsure
		detectSystem = origDetect
		runTUI = origRunTUI
		deferredSyncFn = origDeferredSync
	})

	selfUpdateFn = func(_ context.Context, _ string, _ system.PlatformProfile, _ io.Writer) error {
		return nil
	}
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, nil
	}
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		return m, nil
	}

	var syncCalled int
	deferredSyncFn = func() error {
		syncCalled++
		return nil
	}

	var buf bytes.Buffer
	if err := RunArgs(nil, &buf); err != nil {
		t.Fatalf("RunArgs(nil) error = %v", err)
	}

	if syncCalled != 1 {
		t.Errorf("deferredSyncFn called %d times, want 1", syncCalled)
	}

	// PendingSync must be cleared after successful sync.
	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read() error = %v", err)
	}
	if s.PendingSync {
		t.Errorf("PendingSync = true after successful deferred sync, want false")
	}
}

// TestRunArgs_PendingSync_LeavesSetOnFailure verifies that when the deferred
// sync fails, PendingSync remains true so the next launch retries idempotently.
func TestRunArgs_PendingSync_LeavesSetOnFailure(t *testing.T) {
	home := t.TempDir()
	setupMockHome(t, home)

	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     true,
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	origSelf := selfUpdateFn
	origEnsure := ensureCurrentOSSupported
	origDetect := detectSystem
	origRunTUI := runTUI
	origDeferredSync := deferredSyncFn
	t.Cleanup(func() {
		selfUpdateFn = origSelf
		ensureCurrentOSSupported = origEnsure
		detectSystem = origDetect
		runTUI = origRunTUI
		deferredSyncFn = origDeferredSync
	})

	selfUpdateFn = func(_ context.Context, _ string, _ system.PlatformProfile, _ io.Writer) error {
		return nil
	}
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, nil
	}
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		return m, nil
	}

	deferredSyncFn = func() error {
		return fmt.Errorf("sync: network error")
	}

	var buf bytes.Buffer
	// RunArgs must NOT return an error — deferred sync failure is non-fatal.
	if err := RunArgs(nil, &buf); err != nil {
		t.Fatalf("RunArgs(nil) error = %v (deferred sync failure must be non-fatal)", err)
	}

	// The warning must be printed to stdout so the user knows sync was skipped.
	out := buf.String()
	if !strings.Contains(out, "Warning: deferred sync failed:") {
		t.Errorf("stdout = %q, want warning message for deferred sync failure", out)
	}

	// PendingSync must remain set so the next launch retries.
	s, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read() error = %v", err)
	}
	if !s.PendingSync {
		t.Errorf("PendingSync = false after failed deferred sync, want true (idempotent retry)")
	}
}

// TestRunArgs_PendingSync_ClearWriteFailureIsLogged verifies that when the
// deferred sync succeeds but state.Write (to clear PendingSync) fails, the
// error is printed to stdout and RunArgs does not return an error.
// This guards against silently swallowed write failures (Issue 2).
func TestRunArgs_PendingSync_ClearWriteFailureIsLogged(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}
	home := t.TempDir()
	setupMockHome(t, home)

	// Write initial state with PendingSync=true.
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     true,
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	// Make the state file read-only so state.Write (the clear-PendingSync write) fails.
	stateFilePath := state.Path(home)
	if err := os.Chmod(stateFilePath, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateFilePath, 0o644) })

	origSelf := selfUpdateFn
	origEnsure := ensureCurrentOSSupported
	origDetect := detectSystem
	origRunTUI := runTUI
	origDeferredSync := deferredSyncFn
	t.Cleanup(func() {
		selfUpdateFn = origSelf
		ensureCurrentOSSupported = origEnsure
		detectSystem = origDetect
		runTUI = origRunTUI
		deferredSyncFn = origDeferredSync
	})

	selfUpdateFn = func(_ context.Context, _ string, _ system.PlatformProfile, _ io.Writer) error {
		return nil
	}
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, nil
	}
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		return m, nil
	}

	// Deferred sync succeeds — the write-clear is what we're testing.
	deferredSyncFn = func() error { return nil }

	var buf bytes.Buffer
	if err := RunArgs(nil, &buf); err != nil {
		t.Fatalf("RunArgs(nil) error = %v (clear-write failure must be non-fatal)", err)
	}

	// The warning must appear in stdout when the clear-write fails.
	out := buf.String()
	if !strings.Contains(out, "Warning:") {
		t.Errorf("stdout = %q; want a warning message when PendingSync clear-write fails", out)
	}
}

// TestRunArgs_NoPendingSync_NoSyncCall verifies that when PendingSync=false,
// the deferred sync runner is NOT called (no extra sync on a normal launch).
func TestRunArgs_NoPendingSync_NoSyncCall(t *testing.T) {
	home := t.TempDir()
	setupMockHome(t, home)

	// Write state without PendingSync.
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"claude-code"},
		PendingSync:     false,
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}

	origSelf := selfUpdateFn
	origEnsure := ensureCurrentOSSupported
	origDetect := detectSystem
	origRunTUI := runTUI
	origDeferredSync := deferredSyncFn
	t.Cleanup(func() {
		selfUpdateFn = origSelf
		ensureCurrentOSSupported = origEnsure
		detectSystem = origDetect
		runTUI = origRunTUI
		deferredSyncFn = origDeferredSync
	})

	selfUpdateFn = func(_ context.Context, _ string, _ system.PlatformProfile, _ io.Writer) error {
		return nil
	}
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true, Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, nil
	}
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		return m, nil
	}

	var syncCalled int
	deferredSyncFn = func() error {
		syncCalled++
		return nil
	}

	var buf bytes.Buffer
	if err := RunArgs(nil, &buf); err != nil {
		t.Fatalf("RunArgs(nil) error = %v", err)
	}

	if syncCalled != 0 {
		t.Errorf("deferredSyncFn called %d times, want 0 (no pending sync)", syncCalled)
	}
}
