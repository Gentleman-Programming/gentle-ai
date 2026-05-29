package screens

import (
	"os"
	"path/filepath"
	"strings"
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
	// 1 orchestrator + 1 "Set all" + 10 sub-agents + 1 separator + 3 JD agents = 16
	want := 16
	if len(rows) != want {
		t.Fatalf("ModelPickerRows() len = %d, want %d; rows = %v", len(rows), want, rows)
	}
}

func TestModelPickerRows_OrchestratorIsFirst(t *testing.T) {
	rows := ModelPickerRows()
	if rows[0] != "gentle-orchestrator" {
		t.Fatalf("ModelPickerRows()[0] = %q, want %q", rows[0], "gentle-orchestrator")
	}
}

func TestModelPickerRows_SetAllIsSecond(t *testing.T) {
	rows := ModelPickerRows()
	if rows[1] != "Set all SDD phases" {
		t.Fatalf("ModelPickerRows()[1] = %q, want %q", rows[1], "Set all SDD phases")
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
	state := makeTestState(0) // row 0 = gentle-orchestrator
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}

	// "gentle-orchestrator" key must be set
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

	// All 10 sub-agents must be assigned
	phases := opencode.SDDPhases()
	for _, phase := range phases {
		a, ok := updated[phase]
		if !ok || a.ProviderID == "" {
			t.Errorf("sub-agent phase %q should be assigned; assignments: %v", phase, updated)
		}
	}

	// gentle-orchestrator must NOT be touched by "Set all phases"
	if _, exists := updated[SDDOrchestratorPhase]; exists {
		t.Errorf("gentle-orchestrator should NOT be assigned by 'Set all phases'; assignments: %v", updated)
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
				t.Errorf("gentle-orchestrator should not be assigned; assignments: %v", updated)
			}
		})
	}
}

// ─── SDDOrchestratorPhase constant ────────────────────────────────────────

func TestSDDOrchestratorPhaseConstant(t *testing.T) {
	if SDDOrchestratorPhase != "gentle-orchestrator" {
		t.Fatalf("SDDOrchestratorPhase = %q, want %q", SDDOrchestratorPhase, "gentle-orchestrator")
	}
}

func TestRenderModelPickerShowsConfigWarning(t *testing.T) {
	output := RenderModelPicker(nil, ModelPickerState{ConfigWarning: "invalid opencode.json"}, 0)
	if !strings.Contains(output, "invalid opencode.json") {
		t.Fatalf("RenderModelPicker() missing config warning; got:\n%s", output)
	}
}

func TestFilteredModelEntriesSortsNewestFirst(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-3-5-sonnet", Name: "Claude 3.5 Sonnet"},
				{ID: "claude-opus-4-6", Name: "Claude Opus 4.6"},
				{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
			},
		},
	}

	models := FilteredModelEntries(state)
	got := []string{models[0].ID, models[1].ID, models[2].ID}
	want := []string{"claude-opus-4-6", "claude-sonnet-4-5", "claude-3-5-sonnet"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted models = %v, want %v", got, want)
		}
	}
}

func TestFilteredModelEntriesPrefersSemanticVersionOverDate(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet 20241022"},
				{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
			},
		},
	}

	models := FilteredModelEntries(state)
	if models[0].ID != "claude-sonnet-4-5" {
		t.Fatalf("first model = %q, want claude-sonnet-4-5", models[0].ID)
	}
}

func TestFilteredModelEntriesIgnoresShortDateSuffixes(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "google",
		SDDModels: map[string][]opencode.Model{
			"google": {
				{ID: "gemini-2.5-pro-exp-03-25", Name: "Gemini 2.5 Pro Experimental 03-25"},
				{ID: "gemini-3-pro", Name: "Gemini 3 Pro"},
			},
		},
	}

	models := FilteredModelEntries(state)
	if models[0].ID != "gemini-3-pro" {
		t.Fatalf("first model = %q, want gemini-3-pro", models[0].ID)
	}
}

func TestFilteredModelEntriesFiltersBySearch(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "openai",
		ModelSearch:      "mini",
		SDDModels: map[string][]opencode.Model{
			"openai": {
				{ID: "gpt-5", Name: "GPT-5"},
				{ID: "gpt-5-mini", Name: "GPT-5 Mini"},
			},
		},
	}

	models := FilteredModelEntries(state)
	if len(models) != 1 || models[0].ID != "gpt-5-mini" {
		t.Fatalf("filtered models = %#v, want only gpt-5-mini", models)
	}
}

func TestHandleModelNavTypingUpdatesSearchAndBackspaceClears(t *testing.T) {
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "openai",
		SDDModels: map[string][]opencode.Model{
			"openai": {{ID: "gpt-5-mini", Name: "GPT-5 Mini"}},
		},
	}

	handled, _ := handleModelNav("m", state, nil)
	if !handled || state.ModelSearch != "m" {
		t.Fatalf("typing search handled=%v query=%q, want handled=true query=m", handled, state.ModelSearch)
	}

	handled, _ = handleModelNav("backspace", state, nil)
	if !handled || state.ModelSearch != "" {
		t.Fatalf("backspace handled=%v query=%q, want handled=true empty query", handled, state.ModelSearch)
	}
}

