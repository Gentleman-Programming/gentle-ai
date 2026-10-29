//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// userWindowsRecordSDKVersion rewrites the fixture manifest with the given
// stable SDK version and current pins, so the stable update check compares a
// fully recorded installation against this build.
func userWindowsRecordSDKVersion(t *testing.T, target, version string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest userWindowsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	self, err := userWindowsExecutable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := userWindowsReadTrusted(self, 256<<20, false)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SupervisorSHA = userWindowsSHA(image)
	if manifest.SDKLockSHA, err = userWindowsSDKLockSHA(); err != nil {
		t.Fatal(err)
	}
	if manifest.ToolsSHA, err = userWindowsToolsPinSHA(); err != nil {
		t.Fatal(err)
	}
	if manifest.GoSHA, err = userWindowsGoPinSHA(); err != nil {
		t.Fatal(err)
	}
	manifest.SDKVersion = version
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReplaceFile(filepath.Join(target, "manifest.json"), append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
}

// userWindowsStableCheckWithList runs the stable update check against a stub
// reviewed list whose newest release is the given version: no test touches the
// network. Every other component is made current — the fixture node bytes are
// pinned as this build's node artifact — so the SDK scope is the only signal
// the assertions read.
func userWindowsStableCheckWithList(t *testing.T, target, newest string) UserShellUpdate {
	t.Helper()
	embedded, bytes, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	stub := embedded
	stub.Releases = map[string]userReviewedSupplier{"4.3.0": embedded.Releases["4.3.0"]}
	stub.Releases[newest] = embedded.Releases["4.5.0"]
	node, err := os.ReadFile(filepath.Join(target, "runtime", "node", "node.exe"))
	if err != nil {
		t.Fatal(err)
	}
	previous := userWindowsArtifacts[0].SHA
	userWindowsArtifacts[0].SHA = userWindowsSHA(node)
	t.Cleanup(func() { userWindowsArtifacts[0].SHA = previous })
	previousList := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return stub, bytes, nil }
	defer func() { userSelectedReviewedListFunc = previousList }()
	update, err := CheckUserShellUpdate(context.Background(), UserShellInstall{Target: target, Mode: "separate", Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	return update
}

// S17(d): the stable SDK convergence test asserts what its messages say. The
// recorded gentle-pi version is the detection input: a version equal to the
// newest listed release is current (sdk out of scope), a stale version reports
// the SDK outdated, a pre-feature manifest without the field is outdated once,
// and a strictly newer recorded version never offers a downgrade (S17(c)).
func TestWindowsStableCheckSDKVersionDetection(t *testing.T) {
	_, target, _ := writeWindowsChannelFixture(t)
	userWindowsRecordSDKVersion(t, target, "4.6.0")
	update := userWindowsStableCheckWithList(t, target, "4.6.0")
	if slices.Contains(update.Scope, UserComponentSDK) {
		t.Fatalf("expected the SDK current for the recorded newest version, got: %+v", update)
	}
	userWindowsRecordSDKVersion(t, target, "4.5.0")
	if update = userWindowsStableCheckWithList(t, target, "4.6.0"); !slices.Contains(update.Scope, UserComponentSDK) {
		t.Fatalf("expected the SDK outdated for a stale recorded version, got: %+v", update)
	}
	userWindowsRecordSDKVersion(t, target, "")
	if update = userWindowsStableCheckWithList(t, target, "4.6.0"); !slices.Contains(update.Scope, UserComponentSDK) {
		t.Fatalf("expected the SDK outdated once for a pre-version manifest, got: %+v", update)
	}
	userWindowsRecordSDKVersion(t, target, "4.6.0")
	if update = userWindowsStableCheckWithList(t, target, "4.5.0"); slices.Contains(update.Scope, UserComponentSDK) {
		t.Fatalf("expected no downgrade offer for a newer recorded version, got: %+v", update)
	}
}

// The staged generated lock binds the recorded version: the manifest records
// the exact gentle-pi version the install resolved.
func TestUserWindowsLockGentlePiVersion(t *testing.T) {
	lock := []byte(`{"lockfileVersion":3,"packages":{"":{"dependencies":{"gentle-pi":"latest"}},"node_modules/gentle-pi":{"version":"4.5.0","integrity":"sha512-x","resolved":"https://registry.npmjs.org/gentle-pi/-/gentle-pi-4.5.0.tgz"}}}`)
	version, err := userWindowsLockGentlePiVersion(lock)
	if err != nil || version != "4.5.0" {
		t.Fatalf("staged lock version differs: %q %v", version, err)
	}
	for name, raw := range map[string]string{
		"absent-package": `{"lockfileVersion":3,"packages":{}}`,
		"range-version":  `{"lockfileVersion":3,"packages":{"node_modules/gentle-pi":{"version":"latest"}}}`,
		"not-json":       `{`,
	} {
		if _, err := userWindowsLockGentlePiVersion([]byte(raw)); err == nil {
			t.Fatalf("%s staged lock accepted", name)
		}
	}
}

// The SDKVersion manifest pin is omitempty like every other pin: a pre-version
// manifest still re-serializes to its exact on-disk bytes.
func TestUserWindowsManifestSDKVersionOmitEmpty(t *testing.T) {
	manifest := userWindowsManifest{Schema: userWindowsSchema, Destination: "X", SDKLockSHA: "a"}
	without, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if bytesContains(without, []byte("SDKVersion")) {
		t.Fatalf("pre-version manifest serializes the version field: %s", without)
	}
	with, err := json.Marshal(userWindowsManifest{Schema: userWindowsSchema, Destination: "X", SDKVersion: "4.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytesContains(with, []byte(`"SDKVersion":"4.5.0"`)) {
		t.Fatalf("recorded version field differs: %s", with)
	}
}

func bytesContains(data, needle []byte) bool { return bytes.Contains(data, needle) }
