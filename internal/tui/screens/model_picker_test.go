package screens

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

func pickerTestState(index int) *ModelPickerState {
	return &ModelPickerState{
		Mode: ModeModelSelect, SelectedPhaseIdx: index, SelectedProvider: "test-provider",
		SDDModels: map[string][]opencode.Model{"test-provider": {
			{ID: "model-alpha", Name: "Alpha Model", ToolCall: true},
			{ID: "model-beta", Name: "Beta Model", ToolCall: true},
		}},
	}
}

func pickerRowIndex(t *testing.T, state ModelPickerState, agent string) int {
	t.Helper()
	for i, row := range ModelPickerRowsForStateWithIdentity(state) {
		if row.Kind == ModelPickerRowKindAgent && row.AgentID == agent {
			return i
		}
	}
	t.Fatalf("no row for %q", agent)
	return -1
}

func TestModelPickerRowsOfferInstalledAgentsWithoutRetiredSDD(t *testing.T) {
	rows := ModelPickerRows()
	want := []string{"gentle-orchestrator", "--- Judgment Day ---", "jd-judge-a", "jd-judge-b", "jd-fix-agent", "--- Review agents ---", "review-risk", "review-readability", "review-reliability", "review-resilience", "review-refuter", "review-validator", "--- OpenCode native agents ---", "general", "explore"}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("active OpenCode rows = %v, want %v", rows, want)
	}
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row), "sdd") || strings.HasPrefix(row, "odd-") {
			t.Errorf("inactive or prompt-only role offered: %q", row)
		}
	}
	if SeparatorRowIdx() != 1 {
		t.Fatalf("Judgment Day separator index = %d, want 1", SeparatorRowIdx())
	}
}

func TestModelPickerAssignsOnlySelectedInstalledAgent(t *testing.T) {
	for _, agent := range []string{"gentle-orchestrator", "jd-judge-a", "review-risk", "review-refuter", "general", "explore"} {
		t.Run(agent, func(t *testing.T) {
			state := pickerTestState(0)
			state.SelectedPhaseIdx = pickerRowIndex(t, *state, agent)
			legacy := model.ModelAssignment{ProviderID: "legacy", ModelID: "saved"}
			assignments := map[string]model.ModelAssignment{"sdd-apply": legacy}
			handled, got := HandleModelPickerNav("enter", state, assignments)
			if !handled || got[agent].ModelID != "model-alpha" || got["sdd-apply"] != legacy || len(got) != 2 {
				t.Fatalf("selection for %q changed unrelated or persisted assignment: %v", agent, got)
			}
		})
	}
}

func TestModelPickerSeparatorDoesNotAssign(t *testing.T) {
	state := pickerTestState(SeparatorRowIdx())
	_, got := HandleModelPickerNav("enter", state, nil)
	if len(got) != 0 || state.Mode != ModePhaseList {
		t.Fatalf("separator selected: assignments=%v mode=%d", got, state.Mode)
	}
}

func TestRenderModelPickerScrollsToReviewAgents(t *testing.T) {
	rows := ModelPickerRows()
	cursor := pickerRowIndex(t, ModelPickerState{}, "review-refuter")
	output := RenderModelPicker(nil, ModelPickerState{AvailableIDs: []string{"openai"}}, cursor)
	if !strings.Contains(output, "review-refuter") {
		t.Fatalf("review row not visible at cursor %d: %s", cursor, output)
	}
	if len(rows) > maxVisiblePhaseRows && !strings.Contains(output, "↑ more assignments") {
		t.Fatalf("windowing omitted scroll indicator: %s", output)
	}
}

func TestModelPickerCustomBulkDoesNotTouchNativeOrLegacyAssignments(t *testing.T) {
	state := ModelPickerState{CustomAgents: []string{"custom-a", "custom-b"}}
	for i, row := range ModelPickerRowsForStateWithIdentity(state) {
		if row.Kind == ModelPickerRowKindSetAllCustom {
			state.SelectedPhaseIdx = i
			break
		}
	}
	existing := model.ModelAssignment{ProviderID: "old", ModelID: "saved"}
	choice := model.ModelAssignment{ProviderID: "new", ModelID: "model"}
	got := applyAssignment(state, map[string]model.ModelAssignment{"general": existing, "sdd-apply": existing, "review-risk": existing}, choice)
	if got["custom-a"] != choice || got["custom-b"] != choice || got["general"] != existing || got["sdd-apply"] != existing || got["review-risk"] != existing {
		t.Fatalf("custom bulk changed unrelated assignments: %v", got)
	}
}

