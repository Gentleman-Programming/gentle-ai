package cli

import (
	"os"
	"path/filepath"
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
	// Put both direct-download locations in PATH so the Engram guidance note is skipped.
	home, _ := os.UserHomeDir()
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/usr/local/bin"+string(os.PathListSeparator)+filepath.Join(home, ".local", "bin"))

	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{OrderedComponents: []model.ComponentID{model.ComponentEngram}}

	updated := withPostInstallNotes(report, resolved)
	if updated.FinalNote != report.FinalNote {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}