func TestRenderModelSelectShowsSearchInput(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "openai",
		ModelSearch:      "mini",
		Providers: map[string]opencode.Provider{
			"openai": {Name: "OpenAI"},
		},
		SDDModels: map[string][]opencode.Model{
			"openai": {{ID: "gpt-5-mini", Name: "GPT-5 Mini"}},
		},
	}

	out := renderModelSelect(state)
	if !strings.Contains(out, "Search: mini_") {
		t.Fatalf("renderModelSelect() missing search input; got:\n%s", out)
	}
}

func TestRenderModelPickerShowsSetAllPhasesEffort(t *testing.T) {
	state := ModelPickerState{
		AvailableIDs: []string{"anthropic"},
		Providers: map[string]opencode.Provider{
			"anthropic": {
				Name: "Anthropic",
				Models: map[string]opencode.Model{
					"claude-opus-4": {Name: "Claude Opus 4"},
				},
			},
		},
		AllPhasesModel: model.ModelAssignment{
			ProviderID: "anthropic",
			ModelID:    "claude-opus-4",
			Effort:     "high",
		},
	}

	output := RenderModelPicker(nil, state, 1)
	if !strings.Contains(output, "Set all SDD phases") || !strings.Contains(output, "Anthropic / Claude Opus 4 [high]") {
		t.Fatalf("RenderModelPicker() missing Set all phases effort label; got:\n%s", output)
	}
}

// ─── Issue #146: "Set all phases" label must not change when individual phase selected ─

// TestSetAllPhasesLabelSeparateFromIndividualPhases verifies that the ModelPickerState
// has a dedicated AllPhasesModel field that only gets updated when "Set all phases"
// is selected (row idx 1), NOT when an individual sub-agent phase (idx >= 2) is selected.
//
// The "Set all phases" row label should show AllPhasesModel, not phases[0].
//
// Closes #146.
func TestSetAllPhasesLabelSeparateFromIndividualPhases(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-alpha", Name: "Alpha"},
		{ID: "model-beta", Name: "Beta"},
	}

	// Step 1: "Set all phases" — AllPhasesModel should be set to alpha.
	setAllState := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 1, // "Set all phases" row
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0, // alpha
	}
	assignments := make(map[string]model.ModelAssignment)
	_, assignments = handleModelNav("enter", setAllState, assignments)

	// AllPhasesModel must record the "Set all" assignment.
	if setAllState.AllPhasesModel.ModelID != "model-alpha" {
		t.Fatalf("after Set all: AllPhasesModel.ModelID = %q, want model-alpha", setAllState.AllPhasesModel.ModelID)
	}

	// Step 2: Select an individual sub-agent phase (idx 2 = phases[0]).
	// This is the tricky case: selecting the FIRST sub-agent should NOT change AllPhasesModel.
	individualState := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2, // sub-agent row idx 2 → phases[0]
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      1, // beta — different from what "Set all" used
	}
	_, assignments = handleModelNav("enter", individualState, assignments)

	// AllPhasesModel must NOT be changed by individual phase selection.
	if individualState.AllPhasesModel.ModelID != "" {
		t.Errorf("individual selection changed AllPhasesModel to %q, want empty — bug: 'Set all phases' label would be wrong",
			individualState.AllPhasesModel.ModelID)
	}
}

// TestSetAllPhasesSetsAllPhasesModelField verifies that selecting "Set all phases"
// sets AllPhasesModel on the state to the chosen model assignment.
//
// Closes #146.
func TestSetAllPhasesSetsAllPhasesModelField(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-alpha", Name: "Alpha"},
	}

	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 1, // "Set all phases"
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	assignments := make(map[string]model.ModelAssignment)
	_, _ = handleModelNav("enter", state, assignments)

	if state.AllPhasesModel.ProviderID != providerID {
		t.Errorf("AllPhasesModel.ProviderID = %q, want %q", state.AllPhasesModel.ProviderID, providerID)
	}
	if state.AllPhasesModel.ModelID != "model-alpha" {
		t.Errorf("AllPhasesModel.ModelID = %q, want model-alpha", state.AllPhasesModel.ModelID)
	}
}

// ─── ModeEffortSelect constant ────────────────────────────────────────────

func TestModeEffortSelectConstantValue(t *testing.T) {
	// ModeEffortSelect must be 3 (the 4th constant after 0, 1, 2).
	if ModeEffortSelect != 3 {
		t.Fatalf("ModeEffortSelect = %d, want 3", ModeEffortSelect)
	}
}

// ─── makeTestStateReasoning helper ────────────────────────────────────────

