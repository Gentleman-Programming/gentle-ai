package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui"
)

func TestCustomAgents_NavigationAndSelection(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir) // os.UserHomeDir resolves here on Windows

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

func TestCustomAgents_DeleteExecution_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir) // os.UserHomeDir resolves here on Windows

	regDir := filepath.Join(tempDir, ".config", "gentle-ai")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(regDir, "custom-agents.json")
	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{Name: "agent-alpha", Title: "Alpha Agent", CreatedAt: time.Now()},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(tempDir, ".claude", "skills", "agent-alpha")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# Alpha"), 0644); err != nil {
		t.Fatal(err)
	}

	detection := system.DetectionResult{
		Configs: []system.ConfigState{
			{Agent: string(model.AgentClaudeCode), Exists: true},
		},
	}
	m := tui.NewModel(detection, "dev")
	m.Cursor = 5
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(tui.Model)

	// Enter ScreenCustomAgentDelete
	state.Cursor = 0
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)

	// Cursor on "Delete Selected" (index 1: agent-alpha is index 0, Delete Selected is index 1)
	state.Cursor = 1
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)

	if state.Screen != tui.ScreenCustomAgents {
		t.Fatalf("expected ScreenCustomAgents, got %v", state.Screen)
	}
	if len(state.CustomAgentsList) != 0 {
		t.Fatalf("expected 0 custom agents after deletion, got %d", len(state.CustomAgentsList))
	}
	if state.CustomAgentsErr != nil {
		t.Fatalf("unexpected CustomAgentsErr: %v", state.CustomAgentsErr)
	}
	if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
		t.Fatalf("expected skill file removed, got err = %v", err)
	}
}

func TestCustomAgents_DeleteExecution_ErrorPreserved(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir) // os.UserHomeDir resolves here on Windows

	regDir := filepath.Join(tempDir, ".config", "gentle-ai")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(regDir, "custom-agents.json")
	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{Name: "bad-agent", Title: "Bad Agent", CreatedAt: time.Now()},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	// Create SKILL.md as directory to trigger Uninstall error (payload is directory)
	skillPathDir := filepath.Join(tempDir, ".claude", "skills", "bad-agent", "SKILL.md")
	if err := os.MkdirAll(skillPathDir, 0755); err != nil {
		t.Fatal(err)
	}

	detection := system.DetectionResult{
		Configs: []system.ConfigState{
			{Agent: string(model.AgentClaudeCode), Exists: true},
		},
	}
	m := tui.NewModel(detection, "dev")
	m.Cursor = 5
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(tui.Model)

	// Enter ScreenCustomAgentDelete
	state.Cursor = 0
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)

	// Make the registry file read-only so SaveRegistry fails after the
	// payload removals: the uninstall error must survive the list reload.
	if err := os.Chmod(regPath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(regPath, 0o600) })

	// Press Enter on "Delete Selected" (cursor index 1)
	state.Cursor = 1
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(tui.Model)

	if state.Screen != tui.ScreenCustomAgents {
		t.Fatalf("expected ScreenCustomAgents, got %v", state.Screen)
	}
	if state.CustomAgentsErr == nil {
		t.Fatal("expected CustomAgentsErr to be non-nil on uninstall failure, got nil")
	}
	view := state.View()
	if !strings.Contains(view, "Error:") {
		t.Fatalf("expected rendered view to show error, got: %s", view)
	}
}
