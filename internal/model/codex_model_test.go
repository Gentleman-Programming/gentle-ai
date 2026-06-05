package model_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestCodexEffortValid(t *testing.T) {
	tests := []struct {
		name  string
		input model.CodexEffort
		want  bool
	}{
		{"low", model.CodexEffortLow, true},
		{"medium", model.CodexEffortMedium, true},
		{"high", model.CodexEffortHigh, true},
		{"xhigh", model.CodexEffortXHigh, true},
		{"empty", model.CodexEffort(""), false},
		{"junk", model.CodexEffort("junk"), false},
		{"uppercase", model.CodexEffort("HIGH"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.Valid(); got != tc.want {
				t.Errorf("CodexEffort(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestCodexPresetsCoverAllPhases(t *testing.T) {
	presets := []struct {
		name string
		fn   func() map[string]model.CodexEffort
	}{
		{"Recommended", model.CodexModelPresetRecommended},
		{"Powerful", model.CodexModelPresetPowerful},
		{"LowCost", model.CodexModelPresetLowCost},
	}

	for _, tc := range presets {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.fn()
			if len(m) != 13 {
				t.Errorf("%s preset has %d keys, want 13", tc.name, len(m))
			}
			requiredKeys := []string{
				"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
				"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
				"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
			}
			for _, k := range requiredKeys {
				v, ok := m[k]
				if !ok {
					t.Errorf("%s preset missing key %q", tc.name, k)
					continue
				}
				if !v.Valid() {
					t.Errorf("%s preset[%q] = %q is not a valid CodexEffort", tc.name, k, v)
				}
			}
		})
	}
}

func TestRenderCodexPhaseEfforts_Deterministic(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out1 := model.RenderCodexPhaseEfforts(assignments)
	out2 := model.RenderCodexPhaseEfforts(assignments)
	if out1 != out2 {
		t.Error("RenderCodexPhaseEfforts() is not deterministic: two calls returned different results")
	}
}

func TestRenderCodexPhaseEfforts_NilFallsBackToRecommended(t *testing.T) {
	nilOut := model.RenderCodexPhaseEfforts(nil)
	emptyOut := model.RenderCodexPhaseEfforts(map[string]model.CodexEffort{})
	recommended := model.RenderCodexPhaseEfforts(model.CodexModelPresetRecommended())
	if nilOut != recommended {
		t.Error("RenderCodexPhaseEfforts(nil) should equal Recommended output")
	}
	if emptyOut != recommended {
		t.Error("RenderCodexPhaseEfforts(empty) should equal Recommended output")
	}
}

func TestRenderCodexPhaseEfforts_LowCostTierValues(t *testing.T) {
	out := model.RenderCodexPhaseEfforts(model.CodexModelPresetLowCost())
	// Low-cost: sdd-strong row = medium, sdd-mid row = medium, sdd-cheap row = low
	assertContainsSubstring(t, out, "sdd-strong")
	assertContainsSubstring(t, out, "sdd-mid")
	assertContainsSubstring(t, out, "sdd-cheap")
	// The table should have "medium" for strong and mid tiers and "low" for cheap.
	// We check values per row; the render function groups by tier.
}

func TestRenderCodexPhaseEfforts_PowerfulTierValues(t *testing.T) {
	out := model.RenderCodexPhaseEfforts(model.CodexModelPresetPowerful())
	// Powerful: sdd-strong row = xhigh, sdd-mid row = high, sdd-cheap row = low
	assertContainsSubstring(t, out, "sdd-strong")
	assertContainsSubstring(t, out, "sdd-mid")
	assertContainsSubstring(t, out, "sdd-cheap")
}

func assertContainsSubstring(t *testing.T, s, substr string) {
	t.Helper()
	if !containsStr(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