// makeTestStateReasoning is like makeTestState but includes a reasoning model.
func makeTestStateReasoning(phaseIdx int) *ModelPickerState {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-reason", Name: "Reasoning Model", Reasoning: true, ToolCall: true, Variants: []string{"high", "low", "medium"}},
		{ID: "model-plain", Name: "Plain Model", Reasoning: false, ToolCall: true},
	}
	return &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: phaseIdx,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0, // reasoning model is first
	}
}

// ─── handleModelNav: reasoning model triggers ModeEffortSelect ────────────

func TestHandleModelNav_ReasoningModelSetsModeEffortSelect(t *testing.T) {
	state := makeTestStateReasoning(2) // any sub-agent row
	assignments := make(map[string]model.ModelAssignment)

	handled, _ := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}
	if state.Mode != ModeEffortSelect {
		t.Errorf("Mode after selecting reasoning model = %v, want ModeEffortSelect (%d)", state.Mode, ModeEffortSelect)
	}
	// PendingAssignment must be populated with provider + model
	if state.PendingAssignment.ProviderID == "" {
		t.Error("PendingAssignment.ProviderID should be set after selecting reasoning model")
	}
	if state.PendingAssignment.ModelID != "model-reason" {
		t.Errorf("PendingAssignment.ModelID = %q, want %q", state.PendingAssignment.ModelID, "model-reason")
	}
}

func TestHandleModelNav_NonReasoningModelSkipsEffortPicker(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-plain", Name: "Plain Model", Reasoning: false, ToolCall: true},
	}
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}
	if state.Mode != ModePhaseList {
		t.Errorf("Mode after non-reasoning model = %v, want ModePhaseList (%d)", state.Mode, ModePhaseList)
	}
	// Effort must be empty on the assignment
	phases := opencode.SDDPhases()
	phase := phases[0] // phaseIdx 2 = phases[0]
	a := updated[phase]
	if a.Effort != "" {
		t.Errorf("non-reasoning model assignment Effort = %q, want empty string", a.Effort)
	}
}

// TestHandleModelNav_ReasoningModelWithoutVariantsSkipsEffortPicker covers the
// realistic scenario where the model-variants plugin has not run yet (or failed
// silently): a reasoning-capable model is loaded from the cache but its
// Variants field is nil because EnrichWithVariants found no JSON. The picker
// must skip ModeEffortSelect instead of presenting an empty list.
func TestHandleModelNav_ReasoningModelWithoutVariantsSkipsEffortPicker(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		// Reasoning: true but Variants: nil — plugin cache absent.
		{ID: "model-reason", Name: "Reasoning Model", Reasoning: true, ToolCall: true},
	}
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter")
	}
	if state.Mode != ModePhaseList {
		t.Errorf("Mode after reasoning model without variants = %v, want ModePhaseList (%d)", state.Mode, ModePhaseList)
	}
	phase := opencode.SDDPhases()[0]
	a := updated[phase]
	if a.ProviderID != providerID || a.ModelID != "model-reason" {
		t.Errorf("assignment = %+v, want provider=%q model=%q", a, providerID, "model-reason")
	}
	if a.Effort != "" {
		t.Errorf("Effort = %q, want empty (no variants available)", a.Effort)
	}
}

func TestHandleModelNav_ReasoningModelWithoutVariantsPreservesExistingEffortForSameIndividualPhase(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-reason", Name: "Reasoning Model", Reasoning: true, ToolCall: true},
	}
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	phase := opencode.SDDPhases()[0]
	assignments := map[string]model.ModelAssignment{
		phase: {ProviderID: providerID, ModelID: "model-reason", Effort: "high"},
	}

	_, updated := handleModelNav("enter", state, assignments)

	if got := updated[phase].Effort; got != "high" {
		t.Errorf("Effort after reselecting same model with unknown variants = %q, want preserved %q", got, "high")
	}
}

func TestHandleModelNav_NonReasoningModelClearsExistingEffortForSameIndividualPhase(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-plain", Name: "Plain Model", Reasoning: false, ToolCall: true},
	}
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 2,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	phase := opencode.SDDPhases()[0]
	assignments := map[string]model.ModelAssignment{
		phase: {ProviderID: providerID, ModelID: "model-plain", Effort: "high"},
	}

	_, updated := handleModelNav("enter", state, assignments)

	if got := updated[phase].Effort; got != "" {
		t.Errorf("Effort after reselecting known non-reasoning model = %q, want empty", got)
	}
}

func TestHandleModelNav_SetAllPhasesWithoutVariantsPreservesMatchingExistingEfforts(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-reason", Name: "Reasoning Model", Reasoning: true, ToolCall: true},
	}
	state := &ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedPhaseIdx: 1,
		SelectedProvider: providerID,
		SDDModels:        map[string][]opencode.Model{providerID: testModels},
		ModelCursor:      0,
	}
	phases := opencode.SDDPhases()
	assignments := map[string]model.ModelAssignment{
		SDDOrchestratorPhase: {ProviderID: providerID, ModelID: "model-reason", Effort: "high"},
		phases[0]:            {ProviderID: providerID, ModelID: "model-reason", Effort: "high"},
		phases[1]:            {ProviderID: providerID, ModelID: "other-model", Effort: "medium"},
	}

	_, updated := handleModelNav("enter", state, assignments)

	if got := updated[phases[0]].Effort; got != "high" {
		t.Errorf("matching phase effort = %q, want preserved %q", got, "high")
	}
	if got := updated[phases[1]].Effort; got != "" {
		t.Errorf("non-matching phase effort = %q, want empty", got)
	}
	if got := updated[SDDOrchestratorPhase].Effort; got != "high" {
		t.Errorf("orchestrator effort = %q, want untouched %q", got, "high")
	}
}

