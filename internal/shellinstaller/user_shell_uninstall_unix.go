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
// paths are never touched. An agent installation first restores the agent,
// including Pi's own fd/rg set aside at install; one that changed since is
// left as it is, its original is kept beside the backup and reported.
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
	var keptTools []string
	if install.Mode == "agent" {
		if keptTools, err = userRestoreAgentImages(install.Target, manifest.Agent); err != nil {
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
	// Pi tools uninstall could not put back leave the backup before pruning.
	keptNote := ""
	if len(keptTools) != 0 {
		if keptNote, err = userKeepAgentTools(backup, manifest.Agent, keptTools); err != nil {
			return backup, fmt.Errorf("installation moved to %s but Pi's set-aside tools were not kept apart; preserve that backup: %w", backup, err)
		}
	}
	// The active install is already fully moved aside; prune the backup to
	// the minimal retained set. A pruning failure never deletes the backup.
	pruneErr := userPruneRemovedBackup(backup)
	// A cleanup failure stays a failure; a kept-tool note rides along as text.
	alsoKept := ""
	if keptNote != "" {
		alsoKept = "; also, " + keptNote
	}
	if err := RemoveUserShellInstall(home, install.Target); err != nil {
		return backup, fmt.Errorf("installation moved to %s but registry cleanup failed: %w%s", backup, errors.Join(err, pruneErr), alsoKept)
	}
	if pruneErr != nil {
		return backup, fmt.Errorf("installation moved to %s but minimal-backup pruning failed; the backup keeps more than config, agent, home and the manifest: %w%s", backup, pruneErr, alsoKept)
	}
	if keptNote != "" {
		return backup, &userUninstallNote{backup: backup, note: keptNote}
	}
	return backup, nil
}

// userUninstallNote is the outcome of an uninstall that completed (target
// moved to backup, registry entry dropped) with something the operator must
// know. Callers render it as success with UninstallNote, never as a failure.
type userUninstallNote struct{ backup, note string }

func (n *userUninstallNote) Error() string {
	return fmt.Sprintf("installation moved to %s and unregistered, but %s", n.backup, n.note)
}

func (n *userUninstallNote) UninstallNote() string { return n.note }
