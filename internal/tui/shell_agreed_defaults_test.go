package tui

import (
	"os"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestShellAgreedFreshDefaults(t *testing.T) {
	t.Setenv("HOME", shellTestHome(t))
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
	// CodeGraph ON reaches the plan on macOS arm64 exactly as on Linux amd64.
	planned := want
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnd}, {Type: tea.KeyEnter}} {
		next, cmd = m.Update(key)
		m = next.(Model)
		if cmd != nil {
			t.Fatal("Continue must not start a worker")
		}
	}
	state, ok := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if !ok || m.shellExperience != nil || state.Request.Experience == nil || *state.Request.Experience != want || state.Request.Confirmation != "" || m.ShellInstallSelection().Confirmed || m.shellPlanRequest.Experience == nil || *m.shellPlanRequest.Experience != planned {
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

// Every agreed default except CodeGraph is qualified on Linux amd64 and macOS
// arm64: turning CodeGraph OFF reaches the inspected installation plan with
// the remaining selections intact and without consent or filesystem effects.
func TestShellAgreedDefaultsWithoutCodeGraphReachPlan(t *testing.T) {
	stubShellKernelGate(t)
	m := experienceStart(t)
	m = experienceFocus(t, m, screens.ShellExperienceCodeGraph, "")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = experienceKey(experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd}), tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan == nil || m.shellExperience != nil {
		t.Fatalf("defaults without CodeGraph did not reach the plan: %+v", m.shellExperience)
	}
	want := shellinstaller.UserExperience{
		Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on",
		GlobalRDD: "on", CodeGraph: "off", Context7: "on", GGA: "off",
	}
	request := m.shellPlanRequest
	if request.Experience == nil || *request.Experience != want || request.Mode != "separate" || request.Confirmation == "" || m.ShellInstallSelection().Confirmed {
		t.Fatalf("plan request differs: %+v %+v", request, request.Experience)
	}
	if !strings.Contains(m.View(), "Installation plan") {
		t.Fatalf("plan screen is not shown:\n%s", m.View())
	}
	if _, err := os.Lstat(request.Destination); !os.IsNotExist(err) {
		t.Fatal("plan created the destination before consent", err)
	}
}

// macOS arm64 ships the Main composition: choosing Main preview with every
// agreed default (CodeGraph ON included) reaches the inspected plan.
func TestShellDarwinMainPreviewReachesPlan(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("macOS arm64 channel boundary")
	}
	m := experienceStart(t)
	m = experienceFocus(t, m, screens.ShellExperienceSDK, "main-preview")
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellExperience.err != nil || m.shellExperience.profile.SDKSource != "main-preview" {
		t.Fatalf("Main preview was not selected on macOS: %+v", m.shellExperience)
	}
	if strings.Contains(m.View(), "not available on macOS") {
		t.Fatalf("macOS still names Main preview as unavailable:\n%s", m.View())
	}
	_, request := experiencePlan(t, m)
	want := shellinstaller.UserExperience{
		Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on",
		GlobalRDD: "on", CodeGraph: "on", Context7: "on", GGA: "off", SDKSource: "main-preview",
	}
	if request.Experience == nil || *request.Experience != want || request.Mode != "separate" {
		t.Fatalf("Main preview plan differs: %+v %+v", request, request.Experience)
	}
}
