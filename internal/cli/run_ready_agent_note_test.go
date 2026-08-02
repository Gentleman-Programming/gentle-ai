package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/verify"
)

// TestWithPostInstallNotesNamesOnlyTheInstalledRunnableAgents closes fisidj
// finding 4 (organic-dx Phase 3f task 3f.4): an OpenCode-only install must not
// tell the user to "Run `claude` or `opencode`" -- it is the literal last
// line of the first-run experience and must name only what was installed.
func TestWithPostInstallNotesNamesOnlyTheInstalledRunnableAgents(t *testing.T) {
	tests := []struct {
		name       string
		agents     []model.AgentID
		wantClaude bool
		wantOpen   bool
	}{
		{name: "opencode only", agents: []model.AgentID{model.AgentOpenCode}, wantClaude: false, wantOpen: true},
		{name: "claude only", agents: []model.AgentID{model.AgentClaudeCode}, wantClaude: true, wantOpen: false},
		{name: "both", agents: []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}, wantClaude: true, wantOpen: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := verify.Report{Ready: true, FinalNote: verify.ReadyMessage}
			resolved := planner.ResolvedPlan{Agents: tt.agents}

			updated := withPostInstallNotes(report, resolved)
			hasClaude := strings.Contains(updated.FinalNote, "`claude`")
			hasOpenCode := strings.Contains(updated.FinalNote, "`opencode`")
			if hasClaude != tt.wantClaude {
				t.Fatalf("FinalNote names claude = %v, want %v: %q", hasClaude, tt.wantClaude, updated.FinalNote)
			}
			if hasOpenCode != tt.wantOpen {
				t.Fatalf("FinalNote names opencode = %v, want %v: %q", hasOpenCode, tt.wantOpen, updated.FinalNote)
			}
		})
	}
}

// TestWithPostInstallNotesFallsBackWhenNoRunnableAgentSelected proves an
// install that selected neither of the two named runnable agents (e.g. an
// IDE-integrated agent only) gets the generic ready message instead of naming
// an agent that was never installed.
func TestWithPostInstallNotesFallsBackWhenNoRunnableAgentSelected(t *testing.T) {
	report := verify.Report{Ready: true, FinalNote: verify.ReadyMessage}
	resolved := planner.ResolvedPlan{Agents: []model.AgentID{model.AgentAntigravity}}

	updated := withPostInstallNotes(report, resolved)
	if strings.Contains(updated.FinalNote, "claude") || strings.Contains(updated.FinalNote, "opencode") {
		t.Fatalf("FinalNote names an agent that was never installed: %q", updated.FinalNote)
	}
}

// TestWithPostInstallNotesDoesNotOverrideAnAlreadyCustomizedFinalNote proves
// the override is scoped to exactly the generic production ready message and
// never clobbers a FinalNote a test (or another note-builder) constructed by
// hand with different text.
func TestWithPostInstallNotesDoesNotOverrideAnAlreadyCustomizedFinalNote(t *testing.T) {
	// AgentClaudeCode (not OpenCode) so withOpenCodeExperimentalNote has
	// nothing to append -- isolating the ready-agent-run override.
	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{Agents: []model.AgentID{model.AgentClaudeCode}}

	updated := withPostInstallNotes(report, resolved)
	if updated.FinalNote != "You're ready." {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}
