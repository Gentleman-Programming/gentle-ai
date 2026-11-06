package screens

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func assertShellFlowBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) > height {
		t.Fatalf("flow exceeds viewport height %d: %d", height, len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			t.Fatalf("flow exceeds viewport width %d: %q", width, line)
		}
	}
}

func TestShellFlowDestinationViewportMatrix(t *testing.T) {
	for _, mode := range []string{"separate", "shared"} {
		for _, width := range []int{1, 9, 40, 80, 100, 209} {
			for _, height := range []int{1, 3, 10, 24, 35, 61} {
				t.Run(fmt.Sprintf("%s-%dx%d", mode, width, height), func(t *testing.T) {
					profile := shellinstaller.DefaultUserExperience()
					req := shellinstaller.UserInstallRequest{Destination: "/private/" + strings.Repeat("long-path-", 24), Mode: mode, SharedPrefix: "/shared/prefix", SharedAgent: "/shared/agent", Experience: &profile}
					for field, label := range []string{"Target", "Mode", "Shared prefix", "Shared agent"} {
						if mode != "shared" && field > 1 {
							continue
						}
						for _, problem := range []error{nil, errors.New("unsafe parent: " + strings.Repeat("reason ", 40))} {
							view := RenderUserInstallDestination(req, field, problem, width, height)
							assertShellFlowBounds(t, view, width, height)
							if width >= 100 && height >= 10 && !strings.Contains(ansi.Strip(view), "About: "+label) {
								t.Fatal("wide destination lost contextual field help", label)
							}
							if req.Experience != &profile || req.Confirmation != "" {
								t.Fatal("rendering changed request ownership or consent")
							}
						}
					}
				})
			}
		}
	}
}

func TestShellFlowReviewViewportMatrix(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	profile.SDKSource = "main-preview"
	req := shellinstaller.UserInstallRequest{Destination: "/private/target", Mode: "separate", Confirmation: strings.Repeat("a", 64), Experience: &profile}
	for _, width := range []int{1, 9, 40, 80, 100, 209} {
		for _, height := range []int{1, 3, 10, 24, 35, 61} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				last := ShellReadyLastOffset(req, "inspection\n"+strings.Repeat("evidence ", 40), width, height)
				for action, label := range shellReadyActions {
					for _, offset := range []int{-1, 0, last, last + 50} {
						view := RenderShellReady(req, "inspection\n"+strings.Repeat("evidence ", 40), width, height, offset, action)
						assertShellFlowBounds(t, view, width, height)
						if width >= 100 && height >= 10 {
							for _, part := range []string{"Plan", ">" + label, "About: " + label} {
								if !strings.Contains(ansi.Strip(view), part) {
									t.Fatal("wide review lost historical action context", part)
								}
							}
						}
					}
				}
			})
		}
	}
}

func TestShellFlowReviewEveryDataLineRemainsReachable(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	profile.SDKSource = "main-preview"
	req := shellinstaller.UserInstallRequest{Destination: "/private/" + strings.Repeat("target_", 40), Mode: "shared", SharedPrefix: "/owned/prefix", SharedAgent: "/owned/agent", Confirmation: strings.Repeat("b", 64), Experience: &profile}
	preview := "\x1b[31mEvidence with spaces\x1b[0m\n\n" + strings.Repeat("z", 180)
	for _, width := range []int{1, 9, 40, 80, 100, 209} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			// At height three the original compact review has exactly one data
			// line, so each offset proves reachability through the public view.
			data := shellReadyLines(req, preview, width)
			last := ShellReadyLastOffset(req, preview, width, 3)
			if last != len(data)-1 {
				t.Fatal("pager contract differs from rendered data")
			}
			for offset, line := range data {
				view := RenderShellReady(req, preview, width, 3, offset, 0)
				if !strings.Contains(ansi.Strip(view), ansi.Strip(line)) {
					t.Fatalf("review data line %d became unreachable", offset)
				}
			}
			logical := ansi.Strip(strings.Join(data, ""))
			for _, part := range []string{req.Destination, req.SharedPrefix, req.SharedAgent, req.Confirmation, "Selected experience", "Changes + scopes", "SDK Main + stock provider", "stock native/pins remain", "Confirm this physical selection", "Evidence with spaces", strings.Repeat("z", 180)} {
				if !strings.Contains(logical, part) {
					t.Fatal("complete request, current qualification or preview lost", part)
				}
			}
		})
	}
}
