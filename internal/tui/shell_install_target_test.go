package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
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
	home := shellTestHome(t)
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
	t.Setenv("HOME", shellTestHome(t))
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
	if runtime.GOOS == "windows" {
		// Windows supports Separate only: the row is disabled with the Windows
		// reason and the request stays Separate.
		if m.shellExperience.mode != "separate" || m.shellExperience.err == nil || !strings.Contains(m.shellExperience.err.Error(), "Windows supports Separate install only") {
			t.Fatalf("Windows accepted agent reuse: %+v", m.shellExperience)
		}
		return
	}
	if m.shellExperience.mode != "agent" || m.shellExperience.choice != "agent" {
		t.Fatalf("agent reuse was not selected: %+v", m.shellExperience)
	}
	m = shellHostCodeGraph(t, m)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	request := m.shellPlanRequest
	if m.shellPlan == nil || request.Mode != "agent" || request.SharedAgent != agent || request.SharedPrefix != "" {
		t.Fatalf("agent reuse request differs: %+v", request)
	}
	// Main preview is part of the same selections now. Its SDK selection binds
	// the agent as a private 0700 directory: a stock 0755 agent is refused up
	// front with the exact fix, and a 0700 agent is admitted.
	skipUnqualifiedMainPreview(t)
	stock := restrictedAgentHome(t)
	refused := shellAgentMainSelection(t)
	if refused.shellExperience == nil || refused.shellPlan != nil || refused.shellExperience.err == nil ||
		!strings.Contains(refused.shellExperience.err.Error(), "Main preview on your existing Pi") ||
		!strings.Contains(refused.shellExperience.err.Error(), "chmod 700 "+strconv.Quote(stock)) {
		t.Fatalf("Main on a 0755 agent was not refused up front: plan=%v err=%v", refused.shellPlan != nil, refused.shellExperience)
	}
	if info, err := os.Lstat(stock); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("refused Main selection changed the agent: %v %v", info, err)
	}
	private := restrictedAgentHome(t)
	if err := os.Chmod(private, 0700); err != nil {
		t.Fatal(err)
	}
	main := shellAgentMainSelection(t)
	if main.shellExperience != nil {
		t.Fatalf("Main was not admitted for a 0700 agent reuse: %v", main.shellExperience.err)
	}
	if main.shellPlan == nil || main.shellPlanRequest.SharedAgent != private || main.shellPlanRequest.Experience == nil || main.shellPlanRequest.Experience.SDKSource != "main-preview" {
		t.Fatalf("Main selection was dropped: %+v", main.shellPlanRequest.Experience)
	}
}

// T16: a stock Pi agent holds Pi's own fd/rg in its bin. "On my existing Pi"
// admits it and reaches the plan; the target says the tools are set aside
// and restored on uninstall, and nothing in the agent changes before install.
func TestShellAgentReuseSetsAsidePiTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports Separate install only; TestShellAgentReuseOption pins the refusal")
	}
	agent := restrictedAgentHome(t)
	bin := filepath.Join(agent, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bin, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"fd", "rg"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("pi's own "+tool), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(bin, tool), 0755); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() string {
		var lines []string
		for _, name := range []string{"", "fd", "rg"} {
			info, err := os.Lstat(filepath.Join(bin, name))
			data, _ := os.ReadFile(filepath.Join(bin, name))
			lines = append(lines, name+":"+info.Mode().String()+":"+string(data)+":"+strconv.FormatBool(err == nil))
		}
		return strings.Join(lines, "\n")
	}
	before := snapshot()
	m := openTargetConfiguration(t)
	targets := screens.ShellInstallTargetEntries("agent", true, "", agent, nil, false)
	if len(targets) == 0 || targets[0].Choice != "agent" || !strings.Contains(targets[0].Description, "fd/rg") ||
		!strings.Contains(targets[0].Description, "set aside") || !strings.Contains(targets[0].Description, "restored on uninstall") {
		t.Fatalf("On my existing Pi does not state the set-aside: %+v", targets)
	}
	m = experienceFocus(t, m, screens.ShellExperienceInstallTarget, "agent")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellExperience.mode != "agent" || m.shellExperience.choice != "agent" {
		t.Fatalf("agent reuse with Pi's fd/rg was not selected: %+v", m.shellExperience)
	}
	m = shellHostCodeGraph(t, m)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellExperience != nil {
		t.Fatalf("agent reuse with Pi's fd/rg was refused before the plan: %v", m.shellExperience.err)
	}
	if m.shellPlan == nil || m.shellPlanRequest.Mode != "agent" || m.shellPlanRequest.SharedAgent != agent || m.shellPlanRequest.Confirmation == "" {
		t.Fatalf("agent reuse with Pi's fd/rg did not reach the plan: %+v", m.shellPlanRequest)
	}
	if after := snapshot(); after != before {
		t.Fatalf("planning changed the agent bin:\n%s\n->\n%s", before, after)
	}
}

