package agentguidance_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
)

// TestAntigravityOrchestratorNamesInstalledSharedSkillsDir keeps the global
// convention path the Antigravity orchestrator tells the model to read in step
// with where the skills component writes shared references.
func TestAntigravityOrchestratorNamesInstalledSharedSkillsDir(t *testing.T) {
	content, err := assets.Read("antigravity/orchestrator.md")
	if err != nil {
		t.Fatalf("assets.Read(antigravity/orchestrator.md) error = %v", err)
	}
	want := filepath.ToSlash(antigravity.NewAdapter().SkillsDir("~")) + "/_shared/"
	if !strings.Contains(content, "`"+want+"`") {
		t.Fatalf("antigravity orchestrator must name the installed shared skills dir %q", want)
	}
	for _, stale := range []string{"antigravity-cli/skills", "antigravity-desktop/skills", ".gemini/antigravity/skills"} {
		if strings.Contains(content, stale) {
			t.Fatalf("antigravity orchestrator still names the legacy skills dir %q", stale)
		}
	}
}
