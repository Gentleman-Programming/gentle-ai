package screens

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// TestRenderBackupsShowsDisplayLabel verifies that RenderBackups uses the
// manifest's DisplayLabel (source + timestamp) instead of just the raw ID.
func TestRenderBackupsShowsDisplayLabel(t *testing.T) {
	manifests := []backup.Manifest{
		{
			ID:        "20260322150405.000000000",
			CreatedAt: time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
			Source:    backup.BackupSourceInstall,
		},
	}

	output := RenderBackups(manifests, 0, 0)

	// Must include the source label from DisplayLabel.
	if !strings.Contains(output, "install") {
		t.Errorf("RenderBackups should show source label 'install' from DisplayLabel; got:\n%s", output)
	}
}

// TestRenderBackupsShowsFallbackLabelForOldManifest verifies that an old
// manifest without Source metadata renders with "unknown source" fallback.
func TestRenderBackupsShowsFallbackLabelForOldManifest(t *testing.T) {
	manifests := []backup.Manifest{
		{
			ID:        "old-backup-id",
			CreatedAt: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC),
			// Source intentionally empty — simulates old manifest.
		},
	}

	output := RenderBackups(manifests, 0, 0)

	if !strings.Contains(output, "unknown source") {
		t.Errorf("RenderBackups should show 'unknown source' for old manifests; got:\n%s", output)
	}
}

// TestRenderRestoreConfirmIncludesBackupIdentity verifies the confirm screen
// shows the backup ID and source label so the user knows what they're restoring.
func TestRenderRestoreConfirmIncludesBackupIdentity(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "20260322150405.000000000",
		CreatedAt: time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
		Source:    backup.BackupSourceSync,
	}

	output := RenderRestoreConfirm(manifest, 0)

	if !strings.Contains(output, manifest.ID) {
		t.Errorf("RenderRestoreConfirm should show backup ID; got:\n%s", output)
	}

	if !strings.Contains(output, "sync") {
		t.Errorf("RenderRestoreConfirm should show source label; got:\n%s", output)
	}
}

// TestRenderRestoreConfirmShowsConfirmAndCancelOptions verifies that the
// confirmation screen presents both "Restore" and "Cancel" options.
func TestRenderRestoreConfirmShowsConfirmAndCancelOptions(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "test-backup",
		CreatedAt: time.Now().UTC(),
	}

	output := RenderRestoreConfirm(manifest, 0)

	// Must show a restore/confirm action.
	if !strings.Contains(strings.ToLower(output), "restore") {
		t.Errorf("RenderRestoreConfirm missing restore option; got:\n%s", output)
	}

	// Must show a cancel action.
	if !strings.Contains(strings.ToLower(output), "cancel") && !strings.Contains(strings.ToLower(output), "back") {
		t.Errorf("RenderRestoreConfirm missing cancel/back option; got:\n%s", output)
	}
}

// TestRenderRestoreResultSuccessShowsSuccessMessage verifies that a successful
// restore result screen displays a success confirmation.
func TestRenderRestoreResultSuccessShowsSuccessMessage(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "my-backup-001",
		CreatedAt: time.Now().UTC(),
		Source:    backup.BackupSourceUpgrade,
	}

	output := RenderRestoreResult(manifest, nil)

	// Must include a success indicator.
	lower := strings.ToLower(output)
	if !strings.Contains(lower, "success") && !strings.Contains(lower, "restored") && !strings.Contains(lower, "complete") {
		t.Errorf("RenderRestoreResult(nil err) should show success; got:\n%s", output)
	}

	// Must show the backup identity.
	if !strings.Contains(output, manifest.ID) {
		t.Errorf("RenderRestoreResult should show backup ID; got:\n%s", output)
	}
}

// TestRenderRestoreResultFailureShowsErrorMessage verifies that a failed
// restore result screen displays actionable failure text.
func TestRenderRestoreResultFailureShowsErrorMessage(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "my-backup-002",
		CreatedAt: time.Now().UTC(),
	}

	errText := "snapshot file missing"
	output := RenderRestoreResult(manifest, fmt.Errorf("%s", errText))

	lower := strings.ToLower(output)
	if !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") {
		t.Errorf("RenderRestoreResult(err) should show failure; got:\n%s", output)
	}

	if !strings.Contains(output, errText) {
		t.Errorf("RenderRestoreResult should include error text %q; got:\n%s", errText, output)
	}
}

