package screens

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// makeTestState builds a minimal ModelPickerState with one provider and models
// so that handleModelNav can reach the "enter" branch.
func makeTestState(phaseIdx int) *ModelPickerState {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-alpha", Name: "Alpha Model"},
		{ID: "model-beta", Name: "Beta Model"},
	}
	return &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: phaseIdx,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0, // always pick the first model for simplicity
	}
}

// ─── ModelPickerRows ───────────────────────────────────────────────────────

func TestModelPickerRows_Count(t *testing.T) {
	rows := ModelPickerRows()
	// 1 orchestrator + 1 "Set all" + 9 sub-agents = 11
	want := 11
	if len(rows) != want {
		t.Fatalf("ModelPickerRows() len = %d, want %d; rows = %v", len(rows), want, rows)
	}
}

func TestModelPickerRows_OrchestratorIsFirst(t *testing.T) {
	rows := ModelPickerRows()
	if rows[0] != "sdd-orchestrator" {
		t.Fatalf("ModelPickerRows()[0] = %q, want %q", rows[0], "sdd-orchestrator")
	}
}

func TestModelPickerRows_SetAllIsSecond(t *testing.T) {
	rows := ModelPickerRows()
	if rows[1] != "Set all phases" {
		t.Fatalf("ModelPickerRows()[1] = %q, want %q", rows[1], "Set all phases")
	}
}

func TestModelPickerRows_SubAgentsStartAtIndexTwo(t *testing.T) {
	rows := ModelPickerRows()
	phases := opencode.SDDPhases()
	for i, phase := range phases {
		got := rows[i+2]
		if got != phase {
			t.Errorf("ModelPickerRows()[%d] = %q, want %q", i+2, got, phase)
		}
	}
}

// ─── handleModelNav: orchestrator row (idx 0) ──────────────────────────────

func TestHandleModelNav_OrchestratorRowAssignsOnlyOrchestrator(t *testing.T) {
	state := makeTestState(0) // row 0 = sdd-orchestrator
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}

	// "sdd-orchestrator" key must be set
	orch, ok := updated[SDDOrchestratorPhase]
	if !ok || orch.ProviderID == "" {
		t.Fatalf("expected %q to be assigned, got: %v", SDDOrchestratorPhase, updated)
	}

	// No sub-agent phase must be touched
	for _, phase := range opencode.SDDPhases() {
		if _, exists := updated[phase]; exists {
			t.Errorf("sub-agent phase %q should NOT be assigned when selecting orchestrator row; assignments: %v", phase, updated)
		}
	}
}

func TestHandleModelNav_OrchestratorRow_ModelValues(t *testing.T) {
	state := makeTestState(0)
	assignments := make(map[string]model.ModelAssignment)

	_, updated := handleModelNav("enter", state, assignments)

	orch := updated[SDDOrchestratorPhase]
	if orch.ProviderID != "test-provider" {
		t.Errorf("ProviderID = %q, want %q", orch.ProviderID, "test-provider")
	}
	if orch.ModelID != "model-alpha" {
		t.Errorf("ModelID = %q, want %q", orch.ModelID, "model-alpha")
	}
}

// ─── handleModelNav: "Set all phases" row (idx 1) ──────────────────────────

func TestHandleModelNav_SetAllPhasesRow_SetsOnlySubAgents(t *testing.T) {
	state := makeTestState(1) // row 1 = "Set all phases"
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}

	// All 9 sub-agents must be assigned
	phases := opencode.SDDPhases()
	for _, phase := range phases {
		a, ok := updated[phase]
		if !ok || a.ProviderID == "" {
			t.Errorf("sub-agent phase %q should be assigned; assignments: %v", phase, updated)
		}
	}

	// sdd-orchestrator must NOT be touched by "Set all phases"
	if _, exists := updated[SDDOrchestratorPhase]; exists {
		t.Errorf("sdd-orchestrator should NOT be assigned by 'Set all phases'; assignments: %v", updated)
	}
}

func TestHandleModelNav_SetAllPhasesRow_DoesNotOverwriteExistingOrchestrator(t *testing.T) {
	state := makeTestState(1) // row 1 = "Set all phases"

	// Pre-set orchestrator with a different assignment
	existing := model.ModelAssignment{ProviderID: "existing-provider", ModelID: "existing-model"}
	assignments := map[string]model.ModelAssignment{
		SDDOrchestratorPhase: existing,
	}

	_, updated := handleModelNav("enter", state, assignments)

	// The orchestrator assignment must remain untouched
	orch := updated[SDDOrchestratorPhase]
	if orch.ProviderID != "existing-provider" || orch.ModelID != "existing-model" {
		t.Errorf("orchestrator assignment should be unchanged; got: %v", orch)
	}
}

// ─── handleModelNav: sub-agent rows (idx 2+) ───────────────────────────────

func TestHandleModelNav_SubAgentRow_AssignsCorrectPhase(t *testing.T) {
	phases := opencode.SDDPhases()

	for i, expectedPhase := range phases {
		t.Run(expectedPhase, func(t *testing.T) {
			state := makeTestState(i + 2) // sub-agents start at row idx 2
			assignments := make(map[string]model.ModelAssignment)

			handled, updated := handleModelNav("enter", state, assignments)

			if !handled {
				t.Fatal("handleModelNav should return handled=true on enter")
			}

			// The target phase must be assigned
			a, ok := updated[expectedPhase]
			if !ok || a.ProviderID == "" {
				t.Errorf("phase %q should be assigned; assignments: %v", expectedPhase, updated)
			}

			// Other phases must NOT be assigned
			for _, other := range phases {
				if other == expectedPhase {
					continue
				}
				if _, exists := updated[other]; exists {
					t.Errorf("unrelated phase %q should not be assigned; assignments: %v", other, updated)
				}
			}

			// Orchestrator must NOT be assigned
			if _, exists := updated[SDDOrchestratorPhase]; exists {
				t.Errorf("sdd-orchestrator should not be assigned; assignments: %v", updated)
			}
		})
	}
}

// ─── SDDOrchestratorPhase constant ────────────────────────────────────────

func TestSDDOrchestratorPhaseConstant(t *testing.T) {
	if SDDOrchestratorPhase != "sdd-orchestrator" {
		t.Fatalf("SDDOrchestratorPhase = %q, want %q", SDDOrchestratorPhase, "sdd-orchestrator")
	}
}
