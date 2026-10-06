package screens_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

var codexFastTier = []model.CodexServiceTier{{ID: "priority", Name: "Fast"}}

// codexRuntimeCapabilities mirrors issue #2218: Sol advertises ultra, Luna
// stops at max, and both advertise the priority (Fast) service tier.
func codexRuntimeCapabilities() map[string]model.CodexModelCapabilities {
	return map[string]model.CodexModelCapabilities{
		"gpt-5.6-sol": {
			Efforts:      []model.CodexEffort{model.CodexEffortLow, model.CodexEffortMedium, model.CodexEffortHigh, model.CodexEffortXHigh, model.CodexEffortMax, model.CodexEffortUltra},
			ServiceTiers: codexFastTier, ServiceTiersReported: true,
		},
		"gpt-5.6-luna": {
			Efforts:      []model.CodexEffort{model.CodexEffortLow, model.CodexEffortMedium, model.CodexEffortHigh, model.CodexEffortXHigh, model.CodexEffortMax},
			ServiceTiers: codexFastTier, ServiceTiersReported: true,
		},
	}
}

func openCodexEffortSelect(t *testing.T, modelID string) screens.CodexModelPickerState {
	t.Helper()
	state := screens.NewCodexModelPickerState()
	screens.HandleCodexModelPickerNav("enter", &state, 3) // Custom
	if state.ModelCapabilities != nil {
		t.Fatal("Custom entry kept capabilities from a previous discovery")
	}
	state.ModelCapabilities = codexRuntimeCapabilities() // discovery result
	screens.HandleCodexCustomNav("enter", &state, 0)     // first role
	for _, r := range modelID {
		screens.HandleCodexCustomNav(string(r), &state, 0)
	}
	screens.HandleCodexCustomNav("enter", &state, 0)
	if state.CustomMode != screens.CodexCustomModeEffortSelect || state.CustomPendingModel != modelID {
		t.Fatalf("effort select not reached for %s: mode=%v pending=%q", modelID, state.CustomMode, state.CustomPendingModel)
	}
	return state
}

func renderedEfforts(state screens.CodexModelPickerState) []string {
	var efforts []string
	for _, line := range strings.Split(screens.RenderCodexModelPicker(state, 0), "\n") {
		label := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "▸"))
		if model.CodexEffort(label).Valid() {
			efforts = append(efforts, label)
		}
	}
	return efforts
}

func TestCodexCustomEffortsFollowRuntimeCapabilitiesPerModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    []string
	}{
		{"gpt-5.6-sol", []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		{"gpt-5.6-luna", []string{"low", "medium", "high", "xhigh", "max"}},
		// No runtime capabilities: curated fallback, never max/ultra.
		{"gpt-5.5", []string{"low", "medium", "high", "xhigh"}},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			state := openCodexEffortSelect(t, tt.modelID)
			if got := renderedEfforts(state); !slices.Equal(got, tt.want) {
				t.Fatalf("rendered efforts = %v, want %v", got, tt.want)
			}
			if got := screens.CodexModelPickerOptionCount(state); got != len(tt.want) {
				t.Fatalf("option count = %d, want %d", got, len(tt.want))
			}
			for range len(tt.want) + 2 { // clamps at the last advertised effort
				screens.HandleCodexCustomNav("down", &state, 0)
			}
			screens.HandleCodexCustomNav("enter", &state, 0)
			role := state.CustomAssignments["jd-judge-a"]
			if role.ModelID != tt.modelID || string(role.Effort) != tt.want[len(tt.want)-1] {
				t.Fatalf("assignment = %+v, want %s/%s", role, tt.modelID, tt.want[len(tt.want)-1])
			}
		})
	}
}

func codexPresetCapabilities(tiers []model.CodexServiceTier) map[string]model.CodexModelCapabilities {
	orchestrator := model.CodexPresetOrchestratorAssignment(string(screens.CodexPresetRecommended)).Model
	return map[string]model.CodexModelCapabilities{
		orchestrator: {Efforts: []model.CodexEffort{model.CodexEffortMedium}, ServiceTiers: tiers, ServiceTiersReported: true},
	}
}

