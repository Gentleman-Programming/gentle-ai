package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestManageContinueLabelReflectsPendingChanges(t *testing.T) {
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	current, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	config := shellExperienceConfig{profile: current, mode: "separate", reconfigure: true, existing: []shellinstaller.UserShellInstall{install}}
	if got := manageContinueLabel(config); got != "Continue" {
		t.Fatalf("unchanged label = %q", got)
	}

	preference := config
	preference.profile.Persona = "neutral"
	if got := manageContinueLabel(preference); got != "Apply changes to installation" {
		t.Fatalf("preference label = %q", got)
	}

	component := config
	component.profile.CodeGraph = "on"
	if got := manageContinueLabel(component); got != "Update installation" {
		t.Fatalf("component label = %q", got)
	}

	// SDK source is an update-channel preference: switching Stable ↔ Main
	// offers the in-place apply label, never a component update.
	sdk := config
	sdk.profile.SDKSource = "main-preview"
	if got := manageContinueLabel(sdk); got != "Apply changes to installation" {
		t.Fatalf("SDK source label = %q", got)
	}
	preferences, components := manageChanges(config.profile, sdk.profile, true)
	if len(preferences) != 1 || preferences[0].Label != "SDK source" || len(components) != 0 {
		t.Fatalf("SDK source classification differs: %+v / %+v", preferences, components)
	}
}

func TestShellChannelOnlyDelta(t *testing.T) {
	installed := shellinstaller.UserExperience{Persona: "gentleman", BackgroundSubagents: "on", GGA: "off"}
	channel := installed
	channel.SDKSource = "main-preview"
	if !shellChannelOnlyDelta(installed, channel) {
		t.Fatal("a pure SDK source change is not a channel-only delta")
	}
	if shellChannelOnlyDelta(installed, installed) {
		t.Fatal("an unchanged profile is a channel-only delta")
	}
	preference := installed
	preference.Persona = "neutral"
	preference.SDKSource = "main-preview"
	if shellChannelOnlyDelta(installed, preference) {
		t.Fatal("a preference change counted as channel-only")
	}
	component := installed
	component.CodeGraph = "on"
	component.SDKSource = "main-preview"
	if shellChannelOnlyDelta(installed, component) {
		t.Fatal("a component change counted as channel-only")
	}
	sameChannel := installed
	sameChannel.SDKSource = "stable"
	if shellChannelOnlyDelta(installed, sameChannel) {
		t.Fatal("a stable selection on a stable installation counted as a channel delta")
	}
	foreign := installed
	foreign.SDKSource = "canary"
	if shellChannelOnlyDelta(installed, foreign) {
		t.Fatal("a non-canonical SDK source counted as a channel delta")
	}
}

func shellUpdateModel(t *testing.T, install shellinstaller.UserShellInstall, selected shellinstaller.UserExperience) Model {
	t.Helper()
	// The managed update flow targets the existing installation.
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", install.Target)
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	m.Screen = ScreenShellInstall
	m.shellUpdate = &shellUpdateState{
		install: install,
		config:  shellExperienceConfig{profile: selected, existing: []shellinstaller.UserShellInstall{install}},
		info:    shellinstaller.UserShellUpdate{Channel: selected.SDKSource, Available: true, Components: []string{"SDK Main snapshot"}},
	}
	return m
}

// Update-now on a channel-only delta persists the channel preference in place
// and prepares the channel-pinned overlay request instead of the full
// component reconfigure.
func TestShellUpdateRoutesChannelOnlyDeltaToOverlay(t *testing.T) {
	skipUnqualifiedMainPreview(t)
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	loaded, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	selected := loaded
	selected.SDKSource = "main-preview"
	m := shellUpdateModel(t, install, selected)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellUpdate != nil || m.shellPlan == nil {
		t.Fatalf("channel-only delta did not prepare the plan: %+v", m.shellUpdate)
	}
	if m.shellPlanRequest.Destination != target {
		t.Fatalf("overlay request differs: %+v", m.shellPlanRequest)
	}
	if m.shellPlanRequest.Experience == nil || m.shellPlanRequest.Experience.SDKSource != "main-preview" {
		t.Fatalf("overlay request lost the channel authority: %+v", m.shellPlanRequest.Experience)
	}
	data, err := os.ReadFile(filepath.Join(target, "config", "update-channel.json"))
	if err != nil || string(data) != `{"channel":"main"}` {
		t.Fatalf("channel preference was not persisted in place: %q %v", data, err)
	}
}

// Update-now with any other component delta keeps the full reconfigure: the
// request carries the selected component and no pinned channel. Removing
// CodeGraph is admitted on every OS, so the route is proven natively.
func TestShellUpdateRoutesComponentDeltaToFullReconfigure(t *testing.T) {
	_, target := managedInstallFixture(t)
	setManifestCodeGraph(t, target, true)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	loaded, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	selected := loaded
	selected.CodeGraph = "off"
	m := shellUpdateModel(t, install, selected)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellUpdate != nil || m.shellPlan == nil {
		t.Fatalf("component delta did not prepare the plan: %+v", m.shellUpdate)
	}
	if m.shellPlanRequest.Destination != target {
		t.Fatalf("component update request differs: %+v", m.shellPlanRequest)
	}
	if m.shellPlanRequest.Experience == nil || *m.shellPlanRequest.Experience != selected {
		t.Fatalf("component update dropped the selected profile: %+v", m.shellPlanRequest.Experience)
	}
	if m.shellPlanRequest.Experience.SDKSource != loaded.SDKSource {
		t.Fatalf("component update lost the installed channel authority: %+v", m.shellPlanRequest.Experience)
	}
	if _, err := os.Stat(filepath.Join(target, "config", "update-channel.json")); !os.IsNotExist(err) {
		t.Fatalf("component update wrote a channel preference: %v", err)
	}
}
