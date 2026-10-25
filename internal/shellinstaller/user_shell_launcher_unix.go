//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// userShellLauncherNames are the commands published on the standard user PATH
// (#5740). An occupied name is reported, never overwritten: `gs` is
// Ghostscript on stock distributions and user aliases live in rc files the
// installer must not touch.
var userShellLauncherNames = []string{"gentle-shell", "gs"}

const (
	userShellLauncherPublished = "published"
	userShellLauncherExisting  = "existing"
	userShellLauncherOccupied  = "occupied"
)

type userShellLauncherOutcome struct {
	Name       string
	State      string
	OccupiedBy string
}

type userShellLauncherReport struct {
	Outcomes    []userShellLauncherOutcome
	LauncherDir string
	DirOnPath   bool
}

func userShellLauncherPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", name), nil
}

func userLauncherDirOnPath(dir string) bool {
	for _, element := range filepath.SplitList(os.Getenv("PATH")) {
		if element != "" && filepath.Clean(element) == dir {
			return true
		}
	}
	return false
}

// userShellLauncherNote renders the publication report for the completion
// output. It is empty only when the report is empty.
func userShellLauncherNote(report userShellLauncherReport) string {
	var lines []string
	var published []string
	for _, outcome := range report.Outcomes {
		link, err := userShellLauncherPath(outcome.Name)
		if err != nil {
			continue
		}
		switch outcome.State {
		case userShellLauncherPublished, userShellLauncherExisting:
			published = append(published, link)
		case userShellLauncherOccupied:
			lines = append(lines, fmt.Sprintf("Command %q is occupied by %s; map it manually if you want it to open gentle-shell.", outcome.Name, outcome.OccupiedBy))
		}
	}
	if len(published) > 0 {
		lines = append([]string{fmt.Sprintf("Commands: %s", strings.Join(published, " and "))}, lines...)
	}
	if report.LauncherDir != "" && !report.DirOnPath {
		lines = append(lines, fmt.Sprintf("%s is not on this session's PATH; open a new login session or run: export PATH=\"$HOME/.local/bin:$PATH\"", report.LauncherDir))
	}
	return strings.Join(lines, "\n")
}

// PublishUserShellLauncher exposes the Separate target as `gentle-shell` and
// `gs` on the standard user PATH. It never overwrites a foreign launcher and
// reports every name's outcome instead.
func PublishUserShellLauncher(destination string) (userShellLauncherReport, error) {
	report := userShellLauncherReport{}
	bin := filepath.Join(destination, "bin", "gentle-shell")
	info, err := os.Lstat(bin)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return report, fmt.Errorf("installed launcher %s is missing or not executable", bin)
	}
	dir, err := userShellLauncherPath("")
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return report, err
	}
	report.LauncherDir = dir
	report.DirOnPath = userLauncherDirOnPath(dir)
	for _, name := range userShellLauncherNames {
		link := filepath.Join(dir, name)
		outcome := userShellLauncherOutcome{Name: name}
		if info, err := os.Lstat(link); err == nil {
			target, readErr := os.Readlink(link)
			if info.Mode()&os.ModeSymlink != 0 && readErr == nil && target == bin {
				outcome.State = userShellLauncherExisting
			} else if info.Mode()&os.ModeSymlink != 0 && readErr == nil {
				outcome.State = userShellLauncherOccupied
				outcome.OccupiedBy = fmt.Sprintf("%s (points to %s)", link, target)
			} else {
				outcome.State = userShellLauncherOccupied
				outcome.OccupiedBy = fmt.Sprintf("%s (existing file)", link)
			}
			report.Outcomes = append(report.Outcomes, outcome)
			continue
		} else if !os.IsNotExist(err) {
			return report, err
		}
		if resolved, lookErr := exec.LookPath(name); lookErr == nil && filepath.Clean(resolved) != link {
			outcome.State = userShellLauncherOccupied
			outcome.OccupiedBy = resolved
			report.Outcomes = append(report.Outcomes, outcome)
			continue
		}
		temporary := link + ".new"
		_ = os.Remove(temporary)
		if err := os.Symlink(bin, temporary); err != nil {
			return report, err
		}
		if err := os.Rename(temporary, link); err != nil {
			_ = os.Remove(temporary)
			return report, err
		}
		outcome.State = userShellLauncherPublished
		report.Outcomes = append(report.Outcomes, outcome)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return report, err
	}
	defer directory.Close()
	return report, directory.Sync()
}

// RemoveUserShellLauncher removes only symlinks owned by destination.
func RemoveUserShellLauncher(destination string) error {
	bin := filepath.Join(destination, "bin", "gentle-shell")
	for _, name := range userShellLauncherNames {
		link, err := userShellLauncherPath(name)
		if err != nil {
			return err
		}
		target, err := os.Readlink(link)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			if name == "gentle-shell" {
				return errors.New("existing gentle-shell launcher is not a symlink; leaving it untouched")
			}
			// A foreign gs launcher is never removed by an uninstall.
			continue
		}
		if target != bin {
			continue
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	return nil
}
