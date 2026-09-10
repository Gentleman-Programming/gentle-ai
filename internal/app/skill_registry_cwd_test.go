package app

import (
	"path/filepath"
	"testing"
)

// TestResolveSkillRegistryDirsNormalizesRelativeCwd ensures an explicit
// relative --cwd (e.g. ".") is canonicalized to an absolute path. Without
// this, skill-registry list/refresh classify project-local skills as
// user-scoped and emit relative paths (see issue #3565).
func TestResolveSkillRegistryDirsNormalizesRelativeCwd(t *testing.T) {
	cwd, home, err := resolveSkillRegistryDirs(".")
	if err != nil {
		t.Fatalf("resolveSkillRegistryDirs(\".\") error = %v", err)
	}
	want, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs(\".\") error = %v", err)
	}
	if cwd != want {
		t.Errorf("relative cwd not normalized: got %q, want %q", cwd, want)
	}
	if !filepath.IsAbs(cwd) {
		t.Errorf("expected absolute cwd, got %q", cwd)
	}
	if home == "" {
		t.Errorf("expected non-empty home directory")
	}
}

// TestResolveSkillRegistryDirsDefaultUsesProcessCwd confirms the default
// (empty) cwd resolves to an absolute process working directory.
func TestResolveSkillRegistryDirsDefaultUsesProcessCwd(t *testing.T) {
	cwd, _, err := resolveSkillRegistryDirs("")
	if err != nil {
		t.Fatalf("resolveSkillRegistryDirs(\"\") error = %v", err)
	}
	if !filepath.IsAbs(cwd) {
		t.Errorf("default cwd should be absolute, got %q", cwd)
	}
}
