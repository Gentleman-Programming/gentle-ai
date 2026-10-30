//go:build !windows

package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// managedInstallDisplayTarget is how the detection screen shows the default
// managed install created by managedInstallFixture.
const managedInstallDisplayTarget = "~/.local/share/gentle-shell/install"

// platformManagedInstallFixture reports no platform-specific fixture: Unix
// discovery reads the installation.json fixture in shell_detected_test.go.
func platformManagedInstallFixture(*testing.T) (home, target string, ok bool) {
	return "", "", false
}

// platformSetInstalledCodeGraph reports no platform seam: Unix records the
// installed CodeGraph component in installation.json (setManifestCodeGraph).
func platformSetInstalledCodeGraph(*testing.T, string, bool) bool { return false }

// shellDefaultTestDestination is the default Separate target the TUI selects.
func shellDefaultTestDestination() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "gentle-shell", "install")
}

// assertShellPrivateParent requires the default install parent to be 0700.
func assertShellPrivateParent(t *testing.T, base string) {
	t.Helper()
	if info, err := os.Lstat(base); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("default parent is not private: %v %v", info, err)
	}
}

// shellAssertPlatformWorkerHandoff has no channel field to check on Unix: the
// SDK source travels only inside the experience profile.
func shellAssertPlatformWorkerHandoff(*testing.T, shellinstaller.UserInstallRequest) {}
