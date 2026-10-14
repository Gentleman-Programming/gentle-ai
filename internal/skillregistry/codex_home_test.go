package skillregistry

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestUserSkillDirs_CodexHonorsCodexHome pins issue #5031 for the skill
// registry: with CODEX_HOME set to an existing directory and the supplied
// home equal to the real user home, the codex skills entry must resolve under
// the override instead of <home>/.codex/skills.
func TestUserSkillDirs_CodexHonorsCodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	canonical, err := filepath.EvalSymlinks(override)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	dirs := UserSkillDirs(home)
	want := filepath.Join(canonical, "skills")
	if !slices.Contains(dirs, want) {
		t.Errorf("UserSkillDirs missing %q under CODEX_HOME; got: %v", want, dirs)
	}
	stale := filepath.Join(home, ".codex", "skills")
	if slices.Contains(dirs, stale) {
		t.Errorf("UserSkillDirs must not list stale %q when CODEX_HOME is set; got: %v", stale, dirs)
	}
}

// TestUserSkillDirs_CodexPreservesIsolatedHomeUnderOverride: an ambient
// CODEX_HOME must not redirect the user skill dirs of an isolated home
// (issue #5031, suggested-shape guard).
func TestUserSkillDirs_CodexPreservesIsolatedHomeUnderOverride(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("USERPROFILE", realHome)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	isolated := t.TempDir()
	dirs := UserSkillDirs(isolated)
	want := filepath.Join(isolated, ".codex", "skills")
	if !slices.Contains(dirs, want) {
		t.Errorf("UserSkillDirs(isolated home) missing %q; got: %v", want, dirs)
	}
}