func TestModelPickerCustomAgentLabelCannotChangeRowIdentity(t *testing.T) {
	for _, label := range []string{"Set all custom agents", "--- Review agents ---", "--- Custom / Native agents ---"} {
		t.Run(label, func(t *testing.T) {
			state := pickerTestState(0)
			state.CustomAgents = []string{label, "other-custom-agent"}
			state.SelectedPhaseIdx = pickerRowIndex(t, *state, label)
			old := model.ModelAssignment{ProviderID: "old", ModelID: "saved"}
			_, got := HandleModelPickerNav("enter", state, map[string]model.ModelAssignment{"other-custom-agent": old})
			if got[label].ModelID != "model-alpha" || got["other-custom-agent"] != old || state.AllCustomAgentsModel != (model.ModelAssignment{}) {
				t.Fatalf("collision changed another agent or bulk state: %v", got)
			}
		})
	}
}

func TestModelPickerReasoningEffortAndNoVariantFallback(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	_, got := HandleModelPickerNav("enter", state, nil)
	if state.Mode != ModeEffortSelect || len(got) != 0 {
		t.Fatalf("reasoning variant not deferred: mode=%d assignments=%v", state.Mode, got)
	}
	state.EffortCursor = 2
	_, got = HandleModelPickerNav("enter", state, got)
	if got[SDDOrchestratorPhase].Effort != "high" || state.Mode != ModePhaseList {
		t.Fatalf("effort selection lost: mode=%d assignments=%v", state.Mode, got)
	}
	state = pickerTestState(0)
	state.SDDModels["test-provider"][0].Reasoning = true
	_, got = HandleModelPickerNav("enter", state, got)
	if got[SDDOrchestratorPhase].Effort != "high" {
		t.Fatalf("same reasoning model without variants lost saved effort: %v", got)
	}
	state = pickerTestState(0)
	_, got = HandleModelPickerNav("enter", state, got)
	if got[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("non-reasoning model retained unsupported effort: %v", got)
	}
}

func TestFilteredModelEntriesSearchAndVersionOrder(t *testing.T) {
	state := ModelPickerState{SelectedProvider: "openai", SDDModels: map[string][]opencode.Model{"openai": {
		{ID: "gemini-2.5-pro-exp-03-25", Name: "Gemini 2.5 Pro Experimental"},
		{ID: "gemini-3-pro", Name: "Gemini 3 Pro"},
	}}}
	if got := FilteredModelEntries(state); got[0].ID != "gemini-3-pro" {
		t.Fatalf("newer semantic version lost to date suffix: %v", got)
	}
	state.ModelSearch = "2.5"
	if got := FilteredModelEntries(state); len(got) != 1 || got[0].ID != "gemini-2.5-pro-exp-03-25" {
		t.Fatalf("search result = %v", got)
	}
}

func TestModelPickerProviderAndModelNavigation(t *testing.T) {
	state := pickerTestState(0)
	state.Mode = ModeProviderSelect
	state.AvailableIDs = []string{"test-provider"}
	state.Providers = map[string]opencode.Provider{"test-provider": {ID: "test-provider", Name: "Test Provider"}}
	if handled, _ := HandleModelPickerNav("enter", state, nil); !handled || state.Mode != ModeModelSelect || state.SelectedProvider != "test-provider" {
		t.Fatalf("provider selection failed: %+v", state)
	}
	if handled, _ := HandleModelPickerNav("b", state, nil); !handled || state.ModelSearch != "b" {
		t.Fatalf("model search failed: %+v", state)
	}
	if got := FilteredModelEntries(*state); len(got) != 1 || got[0].ID != "model-beta" {
		t.Fatalf("filtered models = %+v", got)
	}
	if handled, _ := HandleModelPickerNav("backspace", state, nil); !handled || state.ModelSearch != "" {
		t.Fatalf("model backspace failed: %+v", state)
	}
	if handled, _ := HandleModelPickerNav("esc", state, nil); !handled || state.Mode != ModeProviderSelect {
		t.Fatalf("model escape failed: %+v", state)
	}
	if handled, _ := HandleModelPickerNav("esc", state, nil); !handled || state.Mode != ModePhaseList {
		t.Fatalf("provider escape failed: %+v", state)
	}
}

func TestModelPickerEffortEscapeAndDefault(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	_, _ = HandleModelPickerNav("enter", state, nil)
	if state.Mode != ModeEffortSelect {
		t.Fatalf("variant did not open effort picker: %+v", state)
	}
	_, _ = HandleModelPickerNav("esc", state, nil)
	if state.Mode != ModeModelSelect || state.PendingAssignment != (model.ModelAssignment{}) {
		t.Fatalf("effort escape failed to clear pending assignment: %+v", state)
	}
	_, _ = HandleModelPickerNav("enter", state, nil)
	_, got := HandleModelPickerNav("enter", state, nil)
	if got[SDDOrchestratorPhase].ModelID != "model-alpha" || got[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("default effort should be empty: %+v", got)
	}
}

