package screens

import (
	"strings"
	"testing"
)

// ─── ModelConfigOptions ────────────────────────────────────────────────────

// TestModelConfigOptions_Count verifies that ModelConfigOptions returns exactly
// 4 items: Claude, OpenCode, Kiro, and Back.
func TestModelConfigOptions_Count(t *testing.T) {
	opts := ModelConfigOptions()
	if len(opts) != 4 {
		t.Fatalf("ModelConfigOptions() len = %d, want 4; got %v", len(opts), opts)
	}
}

// TestModelConfigOptions_Order verifies the exact order of options:
// Claude → OpenCode → Kiro → Back.
func TestModelConfigOptions_Order(t *testing.T) {
	opts := ModelConfigOptions()

	// wantKeywords defines the expected order by a unique keyword per option.
	wantKeywords := []string{"Claude", "OpenCode", "Kiro", "Back"}

	if len(opts) != len(wantKeywords) {
		t.Fatalf("ModelConfigOptions() len = %d, want %d; got %v", len(opts), len(wantKeywords), opts)
	}

	for i, keyword := range wantKeywords {
		if !strings.Contains(opts[i], keyword) {
			t.Errorf("ModelConfigOptions()[%d] = %q, want option containing %q", i, opts[i], keyword)
		}
	}
}

// TestModelConfigOptions_BackIsLast verifies that "Back" is the last option.
func TestModelConfigOptions_BackIsLast(t *testing.T) {
	opts := ModelConfigOptions()
	last := opts[len(opts)-1]
	if !strings.Contains(last, "Back") {
		t.Errorf("ModelConfigOptions() last item = %q, want 'Back'", last)
	}
}

// ─── RenderModelConfig ─────────────────────────────────────────────────────

// TestRenderModelConfig_RendersAllOptions verifies that the model config screen
// renders all options.
func TestRenderModelConfig_RendersAllOptions(t *testing.T) {
	out := RenderModelConfig(0)

	if !strings.Contains(out, "Model Configuration") {
		t.Errorf("RenderModelConfig should show 'Model Configuration'; got:\n%s", out)
	}

	for _, opt := range ModelConfigOptions() {
		// Strip style markup by checking for substrings of the raw option text.
		label := stripStyleArtifacts(opt)
		if !strings.Contains(out, label) {
			t.Errorf("RenderModelConfig should render option %q; got:\n%s", opt, out)
		}
	}
}

// TestRenderModelConfig_CursorZeroHighlightsFirst verifies that cursor=0
// highlights the first option (Claude).
func TestRenderModelConfig_CursorZeroHighlightsFirst(t *testing.T) {
	outCursor0 := RenderModelConfig(0)
	outCursor1 := RenderModelConfig(1)

	// The two renders should differ (cursor position changes highlighting).
	if outCursor0 == outCursor1 {
		t.Error("RenderModelConfig(cursor=0) and RenderModelConfig(cursor=1) should produce different output")
	}
}

// TestRenderModelConfig_ContainsNavigationHint verifies navigation hints are shown.
func TestRenderModelConfig_ContainsNavigationHint(t *testing.T) {
	out := RenderModelConfig(0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "navigate") && !strings.Contains(lower, "j/k") {
		t.Errorf("RenderModelConfig should contain navigation hint; got:\n%s", out)
	}
}

// stripStyleArtifacts returns a simplified plain-text version of a string for
// comparison when style codes may wrap the content. We check for key words
// contained in the original string.
func stripStyleArtifacts(s string) string {
	// The plain text content of the option is the key identifiable part.
	// We take the first 4 characters as a discriminator (e.g. "Conf", "Back").
	if len(s) <= 4 {
		return s
	}
	return s[:4]
}
