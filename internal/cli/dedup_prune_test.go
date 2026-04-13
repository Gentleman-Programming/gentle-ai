package cli

// dedup_prune_test.go — verifies that prepareBackupStep skips duplicate backups
// and prunes old backups after a successful snapshot (BKUP-T16, BKUP-T27).

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// TestPrepareBackupStep_SkipsDuplicateBackup verifies that when the new checksum
// matches the most recent backup, prepareBackupStep skips snapshot creation and
// does NOT create a new backup directory (BKUP-T16).
func TestPrepareBackupStep_SkipsDuplicateBackup(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll backupRoot: %v", err)
	}

	// Create a real config file to back up.
	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	// Create a first backup (baseline) using the same content.
	firstSnapshotDir := filepath.Join(backupRoot, time.Now().UTC().Format("20060102150405.000000001"))
	firstState := &runtimeState{}
	firstStep := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: firstSnapshotDir,
		targets:     []string{configPath},
		state:       firstState,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceInstall,
		description: "first snapshot",
		appVersion:  "1.0.0",
	}
	if err := firstStep.Run(); err != nil {
		t.Fatalf("first prepareBackupStep.Run() error = %v", err)
	}

	// Second run with the SAME config content — should be a duplicate.
	secondSnapshotDir := filepath.Join(backupRoot, time.Now().UTC().Format("20060102150405.000000002"))
	secondState := &runtimeState{}
	secondStep := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: secondSnapshotDir,
		targets:     []string{configPath},
		state:       secondState,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceSync,
		description: "second snapshot",
		appVersion:  "1.0.0",
	}
	if err := secondStep.Run(); err != nil {
		t.Fatalf("second prepareBackupStep.Run() error = %v", err)
	}

	// The second snapshot directory must NOT have been created.
	if _, err := os.Stat(secondSnapshotDir); !os.IsNotExist(err) {
		t.Errorf("second snapshot dir should not exist for duplicate content; err = %v", err)
	}

	// Only the first backup should exist in backupRoot.
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir backupRoot: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("backupRoot should contain 1 backup dir, got %d", len(entries))
	}
}

// TestPrepareBackupStep_ProceedsWhenContentChanged verifies that when file
// content changes, prepareBackupStep creates a new backup (not a duplicate).
func TestPrepareBackupStep_ProceedsWhenContentChanged(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll backupRoot: %v", err)
	}

	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version": 1}`), 0o644); err != nil {
		t.Fatalf("WriteFile config v1: %v", err)
	}

	firstSnapshotDir := filepath.Join(backupRoot, time.Now().UTC().Format("20060102150405.000000001"))
	firstState := &runtimeState{}
	firstStep := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: firstSnapshotDir,
		targets:     []string{configPath},
		state:       firstState,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceInstall,
		description: "first snapshot",
		appVersion:  "1.0.0",
	}
	if err := firstStep.Run(); err != nil {
		t.Fatalf("first prepareBackupStep.Run() error = %v", err)
	}

	// Change the file content.
	if err := os.WriteFile(configPath, []byte(`{"version": 2}`), 0o644); err != nil {
		t.Fatalf("WriteFile config v2: %v", err)
	}

	secondSnapshotDir := filepath.Join(backupRoot, time.Now().UTC().Format("20060102150405.000000002"))
	secondState := &runtimeState{}
	secondStep := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: secondSnapshotDir,
		targets:     []string{configPath},
		state:       secondState,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceSync,
		description: "second snapshot",
		appVersion:  "1.0.0",
	}
	if err := secondStep.Run(); err != nil {
		t.Fatalf("second prepareBackupStep.Run() error = %v", err)
	}

	// The second snapshot directory MUST exist (content changed).
	if _, err := os.Stat(secondSnapshotDir); err != nil {
		t.Errorf("second snapshot dir should exist when content changed: %v", err)
	}

	// Both backups should be present.
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir backupRoot: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("backupRoot should contain 2 backup dirs, got %d", len(entries))
	}
}

