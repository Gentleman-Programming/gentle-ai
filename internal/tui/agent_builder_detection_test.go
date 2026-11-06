package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// Issue #268: View and optionCount run on every frame and keypress, so they
// must reuse the engine availability detected once instead of probing PATH.
func TestWelcomeRenderDoesNotReprobeAgentBuilderEngines(t *testing.T) {
	probes := 0
	original := agentBuilderEngineAvailableFn
	agentBuilderEngineAvailableFn = func(id model.AgentID) bool {
		probes++
		return id == model.AgentClaudeCode
	}
	t.Cleanup(func() { agentBuilderEngineAvailableFn = original })

	m := NewModel(system.DetectionResult{}, "test")
	afterConstruction := probes

	var current tea.Model = m
	for range 5 {
		_ = current.View()
		_ = current.(Model).optionCount()
		current, _ = current.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	if probes != afterConstruction {
		t.Fatalf("PATH probes after construction = %d, want 0 (render/navigation must reuse detection)", probes-afterConstruction)
	}
	if !current.(Model).hasAgentBuilderEngines() {
		t.Fatalf("hasAgentBuilderEngines() = false, want true from the detected claude engine")
	}
}

// Starting agent creation is an explicit user action, so it re-detects and
// picks up an engine installed after the TUI started.
func TestCreateAgentRefreshesAgentBuilderEngines(t *testing.T) {
	available := false
	original := agentBuilderEngineAvailableFn
	agentBuilderEngineAvailableFn = func(id model.AgentID) bool {
		return available && id == model.AgentCodex
	}
	t.Cleanup(func() { agentBuilderEngineAvailableFn = original })

	m := NewModel(system.DetectionResult{}, "test")
	if m.hasAgentBuilderEngines() {
		t.Fatalf("hasAgentBuilderEngines() = true before any engine is installed")
	}

	available = true
	m.setScreen(ScreenCustomAgents)
	m.Cursor = len(m.CustomAgentsList)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if got.Screen != ScreenAgentBuilderEngine {
		t.Fatalf("screen = %v, want ScreenAgentBuilderEngine after refresh", got.Screen)
	}
	if len(got.AgentBuilder.AvailableEngines) != 1 || got.AgentBuilder.AvailableEngines[0] != model.AgentCodex {
		t.Fatalf("AvailableEngines = %v, want [codex]", got.AgentBuilder.AvailableEngines)
	}
}
