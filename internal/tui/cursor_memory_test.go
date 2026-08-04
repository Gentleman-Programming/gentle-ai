package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestEscRestoreCursorPosition(t *testing.T) {
	tests := []struct {
		name   string
		from   Screen
		cursor int
	}{
		{name: "welcome remembers selection after esc", from: ScreenWelcome, cursor: 2},
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
