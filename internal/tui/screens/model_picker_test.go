package screens

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
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
	want := 2 + len(opencode.SDDPhases()) + 1 + len(opencode.JDPhases()) + 1 + len(opencode.ReviewPhases())
	if len(rows) != want {
		t.Fatalf("ModelPickerRows() len = %d, want %d; rows = %v", len(rows), want, rows)
	}
}

func TestModelPickerRows_ReviewAgentsFollowJudgmentDay(t *testing.T) {
	rows := ModelPickerRows()
	wantSuffix := append([]string{"--- Review agents ---"}, opencode.ReviewPhases()...)
	got := rows[len(rows)-len(wantSuffix):]
	for i := range wantSuffix {
		if got[i] != wantSuffix[i] {
			t.Fatalf("review row %d = %q, want %q; rows = %v", i, got[i], wantSuffix[i], rows)
		}
	}
}

func TestRenderModelPickerScrollsToReviewAgents(t *testing.T) {
	rows := ModelPickerRows()
	cursor := len(rows) - 1
	state := ModelPickerState{AvailableIDs: []string{"openai"}}

	output := RenderModelPicker(nil, state, cursor)
	if !strings.Contains(output, "review-refuter") || !strings.Contains(output, "↑ more assignments") {
		t.Fatalf("review rows are not visible at cursor %d:\n%s", cursor, output)
	}
	if strings.Contains(output, "gentle-orchestrator") {
		t.Fatalf("picker did not window rows around review cursor:\n%s", output)
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
		wantAvailable     int      // minimum number of AvailableIDs; custom providers count only with tool_call models
		wantUnavailable   []string // provider IDs that must not appear in AvailableIDs
		wantConfigWarning bool     // whether ConfigWarning must be non-empty
		wantWarningText   string   // substring expected in ConfigWarning and rendered output
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
			name:         "custom provider without tool_call warns without becoming available",
			cacheContent: catalogJSON,
			settingsContent: `{
				"provider": {
					"custom-no-tools": {
						"name": "Custom No Tools",
						"models": {"model-a": {"name": "Model A"}}
					}
				}
			}`,
			wantProviderIDs:   []string{"built-in", "custom-no-tools"},
			wantAvailable:     0,
			wantUnavailable:   []string{"custom-no-tools"},
			wantConfigWarning: true,
			wantWarningText:   `provider["custom-no-tools"].models`,
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
			for _, unavailableID := range tt.wantUnavailable {
				for _, availableID := range state.AvailableIDs {
					if availableID == unavailableID {
						t.Errorf("AvailableIDs = %v, must not include %q", state.AvailableIDs, unavailableID)
					}
				}
			}

			// ConfigWarning check.
			if tt.wantConfigWarning && state.ConfigWarning == "" {
				t.Error("expected ConfigWarning to be set, got empty string")
			}
			if !tt.wantConfigWarning && state.ConfigWarning != "" {
				t.Errorf("expected no ConfigWarning, got %q", state.ConfigWarning)
			}
			if tt.wantWarningText != "" {
				if !strings.Contains(state.ConfigWarning, tt.wantWarningText) {
					t.Errorf("ConfigWarning = %q, want substring %q", state.ConfigWarning, tt.wantWarningText)
				}
				if rendered := RenderModelPicker(nil, state, 0); !strings.Contains(rendered, tt.wantWarningText) {
					t.Errorf("rendered picker missing warning substring %q:\n%s", tt.wantWarningText, rendered)
				}
			}
		})
	}
}

