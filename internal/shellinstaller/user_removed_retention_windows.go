//go:build windows

package shellinstaller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// userWindowsPruneRemovedBackups bounds the <destination>.removed-<UTC> trees
// an uninstall leaves beside the destination: the backup just created (keep)
// stays, every older validated one is removed. It shares the replaced-backup
// policy, so an uncertain candidate is preserved whole and reported in a joined
// error and the pruning never undoes the uninstall that triggered it. Replaced
// backups are a separate family and are never touched here.
func userWindowsPruneRemovedBackups(destination, keep string) error {
	return userWindowsPruneBackups(destination, keep, "removed", func(destination, backup string) error {
		_, err := userWindowsRemovedBackupInspect(destination, backup)
		return err
	}, userWindowsRemoveRemovedBackup)
}

// userWindowsRemovedBackupInspect is the read-only validation a removed backup
// must pass before removal. Older installers left full trees and current ones
// leave the minimal retained set, so the check does not require a complete
// inventory: it requires an exact canonical sibling, a directory owned by the
// current account, a canonical manifest bound to the original destination, and
// an owned identity for every top-level entry. It returns the physical
// identity so removal can recheck it. It never deletes anything.
func userWindowsRemovedBackupInspect(destination, backup string) (string, error) {
	if err := userWindowsBackupSibling(destination, backup, "removed"); err != nil {
		return "", err
	}
	info, err := os.Lstat(backup)
	if err != nil {
		return "", fmt.Errorf("Windows removed backup %q cannot be inspected; preserved: %w", backup, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Windows removed backup %q is not a directory tree; preserved", backup)
	}
	manifest, err := userWindowsRemovedManifestCheck(backup, destination)
	if err != nil {
		return "", fmt.Errorf("uncertain Windows removed backup %q; preserved: %w", backup, err)
	}
	identity, err := userWindowsIdentity(backup, true)
	if err != nil || manifest.Identity != identity {
		return "", errors.Join(err, fmt.Errorf("uncertain Windows removed backup identity; preserved %q", backup))
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		return "", fmt.Errorf("Windows removed backup %q cannot be listed; preserved: %w", backup, err)
	}
	for _, entry := range entries {
		if _, err := userWindowsIdentity(filepath.Join(backup, entry.Name()), true); err != nil {
			return "", errors.Join(err, fmt.Errorf("uncertain Windows removed backup entry %q; preserved in %q", entry.Name(), backup))
		}
	}
	return identity, nil
}

// userWindowsRemovedManifestCheck validates the part of a retired tree that
// survives the minimal uninstall prune: a canonical manifest written by this
// account for the original destination, bound to a canonical selection of that
// same destination. It deliberately does not require the generated lock, the
// Main authority, the prefix or the agent identities, because the uninstall
// prune removes them. userWindowsManifestReadAs stays the check for full trees.
func userWindowsRemovedManifestCheck(backup, destination string) (userWindowsManifest, error) {
	var manifest userWindowsManifest
	data, err := userWindowsRead(filepath.Join(backup, "manifest.json"), 4096)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	canonical, _ := json.Marshal(manifest)
	sid, err := userWindowsSID()
	if err != nil {
		return manifest, err
	}
	if string(data) != string(canonical)+"\n" || manifest.Schema != userWindowsSchema || manifest.Destination != destination || manifest.SID != sid {
		return manifest, errors.New("Windows removed manifest is not canonical for this account and destination")
	}
	selection, err := userWindowsRead(filepath.Join(backup, "selection.json"), 4096)
	if err != nil || userWindowsSHA(selection) != manifest.SelectionSHA {
		return manifest, errors.Join(err, errors.New("Windows removed selection binding differs"))
	}
	if _, err := userWindowsSelection(selection, destination); err != nil {
		return manifest, err
	}
	return manifest, nil
}

// userWindowsRemoveRemovedBackup removes one validated removed backup. Its
// physical identity is rechecked immediately before removal and the backup must
// be absent afterward; anything uncertain is preserved and reported.
func userWindowsRemoveRemovedBackup(destination, backup string) error {
	identity, err := userWindowsRemovedBackupInspect(destination, backup)
	if err != nil {
		return err
	}
	observed, err := userWindowsIdentity(backup, true)
	if err != nil || observed != identity {
		return errors.Join(err, fmt.Errorf("uncertain Windows removed backup identity; preserved %q", backup))
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("Windows removed backup %q could not be removed; preserved: %w", backup, err)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(err, fmt.Errorf("Windows removed backup %q survived removal", backup))
	}
	return nil
}
