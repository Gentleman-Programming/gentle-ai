//go:build !windows && !darwin

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellInstallHelpDisclosesSharedAgentTools(t *testing.T) {
	const disclosure = "Shared creates AGENT/bin/fd and AGENT/bin/rg; existing tools refuse before changes."
	for _, args := range [][]string{{"--help"}, {"install", "--help"}} {
		var output bytes.Buffer
		if err := RunShell(args, &output); err != nil {
			t.Fatalf("help %q failed: %v", args, err)
		}
		if output.String() != shellInstallHelp {
			t.Fatalf("help %q changed its output contract: %q", args, output.String())
		}
		if !strings.Contains(output.String(), "\n"+disclosure+"\n") {
			t.Fatalf("help %q omits the exact Shared tool disclosure: %q", args, output.String())
		}
	}
}

func TestShellInstallHelpHasNoEffects(t *testing.T) {
	var output bytes.Buffer
	if err := RunShell([]string{"install", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--inspect") || !strings.Contains(output.String(), "existing delegated") {
		t.Fatalf("missing consent or prerequisites: %q", output.String())
	}
}

func TestShellInstallHelpDisclosesAgentHelpers(t *testing.T) {
	var output bytes.Buffer
	if err := RunShell([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"pinned fd and rg helpers to AGENT/bin", "selected --agent (Shared)", "channel selection is Windows-only"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help hides %q: %s", expected, output.String())
		}
	}
}

func TestShellInstallTUIIdentifiesLinux(t *testing.T) {
	if shellInstallTitle != "Gentle Shell Linux user installer" {
		t.Fatalf("Linux installer TUI title = %q", shellInstallTitle)
	}
}
