package screens

import (
	"strings"
	"testing"
)

func TestRenderABSDD_NonEmpty(t *testing.T) {
	out := RenderABSDD("standalone", 0)
	if out == "" {
		t.Fatal("RenderABSDD returned empty string")
	}
}

func TestRenderABSDD_HeadingPresent(t *testing.T) {
	out := RenderABSDD("standalone", 0)
	if !strings.Contains(out, "SDD Integration") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABSDD_OptionsPresent(t *testing.T) {
	out := RenderABSDD("standalone", 0)
	if !strings.Contains(out, "Standalone") {
		t.Errorf("Standalone option not found; output:\n%s", out)
	}
	if !strings.Contains(out, "New SDD Phase") {
		t.Errorf("New SDD Phase option not found; output:\n%s", out)
	}
	if !strings.Contains(out, "Phase Support") {
		t.Errorf("Phase Support option not found; output:\n%s", out)
	}
}

func TestRenderABSDD_BackOptionPresent(t *testing.T) {
	out := RenderABSDD("standalone", 0)
	if !strings.Contains(out, "Back") {
		t.Errorf("Back option not found; output:\n%s", out)
	}
}

func TestABSDDOptions_HasFourItems(t *testing.T) {
	opts := ABSDDOptions()
	if len(opts) != 4 {
		t.Errorf("ABSDDOptions() len = %d, want 4", len(opts))
	}
}

func TestRenderABSDDPhase_NonEmpty(t *testing.T) {
	phases := ABSDDPhases()
	out := RenderABSDDPhase(phases, 0, false)
	if out == "" {
		t.Fatal("RenderABSDDPhase returned empty string")
	}
}

func TestRenderABSDDPhase_HeadingPresent(t *testing.T) {
	phases := ABSDDPhases()
	out := RenderABSDDPhase(phases, 0, false)
	if !strings.Contains(out, "Select SDD Phase") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABSDDPhase_NewPhaseMode_InsertAfterLabel(t *testing.T) {
	phases := ABSDDPhases()
	out := RenderABSDDPhase(phases, 0, true)
	if !strings.Contains(out, "Insert after") {
		t.Errorf("'Insert after' label not found for new-phase mode; output:\n%s", out)
	}
}

func TestRenderABSDDPhase_SupportMode_SupportPhaseLabel(t *testing.T) {
	phases := ABSDDPhases()
	out := RenderABSDDPhase(phases, 0, false)
	if !strings.Contains(out, "Support phase") {
		t.Errorf("'Support phase' label not found for support mode; output:\n%s", out)
	}
}

func TestABSDDPhases_HasExpectedPhases(t *testing.T) {
	phases := ABSDDPhases()
	expected := []string{"explore", "propose", "spec", "design", "tasks", "apply", "verify", "archive"}
	if len(phases) != len(expected) {
		t.Errorf("ABSDDPhases() len = %d, want %d", len(phases), len(expected))
	}
	for i, phase := range expected {
		if phases[i] != phase {
			t.Errorf("phases[%d] = %q, want %q", i, phases[i], phase)
		}
	}
}
