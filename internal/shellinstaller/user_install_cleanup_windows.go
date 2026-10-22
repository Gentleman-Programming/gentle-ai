//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// userWindowsEphemeralRelatives are the installer-created download and tool
// caches that are fully reproducible and never carry user state: the npm
// cache, Go modules, the Go build cache and the scratch TEMP directory.
// runtime/archives is intentionally NOT listed: launch and idempotence
// verification re-read the retained supplier archives on every run
// (userWindowsVerifyArchive), so they stay installed for the installation's
// lifetime. runtime/node, runtime/go, runtime/codegraph, prefix, agent,
// config and the home user data are installed or personal artefacts.
var userWindowsEphemeralRelatives = []string{"runtime/cache", "home/go", "state/go-build", "tmp"}

// userWindowsEphemeralCleanup removes only the installer-owned ephemeral
// caches of a successfully published installation. Absent paths are fine;
// any reparse point, foreign owner, DACL surprise or non-canonical path
// fails closed without removing anything.
func userWindowsEphemeralCleanup(root string) error {
	if _, err := userWindowsIdentity(root, true); err != nil {
		return err
	}
	for _, relative := range userWindowsEphemeralRelatives {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		// Fail closed: refuse reparse points, foreign owners and aliases.
		if _, err := userWindowsIdentity(path, true); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("Windows ephemeral path %q still exists after removal", path)
		}
	}
	return nil
}

// userWindowsBackupKeep is the minimal backup retained after uninstall: the
// user preferences, the agent home, the selection and the manifest.
var userWindowsBackupKeep = map[string]bool{
	"config": true, "agent": true, "home": true,
	"selection.json": true, "manifest.json": true,
}

// userWindowsPruneRemovedBackup reduces a target that has already been
// renamed to its .removed-<UTC> backup to the minimal retained set. Every
// other entry (runtime including archives, prefix, source, state, tmp, bin,
// provision.mjs, supervisor.exe, complete-generated-lock-sri.mjs, main.json
// and any other owned leftover such as project or stage remnants) is
// installer-owned material and is removed. An entry that fails the owned
// identity check is preserved and reported, never removed; the backup
// directory itself is never deleted.
func userWindowsPruneRemovedBackup(backup string) error {
	if _, err := userWindowsIdentity(backup, true); err != nil {
		return err
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if userWindowsBackupKeep[entry.Name()] {
			continue
		}
		path := filepath.Join(backup, entry.Name())
		if _, err := userWindowsIdentity(path, true); err != nil {
			return errors.Join(err, fmt.Errorf("uncertain Windows backup entry %q; preserved in %q", entry.Name(), backup))
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	// Ephemeral caches nested under the retained home (Go modules) are
	// pruned too; the rest of home, including .gentle-ai state and Pi
	// sessions, stays untouched.
	for _, relative := range userWindowsEphemeralRelatives {
		if !strings.HasPrefix(relative, "home/") {
			continue // Top-level ephemeral roots were removed above.
		}
		path := filepath.Join(backup, filepath.FromSlash(relative))
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if _, err := userWindowsIdentity(path, true); err != nil {
			return errors.Join(err, fmt.Errorf("uncertain Windows backup cache %q; preserved in %q", relative, backup))
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(backup)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !userWindowsBackupKeep[entry.Name()] {
			return fmt.Errorf("Windows minimal backup %q retains unpruned entry %q", backup, entry.Name())
		}
	}
	return nil
}
