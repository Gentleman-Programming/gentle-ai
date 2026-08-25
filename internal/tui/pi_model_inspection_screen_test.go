package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
	"testing"
)

func piScreenModel() Model {
	m := NewModel(system.DetectionResult{}, "test")
	m.Selection.Agents = []model.AgentID{model.AgentPi}
	m.Screen = ScreenModelConfig
	return m
}
func piKey(m Model, msg tea.Msg) Model { updated, _ := m.Update(msg); return updated.(Model) }
func TestPiModelConfigGateBackAndEnterLoading(t *testing.T) {
	original := piModelInspectionCmd
	t.Cleanup(func() { piModelInspectionCmd = original })
	var gotContext context.Context
	var gotID uint64
	piModelInspectionCmd = func(ctx context.Context, id uint64) tea.Cmd {
		gotContext, gotID = ctx, id
		return func() tea.Msg { return piModelInspectionMsg{requestID: id} }
	}
	m := piScreenModel()
	m.Selection.Agents, m.Cursor = nil, 4
	if m.piModelsAvailable() || len(screens.ModelConfigOptions(false)) != 5 {
		t.Fatal("non-Pi gate or option order changed")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).Screen != ScreenWelcome {
		t.Fatal("non-Pi Back no longer returns to Welcome")
	}
	m = piScreenModel()
	if !m.piModelsAvailable() || len(screens.ModelConfigOptions(true)) != 6 {
		t.Fatal("selected Pi did not add one option")
	}
	m.Cursor = 4
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.Screen != ScreenPiModelInspection || state.PiModelInspection.Status != screens.PiModelInspectionLoading || state.piModelInspectionRequest != 1 || cmd == nil || gotContext == nil || gotID != 1 {
		t.Fatalf("enter = screen %v state=%#v cmd=%v context=%v id=%d", state.Screen, state.PiModelInspection, cmd != nil, gotContext, gotID)
	}
	if !NewModel(system.DetectionResult{Configs: []system.ConfigState{{Agent: string(model.AgentPi), Exists: true}}}, "test").piModelsAvailable() {
		t.Fatal("detected Pi config did not enable option")
	}
}
func TestPiModelInspectionMessagesBackReentryAndKeys(t *testing.T) {
	m := piScreenModel()
	m.beginPiModelInspection()
	firstID := m.piModelInspectionRequest
	updated, _ := m.Update(piModelInspectionMsg{requestID: firstID})
	m = updated.(Model)
	if m.PiModelInspection.Status != screens.PiModelInspectionSuccess {
		t.Fatal("current success message was ignored")
	}
	before := m.Selection.Agents[0]
	m.leavePiModelInspection()
	if m.Screen != ScreenModelConfig || m.PiModelInspection.Status != 0 || len(m.Selection.Agents) != 1 || m.Selection.Agents[0] != before {
		t.Fatal("back did not reset state without changing selection")
	}
	m.beginPiModelInspection()
	secondID := m.piModelInspectionRequest
	updated, _ = m.Update(piModelInspectionMsg{requestID: firstID, inspection: pi.ModelRoutingInspection{Contract: "stale"}})
	m = updated.(Model)
	if m.PiModelInspection.Status != screens.PiModelInspectionLoading {
		t.Fatal("stale message changed re-entered state")
	}
	updated, _ = m.Update(piModelInspectionMsg{requestID: secondID, err: errors.New("current")})
	m = updated.(Model)
	if m.PiModelInspection.Status != screens.PiModelInspectionError {
		t.Fatal("current error message was ignored")
	}
	m.PiModelInspection.SetResult(pi.ModelRoutingInspection{Targets: map[pi.ModelRoutingTarget]pi.ModelRoutingTargetInspection{pi.ModelRoutingTargetProject: {}}, Agents: []pi.ModelRoutingAgent{{Name: "a", Configurable: true}, {Name: "b", Configurable: true}}}, nil)
	m = piKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.PiModelInspection.Cursor != 1 {
		t.Fatal("j did not move inspection cursor")
	}
	m = piKey(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.PiModelInspection.Cursor != 0 {
		t.Fatal("up did not move inspection cursor")
	}
	m = piKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.PiModelInspection.Target != pi.ModelRoutingTargetGlobal {
		t.Fatal("g did not select global")
	}
	m = piKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.PiModelInspection.Target != pi.ModelRoutingTargetProject {
		t.Fatal("p did not select project")
	}
	m = piKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.PiModelInspection.Target != pi.ModelRoutingTargetGlobal {
		t.Fatal("tab did not switch target")
	}
}