func TestModelPickerCustomBulkEffortAndIndividualIsolation(t *testing.T) {
	state := pickerTestState(0)
	state.CustomAgents = []string{"custom-a", "custom-b"}
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	for i, row := range ModelPickerRowsForStateWithIdentity(*state) {
		if row.Kind == ModelPickerRowKindSetAllCustom {
			state.SelectedPhaseIdx = i
		}
	}
	_, _ = HandleModelPickerNav("enter", state, nil)
	state.EffortCursor = 1
	_, assignments := HandleModelPickerNav("enter", state, nil)
	want := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha", Effort: "low"}
	if assignments["custom-a"] != want || assignments["custom-b"] != want || state.AllCustomAgentsModel != want {
		t.Fatalf("custom bulk effort assignment = %v, state = %+v", assignments, state)
	}
	state.Mode = ModeModelSelect
	state.SelectedPhaseIdx = pickerRowIndex(t, *state, "custom-a")
	state.ModelCursor = 1
	_, assignments = HandleModelPickerNav("enter", state, assignments)
	if assignments["custom-a"].ModelID != "model-beta" || assignments["custom-b"] != want || state.AllCustomAgentsModel != want {
		t.Fatalf("individual custom edit changed bulk label or peer: %v, bulk=%+v", assignments, state.AllCustomAgentsModel)
	}
}

func TestRenderModelPickerAssignmentLabelsAndWarning(t *testing.T) {
	state := ModelPickerState{
		ConfigWarning: "invalid opencode.json", AvailableIDs: []string{"anthropic"},
		Providers: map[string]opencode.Provider{"anthropic": {ID: "anthropic", Name: "Anthropic", Models: map[string]opencode.Model{"claude": {ID: "claude", Name: "Claude"}}}},
	}
	assignment := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude", Effort: "high"}
	output := RenderModelPicker(map[string]model.ModelAssignment{SDDOrchestratorPhase: assignment}, state, 0)
	for _, fragment := range []string{"invalid opencode.json", "gentle-orchestrator", "Anthropic / Claude [high]"} {
		if !strings.Contains(output, fragment) {
			t.Errorf("rendered assignment missing %q: %s", fragment, output)
		}
	}
}