// ─── applyAssignment helper ──────────────────────────────────────────────

func TestApplyAssignment_SinglePhase(t *testing.T) {
	phases := opencode.SDDPhases()
	state := ModelPickerState{SelectedPhaseIdx: 2} // phases[0]
	assignments := make(map[string]model.ModelAssignment)
	assignment := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "high"}

	updated := applyAssignment(state, assignments, assignment)

	// Only phases[0] should be set
	if updated[phases[0]].Effort != "high" {
		t.Errorf("phases[0] Effort = %q, want %q", updated[phases[0]].Effort, "high")
	}
	// Others should not be set
	for _, phase := range phases[1:] {
		if _, ok := updated[phase]; ok {
			t.Errorf("phase %q should not be set in single-phase apply", phase)
		}
	}
}

func TestApplyAssignment_AllPhases(t *testing.T) {
	phases := opencode.SDDPhases()
	state := ModelPickerState{SelectedPhaseIdx: 1} // "Set all phases"
	assignments := make(map[string]model.ModelAssignment)
	assignment := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "low"}

	updated := applyAssignment(state, assignments, assignment)

	for _, phase := range phases {
		a := updated[phase]
		if a.Effort != "low" {
			t.Errorf("phase %q Effort = %q, want %q", phase, a.Effort, "low")
		}
	}
	// Orchestrator must NOT be touched
	if _, ok := updated[SDDOrchestratorPhase]; ok {
		t.Error("applyAssignment with SelectedPhaseIdx==1 should not set orchestrator")
	}
}

// ─── handleEffortNav ──────────────────────────────────────────────────────

