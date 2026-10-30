package tui

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func forceRootRiskGate(t *testing.T) {
	t.Helper()
	t.Setenv(shellinstaller.RootRiskEnv, "")
	original := shellRootRiskGate
	shellRootRiskGate = func() error { return &shellinstaller.RootRiskRequiredError{} }
	t.Cleanup(func() { shellRootRiskGate = original })
}

func typeRootRiskPhrase(m Model, phrase string) Model {
	for _, r := range phrase {
		if r == ' ' {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			m = next.(Model)
			continue
		}
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func waitForRunner(t *testing.T, called *atomic.Bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the installer runner never started after the typed acceptance")
}

func TestShellPlanRootRiskTypedAcceptance(t *testing.T) {
	forceRootRiskGate(t)
	m, _ := openInstallationPlan(t)

	// Enter on the plan must show the typed warning, never start a worker.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellRootRisk == nil || m.shellInstallRun != nil {
		t.Fatal("root plan confirmation must open the typed acceptance instead of launching")
	}
	view := m.View()
	for _, part := range []string{"Root installation warning", "adduser", shellinstaller.RootRiskPhrase, "Enter: accept"} {
		if !strings.Contains(view, part) {
			t.Fatalf("root warning screen omits %q:\n%s", part, view)
		}
	}

	// A wrong phrase keeps the operator in the prompt without launching.
	m = typeRootRiskPhrase(m, "I accept")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellRootRisk == nil || m.shellRootRisk.input != "" || m.shellInstallRun != nil {
		t.Fatal("a partial phrase must be refused without launching")
	}

	// The exact phrase records the binding and launches the plan install.
	m = typeRootRiskPhrase(m, shellinstaller.RootRiskPhrase)
	var called atomic.Bool
	m.shellInstallRunner = func(_ context.Context, _ shellinstaller.UserInstallRequest, _ func(shellinstaller.UserInstallProgress)) error {
		called.Store(true)
		return nil
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellRootRisk != nil || m.shellInstallRun == nil {
		t.Fatal("the exact phrase must clear the prompt and start the planned install")
	}
	if os.Getenv(shellinstaller.RootRiskEnv) != shellinstaller.RootRiskBinding {
		t.Fatal("accepting a root install must record the binding for the worker")
	}
	waitForRunner(t, &called)
}

func TestShellPlanRootRiskEscapeCancels(t *testing.T) {
	forceRootRiskGate(t)
	m, _ := openInstallationPlan(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.shellRootRisk != nil || m.shellInstallRun != nil {
		t.Fatal("escape must cancel the root prompt without launching")
	}
	if os.Getenv(shellinstaller.RootRiskEnv) == shellinstaller.RootRiskBinding {
		t.Fatal("escape must not record the acceptance binding")
	}
}
