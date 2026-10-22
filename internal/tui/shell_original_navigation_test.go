package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestShellOriginalOrdinaryWelcomeDispatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for label, want := range map[string]Screen{
		"Start installation":         ScreenDetection,
		"Configure models":           ScreenModelConfig,
		"Manage backups":             ScreenBackups,
		"Receipt-Driven Development": ScreenReviewMode,
		"Managed uninstall":          ScreenUninstallMode,
	} {
		t.Run(label, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "restore-ui-test").WithShellInstaller()
			for index, option := range screens.WelcomeOptionsWithShellInstaller(nil, false, false, true) {
				if option == label {
					m.Cursor = index
				}
			}
			cursor := m.Cursor
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := next.(Model)
			// Commands are deliberately not executed: routing must not install,
			// query global review configuration or mutate external state here.
			if got.Screen != want || want == ScreenWelcome && got.Cursor != cursor {
				t.Fatalf("ordinary welcome row %q routed to %v, want %v", label, got.Screen, want)
			}
		})
	}
	// The agent row label follows the host's engine detection: an enabled host
	// opens the builder, a host without engines keeps the row inert.
	m := NewModel(system.DetectionResult{}, "restore-ui-test").WithShellInstaller()
	hasEngines := m.hasAgentBuilderEngines()
	label := "Create your own Agent"
	if !hasEngines {
		label = "Create your own Agent (no agents)"
	}
	for index, option := range screens.WelcomeOptionsWithShellInstaller(nil, false, hasEngines, true) {
		if option == label {
			m.Cursor = index
		}
	}
	cursor := m.Cursor
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	want := ScreenAgentBuilderEngine
	if !hasEngines {
		want = ScreenWelcome
	}
	if got.Screen != want || want == ScreenWelcome && got.Cursor != cursor {
		t.Fatalf("agent row %q routed to %v, want %v", label, got.Screen, want)
	}
}

func TestShellOriginalMinimumMenuKeys(t *testing.T) {
	for _, key := range []rune{'c', 'C', 'x', 'X'} {
		m := experienceStart(t)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		got := next.(Model)
		cancel := key == 'x' || key == 'X'
		if cmd != nil || got.ShellInstallSelection().Confirmed {
			t.Fatal("minimum menu shortcut executed or confirmed installation", key)
		}
		if cancel && got.Screen != ScreenWelcome || !cancel && got.Screen != ScreenShellInstall || got.shellExperience != nil {
			t.Fatal("minimum menu shortcut did not continue/cancel configuration", key)
		}
	}
}
