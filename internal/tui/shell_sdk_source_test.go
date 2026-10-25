package tui

import (
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func sdkSourceRow(t *testing.T, m Model) Model {
	return experienceFocus(t, m, screens.ShellExperienceSDK, "stable")
}

func TestShellExperienceSDKSourceHandoff(t *testing.T) {
	m := sdkSourceRow(t, experienceInherited(t))
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellExperience == nil || m.shellExperience.profile.SDKSource != "stable" {
		t.Fatal("SDK stock choice missing from Configure")
	}
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		if m.shellExperience.profile.SDKSource != "stable" || m.shellExperience.err == nil {
			t.Fatal("unqualified Main choice offered")
		}
		return
	}
	if m.shellExperience == nil || m.shellExperience.profile.SDKSource != "main-preview" || !strings.Contains(m.View(), "SDK Main + stock provider") {
		t.Fatal("Main composition not selected or disclosed")
	}
	m, request := experiencePlan(t, m)
	if request.Experience == nil || request.Experience.SDKSource != "main-preview" || !strings.Contains(m.View(), "Installation plan") {
		t.Fatalf("Main did not reach the installation plan: %+v", request)
	}
	if m.ShellInstallSelection().Confirmed {
		t.Fatal("plan inspection forged consent")
	}
	back := experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if back.shellExperience == nil {
		t.Fatal("plan escape lost Configure")
	}
	back = sdkSourceRow(t, back)
	back = experienceKey(back, tea.KeyMsg{Type: tea.KeyEnter}) // back to stock
	_, second := experiencePlan(t, back)
	if second.Experience.SDKSource != "stable" || second.Confirmation == request.Confirmation {
		t.Fatal("SDK edit retained the old inspection or source")
	}
	args := cli.ShellInstallArguments(cli.ShellInstallSelection{Request: second, Confirmed: true})
	if len(args) != 8 || args[6] != "--experience" {
		t.Fatal("Main consent changed positional worker ABI", args)
	}
	req, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil || req.Experience == nil || req.Experience.SDKSource != "stable" || req.Confirmation != second.Confirmation {
		t.Fatal("selection lost before worker", err)
	}
}

func TestShellExperienceSDKSourceResponsiveDisclosure(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	profile.SDKSource = "main-preview"
	req := shellinstaller.UserInstallRequest{Destination: "/selected", Mode: "separate", Experience: &profile}
	for _, size := range [][2]int{{120, 30}, {80, 14}, {30, 7}, {5, 1}} {
		views := []string{screens.RenderShellExperience(profile, 8, nil, size[0], size[1]), screens.RenderShellReady(req, "", size[0], size[1], 0, 0), screens.RenderShellReady(req, "", size[0], size[1], screens.ShellReadyLastOffset(req, "", size[0], size[1]), 0)}
		for _, view := range views {
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatal("SDK disclosure exceeds viewport height")
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("SDK disclosure exceeds viewport width")
				}
			}
		}
	}
}