func TestCustomProviderToolCallWarnings(t *testing.T) {
	singleProvider := map[string]opencode.Provider{
		"alpha": {
			ID:   "alpha",
			Name: "Alpha",
			Models: map[string]opencode.Model{
				"model": {ID: "model"},
			},
		},
	}
	singleConfigProvider := map[string]opencode.ConfigProvider{
		"alpha": {
			Models: map[string]opencode.ConfigModel{
				"model": {},
			},
		},
	}
	customWarning := `Custom provider "Alpha" has models, but none declare "tool_call": true. Add "tool_call": true to at least one model in provider["alpha"].models.`

	tests := []struct {
		name            string
		existingWarning string
		providers       map[string]opencode.Provider
		configProviders map[string]opencode.ConfigProvider
		wantWarning     string
	}{
		{
			name: "sorts multiple provider warnings by provider ID",
			providers: map[string]opencode.Provider{
				"zeta": {
					ID:   "zeta",
					Name: "Zeta",
					Models: map[string]opencode.Model{
						"model": {ID: "model"},
					},
				},
				"alpha": singleProvider["alpha"],
			},
			configProviders: map[string]opencode.ConfigProvider{
				"zeta": {
					Models: map[string]opencode.ConfigModel{"model": {}},
				},
				"alpha": singleConfigProvider["alpha"],
			},
			wantWarning: `Custom provider "Alpha" has models, but none declare "tool_call": true. Add "tool_call": true to at least one model in provider["alpha"].models.` +
				"\n" +
				`Custom provider "Zeta" has models, but none declare "tool_call": true. Add "tool_call": true to at least one model in provider["zeta"].models.`,
		},
		{
			name:            "preserves cache warning and renders both warnings",
			existingWarning: "Could not load model cache: parse models cache: invalid character",
			providers:       singleProvider,
			configProviders: singleConfigProvider,
			wantWarning:     "Could not load model cache: parse models cache: invalid character\n" + customWarning,
		},
		{
			name:            "preserves config warning and renders both warnings",
			existingWarning: "Could not load custom providers from opencode.json: parse opencode settings",
			providers:       singleProvider,
			configProviders: singleConfigProvider,
			wantWarning:     "Could not load custom providers from opencode.json: parse opencode settings\n" + customWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendCustomProviderToolCallWarnings(tt.existingWarning, tt.providers, tt.configProviders)
			if got != tt.wantWarning {
				t.Fatalf("warning = %q, want %q", got, tt.wantWarning)
			}
			rendered := RenderModelPicker(nil, ModelPickerState{ConfigWarning: got}, 0)
			for _, warning := range strings.Split(tt.wantWarning, "\n") {
				if !strings.Contains(rendered, warning) {
					t.Fatalf("rendered picker missing warning %q:\n%s", warning, rendered)
				}
			}
		})
	}
}

func TestLMStudioDiscoveryWarningIsNotDuplicated(t *testing.T) {
	state := lmStudioState(t, `{}`, `{"provider":{"lmstudio":{"models":{"model":{}}}}}`)
	state = state.Update(LMStudioDiscoveryMsg{
		BaseURL: state.lmStudioURL,
		Models:  []opencode.ConfigModel{{Name: "model"}},
	})

	warning := `LM Studio models need "tool_call": true in provider.lmstudio.models for SDD.`
	if got := strings.Count(state.ConfigWarning, warning); got != 1 {
		t.Fatalf("LM Studio warning count = %d, want 1; warning = %q", got, state.ConfigWarning)
	}
}

func TestLMStudioDiscoveryErrorWarnsOnceWhenConfiguredModelsLackToolCalls(t *testing.T) {
	state := lmStudioState(t, `{}`, `{"provider":{"lmstudio":{"models":{"model":{"name":"Model"}}}}}`)
	discovery := LMStudioDiscoveryMsg{
		BaseURL: state.lmStudioURL,
		Err:     errors.New("connection refused"),
	}
	state = state.Update(discovery)
	state = state.Update(discovery)

	if len(state.AvailableIDs) != 0 {
		t.Fatalf("AvailableIDs = %v, want LM Studio excluded", state.AvailableIDs)
	}

	warning := lmStudioToolCallWarning
	if got := strings.Count(state.ConfigWarning, warning); got != 1 {
		t.Fatalf("LM Studio tool_call warning count = %d, want 1; warning = %q", got, state.ConfigWarning)
	}
	if !strings.Contains(state.ConfigWarning, "LM Studio discovery failed; using configured models.") {
		t.Fatalf("discovery fallback warning missing: %q", state.ConfigWarning)
	}
	if got := strings.Count(RenderModelPicker(nil, state, 0), warning); got != 1 {
		t.Fatalf("rendered LM Studio tool_call warning count = %d, want 1", got)
	}
}

