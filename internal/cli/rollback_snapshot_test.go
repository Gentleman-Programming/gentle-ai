package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestRollbackRestoreStepRetainsSnapshotWhenRestoreFails(t *testing.T) {
	snapshotDir := filepath.Join(t.TempDir(), "snapshot")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	restore := restoreBackup
	restoreBackup = func(backup.Manifest) error { return errors.New("restore failed") }
	t.Cleanup(func() { restoreBackup = restore })

	state := &runtimeState{
		manifest:            backup.Manifest{Entries: []backup.ManifestEntry{{}}},
		rollbackSnapshotDir: snapshotDir,
	}
	if err := (rollbackRestoreStep{state: state}).Rollback(); err == nil {
		t.Fatal("Rollback() error = nil, want restore failure")
	}
	if _, err := os.Stat(snapshotDir); err != nil {
		t.Fatalf("rollback snapshot removed after failed restore: %v", err)
	}
	if state.rollbackSnapshotDir != snapshotDir {
		t.Fatalf("rollbackSnapshotDir = %q, want %q", state.rollbackSnapshotDir, snapshotDir)
	}
}

func TestRollbackRestoreStepRemovesSnapshotAfterRestoreSucceeds(t *testing.T) {
	snapshotDir := filepath.Join(t.TempDir(), "snapshot")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	restore := restoreBackup
	restoreBackup = func(backup.Manifest) error { return nil }
	t.Cleanup(func() { restoreBackup = restore })

	state := &runtimeState{
		manifest:            backup.Manifest{Entries: []backup.ManifestEntry{{}}},
		rollbackSnapshotDir: snapshotDir,
	}
	if err := (rollbackRestoreStep{state: state}).Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
		t.Fatalf("rollback snapshot exists after successful restore: %v", err)
	}
	if state.rollbackSnapshotDir != "" {
		t.Fatalf("rollbackSnapshotDir = %q, want empty", state.rollbackSnapshotDir)
	}
}

func TestInstallRetainsSnapshotWhenRollbackFails(t *testing.T) {
	selection, resolved := rollbackSnapshotInstallPlan(t)

	t.Run("cli", func(t *testing.T) {
		home := t.TempDir()
		configureRollbackSnapshotFailure(t, home)

		if _, err := RunInstall([]string{"--agent", "opencode", "--component", "engram"}, system.DetectionResult{}); err == nil {
			t.Fatal("RunInstall() error = nil, want install and restore failures")
		}
		assertRollbackSnapshotRetained(t, home)
	})

	t.Run("tui", func(t *testing.T) {
		home := t.TempDir()
		configureRollbackSnapshotFailure(t, home)

		result := ExecuteTUIInstall(home, selection, resolved, ResolveInstallProfile(system.DetectionResult{}), nil)
		if result.Err == nil {
			t.Fatal("ExecuteTUIInstall() error = nil, want install and restore failures")
		}
		assertRollbackSnapshotRetained(t, home)
	})
}

func rollbackSnapshotInstallPlan(t *testing.T) (model.Selection, planner.ResolvedPlan) {
	t.Helper()
	flags, err := ParseInstallFlags([]string{"--agent", "opencode", "--component", "engram"})
	if err != nil {
		t.Fatalf("ParseInstallFlags() error = %v", err)
	}
	input, err := NormalizeInstallFlags(flags, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}
	resolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(input.Selection)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return input.Selection, resolved
}

func configureRollbackSnapshotFailure(t *testing.T, home string) {
	t.Helper()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	previousHome := osUserHomeDir
	previousCommand := runCommand
	previousLookPath := cmdLookPath
	previousRestore := restoreBackup
	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	runCommand = func(name string, args ...string) error {
		if name == "brew" && len(args) == 2 && args[0] == "install" && args[1] == "engram" {
			return errors.New("install failed")
		}
		return nil
	}
	restoreBackup = func(backup.Manifest) error { return errors.New("restore failed") }
	t.Cleanup(func() {
		osUserHomeDir = previousHome
		runCommand = previousCommand
		cmdLookPath = previousLookPath
		restoreBackup = previousRestore
	})
}

func assertRollbackSnapshotRetained(t *testing.T, home string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, ".gentle-ai", "backups"))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("rollback snapshots = %#v, want one retained directory", entries)
	}
}
