//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An installation without a staged reviewed list keeps its launch-time verify
// authenticating both supplier files against the embedded list entry for the
// installed gentle-pi version (the frozen lock's version for a manifest
// predating the recorded field), and refuses tampered bytes with the
// pre-list refusal.
func TestUserVerifyRecordedSuppliersRefusesTamperedLegacyTree(t *testing.T) {
	root, manifest := userLegacySupplierFixture(t, "")
	if err := userVerifyRecordedSuppliers(context.Background(), root, manifest); err == nil {
		t.Fatal("tampered supplier bytes accepted on a legacy tree verify")
	} else if !strings.Contains(err.Error(), "stock supplier pin") {
		t.Fatalf("refusal lost the pre-list message: %v", err)
	}
}

// The recorded version selects the embedded entry: an unknown recorded
// version is refused for having no list entry, never silently skipped.
func TestUserVerifyRecordedSuppliersUsesRecordedVersion(t *testing.T) {
	root, manifest := userLegacySupplierFixture(t, "9.9.9")
	if err := userVerifyRecordedSuppliers(context.Background(), root, manifest); err == nil ||
		!strings.Contains(err.Error(), "has no entry for gentle-pi 9.9.9") {
		t.Fatalf("recorded version not used for the embedded entry lookup: %v", err)
	}
}

// A tree with a staged list is authenticated by its own provisioner verify
// against that staged copy; the launch boundary does not re-authenticate it
// against the embedded copy.
func TestUserVerifyRecordedSuppliersTrustsStagedList(t *testing.T) {
	root, manifest := userLegacySupplierFixture(t, "")
	if err := os.WriteFile(filepath.Join(root, "gentle-pi-suppliers.json"), []byte("{}"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := userVerifyRecordedSuppliers(context.Background(), root, manifest); err != nil {
		t.Fatalf("staged list tree re-authenticated against the embedded copy: %v", err)
	}
}

// userLegacySupplierFixture builds an installation root whose gentle-pi
// package carries supplier bytes matching no reviewed entry.
func userLegacySupplierFixture(t *testing.T, recorded string) (string, userManifest) {
	t.Helper()
	root := userUnixTempRoot(t)
	pkg := filepath.Join(root, "prefix", "lib", "node_modules", "gentle-pi")
	for _, directory := range []string{"scripts", "runtime"} {
		if err := os.MkdirAll(filepath.Join(pkg, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"scripts/gentle-ai-installer.mjs", "runtime/gentle-ai-binary.mjs"} {
		if err := os.WriteFile(filepath.Join(pkg, filepath.FromSlash(name)), []byte("// tampered supplier\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, userManifest{Destination: root, Prefix: filepath.Join(root, "prefix"), Mode: "separate", SDKVersion: recorded}
}

// The shared supplier authentication admits exactly the listed digests and
// refuses a mismatch in either supplier file.
func TestUserAuthenticateSupplierFiles(t *testing.T) {
	pkg := filepath.Join(userUnixTempRoot(t), "prefix", "lib", "node_modules", "gentle-pi")
	for _, directory := range []string{"scripts", "runtime"} {
		if err := os.MkdirAll(filepath.Join(pkg, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	digests := map[string]string{}
	for _, name := range []string{"scripts/gentle-ai-installer.mjs", "runtime/gentle-ai-binary.mjs"} {
		data := []byte("// reviewed supplier " + name + "\n")
		sum := sha256.Sum256(data)
		digests[name] = hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(pkg, filepath.FromSlash(name)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	reviewed := userReviewedSupplier{
		Integrity: "sha512-xXBEZaGqden4bblEPagQEnkF12L0JHhsPr1REBGvhSeDy6mbHDQmEvUU5ATr9lvWb7xsTOXV7gwnfq4U+z7lpA==",
		Installer: digests["scripts/gentle-ai-installer.mjs"], Binary: digests["runtime/gentle-ai-binary.mjs"],
	}
	list := userReviewedList{Releases: map[string]userReviewedSupplier{"1.0.0": reviewed}}
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "1.0.0"); err != nil {
		t.Fatalf("listed supplier bytes refused: %v", err)
	}
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "2.0.0"); err == nil ||
		!strings.Contains(err.Error(), "has no entry for gentle-pi 2.0.0") {
		t.Fatalf("unlisted version accepted or refusal unnamed: %v", err)
	}
	installerOnly := reviewed
	installerOnly.Installer = strings.Repeat("a", 64)
	list.Releases["1.0.0"] = installerOnly
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "1.0.0"); err == nil ||
		!strings.Contains(err.Error(), "gentle-ai-installer.mjs") {
		t.Fatalf("mismatched installer digest accepted or unnamed: %v", err)
	}
	binaryOnly := reviewed
	binaryOnly.Binary = strings.Repeat("b", 64)
	list.Releases["1.0.0"] = binaryOnly
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "1.0.0"); err == nil ||
		!strings.Contains(err.Error(), "gentle-ai-binary.mjs") {
		t.Fatalf("mismatched binary digest accepted or unnamed: %v", err)
	}
}

// The frozen modern lock is the stock base of every install recorded before
// the reviewed list; its gentle-pi is the listed 4.3.0 release, and the
// embedded entry cross-pins its supplier digests literally, so the launch
// boundary keeps failing closed on tampered supplier bytes.
func TestUserFrozenStockSupplierEntryCrossPin(t *testing.T) {
	version, err := userFrozenLockGentlePiVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != "4.3.0" {
		t.Fatalf("frozen modern lock pins %q, not the reviewed 4.3.0 stock base", version)
	}
	list, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := list.Releases[version]
	if !ok {
		t.Fatalf("embedded list has no entry for the frozen stock base %s", version)
	}
	if entry.Installer != "8679f49d3675323bf7ff5c24321f08016bb0f8e79ded307b5900c37648f52470" ||
		entry.Binary != "61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80" {
		t.Fatalf("embedded entry for the frozen stock base differs: %q/%q", entry.Installer, entry.Binary)
	}
}
