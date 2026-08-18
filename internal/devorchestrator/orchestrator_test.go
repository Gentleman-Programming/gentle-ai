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

	// Setup mock repository-registry.md
	regContent := `
| Repository (gitlab_path) | Slug | Owner | Type | Purpose | Profile |
|---|---|---|---|---|---|
| group/repo-a | repo-a | team-1 | Service | x | profile-1 |
| group/repo-b | repo-b | team-2 | Frontend | x | profile-2 |
`
	err := os.WriteFile(filepath.Join(tempDir, "repository-registry.md"), []byte(regContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Setup mock primary artifact with frontmatter
	artifactContent := `---
id: feature-123
implements:
  - spec-01
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

	// Execute Test
	pkg, err := orch.GenerateContextForAgent(
		"EXEC-001",
		"backend-implementer",
		artifactPath,
		[]string{"repo-a", "repo-c"}, // repo-c is invalid, should be filtered
		"spring-rest",
		[]string{"java-spring"},
		"APPLY",
		"APPLY-123",
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
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

	if pkg.ExpectedOutput.Type != "APPLY" {
		t.Errorf("Expected APPLY output, got %s", pkg.ExpectedOutput.Type)
	}
}
