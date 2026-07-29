package model

import "testing"

// The maintainer's preset matrix. Sol reasons, Terra writes, Luna transcribes:
// strong lanes receive blobs and reason over delivered context, mid lanes write
// code in an agentic loop where effort dominates, cheap lanes do structured
// transcription where cost per task dominates.
func TestCodexPresetMatrixMatchesTheMaintainerTable(t *testing.T) {
	for _, testCase := range []struct {
		preset       CodexPresetKey
		orchestrator CodexCarrilDefault
		strong       CodexCarrilDefault
		mid          CodexCarrilDefault
		cheap        CodexCarrilDefault
	}{
		{
			preset:       CodexPresetLowCost,
			orchestrator: CodexCarrilDefault{Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
			strong:       CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
			mid:          CodexCarrilDefault{Model: "gpt-5.6-terra", Effort: CodexEffortMedium},
			cheap:        CodexCarrilDefault{Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
		},
		{
			preset:       CodexPresetRecommended,
			orchestrator: CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
			strong:       CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
			mid:          CodexCarrilDefault{Model: "gpt-5.6-terra", Effort: CodexEffortHigh},
			cheap:        CodexCarrilDefault{Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
		},
		{
			preset:       CodexPresetPowerful,
			orchestrator: CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortMedium},
			strong:       CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortXHigh},
			mid:          CodexCarrilDefault{Model: "gpt-5.6-sol", Effort: CodexEffortHigh},
			cheap:        CodexCarrilDefault{Model: "gpt-5.6-luna", Effort: CodexEffortHigh},
		},
	} {
		t.Run(string(testCase.preset), func(t *testing.T) {
			carriles := CodexPresetCarrilDefaults(string(testCase.preset))
			for lane, want := range map[string]CodexCarrilDefault{
				"sdd-strong": testCase.strong, "sdd-mid": testCase.mid, "sdd-cheap": testCase.cheap,
			} {
				if got := carriles[lane]; got != want {
					t.Errorf("%s = %+v, want %+v", lane, got, want)
				}
			}
			orchestrator := CodexPresetOrchestratorAssignment(string(testCase.preset))
			if orchestrator.Model != testCase.orchestrator.Model || orchestrator.Effort != testCase.orchestrator.Effort {
				t.Errorf("orchestrator = %+v, want %+v", *orchestrator, testCase.orchestrator)
			}
		})
	}
}
