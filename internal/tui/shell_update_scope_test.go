package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// A scoped component update carries the canonical scope in the inspected
// request and shows a plan that names only the refreshed components.
func TestShellUpdateComponentScopeDrivesRequestAndPlan(t *testing.T) {
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	loaded, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	m := shellUpdateModel(t, install, loaded)
	m.shellUpdate.info = shellinstaller.UserShellUpdate{
		Channel: "stable", Available: true,
		Components: []string{"gentle-pi SDK", "CodeGraph"},
		Scope:      []string{shellinstaller.UserComponentSDK, shellinstaller.UserComponentCodeGraph},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellUpdate != nil || m.shellPlan == nil {
		t.Fatalf("scoped update did not prepare the plan: %+v", m.shellUpdate)
	}
	if want := "codegraph,sdk"; m.shellPlanRequest.Scope != want {
		t.Fatalf("request scope = %q, want %q", m.shellPlanRequest.Scope, want)
	}
	plan := strings.Join(m.shellPlan, "\n")
	if !strings.Contains(plan, "CodeGraph") || !strings.Contains(plan, "gentle-pi SDK") {
		t.Fatalf("plan does not name the refreshed components: %v", m.shellPlan)
	}
	if strings.Contains(plan, "Downloading Node") {
		t.Fatalf("plan keeps full-install wording for a scoped update: %v", m.shellPlan)
	}
}

// An unscoped update keeps the classic five-step plan and an empty scope.
func TestShellUpdateWithoutScopeKeepsFullPlan(t *testing.T) {
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	loaded, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	m := shellUpdateModel(t, install, loaded)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellPlanRequest.Scope != "" {
		t.Fatalf("unscoped update gained a scope: %q", m.shellPlanRequest.Scope)
	}
	if len(m.shellPlan) != 5 {
		t.Fatalf("unscoped plan differs from the install plan: %v", m.shellPlan)
	}
}
