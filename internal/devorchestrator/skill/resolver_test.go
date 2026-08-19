package skill

import (
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
