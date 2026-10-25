package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestShellAgreedFreshDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(system.DetectionResult{}, "agreed-defaults-test").WithShellInstaller()
	m.Width, m.Height = 209, 61
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.shellExperience == nil {
		t.Fatal("Welcome must open an inert configuration")
	}
	want := shellinstaller.UserExperience{
		Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on",
		GlobalRDD: "on", CodeGraph: "on", Context7: "on", GGA: "off",
	}
	if m.shellExperience.profile != want {
		t.Errorf("fresh profile does not carry agreed defaults: got %+v, want %+v", m.shellExperience.profile, want)
	}
	for _, line := range []string{"Background subagents: ON", "Strict TDD: ON", "Global RDD: ON", "CodeGraph: ON", "Context7: ON"} {
		if !strings.Contains(m.View(), line) {
			t.Errorf("fresh menu omits concrete default %q", line)
		}
	}
	if strings.Contains(m.View(), "Inherited") || strings.Contains(m.View(), "ADVANCED") || strings.Contains(m.View(), "GGA provider") {
		t.Error("fresh menu still inherits or exposes an inactive provider")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnd}, {Type: tea.KeyEnter}} {
		next, cmd = m.Update(key)
		m = next.(Model)
		if cmd != nil {
			t.Fatal("Continue must not start a worker")
		}
	}
	state, ok := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if !ok || m.shellExperience != nil || state.Request.Experience == nil || *state.Request.Experience != want || state.Request.Confirmation != "" || m.ShellInstallSelection().Confirmed {
		t.Fatal("Continue dropped defaults, reused consent or confirmed installation", state)
	}
}

func TestShellAgreedDefaultsPreserveExistingChoices(t *testing.T) {
	for _, choice := range []string{"", "off"} {
		t.Run("choice-"+choice, func(t *testing.T) {
			profile := shellinstaller.UserExperience{Persona: "neutral", BackgroundSubagents: "off", StrictTDD: choice, GlobalRDD: choice, CodeGraph: choice, Context7: choice, GGA: "off"}
			m := NewModel(system.DetectionResult{}, "agreed-defaults-test").WithShellInstaller()
			m.Screen, m.Width, m.Height = ScreenShellInstall, 209, 61
			m.shellInstallModel = cli.NewShellInstallModelWithRequest(nil, shellinstaller.UserInstallRequest{Mode: "separate", Experience: &profile})
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
			m = next.(Model)
			if cmd != nil || m.shellExperience == nil || m.shellExperience.profile != profile {
				t.Fatal("fresh defaults overrode an existing OFF/inherited request")
			}
		})
	}
}
