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
	ordinary := NewModel(system.DetectionResult{}, "dev")
	ordinary.Width, ordinary.Height = 80, 30
	if ordinary.optionCount() != 12 || strings.Contains(ordinary.View(), "Install Gentle Shell") {
		t.Fatal("ordinary model changed its menu")
	}
	m := ordinary.WithShellInstaller()
	if m.optionCount() != 13 || !strings.Contains(m.View(), "Install Gentle Shell") {
		t.Fatal("opted-in model does not expose the installer")
	}
	m.Cursor = 11 // New action is before Quit; existing action indexes stay stable.
	m.Height = 8
	if !strings.Contains(m.View(), "Install Gentle Shell") {
		t.Fatal("short viewport hides the selected installer action")
	}
	final, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = final.(Model)
	if m.Screen != ScreenShellInstall || cmd != nil || !strings.Contains(m.View(), "Target:") {
		t.Fatal("welcome starts a program or fails to enter the selector")
	}
	final, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = final.(Model)
	selection, err := cli.ShellInstallOutcome(m.shellInstallModel)
	if err != nil || cmd != nil || selection.Request.Destination != "q" || selection.Confirmed {
		t.Fatalf("parent consumed selector input: %+v, %v", selection, err)
	}
}

func TestShellInstallerCancelReturnsWelcome(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
		m.Cursor = 11
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
