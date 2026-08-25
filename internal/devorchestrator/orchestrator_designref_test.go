package devorchestrator

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/design"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/skill"
)

// TestFigmaDesignSkills covers the spec's "Closed Agent Gate for Design
// Skill Injection" requirement: figma-analyzer is injected only when a
// design reference is present AND the requesting agent is in a closed set
// (frontend-implementer, solution-architect). backend-implementer and
// database-specialist are real registered agents (internal/assets/claude/
// agents/*.md) used here as valid negative fixtures -- neither is gated in.
func TestFigmaDesignSkills(t *testing.T) {
	present := design.Ref{FileKey: "ABC12345XY"}
	absent := design.Ref{}

	tests := []struct {
		name      string
		ref       design.Ref
		agentName string
		want      []string
	}{
		{"present/frontend-implementer adds figma-analyzer", present, "frontend-implementer", []string{"figma-analyzer"}},
		{"present/solution-architect adds figma-analyzer", present, "solution-architect", []string{"figma-analyzer"}},
		{"present/backend-implementer adds nothing", present, "backend-implementer", nil},
		{"present/database-specialist adds nothing", present, "database-specialist", nil},
		{"absent/frontend-implementer adds nothing", absent, "frontend-implementer", nil},
		{"absent/solution-architect adds nothing", absent, "solution-architect", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := figmaDesignSkills(tt.ref, tt.agentName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("figmaDesignSkills(%+v, %q) = %v, want %v", tt.ref, tt.agentName, got, tt.want)
			}
		})
	}
}

// TestFigmaDesignSkillsResolveOnDisk mirrors TestDBImpactSkillsResolveOnDisk:
// it enumerates every (ref-state, agentName) combination figmaDesignSkills
// handles, collects every skill name it can ever emit, and resolves each one
// against the REAL workspace skills/ directory using skill.Resolver -- the
// exact production lookup code. A future dangling skill reference fails this
// build-time test instead of surfacing as a runtime hard-fail after the CLI
// wiring lands.
func TestFigmaDesignSkillsResolveOnDisk(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, "skills")); statErr != nil {
		t.Fatalf("resolved repo root %q has no skills/ directory: %v", repoRoot, statErr)
	}

	refs := []design.Ref{{}, {FileKey: "ABC12345XY"}}
	agents := []string{"frontend-implementer", "solution-architect", "backend-implementer", "database-specialist"}

	seen := map[string]bool{}
	var allSkills []string
	for _, ref := range refs {
		for _, agentName := range agents {
			for _, s := range figmaDesignSkills(ref, agentName) {
				if !seen[s] {
					seen[s] = true
					allSkills = append(allSkills, s)
				}
			}
		}
	}

	if len(allSkills) == 0 {
		t.Fatal("figmaDesignSkills emitted no skills across any (ref, agentName) combination -- this guard is vacuous; update it alongside the matrix")
	}

	resolver := skill.New(repoRoot)
	for _, s := range allSkills {
		if _, resolveErr := resolver.Resolve([]string{s}); resolveErr != nil {
			t.Errorf("figmaDesignSkills can emit %q, which does not resolve under the real workspace skills/ directory: %v", s, resolveErr)
		}
	}
}
