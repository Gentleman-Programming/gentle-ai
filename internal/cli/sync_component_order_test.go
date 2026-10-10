package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// TestSyncStagePlanAppliesSoftOrderingToPersistedSelection pins the sync-side
// half of the ordering invariant. install reorders its selection through
// planner.NewResolver(...).Resolve, but sync rehydrates the persisted order from
// state.json (RestorePersistedSelection), where Persona sits LAST. stagePlan
// must schedule Persona before Engram and before SDD; otherwise a
// StrategyFileReplace agent's prompt file is rewritten from the bare persona
// asset and the managed engram/sdd sections are destroyed.
func TestSyncStagePlanAppliesSoftOrderingToPersistedSelection(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Components: []model.ComponentID{
		model.ComponentEngram,
		model.ComponentSDD,
		model.ComponentSkills,
		model.ComponentContext7,
		model.ComponentPermission,
		model.ComponentGGA,
		model.ComponentClaudeTheme,
		model.ComponentOpenCodeGentleLogo,
		model.ComponentPersona,
	}}

	rt, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatalf("newSyncRuntime() error = %v", err)
	}

	const prefix = "sync:component:"
	var ordered []model.ComponentID
	for _, step := range rt.stagePlan().Apply {
		if id := step.ID(); strings.HasPrefix(id, prefix) {
			ordered = append(ordered, model.ComponentID(strings.TrimPrefix(id, prefix)))
		}
	}

	indexOf := func(target model.ComponentID) int {
		for i, component := range ordered {
			if component == target {
				return i
			}
		}
		return -1
	}

	personaIdx := indexOf(model.ComponentPersona)
	engramIdx := indexOf(model.ComponentEngram)
	sddIdx := indexOf(model.ComponentSDD)
	if personaIdx < 0 || engramIdx < 0 || sddIdx < 0 {
		t.Fatalf("stagePlan() emitted components %v, want persona, engram and sdd present", ordered)
	}
	if personaIdx > engramIdx {
		t.Fatalf("stagePlan() scheduled persona (%d) after engram (%d): %v", personaIdx, engramIdx, ordered)
	}
	if personaIdx > sddIdx {
		t.Fatalf("stagePlan() scheduled persona (%d) after sdd (%d): %v", personaIdx, sddIdx, ordered)
	}
}
