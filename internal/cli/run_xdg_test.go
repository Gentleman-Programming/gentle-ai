package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

// OpenCode guidance and its verification both resolve XDG_CONFIG_HOME rather
// than writing to the default ~/.config/opencode directory.
func TestRunInstallOpenCodeReviewVerifiesUnderXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".xdg")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreDownload := engramDownloadFn
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		engramDownloadFn = restoreDownload
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath
	engramDownloadFn = func(system.PlatformProfile) (string, error) {
		return filepath.Join(t.TempDir(), "engram"), nil
	}

	result, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, linuxDetectionResult(system.LinuxDistroUbuntu, "apt"))
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}
	for _, name := range []string{"AGENTS.md"} {
		path := filepath.Join(xdg, "opencode", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("managed guidance %q missing after install: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("install touched ~/.config/opencode although XDG_CONFIG_HOME is set (stat err = %v)", err)
	}
}
