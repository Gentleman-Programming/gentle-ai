package sdd

import "testing"

func TestOpenCodeCommandsIncludesCoreWorkflow(t *testing.T) {
	commands := OpenCodeCommands()
	if len(commands) < 8 {
		t.Fatalf("OpenCodeCommands() length = %d", len(commands))
	}

	if commands[0].Name != "sdd-init" {
		t.Fatalf("first command = %q", commands[0].Name)
	}

	// Verify sdd-onboard command is registered
	found := false
	for _, cmd := range commands {
		if cmd.Name == "sdd-onboard" {
			found = true
			if cmd.Body != "/sdd-onboard" {
				t.Errorf("sdd-onboard Body = %q, want /sdd-onboard", cmd.Body)
			}
			break
		}
	}
	if !found {
		t.Error("OpenCodeCommands() missing sdd-onboard command")
	}
}
