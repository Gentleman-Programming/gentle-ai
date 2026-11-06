package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

// setManifestCodeGraph records the installed CodeGraph component the way a
// completed install does: the installation manifest bit (Windows: the
// runtime tree, through platformSetInstalledCodeGraph).
func setManifestCodeGraph(t *testing.T, target string, on bool) {
	t.Helper()
	if platformSetInstalledCodeGraph(t, target, on) {
		return
	}
	path := filepath.Join(target, "installation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if on {
		manifest["codegraph"] = true
	} else {
		delete(manifest, "codegraph")
	}
	if data, err = json.Marshal(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// openManagedConfiguration walks Welcome -> detection -> Manage and returns
// the prefilled Configure screen of the single detected installation.
func openManagedConfiguration(t *testing.T) Model {
	t.Helper()
	stubShellKernelGate(t)
	m := NewModel(system.DetectionResult{}, "manage-components").WithShellInstaller()
	m.Width, m.Height = 120, 40
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // Manage this installation.
	if m.shellExperience == nil || !m.shellExperience.reconfigure {
		t.Fatalf("Manage did not open the installed configuration:\n%s", m.View())
	}
	return m
}

func continueManagedConfiguration(m Model) Model {
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	return experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
}

func toggleExperience(t *testing.T, m Model, control screens.ShellExperienceControl, choice string) Model {
	t.Helper()
	m = experienceFocus(t, m, control, choice)
	return experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
}

func assertViewContains(t *testing.T, view string, want ...string) {
	t.Helper()
	for _, text := range want {
		if !strings.Contains(view, text) {
			t.Fatalf("screen lacks %q:\n%s", text, view)
		}
	}
}

func assertViewOmits(t *testing.T, view string, unwanted ...string) {
	t.Helper()
	for _, text := range unwanted {
		if strings.Contains(view, text) {
			t.Fatalf("screen shows %q:\n%s", text, view)
		}
	}
}

// captureShellInstallRunner records the request the confirmed plan hands to
// the worker, without running any installer.
func captureShellInstallRunner(m Model) (Model, <-chan shellinstaller.UserInstallRequest) {
	requests := make(chan shellinstaller.UserInstallRequest, 1)
	return m.WithShellInstallRunner(func(_ context.Context, request shellinstaller.UserInstallRequest, _ func(shellinstaller.UserInstallProgress)) error {
		requests <- request
		return nil
	}), requests
}

// confirmShellPlan presses Enter on the installation plan and returns the
// request the worker received.
func confirmShellPlan(t *testing.T, m Model, requests <-chan shellinstaller.UserInstallRequest) shellinstaller.UserInstallRequest {
	t.Helper()
	previous := shellRootRiskGate
	t.Cleanup(func() { shellRootRiskGate = previous })
	shellRootRiskGate = func() error { return nil }
	assertViewContains(t, m.View(), "Installation plan", "1. Validate the selection and target", "Enter: install • Esc: back")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellInstallRun == nil {
		t.Fatalf("plan confirmation did not start the worker:\n%s", m.View())
	}
	select {
	case request := <-requests:
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("worker never received the reconfigure request")
	}
	return shellinstaller.UserInstallRequest{}
}

// assertReconfigureRequest proves the full reconfigure route: the existing
// target, its mode, the SELECTED profile and the reconfigure authority.
func assertReconfigureRequest(t *testing.T, request shellinstaller.UserInstallRequest, target string, want shellinstaller.UserExperience) {
	t.Helper()
	if request.Destination != target || request.Mode != "separate" || request.SharedPrefix != "" || request.SharedAgent != "" {
		t.Fatalf("reconfigure request targets a different installation: %+v", request)
	}
	if request.Experience == nil || *request.Experience != want {
		t.Fatalf("reconfigure request profile = %+v, want the selected %+v", request.Experience, want)
	}
	if request.Confirmation == "" {
		t.Fatal("reconfigure request lacks the inspection token")
	}
	if got := os.Getenv("GENTLE_AI_RECONFIGURE_TARGET"); got != target {
		t.Fatalf("GENTLE_AI_RECONFIGURE_TARGET = %q, want %q", got, target)
	}
}

func installedProfile(t *testing.T, target string) shellinstaller.UserExperience {
	t.Helper()
	profile, err := shellinstaller.LoadUserShellExperience(shellinstaller.UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

// A component delta in Manage offers one clear apply action that runs the
// full reconfigure with the selected profile (D2).
func TestShellManageComponentDeltaAppliesThroughUpdateInstallation(t *testing.T) {
	for _, tt := range []struct {
		name                string
		installed, selected string
		from, to            string
	}{
		{name: "remove CodeGraph", installed: "on", selected: "off", from: "On", to: "Off"},
		{name: "add CodeGraph", installed: "off", selected: "on", from: "Inherit", to: "On"}, // an install without CodeGraph loads as Inherit
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, target := managedInstallFixture(t)
			setManifestCodeGraph(t, target, tt.installed == "on")
			want := installedProfile(t, target)
			want.CodeGraph = tt.selected
			m := openManagedConfiguration(t)
			m, requests := captureShellInstallRunner(m)
			m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
			assertViewContains(t, m.View(), "Update installation")
			m = continueManagedConfiguration(m)
			if m.shellManage == nil {
				t.Fatalf("Continue did not open the change summary:\n%s", m.View())
			}
			view := m.View()
			assertViewContains(t, view, "Manage installation", "COMPONENT CHANGES",
				"  CodeGraph      "+tt.from+" → "+tt.to,
				"Update installation refreshes only the changed components; your configuration, logins and sessions are preserved.",
				" ▸ Update installation", "   Back", "   Uninstall this installation")
			assertViewOmits(t, view, "not applied here", "Apply preferences", "PREFERENCE CHANGES")
			m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // Update installation.
			if m.shellManage != nil || m.shellPlan == nil {
				t.Fatalf("Update installation did not prepare the reconfigure plan: %+v\n%s", m.shellManage, m.View())
			}
			assertReconfigureRequest(t, m.shellPlanRequest, target, want)
			assertReconfigureRequest(t, confirmShellPlan(t, m, requests), target, want)
			if m.shellPlanRequest.Scope != "codegraph" {
				t.Fatalf("component delta scope = %q, want \"codegraph\"", m.shellPlanRequest.Scope)
			}
		})
	}
}

// A mixed preference + component delta applies both in the same reconfigure.
func TestShellManageMixedDeltaAppliesPreferencesAndComponents(t *testing.T) {
	_, target := managedInstallFixture(t)
	setManifestCodeGraph(t, target, true)
	want := installedProfile(t, target)
	want.Persona, want.StrictTDD, want.CodeGraph = "neutral", "on", "off"
	m := openManagedConfiguration(t)
	m, requests := captureShellInstallRunner(m)
	m = toggleExperience(t, m, screens.ShellExperiencePersona, "neutral")
	m = toggleExperience(t, m, screens.ShellExperienceTDD, "")
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	m = continueManagedConfiguration(m)
	view := m.View()
	assertViewContains(t, view, "PREFERENCE CHANGES", "  Persona        Gentleman → Neutral", "  Strict TDD     Inherit → On",
		"COMPONENT CHANGES", "  CodeGraph      On → Off", " ▸ Update installation")
	assertViewOmits(t, view, "Apply preferences", "not applied here")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	assertReconfigureRequest(t, m.shellPlanRequest, target, want)
	assertReconfigureRequest(t, confirmShellPlan(t, m, requests), target, want)
	if installed := installedProfile(t, target); installed.Persona != "gentleman" {
		t.Fatalf("the reconfigure route applied preferences before the worker ran: %+v", installed)
	}
}

// After Apply preferences the installed profile carries explicit values; a
// later component update must still pass admission (D3).
func TestShellManageComponentUpdateAfterAppliedPreferences(t *testing.T) {
	_, target := managedInstallFixture(t)
	setManifestCodeGraph(t, target, true)
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceTDD, "")
	m = toggleExperience(t, m, screens.ShellExperienceRDD, "")
	m = toggleExperience(t, m, screens.ShellExperienceContext7, "")
	m = toggleExperience(t, m, screens.ShellExperienceContext7, "")
	m = continueManagedConfiguration(m)
	assertViewContains(t, m.View(), " ▸ Apply preferences")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	assertViewContains(t, m.View(), "Preferences updated.", "Enter: back to menu")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Screen != ScreenWelcome {
		t.Fatalf("Apply preferences did not return to the menu: %v", m.Screen)
	}
	applied := installedProfile(t, target)
	if applied.StrictTDD != "on" || applied.GlobalRDD == "" {
		t.Fatalf("preferences were not applied: %+v", applied)
	}

	m = openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	m = continueManagedConfiguration(m)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // Update installation.
	if m.shellManage != nil && m.shellManage.err != nil {
		t.Fatalf("component update after applied preferences was refused: %v", m.shellManage.err)
	}
	want := applied
	want.CodeGraph = "off"
	assertReconfigureRequest(t, m.shellPlanRequest, target, want)
}

// Esc on the reconfigure plan returns to the Manage configuration, never to
// the fresh-install destination editor.
func TestShellManageComponentPlanBackReturnsToConfiguration(t *testing.T) {
	_, target := managedInstallFixture(t)
	setManifestCodeGraph(t, target, true)
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	m = continueManagedConfiguration(m)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.shellPlan != nil || m.shellExperience == nil || !m.shellExperience.reconfigure || m.shellExperience.profile.CodeGraph != "off" {
		t.Fatalf("plan Esc lost the Manage configuration: %+v", m.shellExperience)
	}
	assertViewContains(t, m.View(), "Configure Gentle-Shell experience", "[ ] CodeGraph: OFF", "Update installation")
}

// A preference-only delta keeps Apply preferences in place; existing Manage
// actions (Back, Uninstall) keep their behavior.
func TestShellManagePreferenceOnlyDeltaKeepsApplyPreferences(t *testing.T) {
	_, target := managedInstallFixture(t)
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperiencePersona, "neutral")
	assertViewContains(t, m.View(), "Apply changes to installation")
	m = continueManagedConfiguration(m)
	view := m.View()
	assertViewContains(t, view, "PREFERENCE CHANGES", "  Persona        Gentleman → Neutral",
		" ▸ Apply preferences", "   Back", "   Uninstall this installation")
	assertViewOmits(t, view, "COMPONENT CHANGES", "Update installation")

	// Back returns to the configuration with the selection intact.
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellManage != nil || m.shellExperience == nil || m.shellExperience.profile.Persona != "neutral" {
		t.Fatalf("Back lost the configuration: %+v", m.shellExperience)
	}

	// Uninstall opens its confirmation; its Back returns to the summary.
	m = continueManagedConfiguration(m)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	assertViewContains(t, m.View(), "Uninstall installation", " ▸ Uninstall (keep backup)", "   Back")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	assertViewContains(t, m.View(), "Manage installation", " ▸ Apply preferences")

	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan != nil || m.shellInstallSelection.Confirmed {
		t.Fatal("Apply preferences prepared an installation")
	}
	assertViewContains(t, m.View(), "Preferences updated.", "Enter: back to menu")
	if installed := installedProfile(t, target); installed.Persona != "neutral" {
		t.Fatalf("persona was not applied in place: %+v", installed)
	}
}