// TestRenderBackups_WithScroll verifies that when there are more than BackupMaxVisible
// items, scroll indicators (↑ more / ↓ more) are shown appropriately.
func TestRenderBackups_WithScroll(t *testing.T) {
	// Create 15 backups (more than BackupMaxVisible=10).
	manifests := make([]backup.Manifest, 15)
	for i := range manifests {
		manifests[i] = backup.Manifest{
			ID:        fmt.Sprintf("backup-%02d", i),
			CreatedAt: time.Now().UTC(),
			Source:    backup.BackupSourceInstall,
		}
	}

	t.Run("no scroll indicators when all items visible", func(t *testing.T) {
		// Only 5 items — all fit, no scroll needed.
		output := RenderBackups(manifests[:5], 0, 0)
		if strings.Contains(output, "↑ more") {
			t.Errorf("should not show ↑ more indicator when scrollOffset=0")
		}
		if strings.Contains(output, "↓ more") {
			t.Errorf("should not show ↓ more indicator when all items fit")
		}
	})

	t.Run("shows down indicator when more items below", func(t *testing.T) {
		output := RenderBackups(manifests, 0, 0)
		if !strings.Contains(output, "↓ more") {
			t.Errorf("should show ↓ more indicator when list exceeds BackupMaxVisible; got:\n%s", output)
		}
		if strings.Contains(output, "↑ more") {
			t.Errorf("should not show ↑ more indicator when scrollOffset=0; got:\n%s", output)
		}
	})

	t.Run("shows up indicator when scrolled down", func(t *testing.T) {
		output := RenderBackups(manifests, 5, 5)
		if !strings.Contains(output, "↑ more") {
			t.Errorf("should show ↑ more indicator when scrolled down; got:\n%s", output)
		}
	})

	t.Run("shows both indicators when in middle of long list", func(t *testing.T) {
		// 15 items, scrolled to offset 3, cursor at 3 — 10 items visible (3..12), more above and below.
		output := RenderBackups(manifests, 3, 3)
		if !strings.Contains(output, "↑ more") {
			t.Errorf("should show ↑ more indicator; got:\n%s", output)
		}
		if !strings.Contains(output, "↓ more") {
			t.Errorf("should show ↓ more indicator; got:\n%s", output)
		}
	})
}

// TestRenderDeleteConfirm verifies the delete confirmation screen shows
// backup info and Delete/Cancel options.
func TestRenderDeleteConfirm(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "backup-del-001",
		CreatedAt: time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
		Source:    backup.BackupSourceInstall,
	}

	output := RenderDeleteConfirm(manifest, 0)

	if !strings.Contains(output, manifest.ID) {
		t.Errorf("RenderDeleteConfirm should show backup ID; got:\n%s", output)
	}

	lower := strings.ToLower(output)
	if !strings.Contains(lower, "delete") {
		t.Errorf("RenderDeleteConfirm should show 'delete' option; got:\n%s", output)
	}
	if !strings.Contains(lower, "cancel") {
		t.Errorf("RenderDeleteConfirm should show 'cancel' option; got:\n%s", output)
	}
}

// TestRenderDeleteResult_Success verifies the delete success screen.
func TestRenderDeleteResult_Success(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "backup-del-002",
		CreatedAt: time.Now().UTC(),
		Source:    backup.BackupSourceSync,
	}

	output := RenderDeleteResult(manifest, nil)

	if !strings.Contains(output, manifest.ID) {
		t.Errorf("RenderDeleteResult should show backup ID; got:\n%s", output)
	}

	lower := strings.ToLower(output)
	if !strings.Contains(lower, "deleted") && !strings.Contains(lower, "success") {
		t.Errorf("RenderDeleteResult(nil err) should show success message; got:\n%s", output)
	}
}

// TestRenderDeleteResult_Error verifies the delete failure screen shows error details.
func TestRenderDeleteResult_Error(t *testing.T) {
	manifest := backup.Manifest{
		ID:        "backup-del-003",
		CreatedAt: time.Now().UTC(),
	}

	errText := "permission denied"
	output := RenderDeleteResult(manifest, fmt.Errorf("%s", errText))

	lower := strings.ToLower(output)
	if !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") {
		t.Errorf("RenderDeleteResult(err) should show failure; got:\n%s", output)
	}

	if !strings.Contains(output, errText) {
		t.Errorf("RenderDeleteResult should include error text %q; got:\n%s", errText, output)
	}
}

// TestRenderRenameBackup verifies the rename screen shows backup info and text input.
func TestRenderRenameBackup(t *testing.T) {
	manifest := backup.Manifest{
		ID:          "backup-ren-001",
		CreatedAt:   time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
		Source:      backup.BackupSourceUpgrade,
		Description: "before rename",
	}

	t.Run("shows backup ID", func(t *testing.T) {
		output := RenderRenameBackup(manifest, "new name", 8)
		if !strings.Contains(output, manifest.ID) {
			t.Errorf("RenderRenameBackup should show backup ID; got:\n%s", output)
		}
	})

	t.Run("shows current description when set", func(t *testing.T) {
		output := RenderRenameBackup(manifest, "new name", 8)
		if !strings.Contains(output, "before rename") {
			t.Errorf("RenderRenameBackup should show current description; got:\n%s", output)
		}
	})

	t.Run("shows input text", func(t *testing.T) {
		output := RenderRenameBackup(manifest, "hello world", 11)
		if !strings.Contains(output, "hello world") {
			t.Errorf("RenderRenameBackup should show input text; got:\n%s", output)
		}
	})

	t.Run("shows help text with enter and esc", func(t *testing.T) {
		output := RenderRenameBackup(manifest, "", 0)
		lower := strings.ToLower(output)
		if !strings.Contains(lower, "enter") {
			t.Errorf("RenderRenameBackup should mention enter key; got:\n%s", output)
		}
		if !strings.Contains(lower, "esc") {
			t.Errorf("RenderRenameBackup should mention esc key; got:\n%s", output)
		}
	})
}
