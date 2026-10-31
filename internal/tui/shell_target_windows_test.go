package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

// Windows installs Separate only. "On my existing Pi" is shown disabled with
// the Windows reason before Continue, choosing it keeps Separate, and Continue
// still reaches a Separate plan without any reused agent.
func TestShellExistingPiTargetDisabledOnWindows(t *testing.T) {
	stubShellKernelGate(t)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// A personal Pi agent exists; Windows still never offers it for reuse.
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	m := openTargetConfiguration(t)
	// Configure shows a row's description only in the detail panel of the
	// focused row, and the first frame focuses the persona. The first frame
	// must already carry the Windows reason as the detection note (no key
	// has reached the platform hook yet) and never the generic text.
	if view := m.View(); strings.Contains(view, "No Pi agent was detected") {
		t.Fatalf("first Configure frame shows the generic detection text:\n%s", view)
	}
	if m.shellExperience == nil || m.shellExperience.piNote != shellPlatformTargetNote {
		t.Fatalf("Windows detection note differs on the first frame: %+v", m.shellExperience)
	}
	m = experienceFocus(t, m, screens.ShellExperienceInstallTarget, "agent")
	if view := m.View(); !strings.Contains(view, "Windows supports Separate install only") || strings.Contains(view, "No Pi agent was detected") {
		t.Fatalf("existing-Pi row does not show the Windows reason before Continue:\n%s", view)
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellExperience == nil || m.shellExperience.mode != "separate" || m.shellExperience.choice != "separate" || m.shellExperience.agent != "" {
		t.Fatalf("Windows selected an existing-Pi target: %+v", m.shellExperience)
	}
	if m.shellExperience.err == nil || m.shellExperience.err.Error() != shellPlatformTargetNote {
		t.Fatalf("existing-Pi refusal differs: %v", m.shellExperience.err)
	}
	if m.shellPlan != nil || m.shellPlanRequest.Confirmation != "" {
		t.Fatal("refused target acquired inspection")
	}
	_, request := experiencePlan(t, m)
	if request.Mode != "separate" || request.SharedAgent != "" || request.SharedPrefix != "" {
		t.Fatalf("Windows plan is not Separate: %+v", request)
	}
}

// shellAssertPlatformWorkerHandoff requires the worker to parse the inspected
// Main channel (not the selector's default Stable field) with the profile and
// confirmation intact.
func shellAssertPlatformWorkerHandoff(t *testing.T, request shellinstaller.UserInstallRequest) {
	t.Helper()
	args := cli.ShellInstallArguments(cli.ShellInstallSelection{Request: request, Confirmed: true})
	if len(args) != 10 {
		t.Fatal("Main consent changed positional worker ABI", args)
	}
	parsed, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil || parsed.Channel != "main" || parsed.Experience == nil || *parsed.Experience != *request.Experience || parsed.Confirmation != request.Confirmation {
		t.Fatalf("Main selection lost before worker: %+v, %v", parsed, err)
	}
}
