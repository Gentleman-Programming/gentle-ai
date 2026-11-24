//go:build windows

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The Node pin is the digest of the node.zip archive, never of the unpacked
// node.exe. The update check must compare the retained archive (which launch
// verification binds to runtime/node) with the pin; comparing node.exe with
// the archive digest reported "Node runtime" outdated on every installation.
func TestWindowsStableCheckNodeComparesRetainedArchive(t *testing.T) {
	_, target, _ := writeWindowsChannelFixture(t)
	userWindowsRecordSDKVersion(t, target, "4.6.0")
	archive, err := os.ReadFile(filepath.Join(target, "runtime", "archives", "node.zip"))
	if err != nil {
		t.Fatal(err)
	}
	unpacked, err := os.ReadFile(filepath.Join(target, "runtime", "node", "node.exe"))
	if err != nil || userWindowsSHA(unpacked) == userWindowsSHA(archive) {
		t.Fatal("fixture must keep distinct archive and unpacked node bytes:", err)
	}
	embedded, listBytes, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	stub := embedded
	stub.Releases = map[string]userReviewedSupplier{"4.6.0": embedded.Releases["4.5.0"]}
	previousList := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return stub, listBytes, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previousList })
	previous := userWindowsArtifacts[0].SHA
	t.Cleanup(func() { userWindowsArtifacts[0].SHA = previous })
	check := func() UserShellUpdate {
		t.Helper()
		update, err := CheckUserShellUpdate(context.Background(), UserShellInstall{Target: target, Mode: "separate", Channel: "stable"})
		if err != nil {
			t.Fatal(err)
		}
		return update
	}

	userWindowsArtifacts[0].SHA = userWindowsSHA(archive)
	if update := check(); slices.Contains(update.Scope, UserComponentNode) {
		t.Fatalf("Node runtime reported outdated although the retained node.zip matches the pin: %+v", update)
	}
	userWindowsArtifacts[0].SHA = userWindowsSHA([]byte("a newer pinned node archive"))
	if update := check(); !slices.Contains(update.Scope, UserComponentNode) {
		t.Fatalf("Node runtime not reported outdated for a different pinned archive: %+v", update)
	}
}

// A damaged installation without its retained node.zip fails the check
// instead of reporting the Node runtime current.
func TestWindowsStableCheckNodeArchiveMissingErrors(t *testing.T) {
	_, target, _ := writeWindowsChannelFixture(t)
	userWindowsRecordSDKVersion(t, target, "4.6.0")
	if err := os.Remove(filepath.Join(target, "runtime", "archives", "node.zip")); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckUserShellUpdate(context.Background(), UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}); err == nil {
		t.Fatal("a missing retained node.zip must fail the check, not report the runtime current")
	}
}
