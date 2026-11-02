//go:build windows

package shellinstaller

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// userWindowsLauncherNames are the commands published in the user's WindowsApps
// PATH directory (#5740). An occupied name is reported, never overwritten: `gs`
// is Ghostscript for many users and a foreign shim stays untouched.
var userWindowsLauncherNames = []string{"gentle-shell", "gs"}

const (
	userWindowsLauncherPublished = "published"
	userWindowsLauncherExisting  = "existing"
	userWindowsLauncherOccupied  = "occupied"
)

type userWindowsLauncherOutcome struct {
	Name       string
	State      string
	OccupiedBy string
}

type userWindowsLauncherReport struct {
	Outcomes    []userWindowsLauncherOutcome
	LauncherDir string
}

// userWindowsLauncherShim renders the WindowsApps shim that forwards to the
// installed bin/<product>.cmd binding for the destination.
func userWindowsLauncherShim(target string) []byte {
	return []byte("@echo off\r\ncall \"" + target + "\" %*\r\n")
}

// userWindowsLauncherNote renders the publication report for the completion
// output. It is empty only when the report is empty.
func userWindowsLauncherNote(report userWindowsLauncherReport) string {
	var lines []string
	var published []string
	for _, outcome := range report.Outcomes {
		link := filepath.Join(report.LauncherDir, outcome.Name+".cmd")
		switch outcome.State {
		case userWindowsLauncherPublished, userWindowsLauncherExisting:
			published = append(published, link)
		case userWindowsLauncherOccupied:
			lines = append(lines, fmt.Sprintf("Command %q is occupied by %s; map it manually if you want it to open gentle-shell.", outcome.Name, outcome.OccupiedBy))
		}
	}
	if len(published) > 0 {
		lines = append([]string{fmt.Sprintf("Commands: %s", strings.Join(published, " and "))}, lines...)
	}
	return strings.Join(lines, "\n")
}

// userWindowsPublishLauncher exposes the Separate target as `gentle-shell.cmd`
// and `gs.cmd` in the user's WindowsApps PATH directory. A foreign command is
// reported, never overwritten: a shim with different content, or a command on
// the worker's PATH, is reported occupied. Detection of foreign commands on
// the user's original PATH (for example Ghostscript's gs.exe) is a follow-up:
// the install worker PATH is System32-only, so it cannot see them.
func userWindowsPublishLauncher(destination string) (userWindowsLauncherReport, error) {
	report := userWindowsLauncherReport{}
	local, err := userWindowsLocalAppData()
	if err != nil {
		return report, errors.Join(err, errors.New("cannot publish the gentle-shell commands"))
	}
	apps := filepath.Join(local, "Microsoft", "WindowsApps")
	if info, err := os.Stat(apps); err != nil || !info.IsDir() {
		return report, errors.Join(err, errors.New("WindowsApps PATH directory is unavailable"))
	}
	target := filepath.Join(destination, "bin", "gentle-shell.cmd")
	if _, err := os.Stat(target); err != nil {
		return report, err
	}
	report.LauncherDir = apps
	data := userWindowsLauncherShim(target)
	for _, name := range userWindowsLauncherNames {
		link := filepath.Join(apps, name+".cmd")
		outcome := userWindowsLauncherOutcome{Name: name}
		if existing, err := os.ReadFile(link); err == nil {
			if bytes.Equal(existing, data) {
				outcome.State = userWindowsLauncherExisting
			} else {
				outcome.State = userWindowsLauncherOccupied
				outcome.OccupiedBy = fmt.Sprintf("%s (existing command)", link)
			}
			report.Outcomes = append(report.Outcomes, outcome)
			continue
		} else if !os.IsNotExist(err) {
			return report, err
		}
		if resolved, lookErr := exec.LookPath(name); lookErr == nil && !strings.EqualFold(filepath.Clean(resolved), link) {
			outcome.State = userWindowsLauncherOccupied
			outcome.OccupiedBy = resolved
			report.Outcomes = append(report.Outcomes, outcome)
			continue
		}
		temporary := link + ".new"
		tempFile, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
		if err != nil {
			return report, err
		}
		_, writeErr := tempFile.Write(data)
		syncErr := tempFile.Sync()
		closeErr := tempFile.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return report, errors.Join(err, os.Remove(temporary))
		}
		if err := os.Rename(temporary, link); err != nil {
			return report, errors.Join(err, os.Remove(temporary))
		}
		outcome.State = userWindowsLauncherPublished
		report.Outcomes = append(report.Outcomes, outcome)
	}
	return report, nil
}

// userWindowsRemoveLauncher removes only the WindowsApps shims whose bytes are
// exactly the canonical shim forwarding to the given destination's installed
// binding; foreign or user-modified commands stay untouched.
func userWindowsRemoveLauncher(target string) error {
	local, err := userWindowsLocalAppData()
	if err != nil {
		return err
	}
	for _, name := range userWindowsLauncherNames {
		link := filepath.Join(local, "Microsoft", "WindowsApps", name+".cmd")
		data, readErr := os.ReadFile(link)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(data, userWindowsLauncherShim(filepath.Join(target, "bin", "gentle-shell.cmd"))) {
			continue
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	return nil
}
