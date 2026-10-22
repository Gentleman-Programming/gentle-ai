//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
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

func userRecordPublishedShell(manifest userManifest) {
	home := userShellRegistryHome()
	if home == "" {
		return
	}
	channel := "stable"
	if manifest.MainSDK != nil {
		channel = "main-preview"
	}
	_ = RecordUserShellInstall(home, UserShellInstall{Target: manifest.Destination, Mode: manifest.Mode, Channel: channel})
}