// TestPrepareBackupStep_PrunesOldBackups verifies that after a successful backup
// creation, old unpinned backups beyond DefaultRetentionCount are pruned (BKUP-T27).
func TestPrepareBackupStep_PrunesOldBackups(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll backupRoot: %v", err)
	}

	// Override BackupRootFn so DeleteBackup validation accepts test temp dirs.
	origBackupRootFn := backup.BackupRootFn
	t.Cleanup(func() { backup.BackupRootFn = origBackupRootFn })
	backup.BackupRootFn = func() (string, error) { return backupRoot, nil }

	// Pre-populate backupRoot with DefaultRetentionCount backups using distinct
	// content to avoid dedup short-circuit.
	for i := 0; i < backup.DefaultRetentionCount; i++ {
		content := []byte{byte('a' + i)}
		configPath := filepath.Join(home, "dummy_prune_config.json")
		if err := os.WriteFile(configPath, content, 0o644); err != nil {
			t.Fatalf("WriteFile dummy config %d: %v", i, err)
		}
		snapshotDir := filepath.Join(backupRoot, time.Now().UTC().Add(time.Duration(i)*time.Millisecond).Format("20060102150405.000000000")+"-"+string(rune('A'+i)))
		snapshotter := backup.NewSnapshotter()
		if _, err := snapshotter.Create(snapshotDir, []string{configPath}); err != nil {
			t.Fatalf("Snapshotter.Create %d: %v", i, err)
		}
	}

	// Verify we have exactly DefaultRetentionCount backups before the new one.
	entriesBefore, _ := os.ReadDir(backupRoot)
	if len(entriesBefore) != backup.DefaultRetentionCount {
		t.Fatalf("expected %d backups before prune test, got %d", backup.DefaultRetentionCount, len(entriesBefore))
	}

	// Now create a new backup with different content — this should trigger prune.
	configPath := filepath.Join(home, "dummy_prune_config.json")
	if err := os.WriteFile(configPath, []byte(`{"new": true}`), 0o644); err != nil {
		t.Fatalf("WriteFile new config: %v", err)
	}
	newSnapshotDir := filepath.Join(backupRoot, time.Now().UTC().Add(time.Duration(backup.DefaultRetentionCount)*time.Millisecond).Format("20060102150405.000000000")+"-NEW")
	newState := &runtimeState{}
	step := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: newSnapshotDir,
		targets:     []string{configPath},
		state:       newState,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceInstall,
		description: "new snapshot",
		appVersion:  "1.0.0",
	}
	if err := step.Run(); err != nil {
		t.Fatalf("prepareBackupStep.Run() error = %v", err)
	}

	// After creating 1 new backup (total = DefaultRetentionCount+1), prune should
	// have deleted the oldest one, leaving exactly DefaultRetentionCount backups.
	entriesAfter, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir backupRoot after prune: %v", err)
	}
	if len(entriesAfter) != backup.DefaultRetentionCount {
		t.Errorf("after prune: expected %d backups, got %d", backup.DefaultRetentionCount, len(entriesAfter))
	}
}

// TestPrepareBackupStep_NoPruneWhenBackupRootEmpty verifies that when backupRoot
// is empty, prepareBackupStep runs without error and creates the first backup.
func TestPrepareBackupStep_NoPruneWhenBackupRootEmpty(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll backupRoot: %v", err)
	}

	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	snapshotDir := filepath.Join(backupRoot, time.Now().UTC().Format("20060102150405.000000000"))
	state := &runtimeState{}
	step := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: snapshotDir,
		targets:     []string{configPath},
		state:       state,
		backupRoot:  backupRoot,
		source:      backup.BackupSourceInstall,
		description: "first ever snapshot",
		appVersion:  "1.0.0",
	}
	if err := step.Run(); err != nil {
		t.Fatalf("prepareBackupStep.Run() error = %v", err)
	}

	// The backup should have been created.
	if _, err := os.Stat(snapshotDir); err != nil {
		t.Errorf("snapshot dir should exist: %v", err)
	}

	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir backupRoot: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 backup, got %d", len(entries))
	}
}
