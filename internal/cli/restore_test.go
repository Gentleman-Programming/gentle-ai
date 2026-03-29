package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// setupRestoreHome creates a temporary home dir with N backup manifests.
// Returns the home dir path. Manifests are created with predictable IDs.
func setupRestoreHome(t *testing.T, count int) string {
	t.Helper()
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("backup-%03d", i)
		dir := filepath.Join(backupRoot, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
		m := backup.Manifest{
			ID:        id,
			CreatedAt: time.Date(2026, 3, 20+i, 10, 0, 0, 0, time.UTC),
			RootDir:   dir,
			Source:    backup.BackupSourceInstall,
			Entries:   []backup.ManifestEntry{},
		}
		if err := backup.WriteManifest(filepath.Join(dir, backup.ManifestFilename), m); err != nil {
			t.Fatalf("WriteManifest: %v", err)
		}
	}
	return home
}

// TestRunRestore_ListShowsBackupsNewestFirst verifies that `restore --list`
// prints backups newest-first with ID, timestamp, and source label.
func TestRunRestore_ListShowsBackupsNewestFirst(t *testing.T) {
	home := setupRestoreHome(t, 3)
	restoreHomeDir(t, home)

	var out strings.Builder
	err := RunRestore([]string{"--list"}, &out)
	if err != nil {
		t.Fatalf("RunRestore(--list) error = %v", err)
	}

	output := out.String()
	if output == "" {
		t.Fatalf("RunRestore(--list) produced no output")
	}

	// Must mention "backup" somewhere meaningful.
	if !strings.Contains(output, "backup") {
		t.Errorf("--list output should reference backups; got:\n%s", output)
	}

	// Must show backup IDs.
	if !strings.Contains(output, "backup-002") {
		t.Errorf("--list should show newest backup-002 first; got:\n%s", output)
	}

	// Newest backup-002 must appear before backup-000.
	idx002 := strings.Index(output, "backup-002")
	idx000 := strings.Index(output, "backup-000")
	if idx002 < 0 || idx000 < 0 {
		t.Fatalf("--list output missing expected backup IDs; got:\n%s", output)
	}
	if idx002 > idx000 {
		t.Errorf("--list should show newest (backup-002) before oldest (backup-000)")
	}
}

// TestRunRestore_ListEmptyShowsNoBackupsMessage verifies the empty-backup case.
func TestRunRestore_ListEmptyShowsNoBackupsMessage(t *testing.T) {
	home := t.TempDir()
	restoreHomeDir(t, home)

	var out strings.Builder
	err := RunRestore([]string{"--list"}, &out)
	if err != nil {
		t.Fatalf("RunRestore(--list, empty) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "no backup") {
		t.Errorf("expected 'no backup' message when empty; got:\n%s", output)
	}
}

// TestRunRestore_ByIDWithYesRestoresSuccessfully verifies that `restore <id> --yes`
// restores a backup without prompting when the restore function succeeds.
func TestRunRestore_ByIDWithYesRestoresSuccessfully(t *testing.T) {
	home := setupRestoreHome(t, 2)
	restoreHomeDir(t, home)

	// Track which manifest was restored.
	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"backup-001", "--yes"}, restorer, &out)
	if err != nil {
		t.Fatalf("RunRestoreWithFn(backup-001 --yes) error = %v", err)
	}

	if restoredID != "backup-001" {
		t.Errorf("restored ID = %q, want backup-001", restoredID)
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "restor") {
		t.Errorf("output should confirm restore; got:\n%s", output)
	}
}

// TestRunRestore_UnknownIDReturnsError verifies that requesting an unknown backup ID
// returns a clear error and does NOT modify any files (restorer not called).
func TestRunRestore_UnknownIDReturnsError(t *testing.T) {
	home := setupRestoreHome(t, 2)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"does-not-exist", "--yes"}, restorer, &out)
	if err == nil {
		t.Fatalf("RunRestoreWithFn(unknown-id) expected error, got nil")
	}

	if restoreCalled {
		t.Errorf("restorer must NOT be called for unknown backup ID")
	}
}

