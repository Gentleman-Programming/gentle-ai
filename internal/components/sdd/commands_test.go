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
}
