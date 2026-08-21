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

	// Setup mock primary artifact with frontmatter. db_impact is high-risk,
	// not simple: dbImpactSkills only injects database-specialist for
	// db.ImpactHighRisk (see the design-deviation note in the apply-progress
	// artifact for why db.ImpactSimple no longer injects it).
	artifactContent := `---
id: feature-123
implements:
  - spec-01
db_impact: high-risk
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
		"",
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
		t.Errorf("expected database-specialist skill to be injected due to DB impact: high-risk")
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

// TestGenerateContextForAgent_TraceabilityManagerEnforcement wires the
// Traceability Manager (internal/devorchestrator/trace.Manager) into
// GenerateContextForAgent's phase-transition check, per the documented
// Notion architecture flow ("Arquitectura de nuestros agentes":
// TRACE/REPORES/SKILLRES -> CONTEXT). Before this test, trace.Manager
// existed and was unit-tested in isolation, but nothing in the orchestrator
// package ever called it -- this is the gap this fix closes.
func TestGenerateContextForAgent_TraceabilityManagerEnforcement(t *testing.T) {
	tempDir := t.TempDir()

	writeArtifact := func(relPath, content string) string {
		absPath := filepath.Join(tempDir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
		return relPath
	}

	t.Run("dest declaring Implements the source ID proceeds", func(t *testing.T) {
		orch := New(tempDir)
		source := writeArtifact("openspec/changes/trace-ok/proposal.md", "---\nid: PROP-900\n---\n# Proposal\n")
		dest := writeArtifact("openspec/changes/trace-ok/spec.md", "---\nid: SPEC-900\nimplements:\n  - PROP-900\n---\n# Spec\n")

		pkg, err := orch.GenerateContextForAgent(
			"EXEC-TRACE-OK", "dev-specifier", dest, nil, "", nil, "", "", source,
		)
		if err != nil {
			t.Fatalf("GenerateContextForAgent() error = %v, want nil for a dest that implements the source", err)
		}
		if pkg.Trace.ID != "SPEC-900" {
			t.Errorf("Trace.ID = %q, want SPEC-900", pkg.Trace.ID)
		}
	})

	t.Run("dest not declaring the source ID is refused", func(t *testing.T) {
		orch := New(tempDir)
		source := writeArtifact("openspec/changes/trace-bad/proposal.md", "---\nid: PROP-901\n---\n# Proposal\n")
		dest := writeArtifact("openspec/changes/trace-bad/spec.md", "---\nid: SPEC-901\n---\n# Spec, missing implements/originates-from\n")

		_, err := orch.GenerateContextForAgent(
			"EXEC-TRACE-BAD", "dev-specifier", dest, nil, "", nil, "", "", source,
		)
		if err == nil {
			t.Fatal("expected an error for a dest that does not declare the source ID, got nil")
		}
		if !strings.Contains(err.Error(), "strict enforcement") {
			t.Errorf("expected error to mention strict enforcement, got: %v", err)
		}
		if !strings.Contains(err.Error(), "traceability breach") {
			t.Errorf("expected error to surface the Traceability Manager's breach message, got: %v", err)
		}
	})

	t.Run("empty sourceArtifact skips the check entirely (backward compatible)", func(t *testing.T) {
		orch := New(tempDir)
		dest := writeArtifact("openspec/changes/trace-skip/proposal.md", "---\nid: PROP-902\n---\n# Proposal, no predecessor\n")

		if _, err := orch.GenerateContextForAgent(
			"EXEC-TRACE-SKIP", "dev-proposer", dest, nil, "", nil, "", "", "",
		); err != nil {
			t.Fatalf("GenerateContextForAgent() error = %v, want nil when sourceArtifact is empty", err)
		}
	})
}

// TestGenerateContextForAgent_SkillResolutionErrorPropagates covers H-04:
// GenerateContextForAgent must surface a skill-resolution failure to the
// caller instead of silently continuing with a partial context package. The
// unresolvable skill name below has no matching directory under the
// workspace's "skills" tree, so skill.Resolver.Resolve returns a non-nil
// error the same way it would for any genuinely missing skill.
func TestGenerateContextForAgent_SkillResolutionErrorPropagates(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	pkg, err := orch.GenerateContextForAgent(
		"EXEC-SKILL-ERR",
		"dev-explorer",
		"",
		nil,
		"",
		[]string{"nonexistent-skill"},
		"",
		"",
		"",
	)
	if err == nil {
		t.Fatalf("expected error when skill resolution fails, got nil")
	}
	if pkg != nil {
		t.Fatalf("expected nil package when skill resolution fails, got %#v", pkg)
	}
	if !strings.Contains(err.Error(), "resolve skills") {
		t.Errorf("expected error to mention resolve skills, got: %v", err)
	}
}

func TestGenerateContextForAgent_StrictRegistryEnforcement(t *testing.T) {
	tempDir := t.TempDir()
	orch := New(tempDir)

	// dev-specifier canonical tools: Read, Edit, Write, Grep, Glob, mcp__...
	// Has Edit+Write → Code: "write" (derived from the canonical .md — no manual override)
	// This is intentional: specifiers write their spec artifacts to engram/openspec.
	pkg, err := orch.GenerateContextForAgent(
		"EXEC-VALID",
		"dev-specifier",
		"",
		nil,
		"",
		nil,
		"SPEC",
		"SPEC-1",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error for valid agent: %v", err)
	}
	// Permissions are derived from the canonical claude/agents/dev-specifier.md tools list.
	// dev-specifier has Edit+Write but no Bash → Code: write, Git: read
	if pkg.Permissions.Code != "write" {
		t.Errorf("expected dev-specifier Code=write (derived from canonical .md), got %s", pkg.Permissions.Code)
	}
	if pkg.Permissions.Git != "read" {
		t.Errorf("expected dev-specifier Git=read (no Bash in tools), got %s", pkg.Permissions.Git)
	}

	// Invalid agent should fail with strict enforcement error
	_, err = orch.GenerateContextForAgent(
		"EXEC-INVALID",
		"custom-unknown-agent",
		"",
		nil,
		"",
		nil,
		"OUTPUT",
		"ID-1",
		"",
	)
	if err == nil {
		t.Fatalf("expected error for unregistered agent 'custom-unknown-agent', got nil")
	}
	if !strings.Contains(err.Error(), "strict enforcement") {
		t.Errorf("expected error to mention strict enforcement, got: %v", err)
	}
}

// TestGenerateContextForAgent_OwnershipEnforcement covers SPEC-007: phase
// advances are refused when the change is explicitly, recognizably owned by
// a different engine, and proceed for own or unmarked-default changes
// (matching design.md's Data Flow/Testing Strategy and the pre-existing
// TestGenerateContextForAgent fixture, which already exercises the
// unmarked-proceeds case).
func TestGenerateContextForAgent_OwnershipEnforcement(t *testing.T) {
	t.Run("refuses a change explicitly owned by gentle-orchestrator", func(t *testing.T) {
		tempDir := t.TempDir()
		orch := New(tempDir)

		artifactPath := "openspec/changes/gentle-owned/proposal.md"
		absArtifactPath := filepath.Join(tempDir, artifactPath)
		os.MkdirAll(filepath.Dir(absArtifactPath), 0755)
		os.WriteFile(absArtifactPath, []byte("---\nid: gentle-owned\nengine: gentle-orchestrator\n---\n# Proposal\n"), 0644)

		_, err := orch.GenerateContextForAgent(
			"EXEC-FOREIGN", "dev-explorer", artifactPath, nil, "", nil, "", "", "",
		)
		if err == nil {
			t.Fatalf("expected error for gentle-orchestrator-owned change, got nil")
		}
		if !strings.Contains(err.Error(), "strict enforcement") {
			t.Errorf("expected error to mention strict enforcement, got: %v", err)
		}
		if !strings.Contains(err.Error(), "gentle-owned") || !strings.Contains(err.Error(), "gentle-orchestrator") {
			t.Errorf("expected refusal message to name the change and its owner, got: %v", err)
		}
	})

	t.Run("proceeds for a change explicitly owned by dev-orchestrator", func(t *testing.T) {
		tempDir := t.TempDir()
		orch := New(tempDir)

		artifactPath := "openspec/changes/dev-owned/proposal.md"
		absArtifactPath := filepath.Join(tempDir, artifactPath)
		os.MkdirAll(filepath.Dir(absArtifactPath), 0755)
		os.WriteFile(absArtifactPath, []byte("---\nid: dev-owned\nengine: dev-orchestrator\n---\n# Proposal\n"), 0644)

		_, err := orch.GenerateContextForAgent(
			"EXEC-OWN", "dev-explorer", artifactPath, nil, "", nil, "", "", "",
		)
		if err != nil {
			t.Fatalf("expected no error for dev-orchestrator-owned change, got: %v", err)
		}
	})
}
