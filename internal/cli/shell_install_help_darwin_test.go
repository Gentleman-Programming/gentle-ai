//go:build darwin

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
			"--mode shared --prefix /owned/selected-prefix --agent /owned/selected-agent",
			"Requires macOS 14 or newer on Apple silicon (arm64)",
			"Shared creates AGENT/bin/fd and AGENT/bin/rg; existing tools refuse before changes.",
			"gentle-ai shell recover ROOT inspect",
			"Replace inspect with its printed confirmation to restore shared preimages.",
			"pinned fd and rg helpers to AGENT/bin",
			"one process group plus per-process rlimits, not a cgroup",
			"calls setsid escapes the group",
			"there is no memory cap",
			"counts every process of your user",
			"Installer writes use F_FULLFSYNC and refuse filesystems without it;",
			"the Node helper falls back to a weaker fsync there.",
			"channel selection is Windows-only",
		} {
			if !strings.Contains(help, expected) {
				t.Fatalf("help %q hides %q: %s", args, expected, help)
			}
		}
		for _, hidden := range []string{"Linux", "systemd", "cgroup limits", "not yet available", "Separate mode only"} {
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

// Shared inspection on macOS prints the confirmation and the settings
// disclosure for the canonical selection, without effects.
func TestShellInstallSharedInspectOnMacOS(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	prefix, agent, target := filepath.Join(root, "prefix"), filepath.Join(root, "agent"), filepath.Join(root, "shell")
	cli := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	for path, data := range map[string]string{cli: "// inert fixture; never execute\n", filepath.Join(agent, "settings.json"): "{\"theme\":\"dark\"}\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := RunShell([]string{"install", "--mode", "shared", "--prefix", prefix, "--agent", agent, "--target", target, "--inspect"}, &output); err != nil {
		t.Fatalf("Shared inspection refused on macOS: %v", err)
	}
	for _, expected := range []string{"Confirmation:", target + "/bin/pi", filepath.Join(agent, "settings.json"), "npmCommand"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("Shared inspection hides %q: %s", expected, output.String())
		}
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("Shared inspection created the target: %v", err)
	}
}
