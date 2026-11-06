package antigravityrules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
)

func TestStagingPathIsTheAdapterSystemPromptFileOutsideLoadedSurfaces(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".gemini", "config", "gentle-ai", "guidance.md")
	if got := antigravityrules.StagingPath(home); got != want {
		t.Fatalf("StagingPath() = %q, want %q", got, want)
	}
	if got := antigravity.NewAdapter().SystemPromptFile(home); got != want {
		t.Fatalf("adapter SystemPromptFile() = %q, want %q", got, want)
	}
}

// TestPublishRealAntigravityGuidanceWritesRulesNotGeminiMD runs the production
// writers and the publisher end to end: every rendered rule lands on disk,
// GEMINI.md is never created, and a second publish changes nothing.
func TestPublishRealAntigravityGuidanceWritesRulesNotGeminiMD(t *testing.T) {
	home := t.TempDir()
	staging := renderRealGuidance(t, home)

	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	rendered := antigravityrules.Render(staging)
	if !result.Changed || len(result.Files) != len(rendered) {
		t.Fatalf("Publish() = %+v, want %d written rule files", result, len(rendered))
	}
	var names []string
	for _, rule := range rendered {
		names = append(names, rule.Name)
		if got := readFile(t, filepath.Join(antigravityrules.RulesDir(home), rule.Name)); got != rule.Content {
			t.Errorf("%s on disk differs from the rendered rule", rule.Name)
		}
	}
	if got := strings.Join(publishedRules(t, home), ","); got != strings.Join(names, ",") {
		t.Fatalf("published rules = %s, want %s", got, strings.Join(names, ","))
	}

	if _, err := os.Stat(filepath.Join(home, ".gemini", "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("Antigravity writers must not create ~/.gemini/GEMINI.md; stat err = %v", err)
	}

	again, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}
	if again.Changed {
		t.Fatalf("second Publish() changed = true (%v), want idempotent", again.Files)
	}
}
