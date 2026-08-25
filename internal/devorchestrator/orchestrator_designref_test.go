package devorchestrator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/design"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/router"
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

// TestGenerateContextForAgent_DesignRefNonInterference covers the spec's
// "Non-Interference Invariants" requirement end to end (folding in the
// integration scenario dropped from S2a for budget reasons): (a) an artifact
// with db_impact: set and no design_ref: must resolve DBImpact and skills
// exactly as before this change, with no figma-analyzer skill and no
// design_ref line rendered; (b) an artifact with design_ref: on tasks.md for
// frontend-implementer must resolve figma-analyzer in skills AND render
// design_ref: in the final prompt, without disturbing db_impact.
func TestGenerateContextForAgent_DesignRefNonInterference(t *testing.T) {
	setupSkills := func(t *testing.T, tempDir string, names ...string) {
		t.Helper()
		for _, name := range names {
			skillDir := filepath.Join(tempDir, "skills", name)
			if err := os.MkdirAll(skillDir, 0755); err != nil {
				t.Fatalf("MkdirAll(%s) error = %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(name+" skill"), 0644); err != nil {
				t.Fatalf("WriteFile(%s) error = %v", name, err)
			}
		}
	}

	t.Run("db_impact only: unaffected by this change", func(t *testing.T) {
		tempDir := t.TempDir()
		orch := New(tempDir)
		setupSkills(t, tempDir, "frontend-implementer")

		artifactContent := "---\nid: feature-901\ndb_impact: simple\n---\n# Task content\n"
		artifactPath := "openspec/changes/feature-901/task.md"
		absArtifactPath := filepath.Join(tempDir, artifactPath)
		if err := os.MkdirAll(filepath.Dir(absArtifactPath), 0755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(absArtifactPath, []byte(artifactContent), 0644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}

		pkg, err := orch.GenerateContextForAgent(
			"EXEC-NON-INTERFERENCE-1",
			"frontend-implementer",
			artifactPath,
			nil,
			"",
			[]string{"frontend-implementer"},
			"COMMIT",
			"APPLY-901",
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pkg.DBImpact != "simple" {
			t.Errorf("expected DBImpact = %q, got %q", "simple", pkg.DBImpact)
		}
		if pkg.DesignRef != "" {
			t.Errorf("expected DesignRef = %q, got %q", "", pkg.DesignRef)
		}
		for _, s := range pkg.Skills {
			if strings.Contains(s, "figma-analyzer") {
				t.Errorf("expected no figma-analyzer skill without design_ref, got skills: %v", pkg.Skills)
			}
		}

		out, err := router.FormatPromptSignature("Do work.", pkg)
		if err != nil {
			t.Fatalf("FormatPromptSignature error = %v", err)
		}
		if strings.Contains(out, "design_ref") {
			t.Errorf("expected no design_ref substring anywhere in rendered prompt, got: %s", out)
		}
	})

	t.Run("design_ref present: skill injected and rendered, db_impact untouched", func(t *testing.T) {
		tempDir := t.TempDir()
		orch := New(tempDir)
		setupSkills(t, tempDir, "frontend-implementer", "figma-analyzer", "database-specialist")

		artifactContent := "---\nid: feature-902\ndb_impact: high-risk\ndesign_ref: https://www.figma.com/design/ABC12345XY\n---\n# Task content\n"
		artifactPath := "openspec/changes/feature-902/task.md"
		absArtifactPath := filepath.Join(tempDir, artifactPath)
		if err := os.MkdirAll(filepath.Dir(absArtifactPath), 0755); err != nil {
			t.Fatalf("MkdirAll error = %v", err)
		}
		if err := os.WriteFile(absArtifactPath, []byte(artifactContent), 0644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}

		pkg, err := orch.GenerateContextForAgent(
			"EXEC-NON-INTERFERENCE-2",
			"frontend-implementer",
			artifactPath,
			nil,
			"",
			[]string{"frontend-implementer"},
			"COMMIT",
			"APPLY-902",
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pkg.DBImpact != "high-risk" {
			t.Errorf("expected DBImpact = %q, got %q", "high-risk", pkg.DBImpact)
		}
		if pkg.DesignRef != "https://www.figma.com/design/ABC12345XY" {
			t.Errorf("expected DesignRef = %q, got %q", "https://www.figma.com/design/ABC12345XY", pkg.DesignRef)
		}

		hasFigma := false
		for _, s := range pkg.Skills {
			if strings.Contains(s, "figma-analyzer") {
				hasFigma = true
			}
		}
		if !hasFigma {
			t.Errorf("expected figma-analyzer to be resolved when design_ref is present, got skills: %v", pkg.Skills)
		}

		out, err := router.FormatPromptSignature("Do work.", pkg)
		if err != nil {
			t.Fatalf("FormatPromptSignature error = %v", err)
		}
		if !strings.Contains(out, "design_ref: https://www.figma.com/design/ABC12345XY") {
			t.Errorf("expected rendered prompt to contain design_ref line, got: %s", out)
		}
		if !strings.Contains(out, "db_impact: high-risk") {
			t.Errorf("expected rendered prompt to still contain db_impact line unaffected by design_ref, got: %s", out)
		}
	})
}