func TestHandleEffortNav_EnterAppliesEffortAndReturnsModePhaseList(t *testing.T) {
	phases := opencode.SDDPhases()
	state := ModelPickerState{
		Mode:                      ModeEffortSelect,
		SelectedPhaseIdx:          2, // phases[0]
		EffortCursor:              1, // second option = "low" (after "default" at index 0)
		PendingAssignment:         model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4"},
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	newState, updated := handleEffortNav("enter", state, assignments)

	if newState.Mode != ModePhaseList {
		t.Errorf("Mode after effort selection = %v, want ModePhaseList", newState.Mode)
	}
	// EffortCursor 1 -> "low" (options: ["default", "low", "medium", "high"])
	a := updated[phases[0]]
	if a.Effort != "low" {
		t.Errorf("assignment Effort = %q, want %q", a.Effort, "low")
	}
	if newState.PendingAssignment != (model.ModelAssignment{}) {
		t.Errorf("PendingAssignment after effort selection = %+v, want zero value", newState.PendingAssignment)
	}
	if newState.SelectedModelEffortLevels != nil {
		t.Errorf("SelectedModelEffortLevels after effort selection = %v, want nil", newState.SelectedModelEffortLevels)
	}
}

func TestHandleEffortNav_DefaultOptionMapsToEmptyEffort(t *testing.T) {
	phases := opencode.SDDPhases()
	state := ModelPickerState{
		Mode:                      ModeEffortSelect,
		SelectedPhaseIdx:          2, // phases[0]
		EffortCursor:              0, // "default" option
		PendingAssignment:         model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4"},
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	_, updated := handleEffortNav("enter", state, assignments)

	a := updated[phases[0]]
	if a.Effort != "" {
		t.Errorf("'default' option should yield Effort=\"\", got %q", a.Effort)
	}
}

func TestHandleEffortNav_EscReturnsModeModelSelect(t *testing.T) {
	state := ModelPickerState{
		Mode:                      ModeEffortSelect,
		SelectedPhaseIdx:          2,
		PendingAssignment:         model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4"},
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	newState, _ := handleEffortNav("esc", state, assignments)

	if newState.Mode != ModeModelSelect {
		t.Errorf("Mode after esc = %v, want ModeModelSelect", newState.Mode)
	}
	if newState.PendingAssignment != (model.ModelAssignment{}) {
		t.Errorf("PendingAssignment after esc = %+v, want zero value", newState.PendingAssignment)
	}
	if newState.SelectedModelEffortLevels != nil {
		t.Errorf("SelectedModelEffortLevels after esc = %v, want nil", newState.SelectedModelEffortLevels)
	}
}

func TestHandleEffortNav_NavigationUpdatesEffortCursor(t *testing.T) {
	// options: ["default", "low", "medium", "high"] — 4 items
	state := ModelPickerState{
		Mode:                      ModeEffortSelect,
		EffortCursor:              0,
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	newState, _ := handleEffortNav("j", state, assignments)
	if newState.EffortCursor != 1 {
		t.Errorf("after j: EffortCursor = %d, want 1", newState.EffortCursor)
	}

	newState, _ = handleEffortNav("k", newState, assignments)
	if newState.EffortCursor != 0 {
		t.Errorf("after k: EffortCursor = %d, want 0", newState.EffortCursor)
	}
}

// ─── HandleModelPickerNav dispatches ModeEffortSelect ─────────────────────

func TestHandleModelPickerNav_DispatchesToEffortNav(t *testing.T) {
	phases := opencode.SDDPhases()
	state := &ModelPickerState{
		Mode:                      ModeEffortSelect,
		SelectedPhaseIdx:          2,
		EffortCursor:              2, // "medium"
		PendingAssignment:         model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4"},
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := HandleModelPickerNav("enter", state, assignments)

	if !handled {
		t.Fatal("HandleModelPickerNav should handle enter in ModeEffortSelect")
	}
	a := updated[phases[0]]
	if a.Effort != "medium" {
		t.Errorf("Effort = %q, want %q", a.Effort, "medium")
	}
}

// ─── handleEffortNav: "Set all phases" row (SelectedPhaseIdx==1) ──────────────

// TestHandleEffortNav_SetAllPhasesUpdatesAllPhasesModelAndAllSubAgents verifies
// that when the effort picker is confirmed via the "Set all phases" row
// (SelectedPhaseIdx==1), ALL 10 SDD sub-agent phases receive the effort assignment
// AND state.AllPhasesModel is updated to reflect the chosen effort.
//
// This covers the interaction between the effort picker and the "Set all phases"
// special row — a path not exercised by the single-phase tests above.
func TestHandleEffortNav_SetAllPhasesUpdatesAllPhasesModelAndAllSubAgents(t *testing.T) {
	phases := opencode.SDDPhases()
	pending := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4"}
	state := ModelPickerState{
		Mode:                      ModeEffortSelect,
		SelectedPhaseIdx:          1, // "Set all phases" row
		EffortCursor:              2, // index 2 → "medium" in ["default", "low", "medium", "high"]
		PendingAssignment:         pending,
		SelectedModelEffortLevels: []string{"low", "medium", "high"},
	}
	assignments := make(map[string]model.ModelAssignment)

	newState, updated := handleEffortNav("enter", state, assignments)

	// All 10 sub-agent phases must carry the effort.
	for _, phase := range phases {
		a, ok := updated[phase]
		if !ok {
			t.Errorf("phase %q missing from assignments after Set all phases effort", phase)
			continue
		}
		if a.Effort != "medium" {
			t.Errorf("phase %q Effort = %q, want %q", phase, a.Effort, "medium")
		}
	}

	// AllPhasesModel must be updated with the full assignment including effort.
	if newState.AllPhasesModel.Effort != "medium" {
		t.Errorf("AllPhasesModel.Effort = %q, want %q", newState.AllPhasesModel.Effort, "medium")
	}
	if newState.AllPhasesModel.ProviderID != pending.ProviderID {
		t.Errorf("AllPhasesModel.ProviderID = %q, want %q", newState.AllPhasesModel.ProviderID, pending.ProviderID)
	}

	// PendingAssignment must be cleared after confirmation.
	if newState.PendingAssignment != (model.ModelAssignment{}) {
		t.Errorf("PendingAssignment after Set all effort = %+v, want zero value", newState.PendingAssignment)
	}

	// gentle-orchestrator must NOT be touched by "Set all phases".
	if _, exists := updated[SDDOrchestratorPhase]; exists {
		t.Errorf("gentle-orchestrator should NOT be assigned by Set all phases effort")
	}
}

// ─── TestIndividualPhaseSelectionDoesNotSetAllPhasesModel (unchanged) ──────

// ─── Phase list display — effort annotation ───────────────────────────────

func TestRenderPhaseList_EffortAnnotation(t *testing.T) {
	const providerID = "test-provider"
	state := ModelPickerState{
		Providers: map[string]opencode.Provider{
			providerID: {ID: providerID, Name: "TestProv", Models: map[string]opencode.Model{
				"model-x": {ID: "model-x", Name: "Model X"},
			}},
		},
		AvailableIDs: []string{providerID},
		SDDModels: map[string][]opencode.Model{
			providerID: {{ID: "model-x", Name: "Model X"}},
		},
		Mode: ModePhaseList,
	}
	phases := opencode.SDDPhases()
	assignments := map[string]model.ModelAssignment{
		phases[0]: {ProviderID: providerID, ModelID: "model-x", Effort: "high"},
		phases[1]: {ProviderID: providerID, ModelID: "model-x", Effort: ""},
	}

	rendered := RenderModelPicker(assignments, state, 0)

	// Row for phases[0] must contain "[high]"
	if !strings.Contains(rendered, "[high]") {
		t.Errorf("rendered phase list should contain '[high]' for assignment with Effort=high; got:\n%s", rendered)
	}
}

func TestRenderPhaseList_OrchestratorEffortAnnotation(t *testing.T) {
	const providerID = "test-provider"
	state := ModelPickerState{
		Providers: map[string]opencode.Provider{
			providerID: {ID: providerID, Name: "TestProv", Models: map[string]opencode.Model{
				"model-x": {ID: "model-x", Name: "Model X"},
			}},
		},
		AvailableIDs: []string{providerID},
		SDDModels: map[string][]opencode.Model{
			providerID: {{ID: "model-x", Name: "Model X"}},
		},
		Mode: ModePhaseList,
	}
	assignments := map[string]model.ModelAssignment{
		SDDOrchestratorPhase: {ProviderID: providerID, ModelID: "model-x", Effort: "high"},
	}

	rendered := RenderModelPicker(assignments, state, 0)

	if !strings.Contains(rendered, "[high]") {
		t.Errorf("orchestrator row should contain '[high]' when Effort is set; got:\n%s", rendered)
	}
}

func TestRenderPhaseList_NoEffortAnnotationWhenEmpty(t *testing.T) {
	const providerID = "test-provider"
	state := ModelPickerState{
		Providers: map[string]opencode.Provider{
			providerID: {ID: providerID, Name: "TestProv", Models: map[string]opencode.Model{
				"model-x": {ID: "model-x", Name: "Model X"},
			}},
		},
		AvailableIDs: []string{providerID},
		SDDModels: map[string][]opencode.Model{
			providerID: {{ID: "model-x", Name: "Model X"}},
		},
		Mode: ModePhaseList,
	}
	phases := opencode.SDDPhases()
	assignments := map[string]model.ModelAssignment{
		phases[0]: {ProviderID: providerID, ModelID: "model-x", Effort: ""},
	}

	rendered := RenderModelPicker(assignments, state, 0)

	// No "[" bracket annotation should appear in the phase rows when Effort is empty
	if strings.Contains(rendered, "[high]") || strings.Contains(rendered, "[low]") || strings.Contains(rendered, "[medium]") {
		t.Errorf("rendered phase list should not contain effort bracket when Effort is empty; got:\n%s", rendered)
	}
}

// ─── TestIndividualPhaseSelectionDoesNotSetAllPhasesModel (unchanged) ──────

// TestIndividualPhaseSelectionDoesNotSetAllPhasesModel verifies that selecting
// a model for any individual sub-agent phase does NOT update AllPhasesModel.
//
// Closes #146.
func TestIndividualPhaseSelectionDoesNotSetAllPhasesModel(t *testing.T) {
	const providerID = "test-provider"
	testModels := []opencode.Model{
		{ID: "model-alpha", Name: "Alpha"},
	}
	phases := opencode.SDDPhases()

	for i, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			state := &ModelPickerState{
				Mode:             ModeModelSelect,
				SelectedPhaseIdx: i + 2, // sub-agent rows start at idx 2
				SelectedProvider: providerID,
				SDDModels:        map[string][]opencode.Model{providerID: testModels},
				ModelCursor:      0,
			}
			assignments := make(map[string]model.ModelAssignment)
			_, _ = handleModelNav("enter", state, assignments)

			if state.AllPhasesModel.ProviderID != "" || state.AllPhasesModel.ModelID != "" {
				t.Errorf("individual selection of phase %q set AllPhasesModel to %+v, want zero value",
					phase, state.AllPhasesModel)
			}
		})
	}
}

// ─── NewModelPickerState: custom provider merging ─────────────────────────────

// catalogJSON is a minimal OpenCode models cache with one built-in provider.
const catalogJSON = `{
  "built-in": {
    "id": "built-in",
    "name": "Built-In Provider",
    "env": ["BUILTIN_API_KEY"],
    "models": {
      "builtin-model": {
        "id": "builtin-model",
        "name": "Built-In Model",
        "tool_call": true
      }
    }
  }
}`

// writeTempFile writes content to a file in a temp dir and returns the path.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file %q: %v", path, err)
	}
	return path
}

