package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock skills structure
	// skills/technology/angular/SKILL.md
	// skills/legacy/branch-pr/SKILL.md
	angularDir := filepath.Join(tempDir, "skills", "technology", "angular")
	os.MkdirAll(angularDir, 0755)
	os.WriteFile(filepath.Join(angularDir, "SKILL.md"), []byte("angular profile"), 0644)

	branchPrDir := filepath.Join(tempDir, "skills", "legacy", "branch-pr")
	os.MkdirAll(branchPrDir, 0755)
	os.WriteFile(filepath.Join(branchPrDir, "SKILL.md"), []byte("branch-pr profile"), 0644)

	resolver := New(tempDir)

	t.Run("Valid skills", func(t *testing.T) {
		requested := []string{"angular", "branch-pr"}
		paths, err := resolver.Resolve(requested)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 2 {
			t.Fatalf("expected 2 paths, got %d", len(paths))
		}

		if !strings.HasSuffix(paths[0], "angular"+string(os.PathSeparator)+"SKILL.md") && !strings.HasSuffix(paths[1], "angular"+string(os.PathSeparator)+"SKILL.md") {
			t.Errorf("missing angular path")
		}
	})

	t.Run("Missing skill", func(t *testing.T) {
		requested := []string{"angular", "nonexistent"}
		paths, err := resolver.Resolve(requested)

		if err == nil {
			t.Fatalf("expected error for missing skill")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error should mention missing skill, got %v", err)
		}
		// Should still return the successfully resolved paths
		if len(paths) != 1 {
			t.Fatalf("expected 1 path resolved, got %d", len(paths))
		}
	})
}

// TestResolve_MissingSkillIsCleanNotFound is a regression covering today's
// message shape: a genuinely missing skill must surface as ErrSkillNotFound,
// never as an unexpected filesystem error.
func TestResolve_MissingSkillIsCleanNotFound(t *testing.T) {
	tempDir := t.TempDir()

	angularDir := filepath.Join(tempDir, "skills", "technology", "angular")
	if err := os.MkdirAll(angularDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(angularDir, "SKILL.md"), []byte("angular profile"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	resolver := New(tempDir)

	_, err := resolver.Resolve([]string{"angular", "nonexistent"})
	if err == nil {
		t.Fatalf("expected error for missing skill")
	}
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("expected ErrSkillNotFound, got %v", err)
	}
	if errors.Is(err, ErrSkillLookup) {
		t.Fatalf("missing skill must not be classified as an unexpected lookup error, got %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention missing skill, got %v", err)
	}
}

// TestResolve_SurfacesUnexpectedFilesystemError proves a non-not-found
// filesystem error (e.g. an unreadable category directory) is returned to
// the caller wrapped in ErrSkillLookup, instead of being silently discarded
// like the old filepath.Walk "ignore errors" implementation did.
func TestResolve_SurfacesUnexpectedFilesystemError(t *testing.T) {
	tempDir := t.TempDir()

	skillsDir := filepath.Join(tempDir, "skills")
	restrictedCategory := filepath.Join(skillsDir, "restricted")
	if err := os.MkdirAll(restrictedCategory, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	resolver := New(tempDir)

	restrictedSkillMd := filepath.Join(restrictedCategory, "angular", "SKILL.md")
	injectedErr := &os.PathError{Op: "stat", Path: restrictedSkillMd, Err: os.ErrPermission}
	resolver.stat = func(name string) (os.FileInfo, error) {
		if name == restrictedSkillMd {
			return nil, injectedErr
		}
		return os.Stat(name)
	}

	_, err := resolver.Resolve([]string{"angular"})
	if err == nil {
		t.Fatalf("expected error to surface, got nil")
	}
	if !errors.Is(err, ErrSkillLookup) {
		t.Fatalf("expected ErrSkillLookup, got %v", err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected wrapped permission error to be inspectable via errors.Is, got %v", err)
	}
}
