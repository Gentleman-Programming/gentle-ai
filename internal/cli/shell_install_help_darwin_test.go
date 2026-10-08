//go:build darwin

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// macOS help discloses its own platform, mode and containment contract, never
// the Linux user-manager prerequisites.
func TestShellInstallHelpDisclosesMacOSContract(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"install", "--help"}} {
		var output bytes.Buffer
		if err := RunShell(args, &output); err != nil {
			t.Fatalf("help %q failed: %v", args, err)
		}
		help := output.String()
		if help != shellInstallHelp {
			t.Fatalf("help %q changed its output contract: %q", args, help)
		}
		for _, expected := range []string{
			"--mode separate", "--inspect", "--confirm SHA256",
			"Requires macOS 14 or newer on Apple silicon (arm64)",
			"Separate mode only",
			"Shared mode (--mode shared) and gentle-ai shell recover",
			"are not yet available on macOS",
			"pinned fd and rg helpers to AGENT/bin",
			"one process group plus per-process rlimits, not a cgroup",
			"calls setsid escapes the group",
			"there is no memory cap",
			"counts every process of your user",
			"F_FULLFSYNC",
			"channel selection is Windows-only",
		} {
			if !strings.Contains(help, expected) {
				t.Fatalf("help %q hides %q: %s", args, expected, help)
			}
		}
		for _, hidden := range []string{"Linux", "systemd", "cgroup limits", "--prefix", "--agent /owned"} {
			if strings.Contains(help, hidden) {
				t.Fatalf("help %q discloses a non-macOS contract %q: %s", args, hidden, help)
			}
		}
	}
}

func TestShellInstallTUIIdentifiesMacOS(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := shellInstallModel{cancel: cancel, req: shellinstaller.UserInstallRequest{Mode: "separate"}}
	if !strings.HasPrefix(m.content(), "Gentle Shell macOS user installer\n") || strings.Contains(m.content(), "Linux") {
		t.Fatalf("macOS installer TUI title = %q", m.content())
	}
}
