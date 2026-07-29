package model

import "testing"

func TestPhaseToCarrilModel_UnknownPhaseUsesRecommendedStrongDefault(t *testing.T) {
	want := codexPresetMatrix[CodexPresetRecommended]["sdd-strong"].Model
	if got := phaseToCarrilModel("unknown-phase", nil); got != want {
		t.Errorf("phaseToCarrilModel(unknown) = %q, want %q", got, want)
	}
}

func TestPhaseToCarrilModel_KnownPhaseUsesCarrilOverride(t *testing.T) {
	carrilModels := map[string]string{"sdd-mid": "gpt-5.5"}
	if got := phaseToCarrilModel("sdd-apply", carrilModels); got != "gpt-5.5" {
		t.Errorf("phaseToCarrilModel(sdd-apply) = %q, want gpt-5.5", got)
	}
}
