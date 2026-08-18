package devorchestrator

import (
	"os"
	"path/filepath"
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

	// Execute Test
	pkg, err := orch.GenerateContextForAgent(
		"EXEC-001",
		"backend-implementer",
		artifactPath,
		[]string{"repo-a", "repo-c"}, // repo-c is invalid, should be filtered
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

	if pkg.Permissions.Code != "write" {
		t.Errorf("Expected code write permission for implementer, got %s", pkg.Permissions.Code)
	}

	if pkg.ExpectedOutput.Type != "APPLY" {
		t.Errorf("Expected APPLY output, got %s", pkg.ExpectedOutput.Type)
	}
}
