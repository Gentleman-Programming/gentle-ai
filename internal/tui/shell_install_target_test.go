package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func openTargetConfiguration(t *testing.T) Model {
	t.Helper()
	m := NewModel(system.DetectionResult{}, "install-target-test").WithShellInstaller()
	m.Width, m.Height = 209, 61
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model)
}

func restrictedAgentHome(t *testing.T) string {
	t.Helper()
	t.Setenv("NPM_CONFIG_PREFIX", "")
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	agent := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(agent, 0755); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(home, ".npm-global")
	bundle := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "cli.js"), []byte("stock cli"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(prefix, 0755); err != nil {
		t.Fatal(err)
	}
	return agent
}

func TestShellAgentReuseRequiresDetection(t *testing.T) {
	t.Setenv("NPM_CONFIG_PREFIX", "")
	t.Setenv("HOME", t.TempDir())
	m := openTargetConfiguration(t)
	m = experienceFocus(t, m, screens.ShellExperienceInstallTarget, "agent")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellExperience.mode != "separate" || m.shellExperience.err == nil {
		t.Fatalf("On my existing Pi was accepted without a detected agent: %+v", m.shellExperience)
	}
}

func TestShellAgentReuseOption(t *testing.T) {
	agent := restrictedAgentHome(t)
	m := openTargetConfiguration(t)
	m = experienceFocus(t, m, screens.ShellExperienceInstallTarget, "agent")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellExperience.mode != "agent" || m.shellExperience.choice != "agent" {
		t.Fatalf("agent reuse was not selected: %+v", m.shellExperience)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	request := m.shellPlanRequest
	if m.shellPlan == nil || request.Mode != "agent" || request.SharedAgent != agent || request.SharedPrefix != "" {
		t.Fatalf("agent reuse request differs: %+v", request)
	}
	// Main preview is part of the same selections now.
	restrictedAgentHome(t)
	main := openTargetConfiguration(t)
	main = experienceFocus(t, main, screens.ShellExperienceInstallTarget, "agent")
	next, _ = main.Update(tea.KeyMsg{Type: tea.KeyEnter})
	main = next.(Model)
	profile := main.shellExperience.profile
	profile.SDKSource = "main-preview"
	main.shellExperience.profile = profile
	next, _ = main.Update(tea.KeyMsg{Type: tea.KeyEnd})
	main = next.(Model)
	next, _ = main.Update(tea.KeyMsg{Type: tea.KeyEnter})
	main = next.(Model)
	if main.shellExperience != nil {
		t.Fatalf("Main was not admitted for agent reuse: %v", main.shellExperience.err)
	}
	if main.shellPlan == nil || main.shellPlanRequest.Experience == nil || main.shellPlanRequest.Experience.SDKSource != "main-preview" {
		t.Fatalf("Main selection was dropped: %+v", main.shellPlanRequest.Experience)
	}
}

func containsText(value, part string) bool {
	return part == "" || strings.Contains(value, part)
}
