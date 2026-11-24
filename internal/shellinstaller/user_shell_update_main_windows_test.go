//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// userWindowsMainCheck records a fully current Main installation (every pin
// of this build, the fixture node.zip as the Node pin and the stubbed latest
// snapshot), lets mutate make it stale, then runs the update check. No test
// touches the network.
func userWindowsMainCheck(t *testing.T, newSnapshot bool, mutate func(*userWindowsManifest)) UserShellUpdate {
	t.Helper()
	_, target, _ := writeWindowsChannelFixture(t)
	userWindowsRecordSDKVersion(t, target, "4.6.0")
	commit := "0d181e872b06c6067741513fd6d2f562deaad5c1"
	artifact, _ := userWindowsOverlayMainFixture(t, commit)
	descriptor, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(target, "main.json"), descriptor); err != nil {
		t.Fatal(err)
	}
	selection, err := json.Marshal(UserInstallRequest{Destination: target, Mode: "separate", Channel: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReplaceFile(filepath.Join(target, "selection.json"), selection); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest userWindowsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.MainCommit, manifest.MainArchiveSHA, manifest.SelectionSHA = commit, artifact.SHA, userWindowsSHA(selection)
	// The stock SDK version is a Stable-only input: an unrecorded version
	// must not make a Main installation report its SDK outdated.
	manifest.SDKVersion = ""
	if mutate != nil {
		mutate(&manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReplaceFile(path, append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(target, "runtime", "archives", "node.zip"))
	if err != nil {
		t.Fatal(err)
	}
	previousNode := userWindowsArtifacts[0].SHA
	userWindowsArtifacts[0].SHA = userWindowsSHA(archive)
	t.Cleanup(func() { userWindowsArtifacts[0].SHA = previousNode })
	previousSnapshot := userWindowsUpdateMainSnapshot
	latest := artifact
	if newSnapshot {
		latest.SHA = "newer-main-archive-sha"
	}
	userWindowsUpdateMainSnapshot = func(context.Context) (userWindowsArtifact, []byte, error) { return latest, nil, nil }
	t.Cleanup(func() { userWindowsUpdateMainSnapshot = previousSnapshot })
	update, err := CheckUserShellUpdate(context.Background(), UserShellInstall{Target: target, Mode: "separate", Channel: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "main-preview" {
		t.Fatalf("Main check reported channel %q", update.Channel)
	}
	return update
}

// A current Main installation reports nothing, and the Stable-only stock SDK
// inputs (lock, reviewed version) never apply to it.
func TestWindowsMainCheckCurrentInstallReportsNothing(t *testing.T) {
	if update := userWindowsMainCheck(t, false, nil); update.Available || len(update.Scope) != 0 {
		t.Fatalf("current Main installation reported an update: %+v", update)
	}
}

// Issue #6239: the Main check returned right after the snapshot comparison,
// so a stale supervisor (or Node, or pins) was never offered on Main.
func TestWindowsMainCheckOffersStaleSupervisor(t *testing.T) {
	update := userWindowsMainCheck(t, false, func(m *userWindowsManifest) { m.SupervisorSHA = "older-supervisor" })
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSupervisor}) || !slices.Equal(update.Components, []string{"Gentle Shell supervisor"}) {
		t.Fatalf("stale supervisor not offered on Main: %+v", update)
	}
}

// A new snapshot and a stale Node runtime are offered together, with the
// Main SDK label and the snapshot detail.
func TestWindowsMainCheckCombinesSnapshotAndRuntime(t *testing.T) {
	update := userWindowsMainCheck(t, true, func(m *userWindowsManifest) { m.GoSHA = "older-go-pin" })
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentNode, UserComponentSDK}) {
		t.Fatalf("snapshot and runtime deltas not combined: %+v", update)
	}
	if !slices.Contains(update.Components, "SDK Main snapshot") || !slices.Contains(update.Components, "Node runtime") || update.Detail != "new Main snapshot available" {
		t.Fatalf("Main labels or detail differ: %+v", update)
	}
}

// The tools pin belongs to the SDK component on every channel: on Main it
// offers the Main SDK with the generic Main detail.
func TestWindowsMainCheckToolsPinOffersMainSDK(t *testing.T) {
	update := userWindowsMainCheck(t, false, func(m *userWindowsManifest) { m.ToolsSHA = "older-tools-pin" })
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSDK}) || !slices.Equal(update.Components, []string{"SDK Main snapshot"}) || update.Detail != "main channel components differ from this build" {
		t.Fatalf("tools pin delta not offered as the Main SDK: %+v", update)
	}
}

// The stock gentle-pi lock is a Stable input and never applies to Main.
func TestWindowsMainCheckIgnoresStockLock(t *testing.T) {
	if update := userWindowsMainCheck(t, false, func(m *userWindowsManifest) { m.SDKLockSHA += "-older" }); update.Available {
		t.Fatalf("stock lock applied to a Main installation: %+v", update)
	}
}
