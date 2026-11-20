//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The operator chooses an existing install in the configuration screen; the
// intent travels as an exact target. Publication validates it against the
// installed manifest and moves the previous tree aside before the fresh one.
func userReconfigureTarget() string {
	value := os.Getenv("GENTLE_AI_RECONFIGURE_TARGET")
	if value == "" || !privateHierarchyPath(value) {
		return ""
	}
	return value
}

// The registry home is an explicit caller-provided seam: tests and embedders
// that do not set it never write outside the installation target.
func userShellRegistryHome() string {
	value := os.Getenv("GENTLE_AI_SHELL_REGISTRY_HOME")
	if value == "" || !privateHierarchyPath(value) {
		return ""
	}
	return value
}

// userServiceIntentEnvironment carries the host's managed-install intents
// into a delegated unit, whose environment is rebuilt from scratch (env -i).
// Each value passes the same validation its reader applies; the registry
// home must be the unit's own HOME. The launcher dir PATH decision crosses as
// a boolean computed from the caller's PATH, never as that PATH itself, and
// the startup file choice as a path the unit revalidates.
func userServiceIntentEnvironment(home string) []string {
	var env []string
	if registry := userShellRegistryHome(); registry != "" && registry == home {
		env = append(env, "GENTLE_AI_SHELL_REGISTRY_HOME="+registry)
	}
	if target := userReconfigureTarget(); target != "" {
		env = append(env, "GENTLE_AI_RECONFIGURE_TARGET="+target)
	}
	if os.Getenv("GENTLE_AI_MAIN_FALLBACK") == "1" {
		env = append(env, "GENTLE_AI_MAIN_FALLBACK=1")
	}
	return append(env, userLauncherIntentEnvironment(home)...)
}

// userLauncherIntentEnvironment carries the caller terminal's launcher PATH
// decision, and the startup file to edit when the dir is missing from it.
func userLauncherIntentEnvironment(home string) []string {
	if home == "" {
		return nil
	}
	if userLauncherDirOnPath(filepath.Join(home, ".local", "bin")) {
		return []string{userLauncherOnPathIntent + "=1"}
	}
	if profile, ok := userLauncherProfile(home); ok {
		return []string{userLauncherProfileIntent + "=" + profile}
	}
	return nil
}

func userBeginShellReplacement(destination string) (string, bool, error) {
	if userReconfigureTarget() != destination {
		return "", false, nil
	}
	manifest, err := userReadInstallationManifest(destination)
	if err != nil || manifest.Destination != destination {
		return "", false, errors.Join(err, errors.New("replacement target is not an installed Gentle-Shell"))
	}
	backup := destination + ".replaced-" + time.Now().UTC().Format("20060102T150405Z")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		return "", false, errors.New("replacement backup path is already occupied")
	}
	if err := os.Rename(destination, backup); err != nil {
		return "", false, err
	}
	if err := userDirectorySync(filepath.Dir(destination)); err != nil {
		return "", false, err
	}
	return backup, true, nil
}

// A published destination is never removed to satisfy a failed replacement:
// both trees stay for inspection. Only an untouched failure restores the old.
func userRestoreShellReplacement(destination, backup string) error {
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("replacement failure left a published destination; both trees retained")
	}
	if err := os.Rename(backup, destination); err != nil {
		return err
	}
	return userDirectorySync(filepath.Dir(destination))
}

const userReplacedStampLayout = "20060102T150405Z"

// userReplacedStamp returns the canonical UTC stamp of an exact
// <destination>.replaced-<stamp> sibling, refusing every other path.
func userReplacedStamp(destination, path string) (string, bool) {
	prefix := filepath.Base(destination) + ".replaced-"
	name := filepath.Base(path)
	if filepath.Dir(path) != filepath.Dir(destination) || !strings.HasPrefix(name, prefix) {
		return "", false
	}
	stamp := strings.TrimPrefix(name, prefix)
	parsed, err := time.Parse(userReplacedStampLayout, stamp)
	return stamp, err == nil && parsed.Format(userReplacedStampLayout) == stamp
}

// userPruneReplacedBackups bounds the replaced trees a reconfigure leaves
// beside the active installation, mirroring the Windows retention policy: the
// exact keep (the backup just created) is retained, as is the lexically latest
// candidate when clock skew dated it after the keep. Every other canonical
// sibling is removed only through userRemoveReplacedBackup; uncertain trees
// are preserved and reported in a joined error. Callers treat a failure as a
// warning: it never retracts the published installation.
func userPruneReplacedBackups(destination, keep string) error {
	keepStamp, ok := userReplacedStamp(destination, keep)
	if !ok {
		return fmt.Errorf("replaced backup %q is not a canonical sibling of %q; older backups preserved", keep, destination)
	}
	if _, err := os.Lstat(keep); err != nil {
		return fmt.Errorf("replaced backup %q is missing; older backups preserved: %w", keep, err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("replaced backups beside %q cannot be listed; preserved: %w", destination, err)
	}
	var candidates []string
	latest := keepStamp
	for _, entry := range entries {
		candidate := filepath.Join(filepath.Dir(destination), entry.Name())
		if stamp, ok := userReplacedStamp(destination, candidate); ok && stamp != keepStamp {
			candidates = append(candidates, candidate)
			if stamp > latest {
				latest = stamp
			}
		}
	}
	var errs []error
	for _, candidate := range candidates {
		if stamp, _ := userReplacedStamp(destination, candidate); stamp == latest {
			continue // clock skew dated it after the keep; retain it
		}
		if err := userRemoveReplacedBackup(destination, candidate); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// userRemoveReplacedBackup removes one replaced tree only when it is an owned
// physical private directory (never a symlink) whose canonical manifest names
// destination, and only when its identity is unchanged immediately before
// removal. os.RemoveAll unlinks nested symlinks without following them.
func userRemoveReplacedBackup(destination, backup string) error {
	info, err := privateDirectory(backup)
	if err != nil {
		return errors.Join(err, fmt.Errorf("replaced backup %q is not an owned private directory; preserved", backup))
	}
	manifestPath := filepath.Join(backup, "installation.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Size() > 4096 {
		return errors.Join(err, fmt.Errorf("replaced backup %q has no canonical manifest; preserved", backup))
	}
	data, err := os.ReadFile(manifestPath)
	var manifest userManifest
	if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.Schema != userSchema || manifest.Destination != destination {
		return errors.Join(err, fmt.Errorf("replaced backup %q does not belong to %q; preserved", backup, destination))
	}
	current, err := os.Lstat(backup)
	if err != nil || !os.SameFile(info, current) {
		return errors.Join(err, fmt.Errorf("replaced backup %q changed during retention; preserved", backup))
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("replaced backup %q could not be fully removed: %w", backup, err)
	}
	return nil
}

func userRecordPublishedShell(manifest userManifest) error {
	home := userShellRegistryHome()
	if home == "" {
		return nil
	}
	channel := "stable"
	if manifest.MainSDK != nil {
		channel = "main-preview"
	}
	return RecordUserShellInstall(home, UserShellInstall{Target: manifest.Destination, Mode: manifest.Mode, Channel: channel})
}

// userRegistryNote words a registration refusal for the completion output.
func userRegistryNote(err error) string {
	if err == nil {
		return ""
	}
	return "Installation is published and verified, but it was not registered in ~/.gentle-ai/shell-installs.json for Manage: " + err.Error()
}

func userJoinNote(note, extra string) string {
	if note == "" || extra == "" {
		return note + extra
	}
	return note + "\n" + extra
}
