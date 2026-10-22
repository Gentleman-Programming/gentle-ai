package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

func TestShellRoseFramesDecode(t *testing.T) {
	if ShellBloomFrames < 2 {
		t.Fatalf("bloom frames = %d", ShellBloomFrames)
	}
	_, rows := styles.RoseSize()
	for frame := 0; frame < ShellBloomFrames; frame++ {
		lines := shellRoseFrameLines(frame)
		if rows != 0 && len(lines) != rows {
			t.Fatalf("frame %d has %d rows, want %d", frame, len(lines), rows)
		}
		if strings.TrimSpace(strings.Join(lines, "\n")) == "" {
			t.Fatalf("frame %d is empty", frame)
		}
	}
	steps := []ShellInstallStep{{Label: "Validate", State: "done"}, {Label: "Prepare", State: "done"}, {Label: "Runtime", State: "done"}, {Label: "Optional", State: "done"}, {Label: "Publish", State: "done"}}
	early := RenderShellInstall(steps, "", true, 0, 0, true, false, "separate", "gentle-shell", 120, 35)
	if strings.Contains(early, "Welcome to the gentle-tailored experience") || strings.Contains(early, "Enter: finish") {
		t.Fatal("the completion copy must wait for the animation to finish")
	}
	view := RenderShellInstall(steps, "", true, 0, ShellBloomFrames-1, true, false, "separate", "gentle-shell", 120, 35)
	if !strings.Contains(view, "Welcome to the gentle-tailored experience") || !strings.Contains(view, "gentle-shell") || !strings.Contains(view, "⣿") || !strings.Contains(view, "Enter: finish") {
		t.Fatalf("truecolor rose or completion help is missing:\n%s", view)
	}
}
