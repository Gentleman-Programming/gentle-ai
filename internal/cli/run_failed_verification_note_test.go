package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/verify"
)

// TestWithPostInstallNotesNamesARunnableRetryCommandOnFailure closes finding 1
// of the install/sync surface audit: "Run repair on failed checks" named a
// command that does not exist anywhere in the CLI dispatcher
// (internal/app/app.go has no `repair` case). The honest retry command for
// the install path is `gentle-ai install --agent <agent>`.
func TestWithPostInstallNotesNamesARunnableRetryCommandOnFailure(t *testing.T) {
	report := verify.Report{Ready: false, FinalNote: verify.VerificationIssuesMessage}
	resolved := planner.ResolvedPlan{Agents: []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}}

	updated := withPostInstallNotes(report, resolved)

	if strings.Contains(updated.FinalNote, "repair") {
		t.Fatalf("FinalNote still names the nonexistent repair command: %q", updated.FinalNote)
	}
	want := "gentle-ai install --agent claude-code,opencode"
	if !strings.Contains(updated.FinalNote, want) {
		t.Fatalf("FinalNote = %q, want it to contain %q", updated.FinalNote, want)
	}
}

// TestWithPostInstallNotesFailureFallsBackWithoutAgents proves that when no
// agents were resolved (nothing to name in a retry command), the generic
// fallback text is left untouched rather than naming an empty agent list.
func TestWithPostInstallNotesFailureFallsBackWithoutAgents(t *testing.T) {
	report := verify.Report{Ready: false, FinalNote: verify.VerificationIssuesMessage}
	resolved := planner.ResolvedPlan{}

	updated := withPostInstallNotes(report, resolved)
	if updated.FinalNote != verify.VerificationIssuesMessage {
		t.Fatalf("FinalNote = %q, want unchanged generic fallback", updated.FinalNote)
	}
}

// TestWithPostInstallNotesDoesNotOverrideACustomizedFailureNote proves the
// failure override is scoped to exactly the generic production failure text
// and never clobbers a FinalNote a test (or another note-builder) constructed
// by hand with different text.
func TestWithPostInstallNotesDoesNotOverrideACustomizedFailureNote(t *testing.T) {
	report := verify.Report{Ready: false, FinalNote: "custom failure text"}
	resolved := planner.ResolvedPlan{Agents: []model.AgentID{model.AgentClaudeCode}}

	updated := withPostInstallNotes(report, resolved)
	if updated.FinalNote != "custom failure text" {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}