// TestRunRestore_LatestWithYesRestoresNewest verifies that `restore latest --yes`
// restores the newest backup (highest CreatedAt) without prompting.
func TestRunRestore_LatestWithYesRestoresNewest(t *testing.T) {
	home := setupRestoreHome(t, 3)
	restoreHomeDir(t, home)

	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"latest", "--yes"}, restorer, &out)
	if err != nil {
		t.Fatalf("RunRestoreWithFn(latest --yes) error = %v", err)
	}

	// backup-002 has the latest CreatedAt (2026-03-22).
	if restoredID != "backup-002" {
		t.Errorf("restored ID = %q, want backup-002 (newest)", restoredID)
	}
}

// TestRunRestore_LatestWithNoBackupsReturnsError verifies the empty-backup edge case.
func TestRunRestore_LatestWithNoBackupsReturnsError(t *testing.T) {
	home := t.TempDir()
	restoreHomeDir(t, home)

	restorer := func(m backup.Manifest) error { return nil }
	var out strings.Builder
	err := RunRestoreWithFn([]string{"latest", "--yes"}, restorer, &out)
	if err == nil {
		t.Fatalf("RunRestoreWithFn(latest, empty) expected error")
	}
}

// TestRunRestore_RequiresConfirmationWithoutYes verifies that without --yes,
// restore by ID returns a clear error requiring confirmation (non-interactive).
func TestRunRestore_RequiresConfirmationWithoutYes(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	// Provide a non-terminal reader so the confirmation prompt sees EOF.
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, strings.NewReader(""))
	if err == nil {
		t.Fatalf("RunRestoreWithFnAndInput without --yes and no tty input: expected error (confirmation required)")
	}
	if restoreCalled {
		t.Errorf("restorer must NOT be called when confirmation is not given")
	}
}

// TestRunRestore_InteractiveTypedYesConfirmationRestores verifies the positive
// interactive confirmation path: when the user types "yes" at the prompt,
// the backup is restored successfully without the --yes flag.
//
// This covers the spec scenario: "restore <id> then typed yes".
// Verify gap: no prior test exercised the typed-confirmation success path.
func TestRunRestore_InteractiveTypedYesConfirmationRestores(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	// Supply "yes\n" as stdin — simulates the user typing yes at the prompt.
	stdin := strings.NewReader("yes\n")
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, stdin)
	if err != nil {
		t.Fatalf("RunRestoreWithFnAndInput(typed yes) error = %v", err)
	}

	// The restorer MUST have been called.
	if restoredID != "backup-000" {
		t.Errorf("restored ID = %q, want backup-000 after typed confirmation", restoredID)
	}

	output := out.String()
	// Output must mention restore completion.
	if !strings.Contains(strings.ToLower(output), "restor") {
		t.Errorf("output should confirm restore after typed yes; got:\n%s", output)
	}
}

// TestRunRestore_InteractiveTypedNoDoesNotRestore verifies that typing anything
// other than "yes" at the prompt cancels the restore without error.
func TestRunRestore_InteractiveTypedNoDoesNotRestore(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	// Supply "no\n" — user explicitly declined.
	stdin := strings.NewReader("no\n")
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, stdin)
	if err != nil {
		t.Fatalf("RunRestoreWithFnAndInput(typed no) error = %v (should be nil — cancel is not an error)", err)
	}

	if restoreCalled {
		t.Errorf("restorer must NOT be called when user types 'no'")
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "cancel") {
		t.Errorf("output should mention cancellation; got:\n%s", output)
	}
}

// TestRunRestore_UnknownFlagReturnsError verifies flag parse errors are surfaced.
func TestRunRestore_UnknownFlagReturnsError(t *testing.T) {
	var out strings.Builder
	err := RunRestore([]string{"--this-flag-does-not-exist"}, &out)
	if err == nil {
		t.Fatalf("RunRestore(unknown-flag) expected error")
	}
}

// --- helpers ---

// restoreHomeDir sets HOME to dir for the duration of the test.
func restoreHomeDir(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", orig) })
	os.Setenv("HOME", dir)
}