// A channel-only delta keeps Apply preferences in Manage and the in-place
// overlay on Update now; it never becomes a component reconfigure.
func TestShellManageChannelOnlyDeltaKeepsOverlay(t *testing.T) {
	skipUnqualifiedMainPreview(t)
	_, target := managedInstallFixture(t)
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceSDK, "main-preview")
	m = continueManagedConfiguration(m)
	view := m.View()
	assertViewContains(t, view, "PREFERENCE CHANGES", "  SDK source     Stable → Main preview", " ▸ Apply preferences")
	assertViewOmits(t, view, "COMPONENT CHANGES", "Update installation")

	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc}) // Back to configuration.
	m = experienceFocus(t, m, screens.ShellExperienceUpdate, "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(shellUpdateResult{info: shellinstaller.UserShellUpdate{Channel: "main-preview", Available: true, Components: []string{"SDK Main snapshot"}}})
	m = next.(Model)
	assertViewContains(t, m.View(), "Update available", "SDK Main snapshot", " ▸ Update now")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan == nil || m.shellPlanRequest.Experience == nil || m.shellPlanRequest.Experience.SDKSource != "main-preview" {
		t.Fatalf("channel-only Update now did not prepare the overlay: %+v", m.shellPlanRequest.Experience)
	}
	data, err := os.ReadFile(filepath.Join(target, "config", "update-channel.json"))
	if err != nil || string(data) != `{"channel":"main"}` {
		t.Fatalf("overlay did not persist the channel preference: %q %v", data, err)
	}
}

