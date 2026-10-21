package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestEscRestoreCursorPosition(t *testing.T) {
	tests := []struct {
		name   string
		from   Screen
		cursor int
	}{
		{name: "welcome remembers selection after esc", from: ScreenWelcome, cursor: 4},
		{name: "model config remembers selection after esc", from: ScreenModelConfig, cursor: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = tt.from
			m.Cursor = tt.cursor

			// forward: Enter opens the child screen
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)
			if m.Screen == tt.from {
				t.Fatalf("expected Enter to leave %v", tt.from)
			}

			// back: Esc returns to the parent screen
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(Model)

			if m.Screen != tt.from {
				t.Fatalf("expected to return to %v, got %v", tt.from, m.Screen)
			}
			if m.Cursor != tt.cursor {
				t.Errorf("cursor not restored: got %d, want %d", m.Cursor, tt.cursor)
			}
		})
	}
}

func TestBackMenuOptionRestoresCursorPosition(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenModelConfig
	m.Cursor = 2 // "Configure Kiro models"
	// Position the user had on Welcome before entering Model Config.
	m.CursorMemory[ScreenWelcome] = 3

	// Enter into Kiro picker, saving the position on Model Config.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	// Return to Model Config via Esc, then select its "Back" option.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	m.Cursor = 4 // "Back"
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.Screen != ScreenWelcome {
		t.Fatalf("expected ScreenWelcome, got %v", m.Screen)
	}
	if m.Cursor != 3 {
		t.Errorf("cursor not restored on Back option: got %d, want 3", m.Cursor)
	}
}

func TestBackOptionRestoresCursorAcrossScreens(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *Model)
		want  Screen
		saved int
	}{
		{
			name:  "detection back returns to welcome position",
			setup: func(m *Model) { m.Screen = ScreenDetection; m.Cursor = 1 },
			want:  ScreenWelcome,
			saved: 1,
		},
		{
			name: "agent builder engine back returns to welcome position",
			setup: func(m *Model) {
				m.Screen = ScreenAgentBuilderEngine
				m.AgentBuilder.AvailableEngines = nil
				m.Cursor = 0
			},
			want:  ScreenWelcome,
			saved: 1,
		},
		{
			name: "backups back returns to welcome position",
			setup: func(m *Model) {
				m.Screen = ScreenBackups
				m.Backups = nil
				m.Cursor = 0 // with no backups, the only row is "Back"
			},
			want:  ScreenWelcome,
			saved: 1,
		},
		{
			name: "uninstall agents back returns to uninstall mode position",
			setup: func(m *Model) {
				m.Screen = ScreenUninstall
				m.Cursor = len(screens.UninstallAgentOptions()) + 1
			},
			want:  ScreenUninstallMode,
			saved: 1,
		},
		{
			name: "persona back returns to agents position",
			setup: func(m *Model) {
				m.Screen = ScreenPersona
				m.Cursor = len(screens.PersonaOptions())
			},
			want:  ScreenAgents,
			saved: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			tt.setup(&m)
			m.CursorMemory[tt.want] = tt.saved

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)

			if m.Screen != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, m.Screen)
			}
			if m.Cursor != tt.saved {
				t.Errorf("cursor not restored: got %d, want %d", m.Cursor, tt.saved)
			}
		})
	}
}

func TestModelConfigPickerBackRestoresSourceCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		screen Screen
	}{
		{name: "claude", cursor: 0, screen: ScreenClaudeModelPicker},
		{name: "kiro", cursor: 2, screen: ScreenKiroModelPicker},
		{name: "codex", cursor: 3, screen: ScreenCodexModelPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = ScreenModelConfig
			m.Cursor = tt.cursor

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)
			if m.Screen != tt.screen {
				t.Fatalf("expected picker %v, got %v", tt.screen, m.Screen)
			}

			m.Cursor = m.optionCount() - 1 // Back
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)
			if m.Screen != ScreenModelConfig {
				t.Fatalf("expected ScreenModelConfig, got %v", m.Screen)
			}
			if m.Cursor != tt.cursor {
				t.Errorf("cursor not restored after picker Back: got %d, want %d", m.Cursor, tt.cursor)
			}
		})
	}
}

func TestNoProviderBackRestoresModelConfigCursor(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenModelPicker // zero AvailableIDs → "Continue with defaults" / "Back"
	m.Cursor = 1                 // "Back"
	m.ModelConfigMode = true
	m.CursorMemory[ScreenModelConfig] = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.Screen != ScreenModelConfig {
		t.Fatalf("expected ScreenModelConfig, got %v", m.Screen)
	}
	if m.Cursor != 1 {
		t.Errorf("cursor not restored on no-provider Back: got %d, want 1", m.Cursor)
	}
}

func TestDeleteBackupCancelKeepsRestoredRowVisible(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenBackups
	count := screens.BackupMaxVisible + 5
	m.Backups = make([]backup.Manifest, count)
	for i := range m.Backups {
		m.Backups[i] = backup.Manifest{ID: string(rune('a' + i))}
	}
	m.Cursor = count - 3 // row beyond the first visible window
	m.BackupScroll = m.Cursor - screens.BackupMaxVisible + 1

	// "d" opens the delete confirmation; Cancel returns to the list.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	m.Cursor = 1 // Cancel
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.Screen != ScreenBackups {
		t.Fatalf("expected ScreenBackups, got %v", m.Screen)
	}
	if m.Cursor != count-3 {
		t.Fatalf("cursor not restored: got %d, want %d", m.Cursor, count-3)
	}
	if m.Cursor < m.BackupScroll || m.Cursor >= m.BackupScroll+screens.BackupMaxVisible {
		t.Errorf("restored row %d is outside the visible window [%d, %d)",
			m.Cursor, m.BackupScroll, m.BackupScroll+screens.BackupMaxVisible)
	}
}

func TestDeleteBackupCancelRestoresCursorPosition(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenBackups
	m.Backups = []backup.Manifest{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.Cursor = 2 // third backup row

	// "d" opens the delete confirmation, remembering the row.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if m.Screen != ScreenDeleteConfirm {
		t.Fatalf("expected ScreenDeleteConfirm, got %v", m.Screen)
	}

	// "Cancel" returns to the backup list on the same row.
	m.Cursor = 1 // Cancel
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.Screen != ScreenBackups {
		t.Fatalf("expected ScreenBackups, got %v", m.Screen)
	}
	if m.Cursor != 2 {
		t.Errorf("cursor not restored after cancel: got %d, want 2", m.Cursor)
	}
}
