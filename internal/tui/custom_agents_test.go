package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
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
		agentbuilder.RegistryEntry{Name: "dummy-agent", Title: "Dummy Agent", InstalledAgents: []model.AgentID{model.AgentClaudeCode}},
		agentbuilder.RegistryEntry{Name: "second-agent", Title: "Second Agent", InstalledAgents: []model.AgentID{model.AgentOpenCode}},
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

func TestCustomAgents_DeleteExecution_MultiAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	// Create skill files for two agents using the exact adapters
	claudeSkills := claude.NewAdapter().SkillsDir(home)
	skillDir1 := filepath.Join(claudeSkills, "agent-one")
	if err := os.MkdirAll(skillDir1, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte("# Title 1"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	openCodeSkills := opencode.NewAdapter().SkillsDir(home)
	skillDir2 := filepath.Join(openCodeSkills, "agent-two")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte("# Title 2"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	writeTestCustomAgentsRegistry(t, home,
		agentbuilder.RegistryEntry{
			Name:            "agent-one",
			Title:           "Agent One",
			InstalledAgents: []model.AgentID{model.AgentClaudeCode},
		},
		agentbuilder.RegistryEntry{
			Name:            "agent-two",
			Title:           "Agent Two",
			InstalledAgents: []model.AgentID{model.AgentOpenCode},
		},
	)

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	if len(m.CustomAgentsList) != 2 {
		t.Fatalf("CustomAgentsList len = %d, want 2", len(m.CustomAgentsList))
	}

	// Press 'd' -> enters ScreenCustomAgentDelete with agent-one pre-selected
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	state := res.(Model)

	// Move cursor to agent-two (index 1) and toggle space
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown})
	state = res.(Model)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeySpace})
	state = res.(Model)

	if !state.CustomAgentDeleteSelected["agent-one"] || !state.CustomAgentDeleteSelected["agent-two"] {
		t.Fatalf("expected both agent-one and agent-two selected for deletion")
	}

	// Move cursor to "Delete Selected" (agentCount = 2, so index 2)
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyDown})
	state = res.(Model)

	// Press Enter on "Delete Selected" -> runs batch Uninstall and returns to ScreenCustomAgents
	res, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = res.(Model)

	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen after delete = %v, want ScreenCustomAgents", state.Screen)
	}
	if len(state.CustomAgentsList) != 0 {
		t.Fatalf("CustomAgentsList len = %d, want 0 after deletion", len(state.CustomAgentsList))
	}
	if _, err := os.Stat(filepath.Join(skillDir1, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected skillDir1 SKILL.md to be deleted, got err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir2, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected skillDir2 SKILL.md to be deleted, got err = %v", err)
	}
}

func TestCustomAgents_DeleteTargetRemovedExternally(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	regPath := writeTestCustomAgentsRegistry(t, home,
		agentbuilder.RegistryEntry{
			Name:            "external-agent",
			Title:           "External Agent",
			InstalledAgents: []model.AgentID{model.AgentClaudeCode},
		},
	)

	m := NewModel(system.DetectionResult{}, "test-version")
	m.setScreen(ScreenCustomAgents)

	if len(m.CustomAgentsList) != 1 {
		t.Fatalf("CustomAgentsList len = %d, want 1", len(m.CustomAgentsList))
	}

	// Remove from registry externally before user confirms deletion
	_ = os.Remove(regPath)

	m.Screen = ScreenCustomAgentDelete
	m.CustomAgentDeleteSelected = map[string]bool{"external-agent": true}
	m.Cursor = 1 // "Delete Selected"

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := res.(Model)

	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen = %v, want ScreenCustomAgents when target missing", state.Screen)
	}
	if state.CustomAgentsErr != nil {
		t.Errorf("expected nil error on missing entry, got %v", state.CustomAgentsErr)
	}
	if len(state.CustomAgentsList) != 0 {
		t.Errorf("expected empty list refreshed from disk, got len %d", len(state.CustomAgentsList))
	}
}

func TestCustomAgents_DeleteErrorPreservedOnUninstallFailure(t *testing.T) {
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
	if errors.Is(state.CustomAgentsErr, agentbuilder.ErrAgentNotFound) {
		t.Errorf("did not expect ErrAgentNotFound, got %v", state.CustomAgentsErr)
	}
}
