//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UninstallUserShellInstall detaches a Separate installation by moving its
// target to a timestamped backup and dropping the registry entry. Personal Pi
// paths are never touched.
func UninstallUserShellInstall(home string, install UserShellInstall) (string, error) {
	if install.Mode != "separate" && install.Mode != "agent" {
		return "", errors.New("uninstall keeps a backup only for Separate or agent installations")
	}
	manifest, err := userReadInstallationManifest(install.Target)
	if err != nil || manifest.Destination != install.Target || manifest.Mode != install.Mode {
		return "", errors.New("managed installation manifest does not match the selected install")
	}
	if _, err := privateDirectory(install.Target); err != nil {
		return "", err
	}
	if install.Mode == "agent" {
		if err := userRestoreAgentImages(install.Target, manifest.Agent); err != nil {
			return "", err
		}
	} else if install.Mode == "separate" {
		if err := RemoveUserShellLauncher(install.Target); err != nil {
			return "", err
		}
	}
	backup := install.Target + ".removed-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		return "", errors.New("uninstall backup already exists")
	}
	if err := os.Rename(install.Target, backup); err != nil {
		return "", err
	}
	if directory, err := os.Open(filepath.Dir(install.Target)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	// The active install is already fully moved aside; prune the backup to
	// the minimal retained set. A pruning failure never deletes the backup.
	pruneErr := userPruneRemovedBackup(backup)
	if err := RemoveUserShellInstall(home, install.Target); err != nil {
		return backup, fmt.Errorf("installation moved to %s but registry cleanup failed: %w", backup, errors.Join(err, pruneErr))
	}
	if pruneErr != nil {
		return backup, fmt.Errorf("installation moved to %s but minimal-backup pruning failed; the backup keeps more than config, agent, home and the manifest: %w", backup, pruneErr)
	}
	return backup, nil
}
