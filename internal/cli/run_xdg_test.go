package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// TestRunInstallOpenCodeSDDVerifiesUnderXDGConfigHome pins #3219: the SDD
// writer resolves the OpenCode config directory through the adapter, so with
// XDG_CONFIG_HOME set every plugin lands under $XDG_CONFIG_HOME/opencode.
// Verification then looked for the same files under ~/.config/opencode, failed,
// and the rollback deleted the files the writer had just produced.
func TestRunInstallOpenCodeSDDVerifiesUnderXDGConfigHome(t *testing.T) {
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

	result, err := RunInstall([]string{"--agent", "opencode", "--component", "sdd"}, linuxDetectionResult(system.LinuxDistroUbuntu, "apt"))
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}
	for _, name := range sdd.ManagedOpenCodePluginNames() {
		path := filepath.Join(xdg, "opencode", "plugins", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("managed plugin %q missing after install: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("install touched ~/.config/opencode although XDG_CONFIG_HOME is set (stat err = %v)", err)
	}
}
