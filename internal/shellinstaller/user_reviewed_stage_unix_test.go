//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T4/S19: the Unix Stable supplier pins come from the selected reviewed
// list's entry for the installed gentle-pi version — the newest listed
// release for a fresh install — not from the frozen lock.
func TestUserStableSupplierPinsNewestListedRelease(t *testing.T) {
	list, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	newest := list.newestReviewedRelease()
	entry := list.Releases[newest]
	installer, resolver, err := userStableSupplierPins(list, newest)
	if err != nil {
		t.Fatal(err)
	}
	if installer != entry.Installer || resolver != entry.Binary {
		t.Fatalf("supplier pins differ from the newest listed entry %s: %q/%q", newest, installer, resolver)
	}
}

// A version without a list entry is refused before any supplier file is read;
// the refusal names the version and the reviewed list, never "the frozen
// lock" (S23/L2).
func TestUserStableSupplierPinsRefusesUnlistedVersion(t *testing.T) {
	list, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := userStableSupplierPins(list, "9.9.9"); err == nil || !strings.Contains(err.Error(), "9.9.9") || strings.Contains(err.Error(), "frozen") {
		t.Fatalf("unlisted version accepted or refusal misnamed: %v", err)
	}
}

// The Go worker writes the selected reviewed list into the install stage, so
// the provisioner and the Go supplier check use exactly the same list.
func TestUserStageReviewedListWritesSelectedBytes(t *testing.T) {
	list, listBytes, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return list, listBytes, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	root := userUnixTempRoot(t)
	staged, err := userStageReviewedList(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if staged.newestReviewedRelease() != list.newestReviewedRelease() {
		t.Fatalf("staged list differs from the selected list: %q vs %q", staged.newestReviewedRelease(), list.newestReviewedRelease())
	}
	path := filepath.Join(root, "gentle-pi-suppliers.json")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, listBytes) {
		t.Fatalf("staged list bytes differ: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0400 {
		t.Fatalf("staged list custody differs: %v %v", info, err)
	}
}

// A failed list selection stages nothing and fails the install.
func TestUserStageReviewedListPropagatesSelectionFailure(t *testing.T) {
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) {
		return userReviewedList{}, nil, errors.New("list selection refused")
	}
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	root := userUnixTempRoot(t)
	if _, err := userStageReviewedList(context.Background(), root); err == nil {
		t.Fatal("failed list selection accepted")
	}
	if _, err := os.Lstat(filepath.Join(root, "gentle-pi-suppliers.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed selection staged bytes")
	}
}