func TestCodexPresetOffersServiceTiersAdvertisedForOrchestratorModel(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.ModelCapabilities = codexPresetCapabilities(codexFastTier)
	state.ServiceTier = "priority" // persisted selection

	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 1) // Recommended
	if !handled || assignments != nil || state.CustomMode != screens.CodexCustomModeServiceTier {
		t.Fatalf("preset did not open the service tier step: handled=%v assignments=%v mode=%v", handled, assignments, state.CustomMode)
	}
	if state.ServiceTierCursor != 1 {
		t.Fatalf("ServiceTierCursor = %d, want persisted priority pre-selected (1)", state.ServiceTierCursor)
	}
	view := screens.RenderCodexModelPicker(state, 0)
	for _, want := range []string{"Standard", "Fast", `service_tier = "priority"`} {
		if !strings.Contains(view, want) {
			t.Fatalf("service tier step missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "max") || strings.Contains(view, "reasoning") {
		t.Fatalf("service tier step must not present Fast as a reasoning effort:\n%s", view)
	}

	screens.HandleCodexModelPickerNav("up", &state, 0)
	handled, assignments = screens.HandleCodexModelPickerNav("enter", &state, 0)
	if !handled || assignments == nil {
		t.Fatalf("tier confirm did not return preset assignments: handled=%v", handled)
	}
	if state.ServiceTier != "" || state.Preset != screens.CodexPresetRecommended || state.CustomMode != screens.CodexCustomModeNone {
		t.Fatalf("after Standard: tier=%q preset=%q mode=%v", state.ServiceTier, state.Preset, state.CustomMode)
	}
}

func TestCodexPresetServiceTierStepSelectsAndEscapes(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.Preset = screens.CodexPresetLowCost
	state.ModelCapabilities = codexPresetCapabilities(codexFastTier)

	screens.HandleCodexModelPickerNav("enter", &state, 1)
	if state.ServiceTierCursor != 0 {
		t.Fatalf("ServiceTierCursor = %d, want Standard pre-selected", state.ServiceTierCursor)
	}
	if handled, assignments := screens.HandleCodexModelPickerNav("esc", &state, 0); !handled || assignments != nil {
		t.Fatalf("esc handled=%v assignments=%v, want handled without assignments", handled, assignments)
	}
	if state.CustomMode != screens.CodexCustomModeNone || state.Preset != screens.CodexPresetLowCost || state.ServiceTier != "" {
		t.Fatalf("esc changed selection: mode=%v preset=%q tier=%q", state.CustomMode, state.Preset, state.ServiceTier)
	}

	screens.HandleCodexModelPickerNav("enter", &state, 1)
	screens.HandleCodexModelPickerNav("down", &state, 0)
	screens.HandleCodexModelPickerNav("down", &state, 0) // clamps at the last tier
	if _, assignments := screens.HandleCodexModelPickerNav("enter", &state, 0); assignments == nil {
		t.Fatal("tier confirm returned no assignments")
	}
	if state.ServiceTier != "priority" || state.Preset != screens.CodexPresetRecommended {
		t.Fatalf("tier=%q preset=%q, want priority/recommended", state.ServiceTier, state.Preset)
	}
}

func TestCodexPresetWithoutAdvertisedTiersConfirmsImmediately(t *testing.T) {
	tests := []struct {
		name         string
		capabilities map[string]model.CodexModelCapabilities
		wantTier     string
	}{
		// The runtime reports the orchestrator model without tiers: nothing to offer.
		{"runtime advertises none", codexPresetCapabilities(nil), ""},
		// Discovery unavailable: keep the persisted selection untouched.
		{"capabilities unknown", nil, "priority"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := screens.NewCodexModelPickerState()
			state.ModelCapabilities = tt.capabilities
			state.ServiceTier = "priority"
			handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 1)
			if !handled || assignments == nil || state.CustomMode != screens.CodexCustomModeNone {
				t.Fatalf("preset did not confirm immediately: handled=%v assignments=%v mode=%v", handled, assignments, state.CustomMode)
			}
			if state.ServiceTier != tt.wantTier {
				t.Fatalf("ServiceTier = %q, want %q", state.ServiceTier, tt.wantTier)
			}
		})
	}
}

// TestCodexCustomEffortCursorClampsToShorterAdvertisedList covers async
// discovery shortening the effort list after the cursor moved on the curated one.
func TestCodexCustomEffortCursorClampsToShorterAdvertisedList(t *testing.T) {
	state := openCodexEffortSelect(t, "gpt-5.5")
	for range 3 {
		screens.HandleCodexCustomNav("down", &state, 0) // curated xhigh
	}
	state.ModelCapabilities["gpt-5.5"] = model.CodexModelCapabilities{Efforts: []model.CodexEffort{model.CodexEffortLow, model.CodexEffortMedium}}

	screens.HandleCodexCustomNav("enter", &state, 0)
	if got := state.CustomAssignments["jd-judge-a"]; got.Effort != model.CodexEffortMedium {
		t.Fatalf("assignment = %+v, want clamped medium", got)
	}
}

// TestCodexPresetPreservesTierWhenRuntimeOmitsServiceTiers covers Codex
// builds that predate service_tiers: unreported tiers are unknown, not none.
func TestCodexPresetPreservesTierWhenRuntimeOmitsServiceTiers(t *testing.T) {
	orchestrator := model.CodexPresetOrchestratorAssignment(string(screens.CodexPresetRecommended)).Model
	state := screens.NewCodexModelPickerState()
	state.ModelCapabilities = map[string]model.CodexModelCapabilities{
		orchestrator: {Efforts: []model.CodexEffort{model.CodexEffortMedium}},
	}
	state.ServiceTier = "priority"
	if _, assignments := screens.HandleCodexModelPickerNav("enter", &state, 1); assignments == nil {
		t.Fatal("preset did not confirm")
	}
	if state.ServiceTier != "priority" {
		t.Fatalf("ServiceTier = %q, want preserved priority", state.ServiceTier)
	}
}
