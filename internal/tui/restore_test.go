package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
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

func TestBackupScreenPreselectsNewestAxiomBackup(t *testing.T) {
	created := func(day int) time.Time { return time.Date(2026, 3, day, 10, 0, 0, 0, time.UTC) }

	tests := []struct {
		name    string
		backups []backup.Manifest
		want    int
	}{
		{
			name: "latest Axiom backup wins over newer Gentle AI history",
			backups: []backup.Manifest{
				{ID: "newer-legacy", CreatedAt: created(23), Origin: backup.BackupOriginGentleAI},
				{ID: "newest-axiom", CreatedAt: created(22), Origin: backup.BackupOriginAxiom},
				{ID: "older-axiom", CreatedAt: created(21), Origin: backup.BackupOriginAxiom},
			},
			want: 1,
		},
		{
			name: "no Axiom backups leaves historical backups unselected",
			backups: []backup.Manifest{
				{ID: "newest-legacy", CreatedAt: created(23), Origin: backup.BackupOriginGentleAI},
				{ID: "older-legacy", CreatedAt: created(22), Origin: backup.BackupOriginGentleAI},
			},
			want: 2, // Focus "Volver"; the user must deliberately choose a historical backup.
		},
		{
			name:    "empty list keeps the only return option selected",
			backups: nil,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Backups = tt.backups
			m.setScreen(ScreenBackups)
			if m.Cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.Cursor, tt.want)
			}
			if tt.name == "no Axiom backups leaves historical backups unselected" {
				updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				if got := updated.(Model); got.Screen != ScreenWelcome || got.SelectedBackup.ID != "" {
					t.Errorf("Enter on the fallback return row must not select/restore history: screen=%v selected=%q", got.Screen, got.SelectedBackup.ID)
				}

				updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
				if got := updated.(Model); got.Cursor != len(tt.backups)-1 {
					t.Errorf("Up from the fallback return row should expose the newest historical row: cursor=%d", got.Cursor)
				}
			}
		})
	}
}

func TestBackupScreenScrollsToDefaultSelectionOutsideFirstPage(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	backups := make([]backup.Manifest, 0, 14)
	for i := 0; i < 12; i++ {
		backups = append(backups, backup.Manifest{
			ID:        fmt.Sprintf("newer-history-%02d", i),
			CreatedAt: base.Add(time.Duration(100+i) * time.Hour),
			Origin:    backup.BackupOriginGentleAI,
		})
	}
	backups = append(backups,
		backup.Manifest{ID: "latest-axiom", CreatedAt: base.Add(50 * time.Hour), Origin: backup.BackupOriginAxiom},
		backup.Manifest{ID: "older-axiom", CreatedAt: base.Add(40 * time.Hour), Origin: backup.BackupOriginAxiom},
	)

	m := NewModel(system.DetectionResult{}, "dev")
	m.Backups = backups
	m.setScreen(ScreenBackups)

	if m.Cursor != 12 {
		t.Fatalf("cursor = %d, want 12 for the newest Axiom backup", m.Cursor)
	}
	if m.BackupScroll != 3 {
		t.Fatalf("BackupScroll = %d, want 3 to show the selected row within the 10-row viewport", m.BackupScroll)
	}
	if !strings.Contains(m.View(), "latest-axiom") {
		t.Fatalf("the selected Axiom backup is outside the visible window:\n%s", m.View())
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
