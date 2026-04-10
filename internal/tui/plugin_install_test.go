package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestWelcomeMenu_PluginInstallNavigation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenWelcome
	m.Cursor = 6

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if cmd == nil {
		t.Fatal("plugin install command = nil, want command")
	}

	msg := cmd()
	updated, _ = state.Update(msg)
	state = updated.(Model)

	if state.Screen != ScreenPluginInstall {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenPluginInstall)
	}
	if state.PluginInstallPayload.Err != nil {
		t.Fatalf("PluginInstallPayload.Err = %v, want nil", state.PluginInstallPayload.Err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "plugins", "plugin-sdd-opencode", "index.tsx")); err != nil {
		t.Fatalf("installed plugin file not found: %v", err)
	}
}
