package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestAgentBuilderSkillsDirIn_KimiPrefersCurrentLayout verifies that the
// agent builder routes generated skills to the native kimi-code v0.11+
// skills root when ~/.kimi-code exists as a directory (issue #782).
func TestAgentBuilderSkillsDirIn_KimiPrefersCurrentLayout(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kimi-code"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := agentBuilderSkillsDirIn(home, model.AgentKimi)
	if !ok {
		t.Fatal("agentBuilderSkillsDirIn(kimi) ok = false, want true")
	}
	if want := filepath.Join(home, ".kimi-code", "skills"); got != want {
		t.Errorf("agentBuilderSkillsDirIn(kimi) = %q, want %q", got, want)
	}
}

// TestAgentBuilderSkillsDirIn_KimiLegacyFallback verifies that without a
// ~/.kimi-code directory the agent builder keeps the shared legacy skills
// path for Kimi.
func TestAgentBuilderSkillsDirIn_KimiLegacyFallback(t *testing.T) {
	home := t.TempDir()

	got, ok := agentBuilderSkillsDirIn(home, model.AgentKimi)
	if !ok {
		t.Fatal("agentBuilderSkillsDirIn(kimi) ok = false, want true")
	}
	if want := filepath.Join(home, ".config", "agents", "skills"); got != want {
		t.Errorf("agentBuilderSkillsDirIn(kimi) = %q, want %q", got, want)
	}
}

// TestAgentBuilderSkillsDirIn_KimiCodeFileIsNotCurrentLayout verifies that a
// plain file named .kimi-code is not treated as the v0.11+ layout.
func TestAgentBuilderSkillsDirIn_KimiCodeFileIsNotCurrentLayout(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".kimi-code"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _ := agentBuilderSkillsDirIn(home, model.AgentKimi)
	if want := filepath.Join(home, ".config", "agents", "skills"); got != want {
		t.Errorf("agentBuilderSkillsDirIn(kimi) = %q, want legacy %q", got, want)
	}
}

// TestAgentBuilderSkillsDirIn_KimiUnexpectedStatErrorOmitsKimi verifies that
// a stat failure other than absence (ENOENT/ENOTDIR) on the current config
// root omits Kimi from the agent builder instead of silently selecting the
// legacy skills path.
func TestAgentBuilderSkillsDirIn_KimiUnexpectedStatErrorOmitsKimi(t *testing.T) {
	home := t.TempDir()
	statErr := os.ErrPermission

	original := osStatPathFn
	osStatPathFn = func(string) (os.FileInfo, error) { return nil, statErr }
	defer func() { osStatPathFn = original }()

	got, ok := agentBuilderSkillsDirIn(home, model.AgentKimi)
	if ok {
		t.Fatalf("agentBuilderSkillsDirIn(kimi) ok = true with dir %q, want false on unexpected stat error", got)
	}
	if got != "" {
		t.Errorf("agentBuilderSkillsDirIn(kimi) = %q, want empty on unexpected stat error", got)
	}
}
