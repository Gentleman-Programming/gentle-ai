package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestShellOriginalWelcomeAndMenu(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const title = "Install Gentle-Shell, our own agent"

	t.Run("campaign is initially selected above the ordinary menu", func(t *testing.T) {
		for _, size := range [][2]int{{80, 30}, {209, 61}} {
			m := NewModel(system.DetectionResult{}, "restore-ui-test").WithShellInstaller()
			m.Width, m.Height = size[0], size[1]
			view := m.View()
			wants := []string{title}
			// The recovered compact layout keeps a one-line hero; the wide
			// layout must retain the complete original campaign and frame.
			if size[1] >= 60 {
				wants = append(wants, "The complete Pi experience.", "MEMORY", "ODD", "SKILLS", "SAFE AUTOMATION", "╭", "╯")
			}
			for _, want := range wants {
				if !strings.Contains(view, want) {
					t.Fatalf("initial %dx%d welcome omits original campaign %q:\n%s", size[0], size[1], want, view)
				}
			}
			if m.Cursor != 0 || strings.Count(view, title) != 1 || strings.Index(view, title) >= strings.Index(view, "Menu") {
				t.Fatalf("original hero is not the single initial action above Menu:\n%s", view)
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if next.(Model).Screen != ScreenShellInstall || cmd != nil {
				t.Fatal("initial Enter must open configuration, not execute installation")
			}
		}
	})

	t.Run("configuration retains the original groups and detail panel", func(t *testing.T) {
		m := NewModel(system.DetectionResult{}, "restore-ui-test").WithShellInstaller()
		m.Width, m.Height = 209, 61
		for index, option := range screens.WelcomeOptionsWithShellInstaller(nil, false, false, true) {
			if option == screens.ShellInstallWelcomeOption {
				m.Cursor = index
			}
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if m.Screen != ScreenShellInstall || cmd != nil {
			t.Fatal("installer entry must open its menu without executing a command")
		}
		for _, want := range []string{
			"Configure Gentle-Shell experience", "REQUIRED FOUNDATION", "Persistent memory + MCP",
			"PERSONA", "RECOMMENDED", "SDK SOURCE", "ACTIONS",
			"ABOUT THIS OPTION", "WHAT IT CHANGES", "CURRENT VALUE",
			"GENTLEMAN PROGRAMMING RECOMMENDS", "WHY WE RECOMMEND IT", "│",
		} {
			if !strings.Contains(m.View(), want) {
				t.Fatalf("configuration omits original menu content %q:\n%s", want, m.View())
			}
		}
		if strings.Contains(m.View(), "ADVANCED") || strings.Contains(m.View(), "GGA provider") {
			t.Fatal("default configuration exposes an unnecessary Advanced/provider row")
		}
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if cmd != nil || m.Screen != ScreenWelcome || m.shellExperience != nil || m.ShellInstallSelection().Confirmed {
			t.Fatal("leaving configuration must return to welcome without retaining consent or launching")
		}
	})

	t.Run("ordinary welcome remains opt in", func(t *testing.T) {
		m := NewModel(system.DetectionResult{}, "restore-ui-test")
		m.Width, m.Height = 209, 61
		if m.Cursor != 0 || m.optionCount() != 12 || strings.Contains(m.View(), title) || !strings.Contains(m.View(), "Start installation") {
			t.Fatal("restoring the opt-in hero changed the ordinary welcome")
		}
	})
}
