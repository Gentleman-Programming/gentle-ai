package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/screens"
)

type flowAction struct {
	key       tea.KeyMsg
	cursor    int
	setCursor bool
	prepare   func(Model) Model
	runCmd    bool
}

func TestPresetSelectionNextScreenFlowMatrix(t *testing.T) {
	for _, tt := range []struct {
		name   string
		agents []model.AgentID
		preset model.PresetID
		want   Screen
	}{
		{"full OpenCode", []model.AgentID{model.AgentOpenCode}, model.PresetFullGentleman, ScreenOpenCodePlugins},
		{"ecosystem OpenCode", []model.AgentID{model.AgentOpenCode}, model.PresetEcosystemOnly, ScreenOpenCodePlugins},
		{"minimal OpenCode", []model.AgentID{model.AgentOpenCode}, model.PresetMinimal, ScreenOpenCodePlugins},
		{"custom OpenCode", []model.AgentID{model.AgentOpenCode}, model.PresetCustom, ScreenDependencyTree},
		{"full Cursor", []model.AgentID{model.AgentCursor}, model.PresetFullGentleman, ScreenDependencyTree},
		{"ecosystem Cursor", []model.AgentID{model.AgentCursor}, model.PresetEcosystemOnly, ScreenDependencyTree},
		{"minimal Cursor", []model.AgentID{model.AgentCursor}, model.PresetMinimal, ScreenDependencyTree},
		{"custom Cursor", []model.AgentID{model.AgentCursor}, model.PresetCustom, ScreenDependencyTree},
		{"full picker agents", []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE, model.AgentCodex, model.AgentOpenCode}, model.PresetFullGentleman, ScreenOpenCodePlugins},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen, m.Selection.Agents = ScreenPreset, tt.agents
			m.Cursor = presetCursor(t, tt.preset)
			state := applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
			if state.Screen != tt.want {
				t.Fatalf("screen = %v, want %v", state.Screen, tt.want)
			}
			title := "Install Plan"
			if tt.preset == model.PresetCustom {
				title = "Select Components"
			} else if tt.want == ScreenOpenCodePlugins {
				title = "Optional OpenCode Community Plugins"
			}
			assertInstallerView(t, state, title)
			if tt.want == ScreenDependencyTree && tt.preset != model.PresetCustom {
				assertViewSections(t, state.View(), "Components to install", "included", "Continue", "Back")
			}
			if tt.preset == model.PresetCustom {
				assertInstallerSnapshot(t, state, customComponentsSnapshot)
			} else if tt.preset == model.PresetMinimal && tt.agents[0] == model.AgentCursor {
				assertInstallerSnapshot(t, state, minimalPlanSnapshot)
			} else if tt.want == ScreenOpenCodePlugins {
				assertInstallerSnapshot(t, state, openCodePluginsSnapshot)
			}
			state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}})
			if state.Screen != ScreenPreset {
				t.Fatalf("back screen = %v, want Preset", state.Screen)
			}
		})
	}
}

func TestCustomPresetPostComponentFlowMatrix(t *testing.T) {
	for _, tt := range []struct {
		name       string
		agent      model.AgentID
		components []model.ComponentID
		want       Screen
	}{
		{"OpenCode Engram", model.AgentOpenCode, []model.ComponentID{model.ComponentEngram}, ScreenOpenCodePlugins},
		{"OpenCode Skills", model.AgentOpenCode, []model.ComponentID{model.ComponentSkills}, ScreenOpenCodePlugins},
		{"Cursor Skills", model.AgentCursor, []model.ComponentID{model.ComponentSkills}, ScreenSkillPicker},
		{"Cursor Engram", model.AgentCursor, []model.ComponentID{model.ComponentEngram}, ScreenInstallReviewMode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen, m.Selection.Preset = ScreenDependencyTree, model.PresetCustom
			m.Selection.Agents, m.Selection.Components = []model.AgentID{tt.agent}, tt.components
			m.Cursor = len(screens.AllComponents())
			m = installReviewModeStatusFixture(m)
			state := applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
			if state.Screen != tt.want {
				t.Fatalf("screen = %v, want %v", state.Screen, tt.want)
			}
			if tt.want == ScreenOpenCodePlugins {
				assertInstallerSnapshot(t, state, openCodePluginsSnapshot)
			} else if tt.want == ScreenSkillPicker {
				assertInstallerSnapshot(t, state, skillPickerSnapshot)
			} else {
				assertInstallerView(t, state, "Receipt-Driven Development")
				assertViewSections(t, state.View(), "RDD is ON by default", "Enable RDD", "Disable RDD", "Back")
			}
			if tt.want == ScreenInstallReviewMode {
				state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}})
				if state.Screen != ScreenDependencyTree {
					t.Fatalf("back screen = %v, want DependencyTree", state.Screen)
				}
			} else {
				state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}})
				if state.Screen != ScreenDependencyTree {
					t.Fatalf("back screen = %v, want DependencyTree", state.Screen)
				}
			}
		})
	}
}

