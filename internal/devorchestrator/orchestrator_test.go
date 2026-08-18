package devorchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateContextForAgent(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	// Setup mock docs/repository-registry.md. Profile is a full workspace-
	// relative path, matching the real registry's convention (it does not
	// always equal "skills/repo-profiles/<slug>/SKILL.md" -- see docs/
	// repository-registry.md's own header comment).
	regContent := `
| Repository (gitlab_path) | Slug | Owner | Type | Purpose | Profile |
|---|---|---|---|---|---|
| group/repo-a | repo-a | team-1 | Service | x | skills/repo-profiles/repo-a/SKILL.md |
| group/repo-b | repo-b | team-2 | Frontend | x | skills/repo-profiles/repo-b/SKILL.md |
`
	docsDir := filepath.Join(tempDir, "docs")
	os.MkdirAll(docsDir, 0755)
	err := os.WriteFile(filepath.Join(docsDir, "repository-registry.md"), []byte(regContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Setup mock primary artifact with frontmatter
	artifactContent := `---
id: feature-123
implements:
  - spec-01
db_impact: simple
---
# Proposal content
`
	artifactPath := "openspec/changes/feature/proposal.md"
	absArtifactPath := filepath.Join(tempDir, artifactPath)
	os.MkdirAll(filepath.Dir(absArtifactPath), 0755)
	os.WriteFile(absArtifactPath, []byte(artifactContent), 0644)

	// Setup mock repo-profile
	profileDir := filepath.Join(tempDir, "skills", "repo-profiles", "repo-a")
	os.MkdirAll(profileDir, 0755)
	os.WriteFile(filepath.Join(profileDir, "SKILL.md"), []byte("This is repo-a profile"), 0644)

	// Setup mock architecture-profile
	archDir := filepath.Join(tempDir, "skills", "architecture", "spring-rest")
	os.MkdirAll(archDir, 0755)
	os.WriteFile(filepath.Join(archDir, "SKILL.md"), []byte("This is spring profile"), 0644)

	// Setup mock skill for resolver
	skillDir := filepath.Join(tempDir, "skills", "backend-implementer")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("backend skill"), 0644)

	// Setup mock DB skill for resolver (injected by db_impact)
	dbSkillDir := filepath.Join(tempDir, "skills", "database-specialist")
	os.MkdirAll(dbSkillDir, 0755)
	os.WriteFile(filepath.Join(dbSkillDir, "SKILL.md"), []byte("db skill"), 0644)

	// Execute Test
	pkg, err := orch.GenerateContextForAgent(
		"EXEC-001",
		"backend-implementer",
		artifactPath,
		[]string{"repo-a", "repo-c"}, // repo-c is invalid, should be filtered
		"spring-rest",
		[]string{"backend-implementer"},
		"COMMIT",
		"APPLY-123",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify DB skill was injected
	if len(pkg.Skills) != 2 {
		t.Fatalf("expected 2 resolved skills (backend + db), got %d", len(pkg.Skills))
	}
	hasDB := false
	for _, s := range pkg.Skills {
		if strings.Contains(s, "database-specialist") {
			hasDB = true
			break
		}
	}
	if !hasDB {
		t.Errorf("expected database-specialist skill to be injected due to DB impact: simple")
	}

	if pkg == nil {
		t.Fatalf("Expected package, got nil")
	}

	if pkg.ExecutionID != "EXEC-001" {
		t.Errorf("Expected EXEC-001, got %s", pkg.ExecutionID)
	}

	if pkg.Agent != "backend-implementer" {
		t.Errorf("Expected backend-implementer, got %s", pkg.Agent)
	}

	if pkg.Trace.ID != "feature-123" {
		t.Errorf("Expected Trace.ID feature-123, got %s", pkg.Trace.ID)
	}

	if len(pkg.Scope.Repositories) != 1 || pkg.Scope.Repositories[0] != "repo-a" {
		t.Errorf("Expected only valid repo-a in scope, got %v", pkg.Scope.Repositories)
	}

	if pkg.Scope.Architecture != "spring-rest" {
		t.Errorf("Expected spring-rest in architecture scope, got %s", pkg.Scope.Architecture)
	}

	if !strings.Contains(pkg.ArchitectureProfile, "This is spring profile") {
		t.Errorf("Expected ArchitectureProfile to contain 'This is spring profile'")
	}

	if pkg.Permissions.Code != "write" {
		t.Errorf("Expected code write permission for implementer, got %s", pkg.Permissions.Code)
	}

	if pkg.ExpectedOutput.Type != "COMMIT" {
		t.Errorf("Expected COMMIT output, got %s", pkg.ExpectedOutput.Type)
	}
}

// TestRouteIntentThenGenerateContextPreservesProvenance is a seam test: it
// exercises RouteIntent and GenerateContextForAgent back-to-back, the way a
// real caller would, to catch frontmatter-field mismatches between the
// Intent Router and the Trace Resolver that per-package unit tests (each
// using its own hand-written fixture) cannot see.
func TestRouteIntentThenGenerateContextPreservesProvenance(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	result, err := orch.RouteIntent("Add a new payments export job", "issue-42")
	if err != nil {
		t.Fatalf("RouteIntent() error = %v", err)
	}

	pkg, err := orch.GenerateContextForAgent(
		"EXEC-002",
		"dev-explorer",
		result.ArtifactPath,
		nil,
		"",
		nil,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("GenerateContextForAgent() error = %v", err)
	}

	if pkg.Trace.ID != result.ChangeID {
		t.Errorf("Trace.ID = %q, want %q", pkg.Trace.ID, result.ChangeID)
	}
	if len(pkg.Trace.OriginatesFrom) != 1 || pkg.Trace.OriginatesFrom[0] != "issue-42" {
		t.Errorf("Trace.OriginatesFrom = %v, want [\"issue-42\"] -- RouteIntent's frontmatter field must match trace.Node's yaml tag", pkg.Trace.OriginatesFrom)
	}
}

func TestGenerateContextForAgent_StrictRegistryEnforcement(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	// Valid agent (dev-specifier) should get read-only permissions
	pkg, err := orch.GenerateContextForAgent(
		"EXEC-VALID",
		"dev-specifier",
		"",
		nil,
		"",
		nil,
		"SPEC",
		"SPEC-1",
	)
	if err != nil {
		t.Fatalf("unexpected error for valid agent: %v", err)
	}
	if pkg.Permissions.Code != "none" || pkg.Permissions.Git != "none" {
		t.Errorf("expected dev-specifier to have none permissions, got code:%s git:%s", pkg.Permissions.Code, pkg.Permissions.Git)
	}

	// Invalid agent should fail
	_, err = orch.GenerateContextForAgent(
		"EXEC-INVALID",
		"custom-unknown-agent",
		"",
		nil,
		"",
		nil,
		"OUTPUT",
		"ID-1",
	)
	if err == nil {
		t.Fatalf("expected error for unregistered agent 'custom-unknown-agent', got nil")
	}
	if !strings.Contains(err.Error(), "strict enforcement") {
		t.Errorf("expected error to mention strict enforcement, got: %v", err)
	}
}