func TestRuntimeModelPickerStateDiscoversCustomAgents(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(settings, []byte(`{"agent":{"gentle-orchestrator":{},"custom-coder-v1":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewRuntimeModelPickerStateWithDiscoverer(settings, nil)
	if !reflect.DeepEqual(state.CustomAgents, []string{"custom-coder-v1"}) {
		t.Fatalf("custom agents = %v", state.CustomAgents)
	}
}

func TestRuntimeCatalogDiscoveryTransitionsAndConfiguredFallback(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	if !strings.Contains(RenderModelPicker(nil, state, 0), "Discovering models") {
		t.Fatal("loading state missing")
	}
	state.ConfiguredProviders = map[string]opencode.Provider{"configured": {ID: "configured", Models: map[string]opencode.Model{"configured/model": {ID: "configured/model", ToolCall: true}}}}
	state.catalogDiscover = func(context.Context, string) (map[string]opencode.Provider, error) {
		return nil, errors.New("unavailable")
	}
	state = state.Update(state.StartRuntimeCatalogDiscovery(1, "project")().(RuntimeCatalogDiscoveryMsg))
	if state.CatalogStatus != RuntimeCatalogReady || !reflect.DeepEqual(state.AvailableIDs, []string{"configured"}) {
		t.Fatalf("configured fallback = %+v", state)
	}
	state = state.Update(RuntimeCatalogDiscoveryMsg{RequestID: 2, ProjectDir: "project", Providers: nil})
	if len(state.AvailableIDs) != 1 {
		t.Fatal("stale discovery clobbered configured models")
	}
	state.ConfiguredProviders = nil
	state = state.Update(RuntimeCatalogDiscoveryMsg{RequestID: 1, ProjectDir: "project", Providers: map[string]opencode.Provider{}})
	if state.CatalogStatus != RuntimeCatalogEmpty {
		t.Fatalf("empty discovery status = %v", state.CatalogStatus)
	}
}

func TestRuntimeCatalogDiscoveryErrorDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		kind        opencode.CatalogErrorKind
		wantMessage string
		wantSubtext string
		forbidText  string
	}{
		{opencode.CatalogErrorOutputTooLarge, "OpenCode model catalog is too large.", "OpenCode produced more model data", "Verify OpenCode is installed"},
		{opencode.CatalogErrorTimeout, "Model discovery timed out.", "OpenCode took too long", "Verify OpenCode is installed"},
		{opencode.CatalogErrorCommandFailed, "Could not discover models from OpenCode.", "OpenCode command failed", "Verify OpenCode is installed"},
		{opencode.CatalogErrorMalformed, "Could not parse models from OpenCode.", "unexpected model catalog format", "Verify OpenCode is installed"},
		{opencode.CatalogErrorUnsupportedSchema, "Could not parse models from OpenCode.", "unexpected model catalog format", "Verify OpenCode is installed"},
		{opencode.CatalogErrorMissingBinary, "Could not discover models from OpenCode.", "Verify OpenCode is installed", ""},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
			state := ModelPickerState{CatalogStatus: RuntimeCatalogLoading}
			state = state.Update(RuntimeCatalogDiscoveryMsg{Err: &opencode.CatalogError{Kind: tt.kind}})
			got := RenderModelPicker(nil, state, 0)
			if !strings.Contains(got, tt.wantMessage) || !strings.Contains(got, tt.wantSubtext) {
				t.Fatalf("diagnostic = %q, want message %q and subtext %q", got, tt.wantMessage, tt.wantSubtext)
			}
			if tt.forbidText != "" && strings.Contains(got, tt.forbidText) {
				t.Fatalf("diagnostic %q incorrectly blames missing OpenCode binary", got)
			}
		})
	}
}

func TestModelPickerOrchestratorKeyRemainsStable(t *testing.T) {
	if SDDOrchestratorPhase != "gentle-orchestrator" {
		t.Fatalf("persisted coordinator key changed: %q", SDDOrchestratorPhase)
	}
}

func TestModeEffortSelectConstantValue(t *testing.T) {
	if ModeEffortSelect != 3 {
		t.Fatalf("ModeEffortSelect = %d, want 3", ModeEffortSelect)
	}
}

func TestFilteredModelEntriesSortsNewestFirst(t *testing.T) {
	state := ModelPickerState{SelectedProvider: "anthropic", SDDModels: map[string][]opencode.Model{"anthropic": {
		{ID: "claude-3-5-sonnet", Name: "Claude 3.5 Sonnet"},
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6"},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
	}}}
	got := FilteredModelEntries(state)
	want := []string{"claude-opus-4-6", "claude-sonnet-4-5", "claude-3-5-sonnet"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("model %d = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestFilteredModelEntriesPrefersSemanticVersionOverReleaseDate(t *testing.T) {
	state := ModelPickerState{SelectedProvider: "anthropic", SDDModels: map[string][]opencode.Model{"anthropic": {
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet 20241022"},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
	}}}
	if got := FilteredModelEntries(state); got[0].ID != "claude-sonnet-4-5" {
		t.Fatalf("first model = %q, want semantic version 4.5", got[0].ID)
	}
}

func TestFilteredModelEntriesIgnoresShortDateSuffix(t *testing.T) {
	state := ModelPickerState{SelectedProvider: "google", SDDModels: map[string][]opencode.Model{"google": {
		{ID: "gemini-2.5-pro-exp-03-25", Name: "Gemini 2.5 Pro Experimental 03-25"},
		{ID: "gemini-3-pro", Name: "Gemini 3 Pro"},
	}}}
	if got := FilteredModelEntries(state); got[0].ID != "gemini-3-pro" {
		t.Fatalf("first model = %q, want Gemini 3", got[0].ID)
	}
}

func TestFilteredModelEntriesSearchMatchesNameAndID(t *testing.T) {
	state := ModelPickerState{SelectedProvider: "openai", ModelSearch: "mini", SDDModels: map[string][]opencode.Model{"openai": {
		{ID: "gpt-5", Name: "GPT-5"}, {ID: "gpt-5-mini", Name: "GPT-5 Mini"},
	}}}
	if got := FilteredModelEntries(state); len(got) != 1 || got[0].ID != "gpt-5-mini" {
		t.Fatalf("filtered models = %v, want only gpt-5-mini", got)
	}
}

func TestRenderModelSelectShowsSearchInput(t *testing.T) {
	state := ModelPickerState{
		SelectedProvider: "openai", ModelSearch: "mini",
		Providers: map[string]opencode.Provider{"openai": {Name: "OpenAI"}},
		SDDModels: map[string][]opencode.Model{"openai": {{ID: "gpt-5-mini", Name: "GPT-5 Mini"}}},
	}
	if got := renderModelSelect(state); !strings.Contains(got, "Search: mini_") {
		t.Fatalf("search input not visible: %s", got)
	}
}

func TestModelPickerSearchBackspaceAndCtrlUClears(t *testing.T) {
	state := pickerTestState(0)
	HandleModelPickerNav("m", state, nil)
	if state.ModelSearch != "m" {
		t.Fatalf("typed search = %q, want m", state.ModelSearch)
	}
	HandleModelPickerNav("backspace", state, nil)
	if state.ModelSearch != "" {
		t.Fatalf("backspace left search %q", state.ModelSearch)
	}
	HandleModelPickerNav("b", state, nil)
	HandleModelPickerNav("ctrl+u", state, nil)
	if state.ModelSearch != "" {
		t.Fatalf("ctrl+u left search %q", state.ModelSearch)
	}
}

func TestModelPickerNoSearchMatchesCannotAssign(t *testing.T) {
	state := pickerTestState(0)
	state.ModelSearch = "no-such-model"
	_, got := HandleModelPickerNav("enter", state, nil)
	if len(got) != 0 || state.Mode != ModeModelSelect {
		t.Fatalf("empty search assigned a model: %v, mode=%d", got, state.Mode)
	}
}

func TestModelPickerModelNavigationClampsAtBounds(t *testing.T) {
	state := pickerTestState(0)
	HandleModelPickerNav("up", state, nil)
	if state.ModelCursor != 0 {
		t.Fatalf("up underflow: %d", state.ModelCursor)
	}
	HandleModelPickerNav("down", state, nil)
	HandleModelPickerNav("down", state, nil)
	if state.ModelCursor != 1 {
		t.Fatalf("down overflow: %d", state.ModelCursor)
	}
	HandleModelPickerNav("up", state, nil)
	if state.ModelCursor != 0 {
		t.Fatalf("up did not navigate: %d", state.ModelCursor)
	}
}

func TestModelPickerReasoningVariantsDeferAssignment(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Reasoning = true
	state.SDDModels["test-provider"][0].Variants = []string{"high", "low", "medium"}
	handled, assignments := HandleModelPickerNav("enter", state, nil)
	if !handled || state.Mode != ModeEffortSelect || len(assignments) != 0 {
		t.Fatalf("variant picker: handled=%t mode=%d assignments=%v", handled, state.Mode, assignments)
	}
	if state.PendingAssignment.ModelID != "model-alpha" || state.PendingAssignment.ProviderID != "test-provider" {
		t.Fatalf("pending assignment = %+v", state.PendingAssignment)
	}
}

func TestModelPickerNonReasoningModelSkipsEffortPicker(t *testing.T) {
	state := pickerTestState(0)
	handled, assignments := HandleModelPickerNav("enter", state, nil)
	if !handled || state.Mode != ModePhaseList || assignments[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("plain model selection: mode=%d assignments=%v", state.Mode, assignments)
	}
}

func TestModelPickerReasoningWithoutVariantsSkipsEffortPicker(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Reasoning = true
	_, got := HandleModelPickerNav("enter", state, nil)
	if state.Mode != ModePhaseList || got[SDDOrchestratorPhase].ModelID != "model-alpha" {
		t.Fatalf("reasoning model without variants: mode=%d assignment=%v", state.Mode, got)
	}
}

func TestModelPickerReasoningWithoutVariantsPreservesMatchingEffort(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Reasoning = true
	existing := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha", Effort: "high"}
	_, got := HandleModelPickerNav("enter", state, map[string]model.ModelAssignment{SDDOrchestratorPhase: existing})
	if got[SDDOrchestratorPhase] != existing {
		t.Fatalf("matching reasoning effort discarded: %v", got)
	}
}

func TestModelPickerReasoningWithoutVariantsClearsNonMatchingEffort(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Reasoning = true
	existing := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-beta", Effort: "medium"}
	_, got := HandleModelPickerNav("enter", state, map[string]model.ModelAssignment{SDDOrchestratorPhase: existing})
	if got[SDDOrchestratorPhase].ModelID != "model-alpha" || got[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("different model inherited stale effort: %v", got)
	}
}

func TestModelPickerNonReasoningClearsMatchingStaleEffort(t *testing.T) {
	state := pickerTestState(0)
	existing := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha", Effort: "high"}
	_, got := HandleModelPickerNav("enter", state, map[string]model.ModelAssignment{SDDOrchestratorPhase: existing})
	if got[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("plain model retained stale effort: %v", got)
	}
}

func TestModelPickerEffortSelectionAppliesAndClearsPending(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "medium", "high"}
	HandleModelPickerNav("enter", state, nil)
	state.EffortCursor = 2
	_, got := HandleModelPickerNav("enter", state, nil)
	if got[SDDOrchestratorPhase].Effort != "medium" || state.Mode != ModePhaseList {
		t.Fatalf("confirmed effort: mode=%d assignments=%v", state.Mode, got)
	}
	if state.PendingAssignment != (model.ModelAssignment{}) || state.SelectedModelEffortLevels != nil {
		t.Fatalf("pending effort not cleared: %+v", state)
	}
}

func TestModelPickerEffortDefaultMeansProviderDefault(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	HandleModelPickerNav("enter", state, nil)
	_, got := HandleModelPickerNav("enter", state, nil)
	if got[SDDOrchestratorPhase].ModelID != "model-alpha" || got[SDDOrchestratorPhase].Effort != "" {
		t.Fatalf("provider default effort = %+v", got[SDDOrchestratorPhase])
	}
}

func TestModelPickerEffortEscapeRestoresModelSelection(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	HandleModelPickerNav("enter", state, nil)
	HandleModelPickerNav("esc", state, nil)
	if state.Mode != ModeModelSelect || state.PendingAssignment != (model.ModelAssignment{}) || state.SelectedModelEffortLevels != nil {
		t.Fatalf("effort escape left pending selection: %+v", state)
	}
}

func TestModelPickerEffortCursorNavigation(t *testing.T) {
	state := pickerTestState(0)
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	HandleModelPickerNav("enter", state, nil)
	HandleModelPickerNav("j", state, nil)
	if state.EffortCursor != 1 {
		t.Fatalf("effort down cursor = %d, want 1", state.EffortCursor)
	}
	HandleModelPickerNav("k", state, nil)
	if state.EffortCursor != 0 {
		t.Fatalf("effort up cursor = %d, want 0", state.EffortCursor)
	}
}

func TestRenderModelPickerShowsOrchestratorEffort(t *testing.T) {
	state := ModelPickerState{AvailableIDs: []string{"anthropic"}, Providers: map[string]opencode.Provider{"anthropic": {
		Name: "Anthropic", Models: map[string]opencode.Model{"claude": {Name: "Claude"}},
	}}}
	assignment := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude", Effort: "high"}
	got := RenderModelPicker(map[string]model.ModelAssignment{SDDOrchestratorPhase: assignment}, state, 0)
	if !strings.Contains(got, "Anthropic / Claude [high]") {
		t.Fatalf("orchestrator effort missing: %s", got)
	}
}

func TestRenderModelPickerDoesNotAnnotateEmptyEffort(t *testing.T) {
	state := ModelPickerState{AvailableIDs: []string{"test-provider"}}
	assignment := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha"}
	got := RenderModelPicker(map[string]model.ModelAssignment{SDDOrchestratorPhase: assignment}, state, 0)
	if strings.Contains(got, "[high]") || strings.Contains(got, "[low]") || strings.Contains(got, "[medium]") {
		t.Fatalf("rendered phantom effort: %s", got)
	}
}

func TestModelPickerJudgmentDayRowsAssignIndependently(t *testing.T) {
	for _, role := range opencode.JDPhases() {
		t.Run(role, func(t *testing.T) {
			state := pickerTestState(0)
			state.SelectedPhaseIdx = pickerRowIndex(t, *state, role)
			_, got := HandleModelPickerNav("enter", state, nil)
			if got[role].ProviderID != "test-provider" || got[role].ModelID != "model-alpha" || len(got) != 1 {
				t.Fatalf("JD role assignment = %v", got)
			}
		})
	}
}

func TestModelPickerFirstAndLastJudgmentDayRows(t *testing.T) {
	roles := opencode.JDPhases()
	for _, tt := range []struct {
		name, agent string
		index       int
	}{
		{"first", roles[0], SeparatorRowIdx() + 1},
		{"last", roles[len(roles)-1], SeparatorRowIdx() + len(roles)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := pickerTestState(tt.index)
			_, got := HandleModelPickerNav("enter", state, nil)
			if got[tt.agent].ModelID != "model-alpha" || len(got) != 1 {
				t.Fatalf("JD row %d assigned %v, want only %s", tt.index, got, tt.agent)
			}
		})
	}
}

func TestModelPickerReviewRolesAssignIndependently(t *testing.T) {
	for _, role := range opencode.ReviewPhases() {
		t.Run(role, func(t *testing.T) {
			state := pickerTestState(0)
			state.SelectedPhaseIdx = pickerRowIndex(t, *state, role)
			_, got := HandleModelPickerNav("enter", state, nil)
			if got[role].ModelID != "model-alpha" || len(got) != 1 {
				t.Fatalf("RDD role assignment = %v", got)
			}
		})
	}
}

func TestModelPickerReviewRolesFollowJudgmentDay(t *testing.T) {
	rows := ModelPickerRows()
	separator := SeparatorRowIdx() + 1 + len(opencode.JDPhases())
	if rows[separator] != "--- Review agents ---" {
		t.Fatalf("review separator = %q", rows[separator])
	}
	for i, role := range opencode.ReviewPhases() {
		if rows[separator+i+1] != role {
			t.Fatalf("review row %d = %q, want %q", i, rows[separator+i+1], role)
		}
	}
}

func TestModelPickerNativeAgentRowsAreNotCustomBulkTargets(t *testing.T) {
	state := ModelPickerState{CustomAgents: []string{"custom"}}
	rows := ModelPickerRowsForStateWithIdentity(state)
	var native, customBulk int
	for i, row := range rows {
		if row.Label == "--- OpenCode native agents ---" {
			native = i
		}
		if row.Kind == ModelPickerRowKindSetAllCustom {
			customBulk = i
		}
	}
	if native == 0 || rows[native+1].AgentID != "general" || rows[native+2].AgentID != "explore" || customBulk <= native+2 {
		t.Fatalf("native and custom section order = %v", rows)
	}
	assignment := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude"}
	state.SelectedPhaseIdx = customBulk
	got := applyAssignment(state, map[string]model.ModelAssignment{"general": assignment, "explore": assignment}, model.ModelAssignment{ProviderID: "openai", ModelID: "gpt"})
	if got["general"] != assignment || got["explore"] != assignment || got["custom"].ModelID != "gpt" {
		t.Fatalf("custom bulk changed native assignments: %v", got)
	}
}

func TestModelPickerCustomSectionRowsAreDiscoverable(t *testing.T) {
	state := ModelPickerState{CustomAgents: []string{"custom-1", "custom-2"}}
	rows := ModelPickerRowsForState(state)
	for _, row := range []string{"--- Custom / Native agents ---", "Set all custom agents", "custom-1", "custom-2"} {
		if !strings.Contains(strings.Join(rows, "\n"), row) {
			t.Fatalf("custom row %q missing from %v", row, rows)
		}
	}
}

func TestModelPickerCustomBulkWithoutEffortAssignsAll(t *testing.T) {
	state := pickerTestState(0)
	state.CustomAgents = []string{"custom-1", "custom-2"}
	for i, row := range ModelPickerRowsForStateWithIdentity(*state) {
		if row.Kind == ModelPickerRowKindSetAllCustom {
			state.SelectedPhaseIdx = i
		}
	}
	_, got := HandleModelPickerNav("enter", state, nil)
	want := model.ModelAssignment{ProviderID: "test-provider", ModelID: "model-alpha"}
	if got["custom-1"] != want || got["custom-2"] != want || state.AllCustomAgentsModel != want {
		t.Fatalf("custom bulk assignment = %v, label=%+v", got, state.AllCustomAgentsModel)
	}
}

func TestModelPickerIndividualCustomEditDoesNotChangeBulkLabel(t *testing.T) {
	state := pickerTestState(0)
	state.CustomAgents = []string{"custom-1", "custom-2"}
	state.SelectedPhaseIdx = pickerRowIndex(t, *state, "custom-1")
	state.AllCustomAgentsModel = model.ModelAssignment{ProviderID: "old", ModelID: "saved"}
	_, got := HandleModelPickerNav("enter", state, nil)
	if got["custom-1"].ModelID != "model-alpha" || state.AllCustomAgentsModel.ModelID != "saved" {
		t.Fatalf("individual custom edit changed bulk label: %v, label=%v", got, state.AllCustomAgentsModel)
	}
}

func TestModelPickerCustomBulkLabelCollisionUsesAgentIdentity(t *testing.T) {
	state := pickerTestState(0)
	state.CustomAgents = []string{"Set all custom agents", "other-custom-agent"}
	state.SelectedPhaseIdx = pickerRowIndex(t, *state, "Set all custom agents")
	old := model.ModelAssignment{ProviderID: "old", ModelID: "saved"}
	_, got := HandleModelPickerNav("enter", state, map[string]model.ModelAssignment{"other-custom-agent": old})
	if got["Set all custom agents"].ModelID != "model-alpha" || got["other-custom-agent"] != old || state.AllCustomAgentsModel != (model.ModelAssignment{}) {
		t.Fatalf("custom label collision changed peer/bulk: %v, state=%+v", got, state.AllCustomAgentsModel)
	}
}

func TestModelPickerCustomBulkLabelCollisionPreservesEffortPath(t *testing.T) {
	state := pickerTestState(0)
	state.CustomAgents = []string{"Set all custom agents", "other-custom-agent"}
	state.SelectedPhaseIdx = pickerRowIndex(t, *state, "Set all custom agents")
	state.SDDModels["test-provider"][0].Variants = []string{"low", "high"}
	old := model.ModelAssignment{ProviderID: "old", ModelID: "saved"}
	assignments := map[string]model.ModelAssignment{"other-custom-agent": old}
	_, assignments = HandleModelPickerNav("enter", state, assignments)
	if state.Mode != ModeEffortSelect {
		t.Fatalf("effort picker not opened: %d", state.Mode)
	}
	_, assignments = HandleModelPickerNav("enter", state, assignments)
	if assignments["Set all custom agents"].ModelID != "model-alpha" || assignments["other-custom-agent"] != old || state.AllCustomAgentsModel != (model.ModelAssignment{}) {
		t.Fatalf("effort collision changed peer/bulk: %v, state=%+v", assignments, state.AllCustomAgentsModel)
	}
}

func TestRuntimeCatalogDiscoveryUsesActiveProject(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	var project string
	state.catalogDiscover = func(_ context.Context, dir string) (map[string]opencode.Provider, error) {
		project = dir
		return map[string]opencode.Provider{"custom": {ID: "custom", Models: map[string]opencode.Model{"qwen": {ID: "qwen", ToolCall: true}}}}, nil
	}
	state = state.Update(state.StartRuntimeCatalogDiscovery(1, "active-project")().(RuntimeCatalogDiscoveryMsg))
	if project != "active-project" || state.CatalogStatus != RuntimeCatalogReady || !reflect.DeepEqual(state.AvailableIDs, []string{"custom"}) {
		t.Fatalf("active project %q, picker status=%d ids=%v", project, state.CatalogStatus, state.AvailableIDs)
	}
	if len(state.SDDModels["custom"]) != 1 || state.SDDModels["custom"][0].ID != "qwen" {
		t.Fatalf("tool-capable model missing: %v", state.SDDModels)
	}
}

func TestRuntimeCatalogDiscoveryIgnoresLateRequestAndWrongProject(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state.CatalogRequestID = 3
	state.CatalogProjectDir = "current-project"
	for _, stale := range []RuntimeCatalogDiscoveryMsg{
		{RequestID: 2, ProjectDir: "current-project", Err: errors.New("stale")},
		{RequestID: 3, ProjectDir: "other-project", Err: errors.New("stale")},
	} {
		state = state.Update(stale)
		if state.CatalogStatus != RuntimeCatalogLoading || state.CatalogError != nil {
			t.Fatalf("stale request changed picker: %+v", state)
		}
	}
}

func TestRuntimeCatalogDiscoveryExcludesNonToolModels(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state = state.Update(RuntimeCatalogDiscoveryMsg{Providers: map[string]opencode.Provider{
		"available": {ID: "available", Models: map[string]opencode.Model{"tool": {ID: "tool", ToolCall: true}}},
		"no-tools":  {ID: "no-tools", Models: map[string]opencode.Model{"plain": {ID: "plain"}}},
	}})
	if !reflect.DeepEqual(state.AvailableIDs, []string{"available"}) || len(state.SDDModels["no-tools"]) != 0 {
		t.Fatalf("non-tool model offered: ids=%v models=%v", state.AvailableIDs, state.SDDModels)
	}
}

func TestRuntimeCatalogDiscoveryClearsPreviousErrorOnSuccess(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state = state.Update(RuntimeCatalogDiscoveryMsg{Err: errors.New("unavailable")})
	if state.CatalogStatus != RuntimeCatalogFailed || state.CatalogError == nil {
		t.Fatalf("failed discovery not recorded: %+v", state)
	}
	state = state.Update(RuntimeCatalogDiscoveryMsg{Providers: map[string]opencode.Provider{"openai": {
		ID: "openai", Models: map[string]opencode.Model{"gpt": {ID: "gpt", ToolCall: true}},
	}}})
	if state.CatalogStatus != RuntimeCatalogReady || state.CatalogError != nil {
		t.Fatalf("successful discovery kept old error: %+v", state)
	}
}

func TestRuntimeCatalogDiscoveryFailureKeepsConfiguredProvider(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state.ConfiguredProviders = map[string]opencode.Provider{"configured": {
		ID: "configured", Name: "Configured", Models: map[string]opencode.Model{"configured/model": {ID: "configured/model", ToolCall: true}},
	}}
	state = state.Update(RuntimeCatalogDiscoveryMsg{Err: errors.New("catalog unavailable")})
	if state.CatalogStatus != RuntimeCatalogReady || !reflect.DeepEqual(state.AvailableIDs, []string{"configured"}) {
		t.Fatalf("configured model unavailable after discovery failure: %+v", state)
	}
	if got := state.SDDModels["configured"]; len(got) != 1 || got[0].ID != "configured/model" {
		t.Fatalf("configured model = %+v", got)
	}
}

func TestRuntimeCatalogDiscoveryEmptyCatalogRendersActionableState(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state = state.Update(RuntimeCatalogDiscoveryMsg{Providers: map[string]opencode.Provider{}})
	got := RenderModelPicker(nil, state, 0)
	if state.CatalogStatus != RuntimeCatalogEmpty || !strings.Contains(got, "reported no tool-capable models") {
		t.Fatalf("empty catalog misreported: status=%d output=%s", state.CatalogStatus, got)
	}
}

func TestRuntimeCatalogFailureShowsFallbackWithoutPrivateFixtures(t *testing.T) {
	state := NewRuntimeModelPickerStateWithDiscoverer(filepath.Join(t.TempDir(), "missing.json"), nil)
	state = state.Update(RuntimeCatalogDiscoveryMsg{Err: errors.New("unavailable")})
	got := RenderModelPicker(nil, state, 0)
	if !strings.Contains(got, "Could not discover models from OpenCode") || strings.Contains(got, "private cache") {
		t.Fatalf("fallback diagnosis leaked private data or disappeared: %s", got)
	}
}

func TestProviderEntriesSortedByNameWithModelCounts(t *testing.T) {
	state := ModelPickerState{
		AvailableIDs: []string{"zeta", "alpha"},
		Providers:    map[string]opencode.Provider{"zeta": {Name: "Zeta"}, "alpha": {Name: "Alpha"}},
		SDDModels:    map[string][]opencode.Model{"zeta": {{ID: "one"}}, "alpha": {{ID: "one"}, {ID: "two"}}},
	}
	got := ProviderEntries(state)
	if len(got) != 2 || got[0].Name != "Alpha" || got[0].ModelCount != 2 || got[1].Name != "Zeta" || got[1].ModelCount != 1 {
		t.Fatalf("provider entries = %+v", got)
	}
}
