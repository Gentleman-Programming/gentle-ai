package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
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

	out := RenderReview(payload, 0, "")

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

	out := RenderReview(payload, 0, "")

	// Should not panic and should render something.
	if len(out) == 0 {
		t.Fatal("RenderReview returned empty string")
	}
}

// ─── Legacy Strict TDD selections are not installer review choices ───────────

func TestRenderReviewOmitsLegacyStrictTDDChoice(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[strict], func(t *testing.T) {
			payload := planner.ReviewPayload{
				Components: []planner.ComponentAction{{ID: model.ComponentSDD, Action: "selected"}},
				HasSDD:     true, StrictTDD: strict,
			}
			if out := RenderReview(payload, 0, ""); strings.Contains(out, "Strict TDD") {
				t.Fatalf("legacy choice appears in review:\n%s", out)
			}
		})
	}
}

func TestRenderReviewShowsDeferredInstallReviewMode(t *testing.T) {
	out := RenderReview(planner.ReviewPayload{}, 0, "RDD OFF (global setting after successful installation)")
	if !strings.Contains(out, "Receipt-Driven Development") || !strings.Contains(out, "RDD OFF") {
		t.Fatalf("review summary omitted selected RDD mode:\n%s", out)
	}
}

func TestRenderReviewClarifiesCustomPersonaAndPreset(t *testing.T) {
	payload := planner.ReviewPayload{
		Agents:  []model.AgentID{model.AgentClaudeCode},
		Persona: model.PersonaCustom,
		Preset:  model.PresetCustom,
	}

	out := RenderReview(payload, 0, "")

	if !strings.Contains(out, "keep existing persona unmanaged") {
		t.Fatalf("RenderReview missing custom persona clarification; output:\n%s", out)
	}
	if !strings.Contains(out, "choose components and skills manually") {
		t.Fatalf("RenderReview missing custom preset clarification; output:\n%s", out)
	}
	if strings.Contains(out, "Persona  custom") {
		t.Fatalf("RenderReview should not show raw custom persona label; output:\n%s", out)
	}
	if strings.Contains(out, "Preset  custom") {
		t.Fatalf("RenderReview should not show raw custom preset label; output:\n%s", out)
	}
}

func TestRenderReviewSummarizesPersonaConversationAndArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		persona model.PersonaID
		want    string
	}{
		{name: "Gentleman", persona: model.PersonaGentleman, want: "Voseo conversation; English technical artifacts"},
		{name: "Gentleman with English artifacts", persona: model.PersonaGentlemanNeutralArtifacts, want: "No regional conversation tone; English technical artifacts (legacy alias, remapped)"},
		{name: "Neutral", persona: model.PersonaNeutral, want: "No regional conversation tone; English technical artifacts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := planner.ReviewPayload{Persona: tt.persona}
			out := RenderReview(payload, 0, "")
			if !strings.Contains(out, tt.want) {
				t.Fatalf("RenderReview() missing %q; output:\n%s", tt.want, out)
			}
		})
	}
}
