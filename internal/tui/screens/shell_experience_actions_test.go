package screens

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellExperienceActionsPutUninstallLast(t *testing.T) {
	options := ShellExperienceOptionsWithUninstallAction(shellinstaller.DefaultUserExperience(), true, "Update installation")
	var order []ShellExperienceControl
	for _, option := range options {
		switch option.Control {
		case ShellExperienceUpdate, ShellExperienceContinue, ShellExperienceCancel, ShellExperienceUninstall:
			order = append(order, option.Control)
		}
	}
	want := []ShellExperienceControl{ShellExperienceUpdate, ShellExperienceContinue, ShellExperienceCancel, ShellExperienceUninstall}
	if len(order) != len(want) {
		t.Fatalf("action controls = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("action order = %v, want %v", order, want)
		}
	}
	for _, option := range options {
		if option.Control == ShellExperienceContinue && option.Label != "Update installation" {
			t.Fatalf("continue label = %q", option.Label)
		}
		if option.Control == ShellExperienceUninstall && !option.SeparatorBefore {
			t.Fatal("uninstall action has no visual separator")
		}
	}
}

func TestShellExperienceContinueDefaultsToContinue(t *testing.T) {
	options := ShellExperienceOptionsWithUninstall(shellinstaller.DefaultUserExperience(), false)
	for _, option := range options {
		if option.Control == ShellExperienceContinue && option.Label != "Continue" {
			t.Fatalf("default continue label = %q", option.Label)
		}
	}
}
