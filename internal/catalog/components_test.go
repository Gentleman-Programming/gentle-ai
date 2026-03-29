package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestMVPComponents(t *testing.T) {
	components := MVPComponents()

	if len(components) == 0 {
		t.Fatal("MVPComponents() returned empty list")
	}

	// Verify all expected components are present
	expected := map[model.ComponentID]bool{
		model.ComponentEngram:     false,
		model.ComponentSDD:        false,
		model.ComponentSkills:     false,
		model.ComponentContext7:   false,
		model.ComponentPersona:    false,
		model.ComponentPermission: false,
		model.ComponentGGA:        false,
		model.ComponentRTK:        false,
		model.ComponentTheme:      false,
	}

	for _, c := range components {
		if _, ok := expected[c.ID]; ok {
			expected[c.ID] = true
		} else {
			t.Errorf("unexpected component ID: %s", c.ID)
		}
	}

	for id, found := range expected {
		if !found {
			t.Errorf("missing expected component: %s", id)
		}
	}
}

func TestRTKComponentDetails(t *testing.T) {
	components := MVPComponents()

	var rtk Component
	for _, c := range components {
		if c.ID == model.ComponentRTK {
			rtk = c
			break
		}
	}

	if rtk.ID != model.ComponentRTK {
		t.Fatal("RTK component not found in MVPComponents()")
	}

	if rtk.Name != "RTK" {
		t.Errorf("RTK.Name = %q, want %q", rtk.Name, "RTK")
	}

	if rtk.Description == "" {
		t.Error("RTK.Description is empty")
	}
}