func TestNewModelPickerStateCacheErrorStillDiscovers(t *testing.T) {
	state := NewModelPickerState(writeTempFile(t, "models.json", `{`), writeTempFile(t, "opencode.json", `{"provider":{"lmstudio":{"url":"http://gateway:1234/v1","models":{"model":{"tool_call":true}}}}}`))
	if state.Providers == nil || state.lmStudioURL != "http://gateway:1234/v1" || len(state.AvailableIDs) != 1 || !strings.Contains(state.ConfigWarning, "model cache") || state.DiscoverLMStudioCmd() == nil {
		t.Fatalf("cache fallback state = %+v", state)
	}
}

func TestNewModelPickerStateDiscoveryErrorAddsConfigWarning(t *testing.T) {
	settingsPath := t.TempDir()
	state := NewModelPickerState(writeTempFile(t, "models.json", catalogJSON), settingsPath)

	if !strings.Contains(state.ConfigWarning, "Could not discover custom agents from opencode.json") {
		t.Fatalf("ConfigWarning = %q, want custom-agent discovery error", state.ConfigWarning)
	}
	if len(state.CustomAgents) != 0 {
		t.Fatalf("CustomAgents = %v, want empty after discovery error", state.CustomAgents)
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

func lmStudioState(t *testing.T, catalog, settings string) ModelPickerState {
	t.Helper()
	return NewModelPickerState(writeTempFile(t, "models.json", catalog), writeTempFile(t, "opencode.json", settings))
}

func TestLMStudioDiscovery(t *testing.T) {
	for name, tt := range map[string]struct{ settings, url string }{
		"default URL":    {`{}`, "http://127.0.0.1:1234/v1"},
		"configured URL": {`{"provider":{"lmstudio":{"url":"http://gateway:1234/v1"}}}`, "http://gateway:1234/v1"},
	} {
		t.Run(name, func(t *testing.T) {
			calls, gotURL, original := 0, "", fetchDynamicModels
			fetchDynamicModels = func(ctx context.Context, url string) ([]opencode.ConfigModel, error) {
				calls++
				gotURL = url
				return nil, nil
			}
			t.Cleanup(func() { fetchDynamicModels = original })
			state := lmStudioState(t, catalogJSON, tt.settings)
			if calls != 0 {
				t.Fatalf("NewModelPickerState made %d discovery calls", calls)
			}
			msg := state.DiscoverLMStudioCmd()().(LMStudioDiscoveryMsg)
			if calls != 1 || gotURL != tt.url || msg.BaseURL != tt.url {
				t.Fatalf("discovery = calls:%d URL:%q message:%q", calls, gotURL, msg.BaseURL)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`)) }))
	defer server.Close()
	fetched := lmStudioState(t, `{}`, `{"provider":{"lmstudio":{"url":"`+server.URL+`"}}}`).DiscoverLMStudioCmd()().(LMStudioDiscoveryMsg)
	if len(fetched.Models) != 1 || fetched.Models[0].Name != "model" || fetched.Models[0].ToolCall {
		t.Fatalf("unexpected fetched models: %+v", fetched.Models)
	}
	state := lmStudioState(t, `{"lmstudio":{"models":{"unloaded":{"id":"unloaded","tool_call":true},"static":{"id":"static","name":"Static","tool_call":true},"configured":{"id":"configured"}}}}`, `{"provider":{"lmstudio":{"models":{"configured":{"name":"User","tool_call":true}}}}}`)
	state = state.Update(LMStudioDiscoveryMsg{BaseURL: state.lmStudioURL, Models: []opencode.ConfigModel{{Name: "configured"}, {Name: "static"}, {Name: "unknown"}}})
	lm := state.Providers["lmstudio"]
	if _, ok := lm.Models["unloaded"]; ok || lm.Models["configured"].Name != "User" || !lm.Models["configured"].ToolCall || lm.Models["static"].Name != "Static" || lm.Models["unknown"].ToolCall || len(state.SDDModels["lmstudio"]) != 2 {
		t.Fatalf("unexpected models: %+v", lm.Models)
	}
	state = lmStudioState(t, `{}`, `{}`).Update(LMStudioDiscoveryMsg{BaseURL: "http://127.0.0.1:1234/v1", Models: []opencode.ConfigModel{{Name: "unknown"}}})
	if len(state.AvailableIDs) != 0 || !strings.Contains(state.ConfigWarning, `"tool_call": true`) {
		t.Fatalf("unsafe unknown model state: %+v", state)
	}
	state = lmStudioState(t, `{"lmstudio":{"models":{"catalog":{"id":"catalog","tool_call":true}}}}`, `{"provider":{"lmstudio":{"models":{"configured":{"tool_call":true}}}}}`)
	state = state.Update(LMStudioDiscoveryMsg{BaseURL: state.lmStudioURL, Err: errors.New("connection refused")})
	if _, ok := state.Providers["lmstudio"].Models["catalog"]; !ok || !state.Providers["lmstudio"].Models["configured"].ToolCall || !strings.Contains(state.ConfigWarning, "discovery failed") {
		t.Fatalf("fallback lost: %+v", state)
	}
	state = lmStudioState(t, `{}`, `{"provider":{"lmstudio":{"url":"http://gateway:1234/v1"}}}`)
	state = state.Update(LMStudioDiscoveryMsg{BaseURL: "http://127.0.0.1:1234/v1", Models: []opencode.ConfigModel{{Name: "stale"}}})
	if _, ok := state.Providers["lmstudio"].Models["stale"]; ok || state.ConfigWarning != "" {
		t.Fatalf("stale response changed state: %+v", state)
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

func TestHandleModelNav_ReviewAgentRowsAssignCorrectly(t *testing.T) {
	rows := ModelPickerRows()
	for _, agent := range opencode.ReviewPhases() {
		t.Run(agent, func(t *testing.T) {
			rowIdx := -1
			for i, row := range rows {
				if row == agent {
					rowIdx = i
					break
				}
			}
			state := makeTestState(rowIdx)
			_, updated := handleModelNav("enter", state, map[string]model.ModelAssignment{})
			if got := updated[agent]; got.ProviderID != "test-provider" || got.ModelID != "model-alpha" {
				t.Fatalf("%s assignment = %+v, want test-provider/model-alpha", agent, got)
			}
		})
	}
}

// ─── ModelPickerRowsForProfile ──────────────────────────────────────────

func TestModelPickerRowsForProfile(t *testing.T) {
	rows := ModelPickerRowsForProfile()
	want := 2 + len(opencode.SDDPhases()) + 1 + len(opencode.JDPhases())
	if len(rows) != want {
		t.Fatalf("ModelPickerRowsForProfile() len = %d, want %d; rows = %v", len(rows), want, rows)
	}

	sepIdx := SeparatorRowIdx()
	if sepIdx < 0 || sepIdx >= len(rows) {
		t.Fatalf("SeparatorRowIdx() = %d out of profile rows range %d", sepIdx, len(rows))
	}
	if rows[sepIdx] != "--- Judgment Day ---" {
		t.Fatalf("ModelPickerRowsForProfile()[%d] = %q, want Judgment Day separator; rows = %v", sepIdx, rows[sepIdx], rows)
	}

	for _, jd := range opencode.JDPhases() {
		found := false
		for _, row := range rows {
			if row == jd {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ModelPickerRowsForProfile() missing JD agent %q; got: %v", jd, rows)
		}
	}
	for _, reviewAgent := range opencode.ReviewPhases() {
		for _, row := range rows {
			if row == reviewAgent {
				t.Fatalf("profile rows must use global reviewer %q, got: %v", reviewAgent, rows)
			}
		}
	}
}

func TestModelPickerRowsForState_WithCustomAgents(t *testing.T) {
	state := ModelPickerState{
		CustomAgents: []string{"my-custom-agent-1", "my-custom-agent-2"},
	}
	rows := ModelPickerRowsForState(state)

	var hasCustomSep, hasSetAllCustom, hasAgent1, hasAgent2 bool
	for _, r := range rows {
		if r == "--- Custom / Native agents ---" {
			hasCustomSep = true
		}
		if r == "Set all custom agents" {
			hasSetAllCustom = true
		}
		if r == "my-custom-agent-1" {
			hasAgent1 = true
		}
		if r == "my-custom-agent-2" {
			hasAgent2 = true
		}
	}

	if !hasCustomSep || !hasSetAllCustom || !hasAgent1 || !hasAgent2 {
		t.Fatalf("ModelPickerRowsForState() missing custom agent rows: got %v", rows)
	}
}

func TestModelPickerRowsForState_ProfileExcludesCustomAgents(t *testing.T) {
	state := ModelPickerState{
		ForProfile:   true,
		CustomAgents: []string{"custom-profile-agent"},
	}
	want := ModelPickerRowsForProfile()
	got := ModelPickerRowsForState(state)

	if len(got) != len(want) {
		t.Fatalf("ModelPickerRowsForState(profile) len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ModelPickerRowsForState(profile)[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestNewModelPickerState_DiscoversCustomAgents(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")
	cachePath := filepath.Join(dir, "models.json")

	settings := `{
  "agent": {
    "gentle-orchestrator": { "model": "anthropic/claude-sonnet-4" },
    "custom-coder-v1": { "model": "openai/gpt-4o-mini" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	state := NewModelPickerState(cachePath, settingsPath)
	if len(state.CustomAgents) != 1 || state.CustomAgents[0] != "custom-coder-v1" {
		t.Fatalf("NewModelPickerState CustomAgents = %v, want [custom-coder-v1]", state.CustomAgents)
	}
}

func TestApplyAssignment_SetAllCustomAgents(t *testing.T) {
	state := ModelPickerState{
		CustomAgents:     []string{"custom-1", "custom-2"},
		SelectedPhaseIdx: 0,
	}
	rows := ModelPickerRowsForState(state)
	setAllIdx := -1
	for i, r := range rows {
		if r == "Set all custom agents" {
			setAllIdx = i
			break
		}
	}
	if setAllIdx < 0 {
		t.Fatalf("Set all custom agents row not found in %v", rows)
	}

	state.SelectedPhaseIdx = setAllIdx
	assignments := make(map[string]model.ModelAssignment)
	assigned := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-3-5-haiku"}

	res := applyAssignment(state, assignments, assigned)
	if res["custom-1"] != assigned || res["custom-2"] != assigned {
		t.Fatalf("applyAssignment Set all custom agents = %v, want assigned %v for custom-1 and custom-2", res, assigned)
	}
}

func TestHandleModelPickerNav_SetAllCustomAgentsWithoutEffortAssignsEachAgent(t *testing.T) {
	state := makeTestState(0)
	state.CustomAgents = []string{"custom-1", "custom-2"}
	for i, row := range ModelPickerRowsForState(*state) {
		if row == "Set all custom agents" {
			state.SelectedPhaseIdx = i
			break
		}
	}

	handled, assignments := HandleModelPickerNav("enter", state, nil)
	want := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha"}
	if !handled || assignments["custom-1"] != want || assignments["custom-2"] != want {
		t.Fatalf("HandleModelPickerNav() handled=%v assignments=%v, want each custom agent assigned %v", handled, assignments, want)
	}
	if state.AllCustomAgentsModel != want {
		t.Fatalf("AllCustomAgentsModel = %v, want %v", state.AllCustomAgentsModel, want)
	}
}

func TestClearModelPickerAssignment_SetAllCustomAgents(t *testing.T) {
	state := ModelPickerState{
		CustomAgents:         []string{"custom-1", "custom-2"},
		AllCustomAgentsModel: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-4o-mini"},
	}
	rows := ModelPickerRowsForState(state)
	setAllIdx := -1
	for i, r := range rows {
		if r == "Set all custom agents" {
			setAllIdx = i
			break
		}
	}
	if setAllIdx < 0 {
		t.Fatalf("Set all custom agents row not found in %v", rows)
	}

	state.SelectedPhaseIdx = setAllIdx
	assignments := map[string]model.ModelAssignment{
		"custom-1": {ProviderID: "openai", ModelID: "gpt-4o-mini"},
		"custom-2": {ProviderID: "openai", ModelID: "gpt-4o-mini"},
	}

	res := ClearModelPickerAssignment(&state, assignments)
	if _, ok := res["custom-1"]; ok {
		t.Error("custom-1 should be cleared")
	}
	if _, ok := res["custom-2"]; ok {
		t.Error("custom-2 should be cleared")
	}
	if state.AllCustomAgentsModel.ProviderID != "" {
		t.Errorf("AllCustomAgentsModel = %v, want empty after clear", state.AllCustomAgentsModel)
	}
}

func TestHandleModelPickerNav_CustomAgentLabelsDoNotChangeRowIdentity(t *testing.T) {
	for _, tt := range []struct {
		name      string
		agentName string
	}{
		{name: "bulk label", agentName: "Set all custom agents"},
		{name: "custom separator label", agentName: "--- Custom / Native agents ---"},
		{name: "review separator label", agentName: "--- Review agents ---"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := makeTestState(0)
			state.CustomAgents = []string{tt.agentName, "other-custom-agent"}
			selectedIdx := -1
			for index, row := range ModelPickerRowsForStateWithIdentity(*state) {
				if row.Kind == ModelPickerRowKindAgent && row.AgentID == tt.agentName {
					selectedIdx = index
					break
				}
			}
			if selectedIdx < 0 {
				t.Fatalf("custom agent row %q not found", tt.agentName)
			}
			state.SelectedPhaseIdx = selectedIdx

			old := model.ModelAssignment{ProviderID: "old-provider", ModelID: "old-model", Effort: "high"}
			assignments := map[string]model.ModelAssignment{"other-custom-agent": old}
			handled, got := HandleModelPickerNav("enter", state, assignments)
			want := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha"}
			if !handled || got[tt.agentName] != want {
				t.Fatalf("HandleModelPickerNav() handled=%v assignment=%v, want %v for %q", handled, got[tt.agentName], want, tt.agentName)
			}
			if got["other-custom-agent"] != old {
				t.Fatalf("collision row changed another custom agent: got %v, want %v", got["other-custom-agent"], old)
			}
			if state.AllCustomAgentsModel != (model.ModelAssignment{}) {
				t.Fatalf("custom agent row changed bulk state: %v", state.AllCustomAgentsModel)
			}
		})
	}
}

func TestHandleModelPickerNav_CustomAgentBulkLabelPreservesEffortPath(t *testing.T) {
	state := makeTestState(0)
	state.CustomAgents = []string{"Set all custom agents", "other-custom-agent"}
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	for index, row := range ModelPickerRowsForStateWithIdentity(*state) {
		if row.Kind == ModelPickerRowKindAgent && row.AgentID == "Set all custom agents" {
			state.SelectedPhaseIdx = index
			break
		}
	}
	old := model.ModelAssignment{ProviderID: "old-provider", ModelID: "old-model", Effort: "high"}
	assignments := map[string]model.ModelAssignment{"other-custom-agent": old}

	handled, assignments := HandleModelPickerNav("enter", state, assignments)
	if !handled || state.Mode != ModeEffortSelect {
		t.Fatalf("enter should open effort selection: handled=%v mode=%v", handled, state.Mode)
	}
	handled, assignments = HandleModelPickerNav("enter", state, assignments)
	want := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha"}
	if !handled || assignments["Set all custom agents"] != want {
		t.Fatalf("effort selection assignment = %v, want %v", assignments["Set all custom agents"], want)
	}
	if assignments["other-custom-agent"] != old {
		t.Fatalf("effort selection changed another custom agent: got %v, want %v", assignments["other-custom-agent"], old)
	}
	if state.AllCustomAgentsModel != (model.ModelAssignment{}) {
		t.Fatalf("effort selection changed bulk state: %v", state.AllCustomAgentsModel)
	}
}

func TestClearModelPickerAssignment_CustomAgentBulkLabelPreservesOtherRows(t *testing.T) {
	state := ModelPickerState{
		CustomAgents:         []string{"Set all custom agents", "other-custom-agent"},
		AllCustomAgentsModel: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-4o-mini"},
	}
	for index, row := range ModelPickerRowsForStateWithIdentity(state) {
		if row.Kind == ModelPickerRowKindAgent && row.AgentID == "Set all custom agents" {
			state.SelectedPhaseIdx = index
			break
		}
	}
	bulk := state.AllCustomAgentsModel
	assignments := map[string]model.ModelAssignment{
		"Set all custom agents": bulk,
		"other-custom-agent":    bulk,
	}

	result := ClearModelPickerAssignment(&state, assignments)
	if _, ok := result["Set all custom agents"]; ok {
		t.Fatal("custom agent named like the bulk row should be cleared")
	}
	if result["other-custom-agent"] != bulk {
		t.Fatalf("clear changed another custom agent: got %v, want %v", result["other-custom-agent"], bulk)
	}
	if state.AllCustomAgentsModel != bulk {
		t.Fatalf("custom agent clear changed bulk state: got %v, want %v", state.AllCustomAgentsModel, bulk)
	}
}
