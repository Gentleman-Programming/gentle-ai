package devorchestrator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/db"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/skill"
)

// TestDBImpactSkills covers H-05 (design decision D6, corrected): the DB
// router must act on db.ImpactHighRisk distinctly from db.ImpactSimple via a
// declarative (impact, agentName) matrix, replacing the single hardcoded
// `impact == ImpactSimple && agentName == "backend-implementer"` check.
//
// Correction (post-review, see apply-progress design-deviation note): the
// original check injected database-specialist for db.ImpactSimple, which is
// backwards -- skills/agents/database-specialist/SKILL.md describes itself
// as handling "complex database migrations, schema changes, and high-risk DB
// tasks", not simple ones. The pre-existing bug wired the specialist to the
// low-risk case and left the genuinely high-risk case with nothing. The
// matrix below moves database-specialist to db.ImpactHighRisk only, which
// both matches what the skill is actually for and makes the resolved skill
// set for backend-implementer differ measurably between db.ImpactSimple
// (nil) and db.ImpactHighRisk ({database-specialist}) -- satisfying spec
// H-05's first scenario, which the prior revision violated by returning an
// identical set for both impacts.
//
// frontend-implementer at high-risk also resolves to {database-specialist}
// rather than an invented "frontend-schema-impact" skill: that name had no
// corresponding skills/**/frontend-schema-impact/SKILL.md anywhere in the
// workspace, so it was a dangling reference that would only surface as a
// runtime hard-fail (per H-04) once a live CLI dispatch reached it. Only
// skills that exist on disk are named here; see
// TestDBImpactSkillsResolveOnDisk below for the guard that enforces this
// for every future change to the matrix.
func TestDBImpactSkills(t *testing.T) {
	tests := []struct {
		name      string
		impact    db.Impact
		agentName string
		want      []string
	}{
		{"none/backend-implementer adds nothing", db.ImpactNone, "backend-implementer", nil},
		{"none/frontend-implementer adds nothing", db.ImpactNone, "frontend-implementer", nil},
		{"simple/backend-implementer adds nothing (database-specialist is for high-risk, not simple)", db.ImpactSimple, "backend-implementer", nil},
		{"simple/frontend-implementer adds nothing", db.ImpactSimple, "frontend-implementer", nil},
		{"high-risk/backend-implementer adds database-specialist", db.ImpactHighRisk, "backend-implementer", []string{"database-specialist"}},
		{"high-risk/frontend-implementer adds database-specialist", db.ImpactHighRisk, "frontend-implementer", []string{"database-specialist"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dbImpactSkills(tt.impact, tt.agentName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dbImpactSkills(%q, %q) = %v, want %v", tt.impact, tt.agentName, got, tt.want)
			}
		})
	}
}

// TestGenerateContextForAgent_HighRiskFrontendSchemaImpact is the integration
// seam for H-05's frontend-implementer scenario: a real high-risk DB impact
// artifact routed to frontend-implementer must resolve database-specialist
// and the rendered DBImpact field must reflect "high-risk". This is the case
// the pre-existing single `if impact == ImpactSimple && agentName ==
// "backend-implementer"` could never satisfy, since it never considered
// frontend-implementer at all.
//
// Correction (post-review): this test previously also required a
// "frontend-schema-impact" skill that had no matching content anywhere under
// the real workspace skills/ tree -- a dangling reference. The
// frontend-implementer high-risk scenario now resolves only to
// database-specialist (a real skill), while the schema-impact-specific
// branch required by spec H-05 is the rendered `db_impact: high-risk` field
// asserted below, matching design decision D6's own rationale ("the frontend
// implementer is told db_impact: high-risk").
func TestGenerateContextForAgent_HighRiskFrontendSchemaImpact(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	artifactContent := "---\nid: feature-900\ndb_impact: high-risk\n---\n# Proposal content\n"
	artifactPath := "openspec/changes/feature-900/proposal.md"
	absArtifactPath := filepath.Join(tempDir, artifactPath)
	if err := os.MkdirAll(filepath.Dir(absArtifactPath), 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(absArtifactPath, []byte(artifactContent), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	for _, name := range []string{"frontend-implementer", "database-specialist"} {
		skillDir := filepath.Join(tempDir, "skills", name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(name+" skill"), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	pkg, err := orch.GenerateContextForAgent(
		"EXEC-DB-HIGH-RISK",
		"frontend-implementer",
		artifactPath,
		nil,
		"",
		[]string{"frontend-implementer"},
		"COMMIT",
		"APPLY-900",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pkg.DBImpact != "high-risk" {
		t.Errorf("expected DBImpact = %q, got %q", "high-risk", pkg.DBImpact)
	}

	hasDB := false
	for _, s := range pkg.Skills {
		if strings.Contains(s, "database-specialist") {
			hasDB = true
		}
	}
	if !hasDB {
		t.Errorf("expected database-specialist to be resolved for high-risk DB impact, got skills: %v", pkg.Skills)
	}
}

// TestDBImpactSkillsResolveOnDisk is the structural guard that closes the
// defect class this correction fixes, not just the one instance: it
// enumerates every (impact, agentName) combination dbImpactSkills handles,
// collects every skill name it can ever emit, and resolves each one against
// the REAL workspace skills/ directory using skill.Resolver -- the exact
// production lookup code (internal/devorchestrator/skill/resolver.go)
// defines what "exists" means. A future dangling skill reference (like the
// "frontend-schema-impact" name this correction removes) now fails this
// build-time test instead of surfacing as a runtime hard-fail after the
// CLI wiring lands.
func TestDBImpactSkillsResolveOnDisk(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, "skills")); statErr != nil {
		t.Fatalf("resolved repo root %q has no skills/ directory: %v", repoRoot, statErr)
	}

	impacts := []db.Impact{db.ImpactNone, db.ImpactSimple, db.ImpactHighRisk}
	agents := []string{"backend-implementer", "frontend-implementer"}

	seen := map[string]bool{}
	var allSkills []string
	for _, impact := range impacts {
		for _, agentName := range agents {
			for _, s := range dbImpactSkills(impact, agentName) {
				if !seen[s] {
					seen[s] = true
					allSkills = append(allSkills, s)
				}
			}
		}
	}

	if len(allSkills) == 0 {
		t.Fatal("dbImpactSkills emitted no skills across any (impact, agentName) combination -- this guard is vacuous; update it alongside the matrix")
	}

	resolver := skill.New(repoRoot)
	for _, s := range allSkills {
		if _, resolveErr := resolver.Resolve([]string{s}); resolveErr != nil {
			t.Errorf("dbImpactSkills can emit %q, which does not resolve under the real workspace skills/ directory: %v", s, resolveErr)
		}
	}
}
