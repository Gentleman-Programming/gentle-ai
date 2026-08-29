package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func TestCustomAgents_WelcomeOptionSelection(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.AgentBuilder.AvailableEngines = []model.AgentID{model.AgentClaudeCode}

	opts := screens.WelcomeOptions(nil, false, false, 0, true)
	idx := -1
	for i, opt := range opts {
		if opt == "Manage Custom Agents" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("option 'Manage Custom Agents' not found")
	}
	m.Cursor = idx
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).Screen != ScreenCustomAgents {
		t.Fatalf("screen = %v, want ScreenCustomAgents", updated.(Model).Screen)
	}
}

func TestCustomAgents_NavigationAndDeletion(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	regPath := filepath.Join(tempHome, ".config", "gentle-ai", "custom-agents.json")
	if err := os.MkdirAll(filepath.Dir(regPath), 0755); err != nil {
		t.Fatalf("MkdirAll regPath: %v", err)
	}

	claudeSkills := filepath.Join(tempHome, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0755); err != nil {
		t.Fatalf("MkdirAll claudeSkills: %v", err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{
				Name:             "dummy-agent",
				Title:            "Dummy Agent",
				CreatedAt:        time.Now(),
				GenerationEngine: model.AgentClaudeCode,
				InstalledAgents:  []model.AgentID{model.AgentClaudeCode},
			},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	agent := &agentbuilder.GeneratedAgent{Name: "dummy-agent", Title: "Dummy Agent", Content: "# Dummy\n"}
	adapters := []agentbuilder.AdapterInfo{{AgentID: model.AgentClaudeCode, SkillsDir: claudeSkills}}
	if _, err := agentbuilder.Install(agent, adapters, ""); err != nil {
		t.Fatalf("Install: %v", err)
	}

	skillFile := filepath.Join(claudeSkills, "dummy-agent", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("precondition: skill file must exist: %v", err)
	}

	m := NewModel(system.DetectionResult{
		Configs: []system.ConfigState{{Agent: string(model.AgentClaudeCode), Exists: true}},
	}, "dev")
	m.AgentBuilder.AvailableEngines = []model.AgentID{model.AgentClaudeCode}

	m.setScreen(ScreenCustomAgents)
	if len(m.CustomAgentsList) != 1 {
		t.Fatalf("CustomAgentsList len = %d, want 1", len(m.CustomAgentsList))
	}

	// Press 'd' -> ScreenCustomAgentDelete
	m.Cursor = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	state := updated.(Model)
	if state.Screen != ScreenCustomAgentDelete || state.CustomAgentDeleteTarget != "dummy-agent" {
		t.Fatalf("state unexpected on delete trigger: %+v", state)
	}

	// Cancel (cursor 1)
	state.Cursor = 1
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)
	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen after cancel = %v, want ScreenCustomAgents", state.Screen)
	}

	// Confirm delete (cursor 0)
	state.Cursor = 0
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	state = updated.(Model)
	state.Cursor = 0
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)

	if state.Screen != ScreenCustomAgents || len(state.CustomAgentsList) != 0 {
		t.Fatalf("deletion not completed cleanly in state: %+v", state)
	}

	if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
		t.Errorf("skill file still exists: %s", skillFile)
	}
}

func TestCustomAgents_DeleteTargetRemovedExternally(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Seed custom registry with an entry
	regPath := filepath.Join(tempHome, ".config", "gentle-ai", "custom-agents.json")
	if err := os.MkdirAll(filepath.Dir(regPath), 0755); err != nil {
		t.Fatalf("MkdirAll regPath: %v", err)
	}

	claudeSkills := filepath.Join(tempHome, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0755); err != nil {
		t.Fatalf("MkdirAll claudeSkills: %v", err)
	}

	reg := &agentbuilder.Registry{
		Version: 1,
		Agents: []agentbuilder.RegistryEntry{
			{
				Name:             "external-agent",
				Title:            "External Agent",
				CreatedAt:        time.Now(),
				GenerationEngine: model.AgentClaudeCode,
				InstalledAgents:  []model.AgentID{model.AgentClaudeCode},
			},
		},
	}
	if err := agentbuilder.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	agent := &agentbuilder.GeneratedAgent{Name: "external-agent", Title: "External Agent", Content: "# External\n"}
	adapters := []agentbuilder.AdapterInfo{{AgentID: model.AgentClaudeCode, SkillsDir: claudeSkills}}
	if _, err := agentbuilder.Install(agent, adapters, ""); err != nil {
		t.Fatalf("Install: %v", err)
	}

	m := NewModel(system.DetectionResult{}, "dev")
	m.setScreen(ScreenCustomAgents)
	if len(m.CustomAgentsList) != 1 {
		t.Fatalf("CustomAgentsList len = %d, want 1", len(m.CustomAgentsList))
	}

	// Now remove the entry externally before user confirms deletion
	emptyReg := &agentbuilder.Registry{Version: 1, Agents: []agentbuilder.RegistryEntry{}}
	if err := agentbuilder.SaveRegistry(regPath, emptyReg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	// User was on ScreenCustomAgentDelete for "external-agent"
	m.Screen = ScreenCustomAgentDelete
	m.CustomAgentDeleteTarget = "external-agent"
	m.Cursor = 0 // "Delete Agent"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.Screen != ScreenCustomAgents {
		t.Fatalf("screen = %v, want ScreenCustomAgents when target missing", state.Screen)
	}
	if state.CustomAgentsErr != nil {
		t.Errorf("expected nil error on missing entry, got %v", state.CustomAgentsErr)
	}
	if len(state.CustomAgentsList) != 0 {
		t.Errorf("expected empty list refreshed from disk, got len %d", len(state.CustomAgentsList))
	}

	// File should remain intact because target was missing from registry
	skillFile := filepath.Join(claudeSkills, "external-agent", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Errorf("skill file should remain untouched when entry was missing from registry: %v", err)
	}
}

func TestCustomAgents_CreateNewAgentNavigatesToBuilder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := NewModel(system.DetectionResult{}, "dev")
	m.AgentBuilder.AvailableEngines = []model.AgentID{model.AgentClaudeCode}

	m.setScreen(ScreenCustomAgents)
	m.Cursor = 0 // "Create new agent"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).Screen != ScreenAgentBuilderEngine {
		t.Fatalf("screen = %v, want ScreenAgentBuilderEngine", updated.(Model).Screen)
	}
}
