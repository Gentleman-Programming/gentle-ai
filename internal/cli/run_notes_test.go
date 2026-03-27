package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

func TestWithPostInstallNotesAddsGGANextSteps(t *testing.T) {
	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{OrderedComponents: []model.ComponentID{model.ComponentGGA}}

	updated := withPostInstallNotes(report, resolved)
	if !strings.Contains(updated.FinalNote, "GGA is now installed globally") {
		t.Fatalf("FinalNote missing GGA global install note: %q", updated.FinalNote)
	}
	if !strings.Contains(updated.FinalNote, "gga init") || !strings.Contains(updated.FinalNote, "gga install") {
		t.Fatalf("FinalNote missing GGA repo setup steps: %q", updated.FinalNote)
	}
}

func TestWithPostInstallNotesDoesNotChangeNonGGA(t *testing.T) {
	// Set GOBIN to a directory already in PATH so that withGoInstallPathNote
	// does not append a PATH guidance note for the Engram component.
	t.Setenv("GOBIN", "/usr/local/bin")

	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{OrderedComponents: []model.ComponentID{model.ComponentEngram}}

	updated := withPostInstallNotes(report, resolved)
	if updated.FinalNote != report.FinalNote {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}
