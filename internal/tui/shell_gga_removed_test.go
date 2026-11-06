package tui

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestShellInstallerHasNoGGAOption(t *testing.T) {
	m := experienceStart(t)
	if m.shellExperience == nil {
		t.Fatal("configuration did not open")
	}
	m.shellExperience.profile.GGA = "on"
	m.shellExperience.profile.GGAProvider = "codex"
	view := m.View()
	if strings.Contains(view, "GGA") || strings.Contains(view, "Optional workflows") || strings.Contains(view, "provider") {
		t.Fatalf("shell installer still exposes GGA:\n%s", view)
	}
	for _, option := range shellExperienceOptionsForTest(m) {
		if strings.Contains(option.Label, "GGA") || strings.Contains(option.Category, "Optional workflows") {
			t.Fatalf("shell installer still contains a GGA entry: %+v", option)
		}
	}
}

func TestShellPlanForcesGGAOff(t *testing.T) {
	m := experienceStart(t)
	m.shellExperience.profile.GGA = "on"
	m.shellExperience.profile.GGAProvider = "codex"
	m, request := experiencePlan(t, m)
	if request.Experience == nil || request.Experience.GGA != "off" || request.Experience.GGAProvider != "" {
		t.Fatalf("GGA escaped the shell plan: %+v", request.Experience)
	}
	if m.ShellInstallSelection().Confirmed {
		t.Fatal("GGA removal forged installation consent")
	}
}

func shellExperienceOptionsForTest(m Model) []screens.ShellExperienceEntry {
	return screens.ShellExperienceOptionsWithUninstall(m.shellExperience.profile, false)
}