func TestInstallNavigationRoundTrips(t *testing.T) {
	for _, tt := range []struct {
		name       string
		agent      model.AgentID
		preset     model.PresetID
		components []model.ComponentID
		forward    []Screen
		reverse    []Screen
	}{
		{"minimal Cursor", model.AgentCursor, model.PresetMinimal, nil, []Screen{ScreenDependencyTree}, []Screen{ScreenPreset}},
		{"full Cursor", model.AgentCursor, model.PresetFullGentleman, nil, []Screen{ScreenDependencyTree}, []Screen{ScreenPreset}},
		{"minimal OpenCode", model.AgentOpenCode, model.PresetMinimal, nil, []Screen{ScreenOpenCodePlugins, ScreenDependencyTree}, []Screen{ScreenOpenCodePlugins, ScreenPreset}},
		{"full OpenCode", model.AgentOpenCode, model.PresetFullGentleman, nil, []Screen{ScreenOpenCodePlugins, ScreenDependencyTree}, []Screen{ScreenOpenCodePlugins, ScreenPreset}},
		{"custom Cursor Skills", model.AgentCursor, model.PresetCustom, []model.ComponentID{model.ComponentSDD, model.ComponentSkills}, []Screen{ScreenSkillPicker}, []Screen{ScreenDependencyTree}},
		{"custom OpenCode Skills", model.AgentOpenCode, model.PresetCustom, []model.ComponentID{model.ComponentSDD, model.ComponentSkills}, []Screen{ScreenOpenCodePlugins, ScreenSkillPicker}, []Screen{ScreenDependencyTree}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Selection.Agents = []model.AgentID{tt.agent}
			m.Selection.Preset, m.Selection.Components = tt.preset, tt.components
			if tt.preset == model.PresetCustom {
				m.Screen, m.Cursor = ScreenDependencyTree, len(screens.AllComponents())
			} else {
				m.Screen, m.Cursor = ScreenPreset, presetCursor(t, tt.preset)
			}
			for i, want := range tt.forward {
				if i > 0 && m.Screen == ScreenOpenCodePlugins {
					m.Cursor = len(opencodepluginDefinitions()) * 2
				}
				m = applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
				if m.Screen != want {
					t.Fatalf("forward %d = %v, want %v", i, m.Screen, want)
				}
				if want == ScreenSkillPicker {
					assertInstallerSnapshot(t, m, skillPickerSnapshot)
				}
			}
			for i, want := range tt.reverse {
				m = applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}})
				if m.Screen != want {
					t.Fatalf("back %d = %v, want %v", i, m.Screen, want)
				}
			}
		})
	}
}

func TestPiOnlyDependencyTreeBackReturnsToAgentSelection(t *testing.T) {
	for _, tt := range []struct {
		name string
		back flowAction
	}{
		{"Enter on Back", flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}, cursor: 1, setCursor: true}},
		{"Esc", flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = ScreenAgents
			m.Selection.Agents = []model.AgentID{model.AgentPi}
			m.Selection.Components = componentsForPreset(model.PresetFullGentleman, model.PersonaGentleman)
			m.Cursor = len(screens.AgentOptions())
			state := applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
			if state.Screen != ScreenDependencyTree {
				t.Fatalf("screen = %v, want DependencyTree", state.Screen)
			}
			state = applyFlowAction(t, state, tt.back)
			if state.Screen != ScreenAgents {
				t.Fatalf("back screen = %v, want Agents", state.Screen)
			}
			state.Cursor = len(screens.AgentOptions())
			state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
			if state.Screen != ScreenDependencyTree {
				t.Fatalf("round-trip screen = %v, want DependencyTree", state.Screen)
			}
		})
	}
}

func TestCustomEngramRDDChoiceReviewBack(t *testing.T) {
	m := installReviewModeStatusFixture(NewModel(system.DetectionResult{}, "dev"))
	m.Screen, m.Selection.Preset = ScreenDependencyTree, model.PresetCustom
	m.Selection.Agents = []model.AgentID{model.AgentCursor}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram}
	m.Cursor = len(screens.AllComponents())
	state := applyFlowAction(t, m, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}, runCmd: true})
	if state.Screen != ScreenInstallReviewMode {
		t.Fatalf("component continue = %v, want RDD choice", state.Screen)
	}
	assertViewSections(t, state.View(), "Receipt-Driven Development", "Enable RDD", "Disable RDD", "Back")
	state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}, cursor: 1, setCursor: true})
	if state.Screen != ScreenReview || !state.InstallReviewModeChoiceSet || state.InstallReviewModeEnabled {
		t.Fatalf("RDD OFF choice = screen %v, set %v, enabled %v; want Review and OFF", state.Screen, state.InstallReviewModeChoiceSet, state.InstallReviewModeEnabled)
	}
	assertViewSections(t, state.View(), "Review and Confirm", "Receipt-Driven Development", "RDD OFF", "engram", "Install", "Back")
	if strings.Contains(state.View(), "Strict TDD") {
		t.Fatalf("removed choice appeared in review:\n%s", state.View())
	}
	state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEsc}})
	if state.Screen != ScreenInstallReviewMode || state.Cursor != 1 {
		t.Fatalf("review Esc = screen %v cursor %d, want RDD choice with OFF selected", state.Screen, state.Cursor)
	}
	state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}})
	if state.Screen != ScreenReview {
		t.Fatalf("re-confirm RDD OFF = %v, want Review", state.Screen)
	}
	state = applyFlowAction(t, state, flowAction{key: tea.KeyMsg{Type: tea.KeyEnter}, cursor: 1, setCursor: true})
	if state.Screen != ScreenDependencyTree {
		t.Fatalf("review Back = %v, want component selection", state.Screen)
	}
}

