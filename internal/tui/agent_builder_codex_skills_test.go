package tui

import (
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestAgentBuilderSkillsDir_CodexHonorsCodexHome pins issue #5031 for the
// agent builder: with CODEX_HOME set to an existing directory and the
// supplied home equal to the real user home, the codex skills dir resolves
// under the override.
func TestAgentBuilderSkillsDir_CodexHonorsCodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	canonical, err := filepath.EvalSymlinks(override)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	got, ok := agentBuilderSkillsDirIn(home, model.AgentCodex)
	if !ok {
		t.Fatal("agentBuilderSkillsDirIn(codex) ok = false, want true")
	}
	if want := filepath.Join(canonical, "skills"); got != want {
		t.Errorf("agentBuilderSkillsDirIn(codex) = %q, want CODEX_HOME skills dir %q", got, want)
	}
}

// TestAgentBuilderSkillsDir_CodexPreservesIsolatedHomeUnderOverride: an
// ambient CODEX_HOME must not redirect the codex skills dir of an isolated
// home (issue #5031, suggested-shape guard).
func TestAgentBuilderSkillsDir_CodexPreservesIsolatedHomeUnderOverride(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("USERPROFILE", realHome)

	override := t.TempDir()
	t.Setenv("CODEX_HOME", override)

	isolated := t.TempDir()
	got, ok := agentBuilderSkillsDirIn(isolated, model.AgentCodex)
	if !ok {
		t.Fatal("agentBuilderSkillsDirIn(codex, isolated) ok = false, want true")
	}
	if want := filepath.Join(isolated, ".codex", "skills"); got != want {
		t.Errorf("agentBuilderSkillsDirIn(codex, isolated home) = %q, want %q", got, want)
	}
}
