//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// userEphemeralRelatives are the installer-created download and tool caches
// that are fully reproducible from retained suppliers and never carry user
// state: the npm cache, Go modules, the Go build cache and the scratch
// TMPDIR. runtime/tools archives stay installed because every launch
// readback re-verifies them (userReadManifest -> userTools); runtime/node,
// runtime/codegraph, prefix, agent, config and the home user data are
// installed or personal artefacts and are never listed here.
var userEphemeralRelatives = []string{"runtime/cache", "home/go", "state/go-build", "tmp"}

// userInstallEphemeralCleanup removes only the installer-owned ephemeral
// caches of a successfully published installation. It runs on the publish
// success tail (userPublishManifest) and the Windows equivalent; absent
// paths are fine, and any symlink, foreign owner, unexpected mode or
// nonregular object fails closed without removing anything.
func userInstallEphemeralCleanup(root string) error {
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	for _, relative := range userEphemeralRelatives {
		path := filepath.Join(root, relative)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := userOwnedRemovalPath(path); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("ephemeral path %q still exists after removal", path)
		}
	}
	return nil
}

// userOwnedRemovalPath refuses everything except a physical 0700/0755
// directory or a regular file owned by the current user, so RemoveAll can
// never traverse an alias or delete a foreign object.
func userOwnedRemovalPath(path string) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return privateError("filesystem", errors.Join(err, canonicalErr, fmt.Errorf("removal path %q is not an owned physical path", path)))
	}
	if info.IsDir() {
		return privateNativeDirectory(path)
	}
	if !info.Mode().IsRegular() {
		return privateError("filesystem", fmt.Errorf("removal path %q is neither a directory nor a regular file", path))
	}
	if _, err := privatePhysical(path); err != nil {
		return err
	}
	return nil
}

// userUnixBackupKeep is the minimal backup retained after uninstall: the
// user preferences, the agent home and the installation manifest.
// installation.json is the Unix manifest file; selection.json and
// manifest.json are kept when present for cross-platform symmetry.
var userUnixBackupKeep = map[string]bool{
	"config": true, "agent": true, "home": true,
	"selection.json": true, "manifest.json": true, "installation.json": true,
}

// userPruneRemovedBackup reduces a target that has already been renamed to
// its .removed-<UTC> backup to the minimal retained set. Every other entry
// (runtime, prefix, state, tmp, bin, pnpm, source, provision.mjs,
// supervisor, main.json and helper scripts) is installer-owned downloaded or
// reinstallable material and is removed. An entry that fails the owned
// physical-path check is preserved and reported, never removed; the backup
// directory itself is never deleted.
func userPruneRemovedBackup(backup string) error {
	if _, err := privateDirectory(backup); err != nil {
		return err
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if userUnixBackupKeep[entry.Name()] {
			continue
		}
		path := filepath.Join(backup, entry.Name())
		if err := userOwnedRemovalPath(path); err != nil {
			return errors.Join(err, fmt.Errorf("uncertain backup entry %q; preserved in %q", entry.Name(), backup))
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	// Ephemeral caches nested under the retained home (Go modules) are
	// pruned too; the rest of home, including .gentle-ai state and Pi
	// sessions, stays untouched.
	for _, relative := range userEphemeralRelatives {
		if !strings.HasPrefix(relative, "home/") {
			continue // Top-level ephemeral roots were removed above.
		}
		path := filepath.Join(backup, relative)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := userOwnedRemovalPath(path); err != nil {
			return errors.Join(err, fmt.Errorf("uncertain backup cache %q; preserved in %q", relative, backup))
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
		if !userUnixBackupKeep[entry.Name()] {
			return fmt.Errorf("minimal backup %q retains unpruned entry %q", backup, entry.Name())
		}
	}
	return nil
}
