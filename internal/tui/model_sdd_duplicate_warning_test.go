package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

// TestShouldWarnAboutDuplicateAgents covers the detection helper that fires
// the warning screen when SDD multi-mode is paired with VS Code Copilot
// AND a Claude-format adapter.
func TestShouldWarnAboutDuplicateAgents(t *testing.T) {
	tests := []struct {
		name       string
		agents     []model.AgentID
		components []model.ComponentID
		want       bool
	}{
		{
			name:       "vscode + claude + sdd → warn",
			agents:     []model.AgentID{model.AgentVSCodeCopilot, model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentSDD},
			want:       true,
		},
		{
			name:       "vscode + claude WITHOUT sdd → no warn",
			agents:     []model.AgentID{model.AgentVSCodeCopilot, model.AgentClaudeCode},
			components: nil,
			want:       false,
		},
		{
			name:       "vscode alone + sdd → no warn",
			agents:     []model.AgentID{model.AgentVSCodeCopilot},
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
		{
			name:       "claude alone + sdd → no warn",
			agents:     []model.AgentID{model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
		{
			name:       "opencode + claude + sdd → no warn (no vscode)",
			agents:     []model.AgentID{model.AgentOpenCode, model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
		{
			name:       "vscode + opencode + sdd → no warn (no claude)",
			agents:     []model.AgentID{model.AgentVSCodeCopilot, model.AgentOpenCode},
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
		{
			name:       "no agents at all",
			agents:     nil,
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Selection.Agents = tt.agents
			m.Selection.Components = tt.components
			if got := m.shouldWarnAboutDuplicateAgents(); got != tt.want {
				t.Errorf("shouldWarnAboutDuplicateAgents() = %v, want %v (agents=%v, components=%v)",
					got, tt.want, tt.agents, tt.components)
			}
		})
	}
}

// TestSDDMode_TriggersDuplicateAgentsWarning verifies that selecting SDD
// multi-mode with the VS Code + Claude combination routes to the warning
// screen instead of the normal next screen.
func TestSDDMode_TriggersDuplicateAgentsWarning(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentVSCodeCopilot, model.AgentClaudeCode}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Screen = ScreenSDDMode

	// SDD mode options: [Single, Multi]. Cursor 1 → Multi.
	options := screens.SDDModeOptions()
	multiIdx := -1
	for i, opt := range options {
		if opt == model.SDDModeMulti {
			multiIdx = i
		}
	}
	if multiIdx < 0 {
		t.Fatal("SDDModeMulti option not found in SDDModeOptions()")
	}
	m.Cursor = multiIdx

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenSDDDuplicateAgentsWarning {
		t.Fatalf("after selecting Multi with vscode+claude, screen = %v, want ScreenSDDDuplicateAgentsWarning", state.Screen)
	}
	if state.Selection.SDDMode != model.SDDModeMulti {
		t.Errorf("SDDMode = %v, want %v", state.Selection.SDDMode, model.SDDModeMulti)
	}
}

// TestSDDMode_NoWarningWhenNotDuplicating verifies that the warning is NOT
// shown when the adapter set does not trigger duplication (e.g. VS Code
// without Claude).
func TestSDDMode_NoWarningWhenNotDuplicating(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentVSCodeCopilot}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Selection.Preset = model.PresetMinimal
	m.Screen = ScreenSDDMode

	options := screens.SDDModeOptions()
	multiIdx := -1
	for i, opt := range options {
		if opt == model.SDDModeMulti {
			multiIdx = i
		}
	}
	m.Cursor = multiIdx

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen == ScreenSDDDuplicateAgentsWarning {
		t.Fatalf("warning fired without claude adapter; screen = %v", state.Screen)
	}
}

// TestSDDDuplicateAgentsWarning_ContinueAdvances verifies that pressing
// Enter on "Continue anyway" resumes the normal SDDMode flow.
func TestSDDDuplicateAgentsWarning_ContinueAdvances(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentVSCodeCopilot, model.AgentClaudeCode}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Selection.SDDMode = model.SDDModeMulti
	m.Selection.Preset = model.PresetMinimal
	m.Screen = ScreenSDDDuplicateAgentsWarning
	m.Cursor = 0 // "Continue anyway"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen == ScreenSDDDuplicateAgentsWarning {
		t.Fatal("after 'Continue anyway', screen still ScreenSDDDuplicateAgentsWarning — advance did not fire")
	}
	if state.Screen == ScreenSDDMode {
		t.Fatal("after 'Continue anyway', screen returned to SDDMode — should advance forward")
	}
}

// TestSDDDuplicateAgentsWarning_BackReturnsToSDDMode verifies that the
// "Back" option returns to the SDDMode selection so the user can change
// their mind.
func TestSDDDuplicateAgentsWarning_BackReturnsToSDDMode(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentVSCodeCopilot, model.AgentClaudeCode}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Selection.SDDMode = model.SDDModeMulti
	m.Screen = ScreenSDDDuplicateAgentsWarning
	m.Cursor = 1 // "Back to adapter selection"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenSDDMode {
		t.Fatalf("after 'Back', screen = %v, want ScreenSDDMode", state.Screen)
	}
}

// TestRenderSDDDuplicateAgentsWarning_ListsExpectedPhases verifies that the
// rendered output names the 8 phases that visibly duplicate. Documents the
// list as part of the contract, so a future contributor cannot silently
// drop one without updating the test.
func TestRenderSDDDuplicateAgentsWarning_ListsExpectedPhases(t *testing.T) {
	output := screens.RenderSDDDuplicateAgentsWarning(0)
	expected := []string{
		"sdd-apply", "sdd-archive", "sdd-design", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-tasks", "sdd-verify",
	}
	for _, phase := range expected {
		if !strings.Contains(output, phase) {
			t.Errorf("warning output missing duplicated phase %q", phase)
		}
	}
	// The two phases that don't duplicate must NOT be in the list.
	for _, phase := range []string{"sdd-init", "sdd-onboard"} {
		// Allow them only as part of the prose ("you're installing SDD…"),
		// not as bullet-list entries. The bullet entries are prefixed with "•".
		if strings.Contains(output, "• "+phase) {
			t.Errorf("warning output incorrectly lists non-duplicating phase %q as duplicated", phase)
		}
	}
}
