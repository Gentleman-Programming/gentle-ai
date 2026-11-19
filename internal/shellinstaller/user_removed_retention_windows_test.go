//go:build windows

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression fixtures for the Windows uninstall backup retention: at most one
// <destination>.removed-<UTC> tree stays beside the destination, and the
// replaced backups are never touched by that policy.

func userWindowsRemovedBackup(t *testing.T, root, destination, stamp string, corrupt *userWindowsRetentionCorruption) string {
	t.Helper()
	backup := filepath.Join(root, filepath.Base(destination)+".removed-"+stamp)
	userWindowsRetentionWriteTree(t, backup, destination, corrupt)
	return backup
}

// The exact keep is retained and every older valid removed backup goes, both
// the full legacy trees older installers left behind and minimal ones.
func TestUserWindowsPruneRemovedBackupsKeepsOnlyTheNewest(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	userWindowsRemovedBackup(t, root, destination, "20260101T110000Z", nil)
	minimal := userWindowsRemovedBackup(t, root, destination, "20260101T120000Z", nil)
	if err := userWindowsPruneRemovedBackup(minimal); err != nil {
		t.Fatalf("fixture could not be reduced to a minimal backup: %v", err)
	}
	keep := userWindowsRemovedBackup(t, root, destination, "20260101T130000Z", nil)

	if err := userWindowsPruneRemovedBackups(destination, keep); err != nil {
		t.Fatalf("prune refused valid removed backups: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install", "install.removed-20260101T130000Z")
	userWindowsRetentionAssertWhole(t, keep, "the kept backup")
	userWindowsRetentionAssertWhole(t, destination, "the active destination")
}

// Removed and replaced are independent families: pruning one never deletes
// the other, and unrelated or noncanonical names are ignored.
func TestUserWindowsPruneRemovedBackupsIgnoresOtherFamiliesAndNames(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	userWindowsRetentionBackup(t, root, destination, "20260101T100000Z", nil) // replaced
	userWindowsRemovedBackup(t, root, destination, "20260101T110000Z", nil)
	keep := userWindowsRemovedBackup(t, root, destination, "20260101T120000Z", nil)
	foreign := filepath.Join(root, "other.removed-20260101T090000Z")
	if err := os.MkdirAll(foreign, 0700); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(root, "install.removed-latest")
	if err := os.MkdirAll(loose, 0700); err != nil {
		t.Fatal(err)
	}

	if err := userWindowsPruneRemovedBackups(destination, keep); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	userWindowsRetentionAssertEntries(t, root, "install", "install.removed-20260101T120000Z",
		"install.removed-latest", "install.replaced-20260101T100000Z", "other.removed-20260101T090000Z")
}

// A candidate that cannot be positively validated is preserved whole and
// reported, while valid siblings are still removed.
func TestUserWindowsPruneRemovedBackupsPreservesUncertainCandidates(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	wrongTree := userWindowsRemovedBackup(t, root, destination, "20260101T100000Z", &userWindowsRetentionCorruption{
		manifest: func(_ *testing.T, _ string, manifest *userWindowsManifest) {
			manifest.Destination = filepath.Join(root, "elsewhere")
		},
	})
	userWindowsRemovedBackup(t, root, destination, "20260101T110000Z", nil)
	keep := userWindowsRemovedBackup(t, root, destination, "20260101T120000Z", nil)

	err := userWindowsPruneRemovedBackups(destination, keep)
	userWindowsRetentionJoined(t, err, filepath.Base(wrongTree))
	userWindowsRetentionAssertEntries(t, root, "install", "install.removed-20260101T100000Z", "install.removed-20260101T120000Z")
	userWindowsRetentionAssertWhole(t, wrongTree, "the uncertain candidate")
}

// A missing keep refuses the whole prune before anything is touched.
func TestUserWindowsPruneRemovedBackupsRefusesAMissingKeep(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	older := userWindowsRemovedBackup(t, root, destination, "20260101T110000Z", nil)
	missing := filepath.Join(root, "install.removed-20260101T120000Z")
	if err := userWindowsPruneRemovedBackups(destination, missing); err == nil {
		t.Fatal("a missing keep must refuse the prune")
	}
	userWindowsRetentionAssertWhole(t, older, "an older backup beside a missing keep")
}

// The uninstall itself enforces the policy: after N uninstalls at most one
// removed backup remains, and the previous one is gone.
func TestUninstallWindowsKeepsOnlyOneRemovedBackup(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root, target := userWindowsRetentionFixture(t)
	older := userWindowsRemovedBackup(t, root, target, "20200101T000000Z", nil)
	replaced := userWindowsRetentionBackup(t, root, target, "20200101T000000Z", nil)

	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if _, err := os.Lstat(older); !os.IsNotExist(err) {
		t.Fatalf("the previous removed backup survived the uninstall: %v", err)
	}
	// The replaced family is independent and stays untouched; the operation
	// lock file stays beside the target by design.
	userWindowsRetentionAssertEntries(t, root, filepath.Base(backup), filepath.Base(replaced), ".install.gentle-shell.lock")
}

// A previous backup that cannot be validated is preserved and reported, but
// the uninstall already moved the install aside and its backup stays.
func TestUninstallWindowsPreservesAnUncertainPreviousBackup(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root, target := userWindowsRetentionFixture(t)
	older := userWindowsRemovedBackup(t, root, target, "20200101T000000Z", &userWindowsRetentionCorruption{
		manifest: func(_ *testing.T, _ string, manifest *userWindowsManifest) {
			manifest.Destination = filepath.Join(root, "elsewhere")
		},
	})

	backup, err := UninstallUserShellInstall(t.TempDir(), UserShellInstall{Target: target, Mode: "separate"})
	if err == nil || backup == "" {
		t.Fatalf("an uncertain previous backup must be reported: backup=%q err=%v", backup, err)
	}
	if _, statErr := os.Lstat(backup); statErr != nil {
		t.Fatalf("the new backup must stay: %v", statErr)
	}
	userWindowsRetentionAssertWhole(t, older, "the uncertain previous backup")
}

// A forged identity (a copied manifest) and a plain file with a removed-backup
// name are both preserved and reported, never deleted.
func TestUserWindowsPruneRemovedBackupsPreservesForgedAndNonTreeCandidates(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	forged := userWindowsRemovedBackup(t, root, destination, "20260101T100000Z", &userWindowsRetentionCorruption{
		manifest: func(_ *testing.T, _ string, manifest *userWindowsManifest) {
			manifest.Identity = "00000000:0000000000000000"
		},
	})
	file := filepath.Join(root, "install.removed-20260101T110000Z")
	if err := os.WriteFile(file, []byte("not a tree"), 0600); err != nil {
		t.Fatal(err)
	}
	keep := userWindowsRemovedBackup(t, root, destination, "20260101T120000Z", nil)

	err := userWindowsPruneRemovedBackups(destination, keep)
	userWindowsRetentionJoined(t, err, filepath.Base(forged), filepath.Base(file))
	userWindowsRetentionAssertEntries(t, root, "install", "install.removed-20260101T100000Z",
		"install.removed-20260101T110000Z", "install.removed-20260101T120000Z")
}
