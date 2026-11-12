//go:build windows

package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// This file owns the regression fixtures for #6098 Windows replaced-backup
// retention. Every tree is created inside t.TempDir() and made private with
// userWindowsPrivate, so the fixture is exactly what the installer considers
// owned: current-SID owner, protected DACL, no reparse point, local NTFS.

// userWindowsRetentionCorruption lets a test damage exactly one intended
// property of an otherwise honest backup tree. The hooks receive the original
// active destination so a case can rebind the manifest to a wrong tree.
type userWindowsRetentionCorruption struct {
	selection    func(t *testing.T, destination string, selected *UserInstallRequest)
	manifest     func(t *testing.T, destination string, manifest *userWindowsManifest)
	noncanonical bool // write the manifest without the installer's trailing newline
}

// userWindowsRetentionFixture publishes the active destination inside a
// private root: the reinstall that created a backup always leaves a fresh
// installation at destination before removal runs.
func userWindowsRetentionFixture(t *testing.T) (root, destination string) {
	t.Helper()
	root = t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	destination = filepath.Join(root, "install")
	userWindowsRetentionWriteTree(t, destination, destination, nil)
	return root, destination
}

func userWindowsRetentionWrite(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(path, data); err != nil {
		t.Fatal(err)
	}
}

// userWindowsRetentionWriteTree builds one owned installation tree. A tree
// renamed by the installer keeps the bytes it was published with, so the
// retired tree's selection and manifest still record the destination the user
// selected, while the physical identity in the manifest is re-read from
// wherever the tree now lives.
func userWindowsRetentionWriteTree(t *testing.T, root, destination string, corrupt *userWindowsRetentionCorruption) {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"agent", "prefix", "source"} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	supervisor := []byte("fixture supervisor image, never executed")
	lock := []byte(`{"lockfileVersion":3,"packages":{}}`)
	userWindowsRetentionWrite(t, root, "supervisor.exe", supervisor)
	userWindowsRetentionWrite(t, root, "source/package-lock.json", lock)
	userWindowsRetentionWrite(t, root, "agent/settings.json", []byte(`{"fixture":"agent"}`))
	userWindowsRetentionWrite(t, root, "prefix/gentle-shell.mjs", []byte("// fixture prefix"))

	selected := UserInstallRequest{Destination: destination, Mode: "separate", Channel: "stable"}
	if corrupt != nil && corrupt.selection != nil {
		corrupt.selection(t, destination, &selected)
	}
	selection, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := userWindowsSID()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := userWindowsIdentity(root, true)
	if err != nil {
		t.Fatal(err)
	}
	prefixIdentity, err := userWindowsIdentity(filepath.Join(root, "prefix"), true)
	if err != nil {
		t.Fatal(err)
	}
	agentIdentity, err := userWindowsIdentity(filepath.Join(root, "agent"), true)
	if err != nil {
		t.Fatal(err)
	}
	manifest := userWindowsManifest{
		Schema: userWindowsSchema, Destination: selected.Destination, SID: sid,
		Identity: identity, SupervisorSHA: userWindowsSHA(supervisor),
		PrefixIdentity: prefixIdentity, AgentIdentity: agentIdentity,
		SelectionSHA: userWindowsSHA(selection), LockSHA: userWindowsSHA(lock),
	}
	if corrupt != nil && corrupt.manifest != nil {
		corrupt.manifest(t, destination, &manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes := append(encoded, '\n')
	if corrupt != nil && corrupt.noncanonical {
		manifestBytes = encoded
	}
	userWindowsRetentionWrite(t, root, "selection.json", selection)
	userWindowsRetentionWrite(t, root, "manifest.json", manifestBytes)
}

func userWindowsRetentionEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// userWindowsRetentionAssertWhole proves a preserved tree kept every part of
// itself, not just its name: removal creates no partial tree.
func userWindowsRetentionAssertWhole(t *testing.T, tree, reason string) {
	t.Helper()
	for _, relative := range []string{
		"manifest.json", "selection.json", "source/package-lock.json",
		"supervisor.exe", "agent/settings.json", "prefix/gentle-shell.mjs",
	} {
		if _, err := os.Lstat(filepath.Join(tree, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("%s: preserved tree %q lost %s: %v", reason, tree, relative, err)
		}
	}
}

func userWindowsRetentionAssertEntries(t *testing.T, root string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := userWindowsRetentionEntries(t, root); !slices.Equal(got, want) {
		t.Fatalf("retention inventory differs\n got: %v\nwant: %v", got, want)
	}
}

// (a) A backup whose canonical manifest, canonical selection, original active
// destination, current SID and physical identity all validate is removed,
// while the active installation published over it stays untouched.
func TestUserWindowsRemoveReplacedBackupRemovesOwnedBackup(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	backupName := "install.replaced-20260101T000000Z"
	backup := filepath.Join(root, backupName)
	userWindowsRetentionWriteTree(t, backup, destination, nil)

	if err := userWindowsRemoveReplacedBackup(destination, backup); err != nil {
		t.Fatalf("removal refused a valid owned backup: %v", err)
	}
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatalf("valid owned backup %q survived removal: %v", backupName, err)
	}
	userWindowsRetentionAssertEntries(t, root, "install")
	userWindowsRetentionAssertWhole(t, destination, "the active destination")
}

// (b) A backup that cannot be positively validated is preserved whole and
// reported. Each case damages exactly one binding of an otherwise honest tree,
// so a removal that trusted the canonical name would delete it.
func TestUserWindowsRemoveReplacedBackupPreservesUncertainBackup(t *testing.T) {
	cases := []struct {
		name    string
		corrupt *userWindowsRetentionCorruption
	}{
		{
			name:    "noncanonical manifest",
			corrupt: &userWindowsRetentionCorruption{noncanonical: true},
		},
		{
			name: "wrong original destination",
			corrupt: &userWindowsRetentionCorruption{
				selection: func(t *testing.T, destination string, selected *UserInstallRequest) {
					t.Helper()
					selected.Destination = filepath.Join(filepath.Dir(destination), "other-install")
				},
			},
		},
		{
			name: "wrong physical identity",
			corrupt: &userWindowsRetentionCorruption{
				manifest: func(t *testing.T, destination string, manifest *userWindowsManifest) {
					t.Helper()
					identity, identityErr := userWindowsIdentity(destination, true)
					if identityErr != nil {
						t.Fatalf("active destination identity: %v", identityErr)
					}
					manifest.Identity = identity
				},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, destination := userWindowsRetentionFixture(t)
			backupName := "install.replaced-20260101T000000Z"
			backup := filepath.Join(root, backupName)
			userWindowsRetentionWriteTree(t, backup, destination, test.corrupt)

			err := userWindowsRemoveReplacedBackup(destination, backup)
			if err == nil {
				t.Fatal("removal accepted an uncertain backup instead of reporting it")
			}
			if !strings.Contains(err.Error(), backupName) {
				t.Fatalf("removal error does not name the preserved backup %q: %v", backupName, err)
			}
			userWindowsRetentionAssertEntries(t, root, "install", backupName)
			userWindowsRetentionAssertWhole(t, backup, "an uncertain backup")
			userWindowsRetentionAssertWhole(t, destination, "the active destination")
		})
	}
}

// (b) A canonical-looking name that is a plain file has no manifest, so it is
// not a backup tree: preservation here also proves removal never deletes by an
// unvalidated name glob.
func TestUserWindowsRemoveReplacedBackupPreservesNonTreeCandidate(t *testing.T) {
	root, destination := userWindowsRetentionFixture(t)
	foreignName := "install.replaced-20260101T000000Z"
	foreignPath := filepath.Join(root, foreignName)
	foreign := []byte("foreign file that happens to look like a backup name")
	if err := userWindowsWrite(foreignPath, foreign); err != nil {
		t.Fatal(err)
	}

	err := userWindowsRemoveReplacedBackup(destination, foreignPath)
	if err == nil {
		t.Fatal("removal accepted a candidate that is not a backup tree")
	}
	if !strings.Contains(err.Error(), foreignName) {
		t.Fatalf("removal error does not name the preserved non-tree candidate %q: %v", foreignName, err)
	}
	readback, readErr := userWindowsRead(foreignPath, int64(len(foreign)))
	if readErr != nil || string(readback) != string(foreign) {
		t.Fatalf("foreign candidate bytes changed: %q %v", readback, readErr)
	}
	userWindowsRetentionAssertEntries(t, root, "install", foreignName)
	userWindowsRetentionAssertWhole(t, destination, "the active destination")
}
