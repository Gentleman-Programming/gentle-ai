package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Every preference file is optional: a Custom persona, a partial profile or a
// CLI install without --experience leaves some absent, and each absent file
// loads as its default value instead of failing Manage. A present but
// non-regular entry stays a refusal, never a silent default.
func TestWindowsLoadUserShellExperienceMissingOptionalFiles(t *testing.T) {
	_, target, install := writeWindowsChannelFixture(t)
	want := UserExperience{Persona: "unmanaged", BackgroundSubagents: "on", GGA: "off", SDKSource: "stable"}
	loaded, err := LoadUserShellExperience(install)
	if err != nil || loaded != want {
		t.Fatalf("absent preference files did not load as defaults: %+v %v", loaded, err)
	}
	// Only some files present: the rest keep their defaults.
	if err := userWindowsWrite(filepath.Join(target, "config", "background-subagents.json"), []byte(`{"schema":"gentle-pi.background-subagents/v1","policy":"off"}`)); err != nil {
		t.Fatal(err)
	}
	want.BackgroundSubagents = "off"
	if loaded, err = LoadUserShellExperience(install); err != nil || loaded != want {
		t.Fatalf("partial profile differs: %+v %v", loaded, err)
	}
	if err := os.Mkdir(filepath.Join(target, "config", "persona.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if loaded, err = LoadUserShellExperience(install); err == nil {
		t.Fatalf("a non-regular persona entry loaded as a default: %+v", loaded)
	}
}

// The loaded SDK source always names the installed composition, as on Unix:
// a Stable selection loads "stable" and a Main selection "main-preview" even
// without a channel preference file or a discovered channel, and a persisted
// preference overrides the composition for this projection.
func TestWindowsLoadUserShellExperienceReportsInstalledSDKSource(t *testing.T) {
	_, target, _ := writeWindowsChannelFixture(t)
	bare := UserShellInstall{Target: target, Mode: "separate"} // No discovered channel.
	load := func(want string) {
		t.Helper()
		loaded, err := LoadUserShellExperience(bare)
		if err != nil || loaded.SDKSource != want {
			t.Fatalf("SDK source = %q (%v), want %q", loaded.SDKSource, err, want)
		}
	}
	load("stable") // Stable install, no channel preference file.
	main, err := json.Marshal(UserInstallRequest{Destination: target, Mode: "separate", Channel: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReplaceFile(filepath.Join(target, "selection.json"), main); err != nil {
		t.Fatal(err)
	}
	load("main-preview") // Main install, no channel preference file.
	if err := userWindowsReplaceFile(userUpdateChannelPath(target), userUpdateChannelBytes(userChannelStable)); err != nil {
		t.Fatal(err)
	}
	load("stable") // A Stable preference overrides the Main composition.
	if err := userWindowsReplaceFile(userUpdateChannelPath(target), userUpdateChannelBytes(userChannelMain)); err != nil {
		t.Fatal(err)
	}
	load("main-preview")
	if loaded, err := LoadUserShellExperience(UserShellInstall{Target: target, Mode: "separate", Channel: "main"}); err != nil || loaded.SDKSource != "main-preview" {
		t.Fatalf("discovered Main channel did not load as Main preview: %+v %v", loaded, err)
	}
}
