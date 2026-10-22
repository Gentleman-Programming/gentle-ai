package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestShellInstallerWelcomeOptIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ordinary := NewModel(system.DetectionResult{}, "dev")
	ordinary.Width, ordinary.Height = 80, 30
	if ordinary.optionCount() != 12 || strings.Contains(ordinary.View(), "Install Gentle-Shell, our own agent") {
		t.Fatal("ordinary model changed its menu")
	}
	m := ordinary.WithShellInstaller()
	if m.optionCount() != 13 || !strings.Contains(m.View(), "Install Gentle-Shell, our own agent") {
		t.Fatal("opted-in model does not expose the installer")
	}
	m.Cursor = 0 // The restored campaign is the initial action.
	m.Height = 8
	if !strings.Contains(m.View(), "Install Gentle-Shell, our own agent") {
		t.Fatal("short viewport hides the selected installer action")
	}
	final, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = final.(Model)
	if m.Screen != ScreenShellInstall || cmd != nil || !strings.Contains(m.View(), "Configure Gentle-Shell experience") {
		t.Fatal("welcome starts a program or fails to enter the selector")
	}
	final, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = final.(Model)
	final, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = final.(Model)
	if m.shellPlan == nil || m.shellPlanRequest.Confirmation == "" || m.ShellInstallSelection().Confirmed {
		t.Fatalf("welcome did not reach the unconfirmed installation plan: %+v", m.shellPlanRequest)
	}
	final, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = final.(Model)
	if cmd != nil || m.ShellInstallSelection().Confirmed || m.shellPlan == nil {
		t.Fatal("parent consumed plan input or forged consent")
	}
}

func TestShellInstallerCancelReturnsWelcome(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
		m.Cursor = 0
		final, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = final.(Model)
		final, cmd := m.Update(tea.KeyMsg{Type: key})
		m = final.(Model)
		if m.Screen != ScreenWelcome || cmd != nil || m.ShellInstallSelection().Confirmed {
			t.Fatalf("cancel quits the parent or retains a launch: %+v", m.ShellInstallSelection())
		}
	}
}

func TestShellInstallerConfirmationOnlyQuitsForHostHandoff(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
	selection := cli.ShellInstallSelection{Confirmed: true}
	selection.Request.Destination = "/owned/model-fixture"
	final, cmd := m.finishShellInstallSelection(selection, nil)
	if final.(Model).ShellInstallSelection() != selection || cmd == nil {
		t.Fatal("confirmed model selection was not retained for the host")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmation starts an installer instead of exiting the parent")
	}
	if _, cmd := m.finishShellInstallSelection(selection, errors.New("model fixture failure")); cmd != nil {
		t.Fatal("failed selection authorizes a launch")
	}
	if _, cmd := m.finishShellInstallSelection(cli.ShellInstallSelection{}, nil); cmd != nil {
		t.Fatal("unconfirmed selection exits for a launch")
	}
}

func TestShellInstallerQuitKeepsOrdinaryBehavior(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		m := NewModel(system.DetectionResult{}, "dev")
		if enabled {
			m = m.WithShellInstaller()
		}
		m.Cursor = m.optionCount() - 1
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Quit is unreachable")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("Quit selects a different action")
		}
	}
}
