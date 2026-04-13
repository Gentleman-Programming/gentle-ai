package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

// ─── RenderProfileDelete ──────────────────────────────────────────────────────

func TestRenderProfileDelete_ShowsProfileName(t *testing.T) {
	output := screens.RenderProfileDelete("cheap", 0)

	if !strings.Contains(output, "cheap") {
		t.Errorf("expected profile name 'cheap' in output, got:\n%s", output)
	}
}

func TestRenderProfileDelete_ShowsTitle(t *testing.T) {
	output := screens.RenderProfileDelete("premium", 0)

	if !strings.Contains(output, "Delete Profile") {
		t.Errorf("expected title 'Delete Profile' in output, got:\n%s", output)
	}
}

func TestRenderProfileDelete_ShowsDeleteAndSync(t *testing.T) {
	output := screens.RenderProfileDelete("cheap", 0)

	if !strings.Contains(output, "Delete & Sync") {
		t.Errorf("expected 'Delete & Sync' option in output, got:\n%s", output)
	}
}

func TestRenderProfileDelete_ShowsCancel(t *testing.T) {
	output := screens.RenderProfileDelete("cheap", 1)

	if !strings.Contains(output, "Cancel") {
		t.Errorf("expected 'Cancel' option in output, got:\n%s", output)
	}
}

func TestRenderProfileDelete_ShowsAgentKeyCount(t *testing.T) {
	output := screens.RenderProfileDelete("cheap", 0)

	// Should mention 11 agents that will be removed.
	if !strings.Contains(output, "11") {
		t.Errorf("expected agent key count '11' in output, got:\n%s", output)
	}
}

// ─── ProfileDeleteOptionCount ─────────────────────────────────────────────────

func TestProfileDeleteOptionCount_ReturnsTwo(t *testing.T) {
	count := screens.ProfileDeleteOptionCount()

	if count != 2 {
		t.Errorf("expected ProfileDeleteOptionCount() == 2, got %d", count)
	}
}
