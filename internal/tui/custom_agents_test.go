package tui_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui"
)

func TestCustomAgents_NavigationAndSelection(t *testing.T) {
	tempDir := t.TempDir()
	homeBackup := os.Getenv("HOME")
	defer os.Setenv("HOME", homeBackup)
	os.Setenv("HOME", tempDir)

	regDir := filepath.Join(tempDir, ".config", "gentle-ai")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(regDir, "custom-agents.json")
	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{Name: "agent-alpha", Title: "Alpha Agent", CreatedAt: time.Now()},
			{Name: "agent-beta", Title: "Beta Agent", CreatedAt: time.Now()},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	m := tui.NewModel(system.DetectionResult{}, "dev")
	// Navigate from Welcome (index 5: "Manage Custom Agents")
	m.Cursor = 5
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(tui.Model)

	if state.Screen != tui.ScreenCustomAgents {
		t.Fatalf("expected ScreenCustomAgents, got %v", state.Screen)
	}
	if len(state.CustomAgentsList) != 2 {
		t.Fatalf("expected 2 custom agents, got %d", len(state.CustomAgentsList))
	}

	// Press Enter on first agent -> enter ScreenCustomAgentDelete
	state.Cursor = 0
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)

	if state.Screen != tui.ScreenCustomAgentDelete {
		t.Fatalf("expected ScreenCustomAgentDelete, got %v", state.Screen)
	}
	if !state.CustomAgentDeleteSelected["agent-alpha"] {
		t.Errorf("expected agent-alpha to be pre-selected")
	}

	// Toggle agent-beta using space (cursor on index 1)
	state.Cursor = 1
	updated, _ = state.Update(tea.KeyMsg{Runes: []rune{' '}, Type: tea.KeyRunes})
	state = updated.(tui.Model)
	if !state.CustomAgentDeleteSelected["agent-beta"] {
		t.Errorf("expected agent-beta to be selected after space toggle")
	}

	// Navigate to "Cancel" (index 3) and Enter
	state.Cursor = 3
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)
	if state.Screen != tui.ScreenCustomAgents {
		t.Fatalf("expected return to ScreenCustomAgents, got %v", state.Screen)
	}
}
