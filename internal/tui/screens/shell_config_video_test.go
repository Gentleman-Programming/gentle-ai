package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

func TestShellConfigVideoGroupHeadings(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	view := RenderShellExperience(profile, 0, nil, 209, 61)
	left, _ := shellConfigVideoColumns(view)
	position := -1
	for _, heading := range []string{"REQUIRED FOUNDATION", "PERSONA", "RECOMMENDED", "SDK SOURCE", "ACTIONS"} {
		next := strings.Index("\n"+left+"\n", "\n"+heading+"\n")
		if next <= position {
			t.Fatalf("video group heading missing or out of order: %q:\n%s", heading, left)
		}
		position = next
	}
	for _, styled := range []string{
		styles.TitleStyle.Render("REQUIRED FOUNDATION"),
		styles.HeadingStyle.Render("PERSONA"),
		styles.SuccessStyle.Bold(true).Render("RECOMMENDED"),
		styles.WarningStyle.Bold(true).Render("SDK SOURCE"),
	} {
		if !strings.Contains(view, styled) {
			t.Fatalf("video group styling missing %q", styled)
		}
	}
	if strings.Contains(ansi.Strip(view), "Required foundation • Persona") {
		t.Fatal("category breadcrumb replaced the actual group headings")
	}
	if strings.Contains(ansi.Strip(view), "ADVANCED") {
		t.Fatal("removed Advanced section returned")
	}
}

func TestShellConfigVideoFocusedPanels(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	for _, entry := range ShellExperienceOptionsWithUninstall(profile, false) {
		if entry.Kind == "action" {
			continue
		}
		t.Run(entry.Label, func(t *testing.T) {
			panel := shellConfigVideoPanel(t, profile, entry.Control, entry.Choice)
			reason := "WHY WE RECOMMEND IT"
			if entry.Control == ShellExperienceSDK && entry.Choice == "main-preview" {
				reason = "WHEN TO USE MAIN PREVIEW"
			}
			for _, want := range []string{"ABOUT THIS OPTION\n" + entry.Label, "WHAT IT CHANGES", "CURRENT VALUE", "GENTLEMAN PROGRAMMING RECOMMENDS", reason, "Enter:"} {
				if !strings.Contains(panel, want) {
					t.Fatalf("video contextual panel missing %q:\n%s", want, panel)
				}
			}
			if strings.Contains(panel, "Focused:") || strings.Contains(panel, "Entry value:") {
				t.Fatal("generic summary substituted for the original contextual panel")
			}
		})
	}
}

func TestShellConfigVideoCurrentValueIsNotFocusOrRecommendation(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	profile.Persona = "neutral"
	panel := shellConfigVideoPanel(t, profile, ShellExperiencePersona, "gentleman")
	if !strings.Contains(panel, "CURRENT VALUE\nneutral\n") {
		t.Fatalf("focused Gentleman overwrote the actual persona value:\n%s", panel)
	}
	panel = shellConfigVideoPanel(t, profile, ShellExperienceTDD, "")
	if !strings.Contains(panel, "CURRENT VALUE\nInherited\n") {
		t.Fatalf("inherited preference was presented as OFF or a recommendation:\n%s", panel)
	}
	profile.SDKSource = "stable"
	panel = shellConfigVideoPanel(t, profile, ShellExperienceSDK, "main-preview")
	for _, want := range []string{"CURRENT VALUE\nSDK source: stock (stable)\n", "SDK Main + stock provider", "On my existing Pi or Separate", "WHEN TO USE MAIN PREVIEW"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("SDK focus lost current value or qualification %q:\n%s", want, panel)
		}
	}
}

func TestShellConfigVideoActionsHaveShortPanels(t *testing.T) {
	profile := shellinstaller.DefaultUserExperience()
	for _, action := range []struct {
		control ShellExperienceControl
		label   string
		hint    string
	}{
		{ShellExperienceContinue, "Continue", "Enter: continue"},
		{ShellExperienceCancel, "Cancel", "Enter: cancel"},
	} {
		t.Run(action.label, func(t *testing.T) {
			panel := shellConfigVideoPanel(t, profile, action.control, "")
			for _, want := range []string{"ABOUT THIS OPTION\n" + action.label, "WHAT IT CHANGES", action.hint} {
				if !strings.Contains(panel, want) {
					t.Fatalf("video action panel missing %q:\n%s", want, panel)
				}
			}
			for _, unwanted := range []string{"CURRENT VALUE", "GENTLEMAN PROGRAMMING RECOMMENDS", "WHY WE RECOMMEND IT", "SDK source:"} {
				if strings.Contains(panel, unwanted) {
					t.Fatalf("action panel contains unrelated preference summary %q:\n%s", unwanted, panel)
				}
			}
		})
	}
}

func shellConfigVideoPanel(t *testing.T, profile shellinstaller.UserExperience, control ShellExperienceControl, choice string) string {
	t.Helper()
	for _, entry := range ShellExperienceOptionsWithUninstall(profile, false) {
		if entry.Control == control && entry.Choice == choice {
			_, right := shellConfigVideoColumns(RenderShellExperience(profile, entry.Cursor, nil, 209, 61))
			return right
		}
	}
	t.Fatal("configuration entry missing", control, choice)
	return ""
}

func shellConfigVideoColumns(view string) (string, string) {
	var left, right []string
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		l, r, divided := strings.Cut(line, "│")
		if divided {
			left = append(left, strings.TrimSpace(l))
			right = append(right, strings.Trim(r, " │"))
		}
	}
	return strings.Join(left, "\n"), strings.Join(right, "\n")
}