// The manifest records the gentle-pi version the generated lock pinned: the
// actually installed version, not this build's frozen lock.
func TestUserStageGentlePiVersion(t *testing.T) {
	root := userUnixTempRoot(t)
	project := filepath.Join(root, "runtime", "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	lock := `{"lockfileVersion":3,"packages":{"":{"name":"gentle-shell-owned-seed","version":"1.0.0","dependencies":{}},"node_modules/gentle-pi":{"version":"4.5.0","integrity":"sha512-hw4W2hBvyGWPbPEpvAIgx4F+5IgQjZ1Yzhj159DRcbrvFBde3H8Jh9qKQF2EYTH7WcI+ixJqTXFF92pykOuXIg=="}}}`
	if err := os.WriteFile(filepath.Join(project, "package-lock.json"), []byte(lock), 0600); err != nil {
		t.Fatal(err)
	}
	version, err := userStageGentlePiVersion(root)
	if err != nil || version != "4.5.0" {
		t.Fatalf("generated lock version = %q, %v", version, err)
	}
	if err := os.WriteFile(filepath.Join(project, "package-lock.json"), []byte(`{"lockfileVersion":3,"packages":{"node_modules/gentle-pi":{"version":"4"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := userStageGentlePiVersion(root); err == nil {
		t.Fatal("malformed generated lock version accepted")
	}
}

// S23/M4: only a Stable request takes the generated reviewed-list route. A
// Main preview install request stages no list and keeps the frozen modern
// lock's stock base (gentle-pi 4.3.0) exactly as before the route existed.
func TestUserMainPreviewInstallSkipsStagedList(t *testing.T) {
	list, listBytes, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return list, listBytes, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	stable := UserInstallRequest{Mode: "separate", Destination: "/tmp/gentle-stable", Experience: &UserExperience{SDKSource: "stable"}}
	root := userUnixTempRoot(t)
	staged, err := userStageSDKReviewedList(context.Background(), root, stable)
	if err != nil || staged.newestReviewedRelease() != list.newestReviewedRelease() {
		t.Fatalf("stable request did not stage the selected list: %q %v", staged.newestReviewedRelease(), err)
	}
	if _, err := os.Lstat(filepath.Join(root, "gentle-pi-suppliers.json")); err != nil {
		t.Fatalf("stable request staged no list: %v", err)
	}
	mainPreview := UserInstallRequest{Mode: "separate", Destination: "/tmp/gentle-main", Experience: &UserExperience{SDKSource: "main-preview"}}
	root = userUnixTempRoot(t)
	if staged, err = userStageSDKReviewedList(context.Background(), root, mainPreview); err != nil || staged.newestReviewedRelease() != "" {
		t.Fatalf("Main preview request took the generated route: %q %v", staged.newestReviewedRelease(), err)
	}
	if _, err := os.Lstat(filepath.Join(root, "gentle-pi-suppliers.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Main preview request staged a reviewed list")
	}
}

// S23/M1: an installation recorded before the reviewed-list route carries no
// staged list; its launch-time verify keeps authenticating both supplier
// files against the embedded list entry for the installed gentle-pi version —
// the frozen lock's version for a manifest predating the recorded field — and
// refuses tampered bytes with the pre-list refusal.
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
// against the embedded copy (S23/L3).
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

// The shared supplier authentication admits exactly the listed digests.
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
	list := userReviewedList{Releases: map[string]userReviewedSupplier{"1.0.0": {
		Integrity: "sha512-xXBEZaGqden4bblEPagQEnkF12L0JHhsPr1REBGvhSeDy6mbHDQmEvUU5ATr9lvWb7xsTOXV7gwnfq4U+z7lpA==",
		Installer: digests["scripts/gentle-ai-installer.mjs"], Binary: digests["runtime/gentle-ai-binary.mjs"],
	}}}
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "1.0.0"); err != nil {
		t.Fatalf("listed supplier bytes refused: %v", err)
	}
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "2.0.0"); err == nil ||
		!strings.Contains(err.Error(), "has no entry for gentle-pi 2.0.0") {
		t.Fatalf("unlisted version accepted or refusal unnamed: %v", err)
	}
	list.Releases["1.0.0"] = userReviewedSupplier{Installer: strings.Repeat("a", 64), Binary: digests["runtime/gentle-ai-binary.mjs"]}
	if err := userAuthenticateSupplierFiles(context.Background(), pkg, list, "1.0.0"); err == nil {
		t.Fatal("mismatched supplier digest accepted")
	}
}

// S23/M3: a reused-SDK stage records sdk_version from the recorded manifest,
// or the frozen lock's version for a manifest predating the field — it never
// reads a lock the stage does not have. A rebuilt stage reads the generated
// lock it just provisioned.
func TestUserRecordedSDKVersion(t *testing.T) {
	recorded, err := userRecordedSDKVersion(t.TempDir(), true, "4.5.0")
	if err != nil || recorded != "4.5.0" {
		t.Fatalf("reused stage did not record the manifest version: %q %v", recorded, err)
	}
	frozen, err := userFrozenLockGentlePiVersion()
	if err != nil {
		t.Fatal(err)
	}
	recorded, err = userRecordedSDKVersion(t.TempDir(), true, "")
	if err != nil || recorded != frozen {
		t.Fatalf("reused stage without a recorded version: %q %v", recorded, err)
	}
	root := userUnixTempRoot(t)
	project := filepath.Join(root, "runtime", "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package-lock.json"), []byte(`{"packages":{"node_modules/gentle-pi":{"version":"4.4.0"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	recorded, err = userRecordedSDKVersion(root, false, "")
	if err != nil || recorded != "4.4.0" {
		t.Fatalf("rebuilt stage did not read the generated lock: %q %v", recorded, err)
	}
}

// S23/L1: a reused SDK whose recorded version is absent from the selected
// list requires no entry — no supplier runs — and its authentication falls
// back to the embedded entry when one exists.
func TestUserReusedSupplierPinsFallBackToEmbedded(t *testing.T) {
	embedded, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	selected := userReviewedList{Schema: userReviewedListSchema, Releases: map[string]userReviewedSupplier{"9.9.9": embedded.Releases["4.5.0"]}}
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return selected, nil, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	installer, resolver, ok, err := userReusedSupplierPins(context.Background(), "4.5.0")
	if err != nil || !ok || installer != embedded.Releases["4.5.0"].Installer || resolver != embedded.Releases["4.5.0"].Binary {
		t.Fatalf("embedded fallback missing: %q/%q ok=%v err=%v", installer, resolver, ok, err)
	}
	installer, resolver, ok, err = userReusedSupplierPins(context.Background(), "9.9.9")
	if err != nil || !ok || installer != selected.Releases["9.9.9"].Installer || resolver != selected.Releases["9.9.9"].Binary {
		t.Fatalf("selected entry not preferred: %q/%q ok=%v err=%v", installer, resolver, ok, err)
	}
	if _, _, ok, err = userReusedSupplierPins(context.Background(), "8.8.8"); err != nil || ok {
		t.Fatalf("unlisted reused version refused although no supplier runs: ok=%v err=%v", ok, err)
	}
}

// S23/L4: the frozen modern lock remains the stock base of the Main preview
// route and of every install recorded before the reviewed list; its gentle-pi
// is the listed 4.3.0 release, and the embedded entry cross-pins its supplier
// digests literally, so the frozen route keeps failing closed on tampered
// supplier bytes.
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
	installer, resolver, err := userStableSupplierPins(list, version)
	if err != nil {
		t.Fatal(err)
	}
	if installer != "8679f49d3675323bf7ff5c24321f08016bb0f8e79ded307b5900c37648f52470" ||
		resolver != "61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80" {
		t.Fatalf("embedded entry for the frozen stock base differs: %q/%q", installer, resolver)
	}
}
