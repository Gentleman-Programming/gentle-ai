package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func settleReviewMode(t *testing.T, model Model, cmd tea.Cmd) Model {
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		message = batch[0]()
	}
	updated, _ := model.Update(message)
	return updated.(Model)
}
func TestReviewModeTUI(t *testing.T) {
	mode := func(global, local, effective reviewtransaction.RDDMode, source reviewtransaction.RDDModeSource) reviewtransaction.RDDModeStatus {
		return reviewtransaction.RDDModeStatus{Schema: reviewtransaction.RDDModeStatusSchema, Global: global, CloneLocal: local, Effective: effective, Source: source}
	}
	current := mode(reviewtransaction.RDDModeOff, reviewtransaction.RDDModeUnset, reviewtransaction.RDDModeOff, reviewtransaction.RDDModeSourceDefault)
	m, mutations := NewModel(system.DetectionResult{}, "dev"), []bool{}
	m.ReviewModeCwdFn = func() (string, error) { return "/repo", nil }
	m.ReviewModeStatusFn = func(context.Context, string) (reviewtransaction.RDDModeStatus, error) { return current, nil }
	m.ReviewModeSetGlobalFn = func(_ context.Context, _ string, enabled bool) (reviewtransaction.RDDModeStatus, error) {
		mutations = append(mutations, enabled)
		if enabled {
			current = mode(reviewtransaction.RDDModeOn, reviewtransaction.RDDModeOff, reviewtransaction.RDDModeOff, reviewtransaction.RDDModeSourceCloneLocal)
		} else {
			current = mode(reviewtransaction.RDDModeOff, reviewtransaction.RDDModeUnset, reviewtransaction.RDDModeOff, reviewtransaction.RDDModeSourceGlobal)
		}
		return current, nil
	}
	open := func(model Model) Model {
		model.Cursor = len(screens.WelcomeOptions(model.UpdateResults, model.UpdateCheckDone, false, 0, true)) - 4
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return settleReviewMode(t, updated.(Model), cmd)
	}
	m = open(m)
	if view := m.View(); !strings.Contains(view, "RDD runs a bounded review before delivery and records review evidence.") || !strings.Contains(view, "Delivery remains governed by repository policy, including required checks and branch protection.") || !strings.Contains(view, "Individual clones can override this global setting.") || !strings.Contains(view, "RDD is currently DISABLED globally.") || !strings.Contains(view, "Do you want to enable RDD globally?") || !strings.Contains(view, "Enable globally") || !strings.Contains(view, "Back") || !strings.Contains(view, "esc: back") || strings.Contains(view, "Continue") || strings.Contains(view, "Global:") || strings.Contains(view, "Clone-local:") || strings.Contains(view, "Effective:") || strings.Contains(view, "Decided by:") || strings.Contains(view, "Disable globally") {
		t.Fatalf("disabled view was not state-aware:\n%s", view)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = settleReviewMode(t, updated.(Model), cmd)
	if m.Screen != ScreenWelcome || len(mutations) != 1 || !mutations[0] {
		t.Fatalf("enable = screen %v mutations %v", m.Screen, mutations)
	}
	m = open(m)
	if view := m.View(); !strings.Contains(view, "RDD is currently ENABLED globally.") || !strings.Contains(view, "Do you want to disable RDD globally?") || !strings.Contains(view, "Disable globally") || !strings.Contains(view, "Back") || strings.Contains(view, "Enable globally") || strings.Contains(view, "Decided by:") {
		t.Fatalf("enabled view was not state-aware:\n%s", view)
	}
	m.Cursor = 1
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.Screen != ScreenWelcome || len(mutations) != 1 {
		t.Fatalf("back = screen %v cmd %v mutations %v", m.Screen, cmd, mutations)
	}
	m = open(m)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || m.Screen != ScreenWelcome || len(mutations) != 1 {
		t.Fatalf("esc back = screen %v cmd %v mutations %v", m.Screen, cmd, mutations)
	}
	m = open(m)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = settleReviewMode(t, updated.(Model), cmd)
	if m.Screen != ScreenWelcome || len(mutations) != 2 || mutations[1] {
		t.Fatalf("disable = screen %v mutations %v", m.Screen, mutations)
	}
	m.ReviewModeStatusFn = func(context.Context, string) (reviewtransaction.RDDModeStatus, error) {
		return current, errors.New("not a repository")
	}
	m = open(m)
	if view := m.View(); !strings.Contains(view, "not a repository") || !strings.Contains(view, "Back") || strings.Contains(view, "globally") {
		t.Fatalf("load error was not safely actionable:\n%s", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	m.ReviewModeStatusFn = func(context.Context, string) (reviewtransaction.RDDModeStatus, error) { return current, nil }
	m.ReviewModeSetGlobalFn = func(context.Context, string, bool) (reviewtransaction.RDDModeStatus, error) {
		return reviewtransaction.RDDModeStatus{}, errors.New("revision conflict")
	}
	m = open(m)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = settleReviewMode(t, updated.(Model), cmd)
	if view := m.View(); m.Screen != ScreenReviewMode || !strings.Contains(view, "revision conflict") || !strings.Contains(view, "RDD is currently DISABLED globally.") || !strings.Contains(view, "Enable globally") || !strings.Contains(view, "Back") || strings.Contains(view, "Disable globally") {
		t.Fatalf("mutation error was hidden or not retryable:\n%s", view)
	}
}
