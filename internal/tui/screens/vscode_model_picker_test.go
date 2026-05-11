package screens

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// ── VSCodeModelRows ──────────────────────────────────────────────────────────

func TestVSCodeModelRows_Count(t *testing.T) {
	// 1 orchestrator + 1 "Set all phases" + 10 SDD phases = 12 rows.
	rows := VSCodeModelRows()
	if len(rows) != 12 {
		t.Fatalf("VSCodeModelRows() len = %d, want 12", len(rows))
	}
}

func TestVSCodeModelRows_OrchestratorIsFirst(t *testing.T) {
	rows := VSCodeModelRows()
	if rows[0] != VSCodeOrchestratorPhase {
		t.Fatalf("VSCodeModelRows()[0] = %q, want %q", rows[0], VSCodeOrchestratorPhase)
	}
}

func TestVSCodeModelRows_SetAllPhasesIsSecond(t *testing.T) {
	rows := VSCodeModelRows()
	if rows[1] != "Set all phases" {
		t.Fatalf("VSCodeModelRows()[1] = %q, want %q", rows[1], "Set all phases")
	}
}

func TestVSCodeModelRows_PhaseRowsStart_AtIndex2(t *testing.T) {
	rows := VSCodeModelRows()
	// rows[2] must be a real SDD phase, not orchestrator or Set-all.
	if rows[2] == VSCodeOrchestratorPhase || rows[2] == "Set all phases" {
		t.Fatalf("VSCodeModelRows()[2] = %q, expected a real SDD phase", rows[2])
	}
}

// ── VSCodeOrchestratorPhase constant ────────────────────────────────────────

func TestVSCodeOrchestratorPhase_Value(t *testing.T) {
	if VSCodeOrchestratorPhase != "sdd-orchestrator" {
		t.Fatalf("VSCodeOrchestratorPhase = %q, want %q", VSCodeOrchestratorPhase, "sdd-orchestrator")
	}
}

// ── VSCodeModelPickerOptionCount ─────────────────────────────────────────────

func TestVSCodeModelPickerOptionCount(t *testing.T) {
	// 12 rows + Continue + Back = 14.
	got := VSCodeModelPickerOptionCount()
	if got != 14 {
		t.Fatalf("VSCodeModelPickerOptionCount() = %d, want 14", got)
	}
}

// ── HandleVSCodeModelPickerNav ───────────────────────────────────────────────

func makeVSCodeTestState(selectedPhaseIdx int) VSCodeModelPickerState {
	return VSCodeModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: selectedPhaseIdx,
		Models: []opencode.Model{
			{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4"},
		},
	}
}

// Row 0 (orchestrator) must assign only to the orchestrator key.
func TestHandleVSCodeModelNav_OrchestratorRow_AssignsOnlyOrchestrator(t *testing.T) {
	state := VSCodeModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 0, // orchestrator row
		Models:           []opencode.Model{{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4"}},
	}
	assignments := map[string]model.ModelAssignment{}

	handled, updated := HandleVSCodeModelPickerNav("enter", &state, assignments)
	if !handled {
		t.Fatal("expected handled=true")
	}

	// Orchestrator key must be set.
	orch, ok := updated[VSCodeOrchestratorPhase]
	if !ok {
		t.Fatalf("expected %q to be assigned; assignments: %v", VSCodeOrchestratorPhase, updated)
	}
	if orch.ModelID != "claude-sonnet-4-20250514" {
		t.Errorf("orchestrator ModelID = %q, want %q", orch.ModelID, "claude-sonnet-4-20250514")
	}

	// No SDD phase must be assigned.
	rows := VSCodeModelRows()
	for _, phase := range rows[2:] {
		if _, exists := updated[phase]; exists {
			t.Errorf("phase %q should NOT be assigned when selecting orchestrator row; assignments: %v", phase, updated)
		}
	}
}

// Row 1 ("Set all phases") must assign to 10 sub-agents but NOT the orchestrator.
func TestHandleVSCodeModelNav_SetAllPhasesRow_AssignsPhasesNotOrchestrator(t *testing.T) {
	state := VSCodeModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 1, // "Set all phases" row
		Models:           []opencode.Model{{ID: "gpt-4o", Name: "GPT-4o"}},
	}
	assignments := map[string]model.ModelAssignment{}

	_, updated := HandleVSCodeModelPickerNav("enter", &state, assignments)

	// Orchestrator must NOT be set.
	if _, exists := updated[VSCodeOrchestratorPhase]; exists {
		t.Errorf("orchestrator should NOT be assigned by 'Set all phases'; assignments: %v", updated)
	}

	// All 10 SDD phases must be set.
	rows := VSCodeModelRows()
	for _, phase := range rows[2:] {
		if a, ok := updated[phase]; !ok || a.ModelID != "gpt-4o" {
			t.Errorf("phase %q: ModelID = %q, want %q", phase, a.ModelID, "gpt-4o")
		}
	}
}

// Row 1 "Set all phases" must NOT overwrite a pre-existing orchestrator assignment.
func TestHandleVSCodeModelNav_SetAllPhasesRow_DoesNotOverwriteExistingOrchestrator(t *testing.T) {
	existing := model.ModelAssignment{ProviderID: "github-copilot", ModelID: "claude-sonnet-4-20250514"}
	state := VSCodeModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 1, // "Set all phases"
		Models:           []opencode.Model{{ID: "gpt-4o", Name: "GPT-4o"}},
	}
	assignments := map[string]model.ModelAssignment{
		VSCodeOrchestratorPhase: existing,
	}

	_, updated := HandleVSCodeModelPickerNav("enter", &state, assignments)

	orch := updated[VSCodeOrchestratorPhase]
	if orch != existing {
		t.Errorf("orchestrator assignment should be unchanged; got: %v", orch)
	}
}

// Row 2 (first SDD phase) must assign only to that phase.
func TestHandleVSCodeModelNav_PhaseRow_AssignsOnlyThatPhase(t *testing.T) {
	state := VSCodeModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2, // first SDD phase (sdd-init)
		Models:           []opencode.Model{{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"}},
	}
	assignments := map[string]model.ModelAssignment{}

	_, updated := HandleVSCodeModelPickerNav("enter", &state, assignments)

	// Orchestrator must not be touched.
	if _, exists := updated[VSCodeOrchestratorPhase]; exists {
		t.Errorf("orchestrator should not be assigned; assignments: %v", updated)
	}

	// Exactly one phase must be assigned.
	if len(updated) != 1 {
		t.Errorf("expected 1 assigned phase, got %d; assignments: %v", len(updated), updated)
	}
}
