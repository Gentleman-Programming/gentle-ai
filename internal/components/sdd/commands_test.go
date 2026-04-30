package sdd

import "testing"

func TestOpenCodeCommandsIncludesCoreWorkflow(t *testing.T) {
	commands := OpenCodeCommands()
	if len(commands) != 9 {
		t.Fatalf("OpenCodeCommands() length = %d", len(commands))
	}

	if commands[0].Name != "sdd-init" {
		t.Fatalf("first command = %q", commands[0].Name)
	}

	seen := map[string]bool{}
	for _, command := range commands {
		seen[command.Name] = true
	}

	for _, name := range []string{
		"sdd-init", "sdd-new", "sdd-continue", "sdd-explore", "sdd-ff",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	} {
		if !seen[name] {
			t.Fatalf("OpenCodeCommands() missing %q", name)
		}
	}
}