func TestNewModelPickerState(t *testing.T) {
	tests := []struct {
		name              string
		cacheContent      string   // non-empty means write a cache file; empty means skip
		settingsContent   string   // non-empty means write settings file; empty means use missing path
		wantProviderIDs   []string // provider IDs that must appear in Providers map
		wantAvailable     int      // minimum number of AvailableIDs (custom providers always count)
		wantConfigWarning bool     // whether ConfigWarning must be non-empty
	}{
		{
			name:              "missing opencode.json falls back to catalog only",
			cacheContent:      catalogJSON,
			settingsContent:   "", // no file written → path points to nonexistent file
			wantProviderIDs:   []string{"built-in"},
			wantAvailable:     0, // no env var set → built-in not available; just checking providers map
			wantConfigWarning: false,
		},
		{
			name:              "opencode.json with no provider key gives catalog only",
			cacheContent:      catalogJSON,
			settingsContent:   `{"agent": {}}`,
			wantProviderIDs:   []string{"built-in"},
			wantAvailable:     0,
			wantConfigWarning: false,
		},
		{
			name:         "opencode.json with 2 custom providers adds both to picker",
			cacheContent: catalogJSON,
			settingsContent: `{
				"provider": {
					"custom-a": {
						"name": "Custom A",
						"models": {"model-a1": {"name": "Model A1", "tool_call": true}}
					},
					"custom-b": {
						"name": "Custom B",
						"models": {"model-b1": {"name": "Model B1", "tool_call": true}}
					}
				}
			}`,
			wantProviderIDs:   []string{"built-in", "custom-a", "custom-b"},
			wantAvailable:     2, // custom-a and custom-b are always available as custom providers
			wantConfigWarning: false,
		},
		{
			name:         "name collision: custom provider wins over catalog",
			cacheContent: catalogJSON,
			settingsContent: `{
				"provider": {
					"built-in": {
						"name": "My Override",
						"models": {
							"builtin-model": {"name": "Custom Override Name", "tool_call": true}
						}
					}
				}
			}`,
			wantProviderIDs:   []string{"built-in"},
			wantAvailable:     1, // "built-in" now treated as custom → always available
			wantConfigWarning: false,
		},
		{
			name:              "malformed opencode.json produces config warning",
			cacheContent:      catalogJSON,
			settingsContent:   `{"provider":`, // truncated / invalid JSON
			wantProviderIDs:   []string{"built-in"},
			wantAvailable:     0,
			wantConfigWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write the cache file.
			cachePath := writeTempFile(t, "models.json", tt.cacheContent)

			// Determine settings path — missing when settingsContent is empty.
			var settingsPath string
			if tt.settingsContent != "" {
				settingsPath = writeTempFile(t, "opencode.json", tt.settingsContent)
			} else {
				settingsPath = filepath.Join(t.TempDir(), "nonexistent.json")
			}

			state := NewModelPickerState(cachePath, settingsPath)

			// All expected provider IDs must appear in the Providers map.
			for _, id := range tt.wantProviderIDs {
				if _, ok := state.Providers[id]; !ok {
					t.Errorf("Providers missing %q; got keys: %v", id, providerKeys(state.Providers))
				}
			}

			// AvailableIDs count must meet the minimum.
			if len(state.AvailableIDs) < tt.wantAvailable {
				t.Errorf("AvailableIDs = %v (count %d), want at least %d",
					state.AvailableIDs, len(state.AvailableIDs), tt.wantAvailable)
			}

			// ConfigWarning check.
			if tt.wantConfigWarning && state.ConfigWarning == "" {
				t.Error("expected ConfigWarning to be set, got empty string")
			}
			if !tt.wantConfigWarning && state.ConfigWarning != "" {
				t.Errorf("expected no ConfigWarning, got %q", state.ConfigWarning)
			}
		})
	}
}

