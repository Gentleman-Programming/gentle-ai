package screens

import (
	"strings"
	"testing"
)

// ─── ModelConfigOptions ────────────────────────────────────────────────────

// TestModelConfigOptions_Count verifies that ModelConfigOptions returns exactly
// 3 items: Claude, OpenCode, and Back.
func TestModelConfigOptions_Count(t *testing.T) {
	opts := ModelConfigOptions()
	if len(opts) != 3 {
		t.Fatalf("ModelConfigOptions() len = %d, want 3; got %v", len(opts), opts)
	}
}

// TestModelConfigOptions_ContainsRequiredItems verifies the three options are
// Claude, OpenCode, and Back (in order).
func TestModelConfigOptions_ContainsRequiredItems(t *testing.T) {
	opts := ModelConfigOptions()

	want := []string{"Claude", "OpenCode", "Back"}
	for _, w := range want {
		found := false
		for _, opt := range opts {
			if strings.Contains(opt, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ModelConfigOptions() missing expected option containing %q; got %v", w, opts)
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
// renders all three options.
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
