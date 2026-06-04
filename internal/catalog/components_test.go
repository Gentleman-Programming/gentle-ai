package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestMVPComponents(t *testing.T) {
	comps := MVPComponents()

	if len(comps) != 10 {
		t.Fatalf("MVPComponents() returned %d components, want 10", len(comps))
	}

	expectedIDs := map[model.ComponentID]bool{
		model.ComponentEngram:             true,
		model.ComponentSDD:                true,
		model.ComponentSkills:             true,
		model.ComponentContext7:           true,
		model.ComponentPersona:            true,
		model.ComponentPermission:         true,
		model.ComponentGGA:                true,
		model.ComponentTheme:              true,
		model.ComponentClaudeTheme:        true,
		model.ComponentOpenCodeGentleLogo: true,
	}

	for _, comp := range comps {
		if !expectedIDs[comp.ID] {
			t.Errorf("Unexpected Component ID: %q", comp.ID)
		}
		if comp.Name == "" {
			t.Errorf("Component ID %q has empty Name", comp.ID)
		}
		if comp.Description == "" {
			t.Errorf("Component ID %q has empty Description", comp.ID)
		}
	}
}
