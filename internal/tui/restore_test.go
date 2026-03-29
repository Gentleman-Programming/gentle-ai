package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func makeTestBackup(id string, t time.Time, source backup.BackupSource) backup.Manifest {
	return backup.Manifest{
		ID:        id,
		CreatedAt: t,
		Source:    source,
		Entries:   []backup.ManifestEntry{},
	}
}

// TestBackupSelectionNavigatesToRestoreConfirm verifies that pressing Enter on
// a backup navigates to ScreenRestoreConfirm instead of immediately restoring.
func TestBackupSelectionNavigatesToRestoreConfirm(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenBackups
	m.Backups = []backup.Manifest{
		makeTestBackup("backup-001", time.Now(), backup.BackupSourceInstall),
	}
	m.Cursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenRestoreConfirm {
		t.Errorf("pressing Enter on backup should navigate to ScreenRestoreConfirm, got %v", state.Screen)
	}

	// The selected backup must be stored for the confirm screen to display.
	if state.SelectedBackup.ID != "backup-001" {
		t.Errorf("SelectedBackup.ID = %q, want backup-001", state.SelectedBackup.ID)
	}
}

// TestRestoreConfirmEnterExecutesAndNavigatesToResult verifies that confirming
// on ScreenRestoreConfirm triggers restore and navigates to ScreenRestoreResult.
func TestRestoreConfirmEnterExecutesAndNavigatesToResult(t *testing.T) {
	restored := false
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceInstall)
	m.Cursor = 0 // cursor on "Restore" option
	m.RestoreFn = func(manifest backup.Manifest) error {
		restored = true
		return nil
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	// Execute the tea.Cmd to trigger the restore goroutine result.
	if cmd != nil {
		msg := cmd()
		updated2, _ := state.Update(msg)
		state = updated2.(Model)
	}

	if !restored {
		t.Error("RestoreFn should have been called when confirming restore")
	}

	if state.Screen != ScreenRestoreResult {
		t.Errorf("after restore completes, screen should be ScreenRestoreResult, got %v", state.Screen)
	}
}

// TestRestoreConfirmCancelNavigatesBackToBackups verifies that cancelling on
// ScreenRestoreConfirm navigates back to ScreenBackups without running restore.
func TestRestoreConfirmCancelNavigatesBackToBackups(t *testing.T) {
	restoreCalled := false
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceSync)
	m.Cursor = 1 // cursor on "Cancel" option
	m.RestoreFn = func(manifest backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if restoreCalled {
		t.Error("RestoreFn must NOT be called when cancelling")
	}

	if state.Screen != ScreenBackups {
		t.Errorf("cancel should return to ScreenBackups, got %v", state.Screen)
	}
}

// TestRestoreConfirmEscNavigatesBackToBackups verifies that Esc on the confirm
// screen returns to backup selection without running restore.
func TestRestoreConfirmEscNavigatesBackToBackups(t *testing.T) {
	restoreCalled := false
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceInstall)
	m.RestoreFn = func(manifest backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	state := updated.(Model)

	if restoreCalled {
		t.Error("RestoreFn must NOT be called on Esc")
	}

	if state.Screen != ScreenBackups {
		t.Errorf("Esc on confirm should return to ScreenBackups, got %v", state.Screen)
	}
}

// TestRestoreResultSuccessScreenIsShown verifies that BackupRestoreMsg with no
// error navigates to ScreenRestoreResult and marks success.
func TestRestoreResultSuccessScreenIsShown(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceUpgrade)

	updated, _ := m.Update(BackupRestoreMsg{Err: nil})
	state := updated.(Model)

	if state.Screen != ScreenRestoreResult {
		t.Errorf("successful restore should navigate to ScreenRestoreResult, got %v", state.Screen)
	}

	if state.RestoreErr != nil {
		t.Errorf("RestoreErr should be nil on success, got %v", state.RestoreErr)
	}
}

// TestRestoreResultFailureScreenIsShown verifies that BackupRestoreMsg with an
// error navigates to ScreenRestoreResult and stores the error.
func TestRestoreResultFailureScreenIsShown(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceInstall)

	restoreErr := fmt.Errorf("snapshot missing")
	updated, _ := m.Update(BackupRestoreMsg{Err: restoreErr})
	state := updated.(Model)

	if state.Screen != ScreenRestoreResult {
		t.Errorf("failed restore should navigate to ScreenRestoreResult, got %v", state.Screen)
	}

	if state.RestoreErr == nil {
		t.Errorf("RestoreErr should be set on failure")
	}
}

// TestRestoreResultEnterNavigatesBackToBackups verifies that pressing Enter on
// the result screen navigates the user back to backup selection.
func TestRestoreResultEnterNavigatesBackToBackups(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreResult
	m.SelectedBackup = makeTestBackup("backup-001", time.Now(), backup.BackupSourceInstall)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenBackups {
		t.Errorf("pressing Enter on result should navigate to ScreenBackups, got %v", state.Screen)
	}
}
