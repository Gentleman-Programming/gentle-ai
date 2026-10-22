//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func userShellLauncherPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "gentle-shell"), nil
}

// PublishUserShellLauncher exposes the Separate target as `gentle-shell` on
// the standard user PATH. It never overwrites a foreign launcher.
func PublishUserShellLauncher(destination string) error {
	bin := filepath.Join(destination, "bin", "gentle-shell")
	info, err := os.Lstat(bin)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("installed launcher %s is missing or not executable", bin)
	}
	link, err := userShellLauncherPath()
	if err != nil {
		return err
	}
	if info, err := os.Lstat(link); err == nil {
		target, readErr := os.Readlink(link)
		if info.Mode()&os.ModeSymlink == 0 || readErr != nil || target != bin {
			return fmt.Errorf("%s already exists and is not this installation", link)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	temporary := link + ".new"
	_ = os.Remove(temporary)
	if err := os.Symlink(bin, temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, link); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	directory, err := os.Open(filepath.Dir(link))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// RemoveUserShellLauncher removes only a symlink owned by destination.
func RemoveUserShellLauncher(destination string) error {
	bin := filepath.Join(destination, "bin", "gentle-shell")
	link, err := userShellLauncherPath()
	if err != nil {
		return err
	}
	target, err := os.Readlink(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("existing gentle-shell launcher is not a symlink; leaving it untouched")
	}
	if target != bin {
		return nil
	}
	return os.Remove(link)
}
