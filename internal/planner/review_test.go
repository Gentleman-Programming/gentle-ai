package planner

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestBuildReviewPayloadIncludesPlatformDecision(t *testing.T) {
	selection := model.Selection{
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
	}

	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
		PlatformDecision: PlatformDecision{
			OS:             "linux",
			LinuxDistro:    "arch",
			PackageManager: "pacman",
			Supported:      true,
		},
	}

	payload := BuildReviewPayload(selection, resolved)

	if !reflect.DeepEqual(payload.PlatformDecision, resolved.PlatformDecision) {
		t.Fatalf("platform decision = %#v, want %#v", payload.PlatformDecision, resolved.PlatformDecision)
	}
}

// --- Batch E: Platform decision propagation matrix ---

func TestPlatformDecisionFromProfileMatrix(t *testing.T) {
	tests := []struct {
		name    string
		profile system.PlatformProfile
		want    PlatformDecision
	}{
		{
			name: "darwin profile maps to brew decision",
			profile: system.PlatformProfile{
				OS: "darwin", PackageManager: "brew", Supported: true,
			},
			want: PlatformDecision{
				OS: "darwin", PackageManager: "brew", Supported: true,
			},
		},
		{
			name: "ubuntu profile maps to apt decision",
			profile: system.PlatformProfile{
				OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", Supported: true,
			},
			want: PlatformDecision{
				OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", Supported: true,
			},
		},
		{
			name: "arch profile maps to pacman decision",
			profile: system.PlatformProfile{
				OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman", Supported: true,
			},
			want: PlatformDecision{
				OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman", Supported: true,
			},
		},
		{
			name: "fedora profile maps to dnf decision",
			profile: system.PlatformProfile{
				OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", Supported: true,
			},
			want: PlatformDecision{
				OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", Supported: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlatformDecisionFromProfile(tc.profile)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PlatformDecisionFromProfile() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBuildReviewPayloadPlatformDecisionPropagatesPerProfile(t *testing.T) {
	profiles := []PlatformDecision{
		{OS: "darwin", PackageManager: "brew", Supported: true},
		{OS: "linux", LinuxDistro: "ubuntu", PackageManager: "apt", Supported: true},
		{OS: "linux", LinuxDistro: "arch", PackageManager: "pacman", Supported: true},
		{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", Supported: true},
	}

	for _, decision := range profiles {
		t.Run(decision.OS+"/"+decision.LinuxDistro, func(t *testing.T) {
			selection := model.Selection{
				Persona: model.PersonaGentleman,
				Preset:  model.PresetFullGentleman,
			}
			resolved := ResolvedPlan{
				Agents:           []model.AgentID{model.AgentClaudeCode},
				PlatformDecision: decision,
			}

			payload := BuildReviewPayload(selection, resolved)
			if !reflect.DeepEqual(payload.PlatformDecision, decision) {
				t.Fatalf("review payload platform decision = %#v, want %#v", payload.PlatformDecision, decision)
			}
		})
	}
}

// ─── Issue #145: ReviewPayload must include Skills ────────────────────────────

// TestBuildReviewPayloadIncludesSkills verifies that BuildReviewPayload populates
// Skills from selection.Skills.
//
// Closes #145.
func TestBuildReviewPayloadIncludesSkills(t *testing.T) {
	selection := model.Selection{
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		Skills:  []model.SkillID{"sdd-apply", "sdd-spec", "go-testing"},
	}
	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram, model.ComponentSkills},
	}

	payload := BuildReviewPayload(selection, resolved)

	if len(payload.Skills) != 3 {
		t.Fatalf("Skills len = %d, want 3; got %v", len(payload.Skills), payload.Skills)
	}
	if payload.Skills[0] != "sdd-apply" {
		t.Errorf("Skills[0] = %q, want %q", payload.Skills[0], "sdd-apply")
	}
	if payload.Skills[2] != "go-testing" {
		t.Errorf("Skills[2] = %q, want %q", payload.Skills[2], "go-testing")
	}
}

// TestBuildReviewPayloadSkillsNilWhenNotSelected verifies that Skills is nil/empty
// when no skills are in the selection.
//
// Closes #145.
func TestBuildReviewPayloadSkillsNilWhenNotSelected(t *testing.T) {
	selection := model.Selection{
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		// Skills not set
	}
	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
	}

	payload := BuildReviewPayload(selection, resolved)

	if len(payload.Skills) != 0 {
		t.Errorf("Skills = %v, want nil/empty when no skills selected", payload.Skills)
	}
}

// ─── Issue #149: ReviewPayload must include StrictTDD and HasSDD ─────────────

// TestBuildReviewPayloadIncludesStrictTDD verifies that BuildReviewPayload sets
// StrictTDD from selection.StrictTDD and HasSDD when SDD component is present.
//
// Closes #149.
func TestBuildReviewPayloadIncludesStrictTDD(t *testing.T) {
	selection := model.Selection{
		Persona:   model.PersonaGentleman,
		Preset:    model.PresetFullGentleman,
		StrictTDD: true,
	}
	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
	}

	payload := BuildReviewPayload(selection, resolved)

	if !payload.StrictTDD {
		t.Errorf("StrictTDD = false, want true (selection.StrictTDD=true)")
	}
	if !payload.HasSDD {
		t.Errorf("HasSDD = false, want true (SDD component is in resolved plan)")
	}
}

// TestBuildReviewPayloadStrictTDDFalseWhenDisabled verifies that StrictTDD=false
// is correctly propagated.
//
// Closes #149.
func TestBuildReviewPayloadStrictTDDFalseWhenDisabled(t *testing.T) {
	selection := model.Selection{
		Persona:   model.PersonaGentleman,
		Preset:    model.PresetFullGentleman,
		StrictTDD: false,
	}
	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
	}

	payload := BuildReviewPayload(selection, resolved)

	if payload.StrictTDD {
		t.Errorf("StrictTDD = true, want false (selection.StrictTDD=false)")
	}
	if !payload.HasSDD {
		t.Errorf("HasSDD = false, want true (SDD component is in resolved plan)")
	}
}

// TestBuildReviewPayloadHasSDDFalseWithoutSDDComponent verifies that HasSDD=false
// when SDD is not in the resolved plan components.
//
// Closes #149.
func TestBuildReviewPayloadHasSDDFalseWithoutSDDComponent(t *testing.T) {
	selection := model.Selection{
		Persona:   model.PersonaGentleman,
		Preset:    model.PresetFullGentleman,
		StrictTDD: true,
	}
	resolved := ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram}, // no SDD
	}

	payload := BuildReviewPayload(selection, resolved)

	if payload.HasSDD {
		t.Errorf("HasSDD = true, want false (SDD not in resolved components)")
	}
}

func TestResolverOutputIsPlatformAgnostic(t *testing.T) {
	// Planner resolver does NOT set PlatformDecision — it is set by CLI after resolve.
	// This test confirms resolver output has zero-value PlatformDecision.
	resolver := NewResolver(MVPGraph())
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentEngram},
	}

	plan, err := resolver.Resolve(selection)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	zero := PlatformDecision{}
	if plan.PlatformDecision != zero {
		t.Fatalf("resolver should not set PlatformDecision, got %#v", plan.PlatformDecision)
	}
}
