package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ComputeChecksum tests
// ---------------------------------------------------------------------------

// TestComputeChecksum_Deterministic verifies that calling ComputeChecksum
// with the same files always returns the same checksum.
func TestComputeChecksum_Deterministic(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file1, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(file2, []byte("world"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	paths := []string{file1, file2}

	first, err := ComputeChecksum(paths)
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v", err)
	}
	second, err := ComputeChecksum(paths)
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v", err)
	}

	if first == "" {
		t.Fatal("ComputeChecksum() returned empty string for non-empty file list")
	}
	if first != second {
		t.Errorf("ComputeChecksum() is not deterministic: %q != %q", first, second)
	}
}

// TestComputeChecksum_DifferentContent verifies that files with different
// content produce different checksums.
func TestComputeChecksum_DifferentContent(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(file, []byte("version: 1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	checksumV1, err := ComputeChecksum([]string{file})
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v", err)
	}

	if err := os.WriteFile(file, []byte("version: 2"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	checksumV2, err := ComputeChecksum([]string{file})
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v", err)
	}

	if checksumV1 == checksumV2 {
		t.Errorf("ComputeChecksum() returned the same checksum for different file contents: %q", checksumV1)
	}
}

// TestComputeChecksum_OrderIndependent verifies that providing paths in a
// different order produces the same composite checksum (sorting is applied).
func TestComputeChecksum_OrderIndependent(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "z.txt")
	file2 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file1, []byte("zzz"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(file2, []byte("aaa"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	forward, err := ComputeChecksum([]string{file1, file2})
	if err != nil {
		t.Fatalf("ComputeChecksum() forward order error = %v", err)
	}
	backward, err := ComputeChecksum([]string{file2, file1})
	if err != nil {
		t.Fatalf("ComputeChecksum() backward order error = %v", err)
	}

	if forward != backward {
		t.Errorf("ComputeChecksum() is order-dependent: %q != %q", forward, backward)
	}
}

// TestComputeChecksum_EmptyPaths verifies that an empty paths slice returns
// an empty checksum string and no error.
func TestComputeChecksum_EmptyPaths(t *testing.T) {
	checksum, err := ComputeChecksum([]string{})
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v, want nil", err)
	}
	if checksum != "" {
		t.Errorf("ComputeChecksum() = %q, want empty string for empty paths", checksum)
	}
}

// TestComputeChecksum_SkipsMissingFiles verifies that non-existent paths are
// silently skipped. If all paths are missing, the result is an empty string.
func TestComputeChecksum_SkipsMissingFiles(t *testing.T) {
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.txt")
	missingFile := filepath.Join(dir, "does-not-exist.txt")
	if err := os.WriteFile(realFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Mixed: one real, one missing — should not error and should produce a checksum.
	checksum, err := ComputeChecksum([]string{realFile, missingFile})
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v, want nil for missing file", err)
	}
	if checksum == "" {
		t.Error("ComputeChecksum() = empty, want non-empty when at least one file exists")
	}

	// All missing — should return empty string.
	allMissing, err := ComputeChecksum([]string{missingFile})
	if err != nil {
		t.Fatalf("ComputeChecksum() error = %v, want nil when all files missing", err)
	}
	if allMissing != "" {
		t.Errorf("ComputeChecksum() = %q, want empty string when all paths missing", allMissing)
	}
}

// ---------------------------------------------------------------------------
// IsDuplicate tests
// ---------------------------------------------------------------------------

// makeTestBackup is a helper that creates a backup directory with a manifest
// in backupDir, using the provided manifest values. Returns the created dir path.
func makeTestBackup(t *testing.T, backupDir string, m Manifest) string {
	t.Helper()
	dir := filepath.Join(backupDir, m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	m.RootDir = dir
	manifestPath := filepath.Join(dir, ManifestFilename)
	if err := WriteManifest(manifestPath, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return dir
}

// TestIsDuplicate_IdenticalChecksum verifies that IsDuplicate returns true
// when the most recent backup has a Checksum matching newChecksum.
func TestIsDuplicate_IdenticalChecksum(t *testing.T) {
	backupDir := t.TempDir()

	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-01",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Checksum:  "abc123",
		Entries:   []ManifestEntry{},
	})

	got, err := IsDuplicate(backupDir, "abc123")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if !got {
		t.Error("IsDuplicate() = false, want true when checksum matches most recent backup")
	}
}

// TestIsDuplicate_DifferentChecksum verifies that IsDuplicate returns false
// when the most recent backup has a different Checksum.
func TestIsDuplicate_DifferentChecksum(t *testing.T) {
	backupDir := t.TempDir()

	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-01",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Checksum:  "abc123",
		Entries:   []ManifestEntry{},
	})

	got, err := IsDuplicate(backupDir, "def456")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if got {
		t.Error("IsDuplicate() = true, want false when checksum differs from most recent backup")
	}
}

// TestIsDuplicate_NoExistingBackups verifies that IsDuplicate returns false
// when the backup directory contains no prior backups.
func TestIsDuplicate_NoExistingBackups(t *testing.T) {
	backupDir := t.TempDir()

	got, err := IsDuplicate(backupDir, "abc123")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if got {
		t.Error("IsDuplicate() = true, want false when no prior backups exist")
	}
}

// TestIsDuplicate_EmptyNewChecksum verifies that IsDuplicate returns false
// when newChecksum is empty — we never skip a backup when we have no checksum.
func TestIsDuplicate_EmptyNewChecksum(t *testing.T) {
	backupDir := t.TempDir()

	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-01",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Checksum:  "abc123",
		Entries:   []ManifestEntry{},
	})

	got, err := IsDuplicate(backupDir, "")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if got {
		t.Error("IsDuplicate() = true, want false when newChecksum is empty")
	}
}