// TestNewModelPickerStateCollisionCustomWins verifies that when a model ID exists
// in both the catalog cache and opencode.json, the custom entry takes precedence.
func TestNewModelPickerStateCollisionCustomWins(t *testing.T) {
	cachePath := writeTempFile(t, "models.json", catalogJSON)
	settingsPath := writeTempFile(t, "opencode.json", `{
		"provider": {
			"built-in": {
				"name": "Built-In Provider",
				"models": {
					"builtin-model": {"name": "Custom Override Name", "tool_call": true}
				}
			}
		}
	}`)

	state := NewModelPickerState(cachePath, settingsPath)

	p, ok := state.Providers["built-in"]
	if !ok {
		t.Fatal("expected built-in provider in state")
	}
	m, ok := p.Models["builtin-model"]
	if !ok {
		t.Fatal("expected builtin-model in built-in provider")
	}
	if m.Name != "Custom Override Name" {
		t.Errorf("model name = %q, want %q (custom should win on collision)", m.Name, "Custom Override Name")
	}
}

// providerKeys returns the keys of a Provider map for test error messages.
func providerKeys(providers map[string]opencode.Provider) []string {
	keys := make([]string, 0, len(providers))
	for k := range providers {
		keys = append(keys, k)
	}
	return keys
}

// ─── Separator row (non-selectable) ────────────────────────────────────────

func TestSeparatorRowIdx_Value(t *testing.T) {
	got := SeparatorRowIdx()
	want := 2 + len(opencode.SDDPhases()) // after orchestrator + "Set all" + 9 SDD phases
	if got != want {
		t.Fatalf("SeparatorRowIdx() = %d, want %d", got, want)
	}
}

