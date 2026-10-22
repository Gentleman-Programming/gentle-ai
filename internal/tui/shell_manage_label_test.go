package tui

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestManageContinueLabelReflectsPendingChanges(t *testing.T) {
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	current, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	config := shellExperienceConfig{profile: current, mode: "separate", reconfigure: true, existing: []shellinstaller.UserShellInstall{install}}
	if got := manageContinueLabel(config); got != "Continue" {
		t.Fatalf("unchanged label = %q", got)
	}

	preference := config
	preference.profile.Persona = "neutral"
	if got := manageContinueLabel(preference); got != "Apply changes to installation" {
		t.Fatalf("preference label = %q", got)
	}

	component := config
	component.profile.CodeGraph = "on"
	if got := manageContinueLabel(component); got != "Update installation" {
		t.Fatalf("component label = %q", got)
	}

	// SDK source is an update-channel preference: switching Stable ↔ Main
	// offers the in-place apply label, never a component update.
	sdk := config
	sdk.profile.SDKSource = "main-preview"
	if got := manageContinueLabel(sdk); got != "Apply changes to installation" {
		t.Fatalf("SDK source label = %q", got)
	}
	preferences, components := manageChanges(config.profile, sdk.profile, true)
	if len(preferences) != 1 || preferences[0].Label != "SDK source" || len(components) != 0 {
		t.Fatalf("SDK source classification differs: %+v / %+v", preferences, components)
	}
}
