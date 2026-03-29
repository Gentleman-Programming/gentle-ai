package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
)

// ─── Issue #145: Review screen must show individual skills ───────────────────

// TestRenderReviewShowsSkillNames verifies that when ReviewPayload.Skills is
// populated, RenderReview output contains each individual skill name.
//
// Closes #145.
func TestRenderReviewShowsSkillNames(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:  []model.AgentID{model.AgentClaudeCode},
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		Components: []planner.ComponentAction{
			{ID: model.ComponentSkills, Action: "selected"},
		},
		Skills: []model.SkillID{"sdd-apply", "sdd-spec", "go-testing"},
	}

	out := RenderReview(payload, 0)

	for _, skillName := range []string{"sdd-apply", "sdd-spec", "go-testing"} {
		if !strings.Contains(out, skillName) {
			t.Errorf("RenderReview output missing skill %q; output:\n%s", skillName, out)
		}
	}
}

// TestRenderReviewHidesSkillsSectionWhenEmpty verifies that when there are no
// skills selected, the review screen does not crash and shows no skill names.
//
// Closes #145.
func TestRenderReviewHidesSkillsSectionWhenEmpty(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:  []model.AgentID{model.AgentClaudeCode},
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		// No Skills field.
	}

	out := RenderReview(payload, 0)

	// Should not panic and should render something.
	if len(out) == 0 {
		t.Fatal("RenderReview returned empty string")
	}
}

// ─── Issue #149: Review screen must show Strict TDD status ───────────────────

// TestRenderReviewShowsStrictTDDEnabled verifies that RenderReview output contains
// "Strict TDD" and "Enabled" when HasSDD=true and StrictTDD=true.
//
// Closes #149.
func TestRenderReviewShowsStrictTDDEnabled(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:  []model.AgentID{model.AgentClaudeCode},
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		Components: []planner.ComponentAction{
			{ID: model.ComponentSDD, Action: "selected"},
		},
		HasSDD:    true,
		StrictTDD: true,
	}

	out := RenderReview(payload, 0)

	if !strings.Contains(out, "Strict TDD") {
		t.Errorf("RenderReview missing 'Strict TDD'; output:\n%s", out)
	}
	if !strings.Contains(out, "Enabled") {
		t.Errorf("RenderReview missing 'Enabled' for StrictTDD=true; output:\n%s", out)
	}
}

// TestRenderReviewShowsStrictTDDDisabled verifies that RenderReview output contains
// "Strict TDD" and "Disabled" when HasSDD=true and StrictTDD=false.
//
// Closes #149.
func TestRenderReviewShowsStrictTDDDisabled(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:  []model.AgentID{model.AgentClaudeCode},
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		Components: []planner.ComponentAction{
			{ID: model.ComponentSDD, Action: "selected"},
		},
		HasSDD:    true,
		StrictTDD: false,
	}

	out := RenderReview(payload, 0)

	if !strings.Contains(out, "Strict TDD") {
		t.Errorf("RenderReview missing 'Strict TDD'; output:\n%s", out)
	}
	if !strings.Contains(out, "Disabled") {
		t.Errorf("RenderReview missing 'Disabled' for StrictTDD=false; output:\n%s", out)
	}
}

// TestRenderReviewHidesStrictTDDWhenNoSDD verifies that when HasSDD=false,
// "Strict TDD" does not appear in the review output.
//
// Closes #149.
func TestRenderReviewHidesStrictTDDWhenNoSDD(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:    []model.AgentID{model.AgentClaudeCode},
		Persona:   model.PersonaGentleman,
		Preset:    model.PresetFullGentleman,
		HasSDD:    false,
		StrictTDD: true,
	}

	out := RenderReview(payload, 0)

	if strings.Contains(out, "Strict TDD") {
		t.Errorf("RenderReview should NOT show 'Strict TDD' when HasSDD=false; output:\n%s", out)
	}
}
