//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeWindowsChannelFixture builds a minimal canonical Separate installation
// that passes userWindowsManifestRead, plus the stable runtime state the
// stable update check compares.
func writeWindowsChannelFixture(t *testing.T) (home, target string, install UserShellInstall) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("LOCALAPPDATA", home)
	target = filepath.Join(home, "gentle-shell", "install")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(target); err != nil {
		t.Fatal(err)
	}
	selection, err := json.Marshal(UserInstallRequest{Destination: target, Mode: "separate", Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	lock := []byte(`{"lockfileVersion":3,"packages":{}}`)
	if err := os.MkdirAll(filepath.Join(target, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(target, "source", "package-lock.json"), lock); err != nil {
		t.Fatal(err)
	}
	identity, err := userWindowsIdentity(target, true)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := userWindowsSID()
	if err != nil {
		t.Fatal(err)
	}
	manifest := userWindowsManifest{Schema: userWindowsSchema, Destination: target, SID: sid, Identity: identity, SupervisorSHA: "fixture-supervisor", SelectionSHA: userWindowsSHA(selection), LockSHA: userWindowsSHA(lock)}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(target, "selection.json"), selection); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(target, "manifest.json"), append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "runtime", "node"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(target, "runtime", "node", "node.exe"), []byte("fixture node, never executed")); err != nil {
		t.Fatal(err)
	}
	return home, target, UserShellInstall{Target: target, Mode: "separate", Channel: "stable", Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent")}
}

func TestWindowsInventoryAcceptsUpdateChannelPreference(t *testing.T) {
	_, target, _ := writeWindowsChannelFixture(t)
	if err := userWindowsReplaceFile(userUpdateChannelPath(target), userUpdateChannelBytes(userChannelMain)); err != nil {
		t.Fatal(err)
	}
	if channel, ok := userReadUpdateChannel(target); !ok || channel != userChannelMain {
		t.Fatalf("channel preference readback differs: %q %v", channel, ok)
	}
	if err := userWindowsInventory(target, userWindowsInventoryLaunch); err != nil {
		t.Fatalf("launch inventory refused the channel preference: %v", err)
	}
	if err := userWindowsInventory(target, userWindowsInventoryInstalled); err != nil {
		t.Fatalf("installed inventory refused the channel preference: %v", err)
	}
}

func TestWindowsChannelPreferenceAndCheckRouting(t *testing.T) {
	home, target, install := writeWindowsChannelFixture(t)
	if installs := DiscoverUserShellInstalls(home); len(installs) != 1 || installs[0].Channel != "stable" {
		t.Fatalf("stock discovery differs: %+v", installs)
	}
	// Apply preferences switches the persisted channel to Main in place.
	selected := UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: "main-preview"}
	if err := ApplyUserExperiencePreferences(install, selected); err != nil {
		t.Fatal(err)
	}
	if channel, ok := userReadUpdateChannel(target); !ok || channel != userChannelMain {
		t.Fatalf("Main preference bytes differ: %q %v", channel, ok)
	}
	installs := DiscoverUserShellInstalls(home)
	if len(installs) != 1 || installs[0].Channel != "main" {
		t.Fatalf("discovery did not follow the Main preference: %+v", installs)
	}
	loaded, err := LoadUserShellExperience(installs[0])
	if err != nil || loaded.SDKSource != "main-preview" {
		t.Fatalf("load did not follow the Main preference: %+v %v", loaded, err)
	}
	update, err := CheckUserShellUpdate(context.Background(), install)
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "main-preview" || !update.Available || len(update.Components) != 1 || update.Components[0] != "SDK Main snapshot" {
		t.Fatalf("Main routing differs: %+v", update)
	}
	// Applying Stable switches back and routes the check at the stable
	// components of this build.
	selected.SDKSource = "stable"
	if err := ApplyUserExperiencePreferences(install, selected); err != nil {
		t.Fatal(err)
	}
	if loaded, err = LoadUserShellExperience(install); err != nil || loaded.SDKSource != "stable" {
		t.Fatalf("load did not follow the Stable preference: %+v %v", loaded, err)
	}
	if installs = DiscoverUserShellInstalls(home); len(installs) != 1 || installs[0].Channel != "stable" {
		t.Fatalf("discovery did not follow the Stable preference: %+v", installs)
	}
	update, err = CheckUserShellUpdate(context.Background(), install)
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "stable" || !update.Available || len(update.Components) != 2 {
		t.Fatalf("Stable routing differs: %+v", update)
	}
	if _, err := os.Lstat(filepath.Join(target, "config", "update-channel.json")); err != nil {
		t.Fatalf("channel preference lost: %v", err)
	}
}

func TestWindowsApplyRefusesNonSeparateInstall(t *testing.T) {
	if err := ApplyUserExperiencePreferences(UserShellInstall{Target: filepath.Join(t.TempDir(), "agent"), Mode: "agent"}, UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}); err == nil {
		t.Fatal("in-place preferences accepted a non-Separate installation")
	}
}

func TestWindowsChannelBytesRejectNonCanonical(t *testing.T) {
	for _, raw := range []string{`{"channel":"canary"}`, `{"channel":""}`, `not json`, `{"channel":"main"}{"channel":"stable"}`} {
		if _, ok := userUpdateChannelFromBytes([]byte(raw)); ok {
			t.Fatalf("non-canonical channel bytes accepted: %q", raw)
		}
	}
	if channel, ok := userUpdateChannelFromBytes(userUpdateChannelBytes(userChannelStable)); !ok || channel != userChannelStable {
		t.Fatalf("canonical Stable bytes refused: %q %v", channel, ok)
	}
	if channel, ok := userUpdateChannelFromBytes(userUpdateChannelBytes(userChannelMain)); !ok || channel != userChannelMain {
		t.Fatalf("canonical Main bytes refused: %q %v", channel, ok)
	}
}