// TestIsDuplicate_MostRecentNoChecksum verifies that IsDuplicate returns false
// when the most recent backup has an empty Checksum (old manifest without dedup).
func TestIsDuplicate_MostRecentNoChecksum(t *testing.T) {
	backupDir := t.TempDir()

	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-old",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Checksum:  "", // old manifest — no checksum
		Entries:   []ManifestEntry{},
	})

	got, err := IsDuplicate(backupDir, "abc123")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if got {
		t.Error("IsDuplicate() = true, want false when most recent backup has no checksum")
	}
}

// TestIsDuplicate_PicksMostRecent verifies that IsDuplicate compares against
// the MOST RECENT backup by CreatedAt, not the first one found on disk.
func TestIsDuplicate_PicksMostRecent(t *testing.T) {
	backupDir := t.TempDir()

	// Older backup matches the new checksum.
	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-old",
		CreatedAt: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		Checksum:  "match-me",
		Entries:   []ManifestEntry{},
	})
	// Newer backup does NOT match.
	makeTestBackup(t, backupDir, Manifest{
		ID:        "backup-new",
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Checksum:  "different",
		Entries:   []ManifestEntry{},
	})

	// Should compare against the NEWER backup — returns false.
	got, err := IsDuplicate(backupDir, "match-me")
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if got {
		t.Error("IsDuplicate() = true, want false — the most recent backup has a different checksum")
	}
}

// ---------------------------------------------------------------------------
// Prune tests
// ---------------------------------------------------------------------------

// makeBackupAt creates a backup directory with the given ID and timestamp.
// Returns the Manifest that was written.
func makeBackupAt(t *testing.T, backupDir, id string, createdAt time.Time, pinned bool) Manifest {
	t.Helper()
	dir := filepath.Join(backupDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}

	m := Manifest{
		ID:        id,
		CreatedAt: createdAt,
		RootDir:   dir,
		Pinned:    pinned,
		Entries:   []ManifestEntry{},
	}

	// Write manifest manually using JSON to handle the Pinned field
	// regardless of whether the struct field exists yet in manifest.go.
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	return m
}

// TestPrune_DeletesOldestUnpinned verifies that when there are more unpinned
// backups than the retention count, the oldest ones are deleted.
func TestPrune_DeletesOldestUnpinned(t *testing.T) {
	backupDir := t.TempDir()
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return backupDir, nil }

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 6; i++ {
		makeBackupAt(t, backupDir, fmt.Sprintf("backup-%02d", i), base.Add(time.Duration(i)*time.Hour), false)
	}

	deleted, err := Prune(backupDir, 5)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	// Should have deleted exactly 1 (oldest: backup-01).
	if len(deleted) != 1 {
		t.Errorf("Prune() deleted %d backups, want 1", len(deleted))
	}
	if len(deleted) == 1 && deleted[0] != "backup-01" {
		t.Errorf("Prune() deleted %q, want %q (oldest backup)", deleted[0], "backup-01")
	}

	// backup-01 directory must be gone.
	if _, err := os.Stat(filepath.Join(backupDir, "backup-01")); !os.IsNotExist(err) {
		t.Error("backup-01 directory still exists after Prune")
	}
	// backup-02 must still be there (inside the retention window).
	if _, err := os.Stat(filepath.Join(backupDir, "backup-02")); os.IsNotExist(err) {
		t.Error("backup-02 was unexpectedly deleted by Prune")
	}
}

