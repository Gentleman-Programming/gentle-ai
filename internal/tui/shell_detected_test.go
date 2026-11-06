package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func managedInstallFixture(t *testing.T) (string, string) {
	t.Helper()
	if home, target, ok := platformManagedInstallFixture(t); ok {
		return home, target
	}
	home := shellTestHome(t)
	t.Setenv("HOME", home)
	target := filepath.Join(home, ".local", "share", "gentle-shell", "install")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"Schema": "gentle-shell-user-install/v1", "Destination": target,
		"Prefix": filepath.Join(target, "prefix"), "Agent": filepath.Join(target, "agent"),
		"Mode": "separate", "SupervisorSHA": "sha", "NodeSHA": "sha",
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return home, target
}

func addAgentInstall(t *testing.T, home string) string {
	t.Helper()
	target := filepath.Join(home, ".gentle-shell")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"Schema": "gentle-shell-user-install/v1", "Destination": target,
		"Prefix": filepath.Join(target, "prefix"), "Agent": filepath.Join(home, ".pi", "agent"),
		"Mode": "agent", "SupervisorSHA": "sha", "NodeSHA": "sha",
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestShellManageAppliesSDKSourceAsPreference(t *testing.T) {
	skipUnqualifiedMainPreview(t)
	_, target := managedInstallFixture(t)
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Manage.
	m = next.(Model)
	m = experienceFocus(t, m, screens.ShellExperienceSDK, "main-preview")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellManage == nil || len(m.shellManage.preferences) != 1 || m.shellManage.preferences[0].Label != "SDK source" || len(m.shellManage.components) != 0 || m.shellPlan != nil {
		t.Fatalf("SDK source was not classified as a preference: %+v", m.shellManage)
	}
	view := m.View()
	if !strings.Contains(view, "PREFERENCE CHANGES") || !strings.Contains(view, "SDK source") || !strings.Contains(view, "Apply preferences") || strings.Contains(view, "COMPONENT CHANGES") {
		t.Fatalf("preference summary differs:\n%s", view)
	}
	if m.shellInstallSelection.Confirmed {
		t.Fatal("preference summary forged installation consent")
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.shellManage.applied || m.shellManage.err != nil || m.shellInstallSelection.Confirmed {
		t.Fatalf("SDK source apply failed or forged installation: %+v", m.shellManage)
	}
	loaded, err := shellinstaller.LoadUserShellExperience(shellinstaller.UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || loaded.SDKSource != "main-preview" {
		t.Fatalf("SDK source channel was not applied in place: %+v %v", loaded, err)
	}
	data, err := os.ReadFile(filepath.Join(target, "config", "update-channel.json"))
	if err != nil || string(data) != `{"channel":"main"}` {
		t.Fatalf("canonical channel bytes differ: %q %v", data, err)
	}
}

func TestShellManageNoChangesIdentifiesInstallKind(t *testing.T) {
	managedInstallFixture(t)
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Manage.
	m = next.(Model)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	for _, want := range []string{"Separate install", "opens with gentle-shell", "No changes selected. Back to change preferences."} {
		if !strings.Contains(view, want) {
			t.Fatalf("empty manage summary hides %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Apply preferences") {
		t.Fatalf("empty summary offers Apply:\n%s", view)
	}
	agentView := screens.RenderShellManage("/home/test/install", "agent", nil, nil, false, "", []string{"Uninstall this installation", "Back"}, 0, 100, 30)
	if !strings.Contains(agentView, "On my existing Pi") || !strings.Contains(agentView, "opens with pi") {
		t.Fatalf("agent summary does not identify its launcher:\n%s", agentView)
	}
}

func TestShellManageChecksChannelUpdates(t *testing.T) {
	stubShellKernelGate(t) // beginComponentUpdate inspects; the OS guard must not decide this flow.
	_, target := managedInstallFixture(t)
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Manage.
	m = next.(Model)
	if !strings.Contains(m.View(), "Check for updates") {
		t.Fatalf("manage hides the update action:\n%s", m.View())
	}
	m = experienceFocus(t, m, screens.ShellExperienceUpdate, "")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellUpdate == nil || !m.shellUpdate.checking {
		t.Fatalf("update check did not start: %+v", m.shellUpdate)
	}
	next, _ = m.Update(shellUpdateResult{info: shellinstaller.UserShellUpdate{Channel: "stable", Available: true, Components: []string{"Node runtime"}}})
	m = next.(Model)
	if view := m.View(); !strings.Contains(view, "Update available") || !strings.Contains(view, "Node runtime") || !strings.Contains(view, "stable") {
		t.Fatalf("update result is incomplete:\n%s", view)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Update now.
	m = next.(Model)
	if m.shellPlan == nil || m.shellPlanRequest.Destination != target {
		t.Fatalf("update action did not prepare the component update: %+v %+v", m.shellPlan, m.shellUpdate)
	}
}

func TestShellUninstallSeparateKeepsBackup(t *testing.T) {
	home, target := managedInstallFixture(t)
	if err := shellinstaller.RecordUserShellInstall(home, shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Manage.
	m = next.(Model)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if view := m.View(); !strings.Contains(view, "Uninstall this installation") {
		t.Fatalf("managed install hides uninstall:\n%s", view)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})  // Back is first; Uninstall is last.
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter}) // Uninstall.
	if m.shellManage == nil || !m.shellManage.confirm || !strings.Contains(m.View(), "Uninstall (keep backup)") {
		t.Fatalf("uninstall confirmation is missing:\n%s", m.View())
	}
	next, uninstallCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.shellManage.working || !strings.Contains(m.View(), "Working: uninstalling") {
		t.Fatalf("uninstall did not show a working state: %+v\n%s", m.shellManage, m.View())
	}
	if uninstallCmd == nil {
		t.Fatal("uninstall did not schedule a command")
	}
	next, _ = m.Update(uninstallCmd())
	m = next.(Model)
	if m.shellManage == nil || !m.shellManage.uninstalled || m.shellManage.backup == "" {
		t.Fatalf("uninstall did not complete: %+v", m.shellManage)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("uninstall left the active target: %v", err)
	}
	if _, err := os.Stat(m.shellManage.backup); err != nil {
		t.Fatalf("uninstall backup is missing: %v", err)
	}
	if got := shellinstaller.DiscoverUserShellInstalls(home); len(got) != 0 {
		t.Fatalf("uninstall left registry discovery: %+v", got)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Screen != ScreenWelcome {
		t.Fatalf("uninstall did not return to Welcome: %v", m.Screen)
	}
}

func TestShellMultipleInstallsRequireChoice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows discovers only the single LOCALAPPDATA Separate install and has no On my existing Pi mode, so two installs cannot coexist")
	}
	home, _ := managedInstallFixture(t)
	agent := addAgentInstall(t, home)
	installs := shellinstaller.DiscoverUserShellInstalls(home)
	if len(installs) != 2 {
		t.Fatalf("both installations must be discovered: %+v", installs)
	}
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	view := m.View()
	for _, want := range []string{"installations detected", "Separate install", "On my existing Pi", "~/.local/share/gentle-shell/install", "~/.gentle-shell"} {
		if !strings.Contains(view, want) {
			t.Fatalf("multi-install selector hides %q:\n%s", want, view)
		}
	}
	if m.ShellInstallSelection().Confirmed {
		t.Fatal("multi-install detection forged consent")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellExperience == nil || m.shellExperience.target != agent || m.shellExperience.mode != "agent" {
		t.Fatalf("selected install did not open Manage: %+v", m.shellExperience)
	}
	if view := m.View(); !strings.Contains(view, "(*) On my existing Pi  · locked") || !strings.Contains(view, "( ) Separate install (Gentle-Shell)  · locked") {
		t.Fatalf("agent target is not the locked current target:\n%s", view)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellManage == nil || !strings.Contains(m.View(), "Uninstall this installation") {
		t.Fatalf("agent install does not offer uninstall:\n%s", m.View())
	}
}

func TestShellDetectsManagedInstallAndManage(t *testing.T) {
	_, target := managedInstallFixture(t)
	got := shellinstaller.DiscoverUserShellInstalls(os.Getenv("HOME"))
	if len(got) != 1 || got[0].Target != target {
		t.Fatalf("default managed install not discovered: %+v", got)
	}
	if profile, err := shellinstaller.LoadUserShellExperience(got[0]); err != nil {
		t.Fatalf("installed profile did not load: %v", err)
	} else {
		t.Logf("loaded profile: %+v", profile)
	}
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !strings.Contains(m.View(), "Found a managed Gentle-Shell installation") || !strings.Contains(m.View(), "Manage this installation") || !strings.Contains(m.View(), managedInstallDisplayTarget) {
		t.Fatalf("detection screen is incomplete:\n%s", m.View())
	}
	if m.ShellInstallSelection().Confirmed {
		t.Fatal("detection forged installation consent")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Manage.
	m = next.(Model)
	if m.shellExperience == nil || !m.shellExperience.reconfigure || m.shellExperience.target != target || m.shellExperience.choice != "existing" {
		t.Fatalf("manage did not prefill the discovered install: %+v", m.shellExperience)
	}
	view := m.View()
	if !strings.Contains(view, "Configure Gentle-Shell experience") || !strings.Contains(view, "(*) Separate install (Gentle-Shell)  · locked") || !strings.Contains(view, "( ) On my existing Pi  · locked") || !strings.Contains(view, "Uninstall this installation") {
		t.Fatalf("manage did not lock the target or offer actions:\n%s", view)
	}
	if strings.Contains(view, "Preview completion animation") {
		t.Fatal("test-only completion preview is still offered from Manage")
	}
	m = experienceFocus(t, m, screens.ShellExperienceUninstall, "")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellManage == nil || !m.shellManage.confirm || !m.shellManage.fromConfig {
		t.Fatalf("configure uninstall did not open confirmation: %+v", m.shellManage)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.shellExperience == nil || m.shellManage != nil {
		t.Fatal("uninstall confirmation did not return to configuration")
	}
	m = experienceFocus(t, m, screens.ShellExperiencePersona, "neutral")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellManage == nil || m.shellExperience != nil || m.shellPlan != nil {
		t.Fatalf("manage did not open the change summary: %+v", m.shellExperience)
	}
	if len(m.shellManage.preferences) != 1 || m.shellManage.preferences[0].Label != "Persona" || m.shellManage.preferences[0].To != "Neutral" {
		t.Fatalf("manage summary lost the persona change: %+v", m.shellManage.preferences)
	}
	if view := m.View(); !strings.Contains(view, "PREFERENCE CHANGES") || !strings.Contains(view, "Apply preferences") || !strings.Contains(view, "Separate install") || !strings.Contains(view, "opens with gentle-shell") {
		t.Fatalf("manage summary is incomplete:\n%s", view)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.shellManage.applied || m.shellManage.err != nil || m.shellInstallSelection.Confirmed {
		t.Fatalf("preference apply failed or forged installation: %+v", m.shellManage)
	}
	loaded, err := shellinstaller.LoadUserShellExperience(shellinstaller.UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || loaded.Persona != "neutral" {
		t.Fatalf("persona was not applied in place: %+v %v", loaded, err)
	}
	if view := m.View(); !strings.Contains(view, "Preferences updated.") || !strings.Contains(view, "Enter: back to menu") {
		t.Fatalf("applied summary differs:\n%s", view)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Screen != ScreenWelcome || m.shellManage != nil || m.shellExperience != nil || m.shellInstallSelection.Confirmed {
		t.Fatalf("Enter after Preferences updated did not return to the menu: screen=%v manage=%+v experience=%+v", m.Screen, m.shellManage, m.shellExperience)
	}
}

func TestShellDetectionBackReturnsToMenu(t *testing.T) {
	managedInstallFixture(t)
	m := NewModel(system.DetectionResult{}, "detected").WithShellInstaller()
	m.Width, m.Height = 120, 35
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.Screen != ScreenWelcome || len(m.View()) == 0 {
		t.Fatalf("back did not return to Welcome: screen=%v", m.Screen)
	}
}
