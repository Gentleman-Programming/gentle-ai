package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v5/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

func checkingUpdateModel(t *testing.T) Model {
	t.Helper()
	install := shellinstaller.UserShellInstall{Target: "/target", Mode: "separate", Channel: "stable"}
	m := NewModel(system.DetectionResult{}, "update-test").WithShellInstaller()
	m.Width, m.Height = 120, 35
	m.Screen = ScreenShellInstall
	m.shellUpdateGeneration = 7
	m.shellUpdate = &shellUpdateState{install: install, info: shellinstaller.UserShellUpdate{Channel: "stable"}, checking: true, generation: 7}
	return m
}

// A result from an abandoned check never lands on a newer check.
func TestShellUpdateIgnoresStaleCheckResult(t *testing.T) {
	m := checkingUpdateModel(t)
	next, _ := m.Update(shellUpdateResult{generation: 6, info: shellinstaller.UserShellUpdate{Available: true, Components: []string{"stale"}}})
	m = next.(Model)
	if !m.shellUpdate.checking || m.shellUpdate.info.Available {
		t.Fatalf("stale result applied: %+v", m.shellUpdate)
	}
	next, _ = m.Update(shellUpdateResult{generation: 7, info: shellinstaller.UserShellUpdate{Channel: "stable", Available: true, Components: []string{"current"}}})
	m = next.(Model)
	if m.shellUpdate.checking || !m.shellUpdate.info.Available {
		t.Fatalf("current result ignored: %+v", m.shellUpdate)
	}
}

// Reopening the check after Esc issues a new generation.
func TestShellUpdateReopenUsesNewGeneration(t *testing.T) {
	m := checkingUpdateModel(t)
	config := shellExperienceConfig{existing: []shellinstaller.UserShellInstall{m.shellUpdate.install}}
	m.shellUpdate.config = config
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, cmd := m.beginShellUpdate(config)
	m = next.(Model)
	if cmd == nil || m.shellUpdate.generation == 7 || m.shellUpdate.generation != m.shellUpdateGeneration {
		t.Fatalf("reopened check reused the generation: %+v", m.shellUpdate)
	}
}

// A failed check offers Retry, which starts a new check.
func TestShellUpdateFailedCheckOffersRetry(t *testing.T) {
	m := checkingUpdateModel(t)
	next, _ := m.Update(shellUpdateResult{generation: 7, err: errors.New("network unreachable")})
	m = next.(Model)
	view := m.View()
	for _, part := range []string{"Update check failed", "network unreachable", "Retry", "Back"} {
		if !strings.Contains(view, part) {
			t.Fatalf("failed check screen omits %q:\n%s", part, view)
		}
	}
	if strings.Contains(view, "Up to date") {
		t.Fatalf("failed check claims the installation is up to date:\n%s", view)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.shellUpdate == nil || !m.shellUpdate.checking || m.shellUpdate.err != nil || m.shellUpdate.generation == 7 {
		t.Fatalf("Retry did not start a new check: %+v", m.shellUpdate)
	}
	// Back stays the second action.
	next, _ = m.Update(shellUpdateResult{generation: m.shellUpdate.generation, err: errors.New("again")})
	m = next.(Model)
	m = key(m, tea.KeyMsg{Type: tea.KeyDown})
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellUpdate != nil {
		t.Fatal("Back did not leave the failed check")
	}
}
