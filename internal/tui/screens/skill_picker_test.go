package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func TestSkillPickerCanonicalRowsAndActions(t *testing.T) {
	skills := AllSkillsOrdered()
	labels := []string{"SDD Init", "SDD Explore", "SDD Research", "SDD Propose", "SDD Spec", "SDD Design", "SDD Tasks", "SDD Apply", "SDD Verify", "SDD Archive", "SDD Onboard", "Judgment Day", "Go Testing", "Gentle AI Bench", "Skill Creator", "Skill Improver", "Branch & PR", "Issue Creation", "Skill Registry", "Chained PR", "Cognitive Doc Design", "Comment Writer", "Work Unit Commits", "RDD Defect Workflow", "Systemic Issue Triage"}
	if len(skills) != len(labels) {
		t.Fatalf("canonical skills = %d, want %d", len(skills), len(labels))
	}
	for i, skill := range skills {
		if got := skillLabelFor(skill); got != labels[i] || !strings.Contains(RenderSkillPicker(skills, i, 0), styles.Cursor+"[x] "+labels[i]) {
			t.Errorf("canonical row %d for %q has label %q or focus mismatch", i, skill, got)
		}
	}
	for i, action := range SkillPickerOptions() {
		if !strings.Contains(RenderSkillPicker(skills, len(skills)+i, 0), styles.Cursor+action) {
			t.Errorf("cursor %d does not focus action %q", len(skills)+i, action)
		}
	}
	if view := RenderSkillPicker(skills, len(skills)+1, 7); !strings.Contains(view, styles.Cursor+"Back") || strings.Contains(view, "SDD Init") || !strings.Contains(view, "Rows 27-27 of 27") {
		t.Fatal("small viewport did not follow Back with a scroll hint")
	}
}
