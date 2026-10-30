//go:build windows

package shellinstaller

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// userWindowsSystemGitCandidateEnv carries the single caller context item the
// owned launch needs for Git: the absolute git.exe path resolved on the
// original process PATH by the outer entry process. As with system Go, no
// caller PATH is forwarded; the worker revalidates the candidate on every
// launch before adding only its directory to the owned PATH.
const userWindowsSystemGitCandidateEnv = "GENTLE_SHELL_WINDOWS_SYSTEM_GIT"

// userWindowsSystemGitCandidate resolves a caller-installed git.exe by exact
// name on the original process PATH of the outer entry process. It performs
// no execution and no trust decision.
func userWindowsSystemGitCandidate() string {
	candidate, err := exec.LookPath("git.exe")
	if err != nil || !filepath.IsAbs(candidate) {
		return ""
	}
	return filepath.Clean(candidate)
}

// userWindowsSystemGitValidate applies the system-Go custody requirements to
// a Git candidate: an absolute canonical regular git.exe without reparse
// points or aliases anywhere on its path, on a canonical local NTFS drive,
// whose every ancestor passes the owned-identity check System32 passes.
func userWindowsSystemGitValidate(candidate string) error {
	if !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate || !strings.EqualFold(filepath.Base(candidate), "git.exe") {
		return errors.New("Windows system Git path is not an absolute canonical git.exe")
	}
	info, err := os.Lstat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Windows system Git executable is missing or not a regular file")
	}
	if _, err := userWindowsIdentity(candidate, false); err != nil {
		return errors.Join(err, errors.New("Windows system Git directory custody differs"))
	}
	return nil
}

// userWindowsSystemGitBin is the owned PATH entry for the forwarded Git
// candidate. Any absence or doubt omits Git, never failing the launch.
func userWindowsSystemGitBin() (string, bool) {
	candidate := os.Getenv(userWindowsSystemGitCandidateEnv)
	if candidate == "" || userWindowsSystemGitValidate(candidate) != nil {
		return "", false
	}
	return filepath.Dir(candidate), true
}

// userWindowsProgramFilesFolder resolves the system Program Files folder
// from the token, never from the caller environment. It is a seam so focused
// tests never depend on the runner's real folder.
var userWindowsProgramFilesFolder = func() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
}

// userWindowsProgramFiles is the validated ProgramFiles value of the owned
// environment (Pi resolves Git Bash below it). Any doubt omits it.
func userWindowsProgramFiles() (string, bool) {
	folder, err := userWindowsProgramFilesFolder()
	if err != nil || folder == "" {
		return "", false
	}
	if _, err := userWindowsIdentity(folder, false); err != nil {
		return "", false
	}
	return folder, true
}
