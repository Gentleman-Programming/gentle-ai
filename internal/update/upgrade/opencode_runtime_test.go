package upgrade

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

func TestV2CommunityUpgradeRefusesBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	originalHomeDir, originalExecCommand := openCodeHomeDir, execCommand
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() {
		openCodeHomeDir = originalHomeDir
		execCommand = originalExecCommand
		opencode.VersionRunnerOverride = old
	})
	openCodeHomeDir = func() (string, error) { return home, nil }
	// Fail before constructing any process, including the initial RED run.
	// Never delegate to the original executor or a real package manager.
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected subprocess during V2 refusal: %s %q", name, args)
		return nil
	}
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	_, err := opencodePluginUpgrade(context.Background(), update.UpdateResult{Tool: update.ToolInfo{NpmPackage: "opencode-sdd-engram-manage"}, LatestVersion: "1.2.0"})
	if err == nil || !containsV2Refusal(err.Error()) {
		t.Fatalf("expected V2 proof refusal, got %v", err)
	}
}
func containsV2Refusal(s string) bool { return strings.Contains(s, "V2 compatibility") }