func TestHandleModelNav_SeparatorRow_NoAssignment(t *testing.T) {
	sepIdx := SeparatorRowIdx()
	state := makeTestState(sepIdx)
	assignments := make(map[string]model.ModelAssignment)

	handled, updated := handleModelNav("enter", state, assignments)

	if !handled {
		t.Fatal("handleModelNav should return handled=true on enter for separator")
	}

	// Separator should produce NO assignments at all.
	if len(updated) != 0 {
		t.Fatalf("separator row should produce no assignments; got: %v", updated)
	}

	// State should return to phase list.
	if state.Mode != ModePhaseList {
		t.Fatalf("expected ModePhaseList after separator enter, got %d", state.Mode)
	}
}

// ─── JD agent rows ─────────────────────────────────────────────────────────

func TestHandleModelNav_JDAgentRows_AssignCorrectly(t *testing.T) {
	jdPhases := opencode.JDPhases()
	sepIdx := SeparatorRowIdx()

	for i, expectedPhase := range jdPhases {
		t.Run(expectedPhase, func(t *testing.T) {
			state := makeTestState(sepIdx + 1 + i) // JD rows start after separator
			assignments := make(map[string]model.ModelAssignment)

			handled, updated := handleModelNav("enter", state, assignments)

			if !handled {
				t.Fatal("handleModelNav should return handled=true on enter")
			}

			// The target JD phase must be assigned.
			a, ok := updated[expectedPhase]
			if !ok || a.ProviderID == "" {
				t.Errorf("JD phase %q should be assigned; assignments: %v", expectedPhase, updated)
			}
			if a.ProviderID != "test-provider" {
				t.Errorf("JD phase %q ProviderID = %q, want %q", expectedPhase, a.ProviderID, "test-provider")
			}
			if a.ModelID != "model-alpha" {
				t.Errorf("JD phase %q ModelID = %q, want %q", expectedPhase, a.ModelID, "model-alpha")
			}

			// No other JD phase must be assigned.
			for _, other := range jdPhases {
				if other == expectedPhase {
					continue
				}
				if _, exists := updated[other]; exists {
					t.Errorf("unrelated JD phase %q should not be assigned; assignments: %v", other, updated)
				}
			}

			// No SDD phase or orchestrator must be assigned.
			for _, sdd := range opencode.SDDPhases() {
				if _, exists := updated[sdd]; exists {
					t.Errorf("SDD phase %q should not be assigned by JD row; assignments: %v", sdd, updated)
				}
			}
			if _, exists := updated[SDDOrchestratorPhase]; exists {
				t.Errorf("orchestrator should not be assigned by JD row; assignments: %v", updated)
			}
		})
	}
}

func TestHandleModelNav_JDFirstRow(t *testing.T) {
	// Verify the FIRST JD row (right after separator) maps to jd-judge-a.
	jdPhases := opencode.JDPhases()
	if len(jdPhases) == 0 {
		t.Skip("no JD phases defined")
	}
	sepIdx := SeparatorRowIdx()
	state := makeTestState(sepIdx + 1)
	assignments := make(map[string]model.ModelAssignment)

	_, updated := handleModelNav("enter", state, assignments)

	if _, ok := updated[jdPhases[0]]; !ok {
		t.Fatalf("first JD row should assign %q; got: %v", jdPhases[0], updated)
	}
}

func TestHandleModelNav_JDLastRow(t *testing.T) {
	// Verify the LAST JD row maps to the last JD phase.
	jdPhases := opencode.JDPhases()
	if len(jdPhases) == 0 {
		t.Skip("no JD phases defined")
	}
	sepIdx := SeparatorRowIdx()
	state := makeTestState(sepIdx + len(jdPhases))
	assignments := make(map[string]model.ModelAssignment)

	_, updated := handleModelNav("enter", state, assignments)

	lastPhase := jdPhases[len(jdPhases)-1]
	if _, ok := updated[lastPhase]; !ok {
		t.Fatalf("last JD row should assign %q; got: %v", lastPhase, updated)
	}
}

// ─── ModelPickerRowsForProfile ──────────────────────────────────────────

func TestModelPickerRowsForProfile_Count(t *testing.T) {
	rows := ModelPickerRowsForProfile()
	// 1 orchestrator + 1 "Set all SDD phases" + 10 sub-agents = 12
	want := 2 + len(opencode.SDDPhases())
	if len(rows) != want {
		t.Fatalf("ModelPickerRowsForProfile() len = %d, want %d; rows = %v", len(rows), want, rows)
	}
}

func TestModelPickerRowsForProfile_NoSeparator(t *testing.T) {
	rows := ModelPickerRowsForProfile()
	for _, row := range rows {
		if strings.Contains(row, "---") {
			t.Fatalf("ModelPickerRowsForProfile() should not contain separator; got: %v", rows)
		}
	}
}

func TestModelPickerRowsForProfile_NoJDAgents(t *testing.T) {
	rows := ModelPickerRowsForProfile()
	jdPhases := opencode.JDPhases()
	for _, jd := range jdPhases {
		for _, row := range rows {
			if row == jd {
				t.Fatalf("ModelPickerRowsForProfile() should not contain JD agent %q; got: %v", jd, rows)
			}
		}
	}
}