func installReviewModeStatusFixture(state Model) Model {
	state.ReviewModeCwdFn = func() (string, error) { return "/isolated-repo", nil }
	state.ReviewModeStatusFn = func(context.Context, string) (reviewtransaction.RDDModeStatus, error) {
		return reviewtransaction.RDDModeStatus{Schema: reviewtransaction.RDDModeStatusSchema, Global: reviewtransaction.RDDModeUnset}, nil
	}
	return state
}

func applyFlowAction(t *testing.T, state Model, action flowAction) Model {
	t.Helper()
	if action.prepare != nil {
		state = action.prepare(state)
	}
	if action.setCursor {
		state.Cursor = action.cursor
	}
	updated, cmd := state.Update(action.key)
	state = updated.(Model)
	if action.runCmd && cmd != nil {
		updated, _ = state.Update(cmd())
		state = updated.(Model)
	}
	return state
}

// These snapshots inline representative screens because the prior Strict TDD
// golden fixtures are stale and outside this work unit's edit surface.
const customComponentsSnapshot = `Select Components

Toggle components with enter or space.

▸ [ ] engram
    Persistent cross-session memory
  [ ] skills
    Curated coding skill library
  [ ] context7
    Latest framework and library docs
  [ ] persona
    Managed agent behavior and conversation tone
  [ ] permissions
    Security-first defaults and guardrails
  [ ] gga
    Gentleman Guardian Angel — AI provider switcher
  [ ] theme
    Visual polish: OpenCode color theme
  [ ] claude-theme
    Visual polish: Gentleman and Gentleman Cute theme assets
  [ ] opencode-gentle-logo
    Visual polish: OpenCode home logo plugin

  Continue
  Back

j/k: navigate • space/enter: toggle • esc: back
`

const minimalPlanSnapshot = `Install Plan

Components to install
  1. persona included
     Managed agent behavior and conversation tone
  2. engram included
     Persistent cross-session memory

▸ Continue
  Back

j/k: navigate • enter: select • esc: back
`

const openCodePluginsSnapshot = `╔═══════════════════════════════════════════════════════════════════════════════════╗
║                                                                                   ║
║  Optional OpenCode Community Plugins                                              ║
║                                                                                   ║
║  Install community TUI plugins now, or open their repos first to review them.     ║
║                                                                                   ║
║  > [ ] Sub-agent Statusline — OpenCode sidebar/statusline for sub-agent activity  ║
║    View repo: https://github.com/Joaquinvesapa/sub-agent-statusline               ║
║    Continue                                                                       ║
║    Back                                                                           ║
║                                                                                   ║
║  space/enter: toggle • repo row: open browser • esc: back                         ║
║                                                                                   ║
╚═══════════════════════════════════════════════════════════════════════════════════╝
`

const skillPickerSnapshot = `Select Skills

Toggle skills with enter or space. All are pre-selected by default.

Review Skills
▸ [x] Judgment Day

Foundation Skills
  [x] Go Testing
  [x] Gentle AI Bench
  [x] Skill Creator
  [x] Skill Improver
  [x] Branch & PR
  [x] Issue Creation
  [x] Skill Registry
  [x] Chained PR
  [x] Cognitive Doc Design
  [x] Comment Writer
  [x] Work Unit Commits
  [x] RDD Defect Workflow
  [x] Systemic Issue Triage

  Continue
  Back

j/k: navigate • space/enter: toggle • esc: back
`

func assertInstallerSnapshot(t *testing.T, state Model, want string) {
	t.Helper()
	if got := state.View(); got != strings.TrimSuffix(want, "\n") {
		t.Fatalf("screen %v rendering changed:\nwant:\n%s\ngot:\n%s", state.Screen, want, got)
	}
}

func assertViewSections(t *testing.T, view string, sections ...string) {
	t.Helper()
	for _, section := range sections {
		if !strings.Contains(view, section) {
			t.Fatalf("rendering missing %q:\n%s", section, view)
		}
	}
}

func assertInstallerView(t *testing.T, state Model, title string) {
	t.Helper()
	view := state.View()
	if !strings.Contains(view, title) || strings.Contains(view, "STRICT TDD MODE") {
		t.Fatalf("installer view for %v missing %q or retained obsolete choice:\n%s", state.Screen, title, view)
	}
}

func presetCursor(t *testing.T, preset model.PresetID) int {
	t.Helper()
	for idx, option := range screens.PresetOptions() {
		if option == preset {
			return idx
		}
	}
	t.Fatalf("preset %q not found", preset)
	return 0
}