// Update now from Check for updates carries the selected component delta
// instead of the installed profile (D2).
func TestShellUpdateNowCarriesSelectedComponentDelta(t *testing.T) {
	_, target := managedInstallFixture(t)
	setManifestCodeGraph(t, target, true)
	want := installedProfile(t, target)
	want.CodeGraph = "off"
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	m = experienceFocus(t, m, screens.ShellExperienceUpdate, "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(shellUpdateResult{info: shellinstaller.UserShellUpdate{Channel: "stable", Available: true, Components: []string{"Node runtime"}}})
	m = next.(Model)
	assertViewContains(t, m.View(), "Update available", "Node runtime", " ▸ Update now")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	assertReconfigureRequest(t, m.shellPlanRequest, target, want)
}

const shellAgentCodeGraphReason = "CodeGraph needs a Separate install; it stays off for On my existing Pi"

// The agent target cannot install CodeGraph, so the TUI never offers it ON
// there: choosing the target turns it off with the reason, and turning it
// back on is refused with the same reason.
func TestShellAgentTargetNeverOffersCodeGraphOn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports Separate install only; there is no On my existing Pi target to keep CodeGraph off for")
	}
	agent := restrictedAgentHome(t)
	m := openTargetConfiguration(t)
	if m.shellExperience.profile.CodeGraph != "on" {
		t.Fatalf("fresh Separate default lost CodeGraph ON: %+v", m.shellExperience.profile)
	}
	m = toggleExperience(t, m, screens.ShellExperienceInstallTarget, "agent")
	if m.shellExperience.mode != "agent" || m.shellExperience.profile.CodeGraph != "off" {
		t.Fatalf("agent target kept CodeGraph ON: %+v", m.shellExperience)
	}
	assertViewContains(t, m.View(), shellAgentCodeGraphReason, "[ ] CodeGraph: OFF", "(*) On my existing Pi")

	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	if m.shellExperience.profile.CodeGraph != "off" {
		t.Fatalf("agent target accepted CodeGraph ON: %+v", m.shellExperience.profile)
	}
	assertViewContains(t, m.View(), shellAgentCodeGraphReason, "[ ] CodeGraph: OFF")

	m = continueManagedConfiguration(m)
	request := m.shellPlanRequest
	if m.shellPlan == nil || request.Mode != "agent" || request.SharedAgent != agent || request.Experience == nil || request.Experience.CodeGraph != "off" {
		t.Fatalf("agent request differs: %+v %+v", request, request.Experience)
	}
}

