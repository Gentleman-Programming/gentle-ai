package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func experienceKey(m Model, key tea.KeyMsg) Model {
	model, _ := m.Update(key)
	return model.(Model)
}

func experienceFocus(t *testing.T, m Model, control screens.ShellExperienceControl, choice string) Model {
	t.Helper()
	if m.shellExperience == nil {
		t.Fatal("configuration is not active")
	}
	reuseAgent := ""
	if shellAgentUsable(m.shellExperience.agent) {
		reuseAgent = m.shellExperience.agent
	}
	targets := screens.ShellInstallTargetEntries(m.shellExperience.mode, m.shellExperience.piFound, m.shellExperience.piNote, reuseAgent, m.shellExperience.existing, m.shellExperience.reconfigure)
	uninstall := m.shellExperience.reconfigure && (m.shellExperience.mode == "separate" || m.shellExperience.mode == "agent")
	for _, entry := range screens.ShellExperienceOptionsWithUninstall(m.shellExperience.profile, uninstall, targets...) {
		if entry.Control == control && (choice == "" || entry.Choice == choice) {
			m = experienceKey(m, tea.KeyMsg{Type: tea.KeyHome})
			for i := 0; i < entry.Cursor; i++ {
				m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})
			}
			return m
		}
	}
	t.Fatal("supported configuration action is missing", control, choice)
	return m
}

func experienceStart(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
	m.Width, m.Height = 120, 30
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "Configure Gentle-Shell experience") {
		t.Fatal("ordinary installer skips Configure")
	}
	return m
}

// experiencePlan ends configuration and returns the installation plan request.
func experiencePlan(t *testing.T, m Model) (Model, shellinstaller.UserInstallRequest) {
	t.Helper()
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan == nil || m.shellExperience != nil {
		t.Fatalf("configuration did not reach the installation plan: %+v err=%v", m.shellExperience, m.shellExperience.err)
	}
	if m.shellPlanRequest.Confirmation == "" {
		t.Fatal("installation plan lacks the inspection token")
	}
	return m, m.shellPlanRequest
}

// Legacy requests remain independently testable without rewriting fresh defaults.
func experienceInherited(t *testing.T) Model {
	t.Helper()
	m := experienceStart(t)
	m.shellExperience.profile = shellinstaller.DefaultUserExperience()
	return m
}

func TestShellExperienceConfigureReachesPlan(t *testing.T) {
	m := experienceFocus(t, experienceInherited(t), screens.ShellExperiencePersona, "neutral")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceFocus(t, m, screens.ShellExperiencePersona, "unmanaged")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceFocus(t, m, screens.ShellExperienceBackground, "")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // background off
	m, request := experiencePlan(t, m)
	if request.Experience == nil || request.Experience.Persona != "unmanaged" || request.Experience.BackgroundSubagents != "off" {
		t.Fatal("Configure lost profile at the installation plan")
	}
	wantDestination := filepath.Join(os.Getenv("HOME"), ".local", "share", "gentle-shell", "install")
	if request.Destination != wantDestination || request.Mode != "separate" {
		t.Fatalf("default destination differs: %+v", request)
	}
	if m.ShellInstallSelection().Confirmed {
		t.Fatal("plan inspection forged installation consent")
	}
	if _, err := os.Lstat(wantDestination); !os.IsNotExist(err) {
		t.Fatal("plan created the destination before confirmation", err)
	}
	back := experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if back.shellExperience == nil || back.shellPlan != nil || back.shellExperience.profile.Persona != "unmanaged" || back.shellExperience.profile.BackgroundSubagents != "off" {
		t.Fatal("plan escape lost the configuration")
	}
}
