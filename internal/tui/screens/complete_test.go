package screens

import (
	"strings"
	"testing"
)

func TestRenderCompleteSuccessShowsGGANotesWhenInstalled(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		GGAInstalled:        true,
	})

	if !strings.Contains(out, "GGA (per project)") {
		t.Fatalf("missing GGA section: %q", out)
	}
	if !strings.Contains(out, "gga init") || !strings.Contains(out, "gga install") {
		t.Fatalf("missing GGA repo commands: %q", out)
	}
}

func TestRenderCompleteSuccessHidesGGANotesWhenNotInstalled(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		GGAInstalled:        false,
	})

	if strings.Contains(out, "GGA (per project)") {
		t.Fatalf("unexpected GGA section: %q", out)
	}
}

func TestRenderCompleteShowsManualActions(t *testing.T) {
	out := RenderComplete(CompletePayload{ManualActions: []string{"Pi CodeGraph child drifted; preserved: /tmp/worker.md"}})
	if !strings.Contains(out, "Manual actions required") || !strings.Contains(out, "Pi CodeGraph child drifted") {
		t.Fatalf("manual action missing from completion output: %q", out)
	}
}

func TestRenderCompleteDistinguishesPartialRollbackAndKeepsManualActions(t *testing.T) {
	out := RenderComplete(CompletePayload{FailedSteps: []FailedStep{{ID: "install", Error: "failed"}}, RollbackPerformed: true, RollbackComplete: false, ManualActions: []string{"Pi drift requires manual repair"}})
	if !strings.Contains(out, "partially completed") || !strings.Contains(out, "Pi drift requires manual repair") {
		t.Fatalf("output = %q, want partial rollback and manual action", out)
	}
}
