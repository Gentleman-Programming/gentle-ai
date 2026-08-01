package verify

import "testing"

// TestReadyMessageForCommandsNamesOnlyTheGivenCommands closes organic-dx
// Phase 3f task 3f.4: the completion line must name only the agents actually
// installed, never a fixed "claude or opencode" regardless of selection.
func TestReadyMessageForCommandsNamesOnlyTheGivenCommands(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{name: "no runnable commands falls back to the generic ready message", commands: nil, want: readyMessage},
		{name: "single command", commands: []string{"opencode"}, want: "You're ready. Run `opencode` and start building."},
		{name: "two commands", commands: []string{"claude", "opencode"}, want: "You're ready. Run `claude` or `opencode` and start building."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadyMessageForCommands(tt.commands); got != tt.want {
				t.Fatalf("ReadyMessageForCommands(%v) = %q, want %q", tt.commands, got, tt.want)
			}
		})
	}
}
