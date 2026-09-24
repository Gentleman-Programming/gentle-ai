package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func writeTestCustomAgentsRegistry(t *testing.T, home string, agents ...agentbuilder.RegistryEntry) string {
	t.Helper()
	cfgDir := filepath.Join(home, ".config", "gentle-ai")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("MkdirAll config: %v", err)
	}
	regPath := filepath.Join(cfgDir, "custom-agents.json")
	reg := &agentbuilder.Registry{Version: 1, Agents: agents}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	return regPath
}

func TestCustomAgents_WelcomeOptionSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := NewModel(system.DetectionResult{}, "test-version")
	opts := screens.WelcomeOptions(nil, false, false, 0, true)
	idx := -1
	for i, o := range opts {
		if o == "Manage Custom Agents" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("option 'Manage Custom Agents' not found in WelcomeOptions")
	}

	m.Screen = ScreenWelcome
	m.Cursor = idx
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).Screen != ScreenCustomAgents {
		t.Fatalf("screen = %v, want ScreenCustomAgents", updated.(Model).Screen)
	}
}

func TestCustomAgents_NavigationAndCheckboxToggle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeTestCustomAgentsRegistry(t, home,
		agentbuilder.RegistryEntry{
			Name:            "dummy-agent",
			Title:           "Dummy Agent",
			CreatedAt:       time.Now(),
			InstalledAgents: []model.AgentID{model.AgentClaudeCode},
		},
		agentbuilder.RegistryEntry{
			Name:            "second-agent",
			Title:           "Second Agent",
			CreatedAt:       time.Now(),
			InstalledAgents: []model.AgentID{model.AgentOpenCode},
		},
	)

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	if len(m.CustomAgentsList) != 2 {
		t.Fatalf("CustomAgentsList len = %d, want 2", len(m.CustomAgentsList))
	}

	// Press 'd' -> ScreenCustomAgentDelete
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	state := res.(Model)
	if state.Screen != ScreenCustomAgentDelete {
		t.Fatalf("screen = %v, want ScreenCustomAgentDelete", state.Screen)
	}
	// Initial focus on agent 0 should pre-select it
	if !state.CustomAgentDeleteSelected["dummy-agent"] {
		t.Fatalf("expected dummy-agent pre-selected on 'd' press")
	}

	// Move cursor down to second-agent and toggle space
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown})
	state = res.(Model)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeySpace})
	state = res.(Model)
	if !state.CustomAgentDeleteSelected["second-agent"] {
		t.Fatalf("expected second-agent selected after space toggle")
	}

	// Toggle space again to uncheck second-agent
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeySpace})
	state = res.(Model)
	if state.CustomAgentDeleteSelected["second-agent"] {
		t.Fatalf("expected second-agent deselected after second space toggle")
	}

	// Move down to Cancel and press enter
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown}) // Delete Selected
	state = res.(Model)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown}) // Cancel
	state = res.(Model)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = res.(Model)

	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen after cancel = %v, want ScreenCustomAgents", state.Screen)
	}
}

func TestCustomAgents_CreateNewAgentNavigatesToBuilder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Mock an engine executable in PATH so hasAgentBuilderEngines() returns true
	binDir := t.TempDir()
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", binDir)

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	// No custom agents: cursor 0 is "Create new agent"
	if m.Cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.Cursor)
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := res.(Model)
	if state.Screen != ScreenAgentBuilderEngine {
		t.Fatalf("screen = %v, want ScreenAgentBuilderEngine", state.Screen)
	}
}

func TestCustomAgents_DeleteExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillDir := filepath.Join(home, ".claude", "skills", "agent-to-delete")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Title\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	writeTestCustomAgentsRegistry(t, home,
		agentbuilder.RegistryEntry{
			Name:            "agent-to-delete",
			Title:           "Agent To Delete",
			CreatedAt:       time.Now(),
			InstalledAgents: []model.AgentID{model.AgentClaudeCode},
		},
	)

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	if len(m.CustomAgentsList) != 1 {
		t.Fatalf("CustomAgentsList len = %d, want 1", len(m.CustomAgentsList))
	}

	// Press Enter on agent (cursor 0) -> goes to ScreenCustomAgentDelete with agent pre-selected
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := res.(Model)
	if state.Screen != ScreenCustomAgentDelete {
		t.Fatalf("screen = %v, want ScreenCustomAgentDelete", state.Screen)
	}
	if !state.CustomAgentDeleteSelected["agent-to-delete"] {
		t.Fatalf("expected agent-to-delete preselected")
	}

	// Move cursor to "Delete Selected" (agentCount = 1, so index 1)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown})
	state = res.(Model)

	// Press Enter on "Delete Selected" -> runs Uninstall and returns to ScreenCustomAgents
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = res.(Model)

	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen after delete = %v, want ScreenCustomAgents", state.Screen)
	}
	if len(state.CustomAgentsList) != 0 {
		t.Fatalf("CustomAgentsList len = %d, want 0 after deletion", len(state.CustomAgentsList))
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected SKILL.md to be deleted, got err = %v", err)
	}
}

func TestCustomAgents_DeleteExecution_PreservesError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeTestCustomAgentsRegistry(t, home,
		agentbuilder.RegistryEntry{
			Name:            "fail-agent",
			Title:           "Fail Agent",
			InstalledAgents: []model.AgentID{model.AgentClaudeCode},
		},
	)

	// Make skills dir a file to force Uninstall failure
	skillsDir := claude.NewAdapter().SkillsDir(home)
	if err := os.MkdirAll(filepath.Dir(skillsDir), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(skillsDir, []byte("blocker"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	if len(m.CustomAgentsList) != 1 {
		t.Fatalf("CustomAgentsList len = %d, want 1", len(m.CustomAgentsList))
	}

	m.Screen = ScreenCustomAgentDelete
	m.CustomAgentDeleteSelected = map[string]bool{"fail-agent": true}
	m.Cursor = 1 // "Delete Selected"

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := res.(Model)

	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen = %v, want ScreenCustomAgents", state.Screen)
	}
	if state.CustomAgentsErr == nil {
		t.Fatal("expected non-nil CustomAgentsErr on uninstall failure, got nil")
	}
}