// Manage of an agent installation refuses CodeGraph ON with the same reason.
func TestShellManageAgentInstallNeverOffersCodeGraphOn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports Separate install only; there is no On my existing Pi target to keep CodeGraph off for")
	}
	home := shellTestHome(t)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	addAgentInstall(t, home)
	m := openManagedConfiguration(t)
	if m.shellExperience.mode != "agent" {
		t.Fatalf("Manage did not open the agent installation: %+v", m.shellExperience)
	}
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	if m.shellExperience.profile.CodeGraph == "on" {
		t.Fatalf("Manage offered CodeGraph ON for the agent target: %+v", m.shellExperience.profile)
	}
	assertViewContains(t, m.View(), shellAgentCodeGraphReason, "[ ] CodeGraph:")
	assertViewOmits(t, m.View(), "Update installation")
}

// A refused component update stays on the Manage summary with the reason and
// prepares no plan. The target's parent stops being private before the update,
// a refusal every Unix inspection applies.
func TestShellManageRefusedComponentUpdateStaysOnSummary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX parent privacy refusal; Windows custody is covered by its own fixtures")
	}
	_, target := managedInstallFixture(t)
	m := openManagedConfiguration(t)
	m = toggleExperience(t, m, screens.ShellExperienceCodeGraph, "")
	m = continueManagedConfiguration(m)
	if err := os.Chmod(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(target), 0700) })
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // Update installation.
	if m.shellManage == nil || m.shellPlan != nil || m.shellInstallSelection.Confirmed {
		t.Fatalf("refused component update left Manage: %+v", m.shellManage)
	}
	assertViewContains(t, m.View(), "private installation", " ▸ Update installation")
}