// TestPrune_SkipsPinnedBackups verifies that pinned backups are never deleted
// even when total unpinned count exceeds the limit.
func TestPrune_SkipsPinnedBackups(t *testing.T) {
	backupDir := t.TempDir()
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return backupDir, nil }

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 4 unpinned + 2 pinned = 6 backups, retentionCount = 3.
	makeBackupAt(t, backupDir, "unpinned-01", base.Add(1*time.Hour), false)
	makeBackupAt(t, backupDir, "unpinned-02", base.Add(2*time.Hour), false)
	makeBackupAt(t, backupDir, "unpinned-03", base.Add(3*time.Hour), false)
	makeBackupAt(t, backupDir, "unpinned-04", base.Add(4*time.Hour), false)
	makeBackupAt(t, backupDir, "pinned-01", base.Add(5*time.Hour), true)
	makeBackupAt(t, backupDir, "pinned-02", base.Add(6*time.Hour), true)

	deleted, err := Prune(backupDir, 3)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	// 4 unpinned - keep 3 = delete 1 (the oldest unpinned: unpinned-01).
	if len(deleted) != 1 {
		t.Errorf("Prune() deleted %d backups, want 1", len(deleted))
	}

	// Pinned backups must still exist.
	if _, err := os.Stat(filepath.Join(backupDir, "pinned-01")); os.IsNotExist(err) {
		t.Error("pinned-01 was deleted by Prune — pinned backups must be preserved")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "pinned-02")); os.IsNotExist(err) {
		t.Error("pinned-02 was deleted by Prune — pinned backups must be preserved")
	}
}

// TestPrune_ZeroRetention verifies that Prune does nothing when retentionCount
// is 0 (unlimited mode).
func TestPrune_ZeroRetention(t *testing.T) {
	backupDir := t.TempDir()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 10; i++ {
		makeBackupAt(t, backupDir, fmt.Sprintf("backup-%02d", i), base.Add(time.Duration(i)*time.Hour), false)
	}

	deleted, err := Prune(backupDir, 0)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("Prune() deleted %d backups, want 0 for retentionCount=0 (unlimited)", len(deleted))
	}
}

// TestPrune_FewerThanRetention verifies that nothing is deleted when the number
// of unpinned backups is already at or below the retention limit.
func TestPrune_FewerThanRetention(t *testing.T) {
	backupDir := t.TempDir()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	makeBackupAt(t, backupDir, "backup-01", base.Add(1*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-02", base.Add(2*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-03", base.Add(3*time.Hour), false)

	deleted, err := Prune(backupDir, 5)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("Prune() deleted %d backups, want 0 when count (%d) < retention (%d)", len(deleted), 3, 5)
	}
}

// TestPrune_AllPinned verifies that when all backups are pinned, nothing is
// deleted even when the total exceeds retentionCount.
func TestPrune_AllPinned(t *testing.T) {
	backupDir := t.TempDir()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 7; i++ {
		makeBackupAt(t, backupDir, fmt.Sprintf("pinned-%02d", i), base.Add(time.Duration(i)*time.Hour), true)
	}

	deleted, err := Prune(backupDir, 5)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("Prune() deleted %d backups, want 0 when all backups are pinned", len(deleted))
	}

	// Spot-check that a pinned backup still exists.
	if _, err := os.Stat(filepath.Join(backupDir, "pinned-01")); os.IsNotExist(err) {
		t.Error("pinned-01 was deleted by Prune — must not delete pinned backups")
	}
}

// TestPrune_ReturnsDeletedIDs verifies that the returned slice contains the
// IDs (backup directory names) of all deleted backups in order.
func TestPrune_ReturnsDeletedIDs(t *testing.T) {
	backupDir := t.TempDir()
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return backupDir, nil }

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Create 5 unpinned, keep 3 → expect 2 deleted (oldest: backup-01, backup-02).
	makeBackupAt(t, backupDir, "backup-01", base.Add(1*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-02", base.Add(2*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-03", base.Add(3*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-04", base.Add(4*time.Hour), false)
	makeBackupAt(t, backupDir, "backup-05", base.Add(5*time.Hour), false)

	deleted, err := Prune(backupDir, 3)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(deleted) != 2 {
		t.Fatalf("Prune() returned %d IDs, want 2: %v", len(deleted), deleted)
	}

	wantDeleted := map[string]bool{"backup-01": true, "backup-02": true}
	for _, id := range deleted {
		if !wantDeleted[id] {
			t.Errorf("Prune() deleted unexpected backup %q", id)
		}
	}

	// Verify both are gone from disk.
	for id := range wantDeleted {
		if _, err := os.Stat(filepath.Join(backupDir, id)); !os.IsNotExist(err) {
			t.Errorf("backup %q still exists on disk after Prune", id)
		}
	}
}
