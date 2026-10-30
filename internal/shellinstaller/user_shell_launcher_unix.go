//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// The startup file PATH step, planned only when DirOnPath is false.
	Profile       string
	ProfileState  string
	ProfileReason string
	Fish          bool
}

func userShellLauncherPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", name), nil
}

// userLauncherOnPathIntent carries the caller terminal's decision into a
// delegated unit, whose own PATH is the fixed /usr/bin:/bin (#5758). Only the
// boolean crosses the unit; the caller's PATH never does.
const userLauncherOnPathIntent = "GENTLE_AI_LAUNCHER_DIR_ON_PATH"

func userLauncherDirOnPath(dir string) bool {
	if os.Getenv(userLauncherOnPathIntent) == "1" {
		return true
	}
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
	// Without a served launcher no PATH step was planned and none is shown.
	if report.LauncherDir != "" && !report.DirOnPath && report.ProfileState != "" {
		lines = append(lines, report.LauncherDir+" is not on this terminal's PATH.", "Current terminal: "+userLauncherPathCommand(report.Fish))
		switch report.ProfileState {
		case userLauncherProfileAdded:
			lines = append(lines, "Future terminals: open a new terminal", "PATH was added to "+report.Profile)
		case userLauncherProfilePresent:
			lines = append(lines, "Future terminals: open a new terminal", "PATH is already set in "+report.Profile)
		default:
			file := report.Profile
			if file == "" {
				file = "your shell startup file"
			}
			lines = append(lines, "Future terminals: add this line to "+file+", then open a new terminal: "+userLauncherPathLine(report.Fish))
			if report.Profile == "" {
				lines = append(lines, "No startup file was edited: "+report.ProfileReason)
			} else if report.ProfileReason != "" {
				lines = append(lines, report.Profile+" was not edited: "+report.ProfileReason)
			}
		}
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
	if err := directory.Sync(); err != nil {
		return report, err
	}
	// New terminals find the launchers only once the dir is on their PATH.
	served := slices.ContainsFunc(report.Outcomes, func(outcome userShellLauncherOutcome) bool { return outcome.State != userShellLauncherOccupied })
	if !report.DirOnPath && served {
		userApplyLauncherPath(&report, filepath.Dir(filepath.Dir(dir)))
	}
	return report, nil
}

// RemoveUserShellLauncher removes only symlinks owned by destination, then our
// PATH line once no gentle-shell launcher is left to serve.
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
	link, err := userShellLauncherPath("gentle-shell")
	if err != nil {
		return err
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		return err
	}
	return userRemoveLauncherPath(filepath.Dir(filepath.Dir(filepath.Dir(link))))
}
