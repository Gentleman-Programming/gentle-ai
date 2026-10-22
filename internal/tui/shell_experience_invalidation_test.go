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

func shellExperienceTarget(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "target")
}

func TestShellExperienceEditingImmediatelyInvalidatesInspection(t *testing.T) {
	m := experienceInherited(t)
	m, before := experiencePlan(t, m)
	destination := before.Destination
	if destination == "" || before.Experience == nil {
		t.Fatalf("plan lacks the inspected selection: %+v", before)
	}
	back := experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if back.shellExperience == nil || back.shellPlan != nil {
		t.Fatal("plan escape did not return to Configure")
	}
	back = experienceFocus(t, back, screens.ShellExperiencePersona, "neutral")
	back = experienceKey(back, tea.KeyMsg{Type: tea.KeyEnter})
	back, after := experiencePlan(t, back)
	if after.Confirmation == before.Confirmation {
		t.Fatal("re-entering the plan reused the inspected selection")
	}
	if after.Destination != destination || after.Experience == nil || after.Experience.Persona != "neutral" {
		t.Fatalf("editing lost destination or profile: %+v", after)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("plan created the destination", err)
	}
}

func TestShellExperienceNonPrivateParentRefusesBeforeConsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := filepath.Join(home, ".local", "share", "gentle-shell")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(base, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
	m.Width, m.Height = 120, 30
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan != nil || m.shellExperience == nil || m.shellExperience.err == nil || m.shellPlanRequest.Confirmation != "" {
		t.Fatalf("nonprivate default parent acquired inspection or consent: %+v", m.shellExperience)
	}
	if !strings.Contains(m.shellExperience.err.Error(), "private") && !strings.Contains(m.shellExperience.err.Error(), "refused") {
		t.Fatalf("refusal is not explicit: %v", m.shellExperience.err)
	}
	after, err := os.Stat(base)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("refusal changed the default parent", err)
	}
}
