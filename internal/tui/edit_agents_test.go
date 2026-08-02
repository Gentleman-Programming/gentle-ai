package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func editAgentsModel() Model {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenEditAgents
	return m
}

func TestEditAgents_WelcomeDispatchesScreenEditAgents(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen, m.Cursor = ScreenWelcome, 11
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(Model).Screen; got != ScreenEditAgents {
		t.Fatalf("screen = %v, want ScreenEditAgents", got)
	}
}

func TestEditAgents_SpaceTogglesCursorAgent(t *testing.T) {
	m := editAgentsModel()
	want := screens.AgentOptions()[0]
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !slices.Contains(updated.(Model).EditAgentsSelection, want) {
		t.Fatalf("agent %s was not toggled", want)
	}
}

func TestEditAgents_ConfirmPassesExactAgentSetsToSync(t *testing.T) {
	m := editAgentsModel()
	m.Selection.Agents = []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}
	m.EditAgentsSelection = []model.AgentID{model.AgentClaudeCode}
	var captured *model.SyncOverrides
	m.SyncFn = func(overrides *model.SyncOverrides) ([]string, error) { captured = overrides; return nil, nil }
	m.Cursor = len(screens.AgentOptions())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.Screen != ScreenSync {
		t.Fatalf("screen = %v, want ScreenSync", got.Screen)
	}
	_, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = findSyncDoneMsgInBatch(t, cmd)
	if captured == nil || !captured.PersistInstalledAgents || !slices.Equal(captured.TargetAgents, []model.AgentID{model.AgentClaudeCode}) || !slices.Equal(captured.DeselectedAgents, []model.AgentID{model.AgentOpenCode}) {
		t.Fatalf("sync overrides = %+v, want exact target [claude-code] and deselected [opencode]", captured)
	}
}

func TestEditAgents_CancelReturnsToWelcome(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cursor int
		key    tea.KeyMsg
	}{{"back", len(screens.AgentOptions()) + 1, tea.KeyMsg{Type: tea.KeyEnter}}, {"escape", 0, tea.KeyMsg{Type: tea.KeyEsc}}} {
		t.Run(tt.name, func(t *testing.T) {
			m := editAgentsModel()
			m.Cursor = tt.cursor
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).Screen; got != ScreenWelcome {
				t.Fatalf("screen = %v, want ScreenWelcome", got)
			}
		})
	}
}

func TestEditAgents_SelectionCommitsOnlyAfterSuccessfulSync(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want model.AgentID
	}{{"success", nil, model.AgentClaudeCode}, {"failure", errors.New("sync failed"), model.AgentOpenCode}} {
		t.Run(tt.name, func(t *testing.T) {
			m := editAgentsModel()
			m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
			m.PendingAgentSelection = []model.AgentID{model.AgentClaudeCode}
			m.Screen = ScreenSync
			updated, _ := m.Update(SyncDoneMsg{Err: tt.err})
			got := updated.(Model)
			if !slices.Equal(got.Selection.Agents, []model.AgentID{tt.want}) || got.PendingAgentSelection != nil {
				t.Fatalf("agents = %v, pending = %v", got.Selection.Agents, got.PendingAgentSelection)
			}
		})
	}
}

func TestEditAgents_EmptySelectionDoesNotSync(t *testing.T) {
	m := editAgentsModel()
	m.Cursor = len(screens.AgentOptions())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.Screen != ScreenEditAgents || got.PendingSyncOverrides != nil {
		t.Fatalf("empty confirmation changed state: screen=%v overrides=%+v", got.Screen, got.PendingSyncOverrides)
	}
}

func TestEditAgents_RendersOnNarrowScreen(t *testing.T) {
	updated, _ := editAgentsModel().Update(tea.WindowSizeMsg{Width: 45, Height: 20})
	if view := updated.(Model).View(); !strings.Contains(view, "Edit Installed Agents") {
		t.Fatalf("narrow view omitted edit screen: %q", view)
	}
}

func TestEditAgents_CancelledDraftNeverCommitsOnLaterSync(t *testing.T) {
	m := editAgentsModel()
	m.Screen = ScreenSync
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
	m.PendingAgentSelection = []model.AgentID{model.AgentClaudeCode}
	m.PendingSyncOverrides = &model.SyncOverrides{TargetAgents: []model.AgentID{model.AgentClaudeCode}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	updated, _ = got.Update(SyncDoneMsg{})
	got = updated.(Model)
	if got.PendingAgentSelection != nil || got.PendingSyncOverrides != nil || !slices.Equal(got.Selection.Agents, []model.AgentID{model.AgentOpenCode}) {
		t.Fatalf("cancelled draft leaked: agents=%v pending=%v overrides=%+v", got.Selection.Agents, got.PendingAgentSelection, got.PendingSyncOverrides)
	}
}
