package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

func TestShellOriginalHeroCampaign(t *testing.T) {
	view := RenderShellInstallHero(true, 120, 12)
	for _, want := range []string{
		ShellInstallHeroTitle, "The complete Pi experience.",
		"MEMORY • ODD • SKILLS • SAFE AUTOMATION", "A managed foundation for your Pi workflow.", "╭", "╯",
	} {
		if !strings.Contains(ansi.Strip(view), want) {
			t.Fatalf("original campaign copy/frame missing %q:\n%s", want, view)
		}
	}
	for _, want := range []string{
		styles.SelectedStyle.Render(styles.Cursor + ShellInstallHeroTitle),
		styles.TitleStyle.Render("MEMORY"), styles.HeadingStyle.Render("ODD"),
		lipgloss.NewStyle().Foreground(styles.ColorBlue).Bold(true).Render("SKILLS"),
		lipgloss.NewStyle().Foreground(styles.ColorTeal).Bold(true).Render("SAFE AUTOMATION"),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("original campaign styling missing %q", want)
		}
	}
	if strings.Contains(RenderShellInstallHero(false, 120, 12), styles.Cursor+ShellInstallHeroTitle) {
		t.Fatal("unfocused hero claims keyboard selection")
	}
}

func TestShellOriginalPresentationViewportMatrix(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	for _, width := range []int{1, 2, 3, 5, 6, 12, 30, 80, 100, 209} {
		for _, height := range []int{1, 2, 3, 4, 5, 7, 14, 30, 61} {
			for _, selected := range []bool{false, true} {
				assertShellOriginalViewport(t, RenderShellInstallHero(selected, width, height), width, height)
			}
			for _, entry := range ShellExperienceOptionsWithUninstall(profile, false) {
				assertShellOriginalViewport(t, RenderShellExperience(profile, entry.Cursor, nil, width, height), width, height)
			}
		}
	}
	for _, size := range [][2]int{{0, 1}, {1, 0}, {-1, 4}, {80, -1}} {
		if RenderShellInstallHero(true, size[0], size[1]) != "" {
			t.Fatal("invalid hero viewport is not empty", size)
		}
	}
}

func TestShellOriginalMenuPreferenceProjection(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	for _, entry := range shellExperienceEntries(profile, false, "Continue") {
		if entry.Kind == "foundation" && entry.Selectable {
			t.Fatal("foundation information became a configuration action")
		}
		if entry.Kind == "toggle" && entry.Control != ShellExperienceBackground && (entry.Checked || entry.Value != "Inherited") {
			t.Fatal("inherited preference presented as an explicit override", entry)
		}
	}
	view := RenderShellExperience(profile, 0, nil, 209, 61)
	for _, unwanted := range []string{"SDD/OpenSpec", "Healthy", "0 issues", "Release channel"} {
		if strings.Contains(view, unwanted) {
			t.Fatal("restored menu invented an unsupported state or control", unwanted)
		}
	}
}

func assertShellOriginalViewport(t *testing.T, view string, width, height int) {
	t.Helper()
	if lipgloss.Height(view) > height || lipgloss.Width(view) > width {
		t.Fatalf("restored UI exceeds %dx%d:\n%s", width, height, view)
	}
}
