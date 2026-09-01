package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWrapWelcomeBanner_UsesDisplayWidthForWideAdvisory(t *testing.T) {
	const width = 40
	text := "Advisory: 🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀 deployment notice must stay inside the welcome frame"

	wrapped := wrapWelcomeBanner(text, width)

	for i, line := range strings.Split(wrapped, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("wrapped line %d display width = %d, want <= %d\nline: %q\nwrapped:\n%s", i, got, width, line, wrapped)
		}
	}
}

func TestWrapWelcomeBanner_FormatsRecognizableAdvisoryAsList(t *testing.T) {
	const width = 72
	text := "Advisory: 🚀 Big gentle-ai update: SDD now self-validates every phase. Plus: engram survives brew upgrades, Windows install fixes land, and a revamped updater detects beta commits. Thanks for testing. See https://example.test/release"

	wrapped := wrapWelcomeBanner(text, width)

	for _, want := range []string{
		"Advisory: 🚀 Big gentle-ai update:",
		"• SDD now self-validates every phase.",
		"• engram survives brew upgrades",
		"• Windows install fixes land",
		"• a revamped updater detects beta commits.",
	} {
		if !strings.Contains(wrapped, want) {
			t.Fatalf("wrapped advisory missing %q\nwrapped:\n%s", want, wrapped)
		}
	}

	for i, line := range strings.Split(wrapped, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("wrapped line %d display width = %d, want <= %d\nline: %q\nwrapped:\n%s", i, got, width, line, wrapped)
		}
	}
}

func TestWrapWelcomeBanner_PreservesPlainTextAdvisory(t *testing.T) {
	text := "Advisory: maintenance window tonight"

	if got := wrapWelcomeBanner(text, 80); got != text {
		t.Fatalf("wrapWelcomeBanner() = %q, want %q", got, text)
	}
}

func TestRenderWelcome_StaysWithinViewport(t *testing.T) {
	cases := []struct {
		name         string
		width        int
		height       int
		updateBanner string
		advisory     WelcomeAdvisory
		minimum      bool
		wantPrimary  string
		wantControl  string
	}{
		{name: "Windows Terminal startup", width: 120, height: 30},
		{name: "narrow resize", width: 80, height: 24},
		{name: "short viewport", width: 120, height: 19},
		{name: "below compact height", width: 120, height: 2, minimum: true},
		{name: "below compact width", width: 18, height: 20, minimum: true},
		{name: "below frame border width", width: 2, height: 20, minimum: true, wantPrimary: "Go"},
		{name: "tiny viewport uses atomic labels", width: 2, height: 2, minimum: true, wantPrimary: "Go", wantControl: "q"},
		{name: "single column tiny viewport uses atomic labels", width: 1, height: 2, minimum: true, wantPrimary: ">", wantControl: "q"},
		{
			name:         "short viewport with optional content",
			width:        120,
			height:       19,
			updateBanner: strings.Repeat("Updates available with more detail ", 20),
			advisory:     WelcomeAdvisory{Message: "Optional advisory content"},
		},
		{
			name:         "compact viewport with optional content",
			width:        120,
			height:       17,
			updateBanner: "Updates available",
			advisory:     WelcomeAdvisory{Message: "Optional advisory content"},
		},
		{name: "wide resize", width: 160, height: 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := RenderWelcomeWithAdvisory(0, "dev", tc.updateBanner, nil, true, false, 0, true, tc.width, tc.height, tc.advisory)

			if got := lipgloss.Width(view); got > tc.width {
				t.Fatalf("welcome width = %d, want <= %d\nview:\n%s", got, tc.width, view)
			}
			if got := lipgloss.Height(view); got > tc.height {
				t.Fatalf("welcome height = %d, want <= %d\nview:\n%s", got, tc.height, view)
			}
			content := view
			if tc.minimum {
				content = strings.ReplaceAll(view, "\n", "")
			}
			primary := tc.wantPrimary
			if primary == "" {
				primary = "Start installation"
			}
			if !strings.Contains(content, primary) {
				t.Fatalf("welcome lost primary action %q after fitting viewport\nview:\n%s", primary, view)
			}
			if tc.minimum {
				control := tc.wantControl
				if control == "" {
					control = "j/k"
				}
				for _, want := range []string{primary, control} {
					if !strings.Contains(content, want) {
						t.Fatalf("minimum welcome state lost %q\nview:\n%s", want, view)
					}
				}
			} else {
				for _, want := range []string{"Quit", "j/k: navigate • enter: select • q: quit"} {
					if !strings.Contains(view, want) {
						t.Fatalf("welcome lost %q after fitting viewport\nview:\n%s", want, view)
					}
				}
			}
		})
	}
}