// shellUninstallKeptNote is the completion note contract UninstallUserShellInstall
// returns when the uninstall completed but kept a Pi-replaced tool.
type shellUninstallKeptNote struct{ note string }

func (n shellUninstallKeptNote) Error() string         { return "installation moved, but " + n.note }
func (n shellUninstallKeptNote) UninstallNote() string { return n.note }

// T16: an uninstall that completed but kept a Pi-replaced tool is a success
// with a note; any other uninstall error still stays on the confirmation.
func TestShellUninstallKeptToolIsSuccessWithNote(t *testing.T) {
	install := shellinstaller.UserShellInstall{Target: "/home/u/.local/share/gentle-shell/install", Mode: "agent"}
	note := "Pi's fd changed after installation; Pi's original is kept in /x.pi-tools"
	m := NewModel(system.DetectionResult{}, "uninstall-note-test").WithShellInstaller()
	m.Width, m.Height = 160, 40
	m.shellManage = &shellManageState{install: install, confirm: true, working: true}
	next, _ := m.updateShellManage(shellUninstallResult{backup: install.Target + ".removed-1", err: fmt.Errorf("wrapped: %w", shellUninstallKeptNote{note})})
	done := next.(Model)
	if !done.shellManage.uninstalled || done.shellManage.backup != install.Target+".removed-1" {
		t.Fatalf("a completed uninstall with a kept tool was not shown as uninstalled: %+v", done.shellManage)
	}
	view := done.shellInstallView()
	for _, want := range []string{"Installation uninstalled.", "Backup: " + install.Target + ".removed-1", "Note: " + note} {
		if !strings.Contains(view, want) {
			t.Fatalf("uninstalled view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "installation moved, but") || strings.Contains(view, "Uninstall (keep backup)") {
		t.Fatalf("a completed uninstall is rendered as a failure:\n%s", view)
	}
	m.shellManage = &shellManageState{install: install, confirm: true, working: true}
	next, _ = m.updateShellManage(shellUninstallResult{err: errors.New("the Pi agent changed after installation; uninstall aborted without touching it")})
	failed := next.(Model)
	if failed.shellManage.uninstalled || failed.shellManage.uninstallErr == nil || !strings.Contains(failed.shellInstallView(), "uninstall aborted") {
		t.Fatalf("a refused uninstall was not kept as an error: %+v", failed.shellManage)
	}
}

// shellAgentMainSelection selects "On my existing Pi" with Main preview and
// confirms the experience screen.
func shellAgentMainSelection(t *testing.T) Model {
	t.Helper()
	main := openTargetConfiguration(t)
	main = experienceFocus(t, main, screens.ShellExperienceInstallTarget, "agent")
	next, _ := main.Update(tea.KeyMsg{Type: tea.KeyEnter})
	main = next.(Model)
	profile := main.shellExperience.profile
	profile.SDKSource = "main-preview"
	main.shellExperience.profile = profile
	next, _ = main.Update(tea.KeyMsg{Type: tea.KeyEnd})
	main = next.(Model)
	next, _ = main.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model)
}

func containsText(value, part string) bool {
	return part == "" || strings.Contains(value, part)
}
