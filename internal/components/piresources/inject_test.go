package piresources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestPathsForComponentPi(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentPi)
	if err != nil {
		t.Fatalf("NewAdapter(pi) error = %v", err)
	}

	sddPaths := PathsForComponent(home, adapter, model.ComponentSDD)
	if len(sddPaths) == 0 {
		t.Fatal("PathsForComponent(sdd, pi) returned empty paths")
	}

	wantPrompt := filepath.Join(home, ".pi", "agent", "prompts", "sdd-new.md")
	if !containsPath(sddPaths, wantPrompt) {
		t.Fatalf("PathsForComponent(sdd, pi) missing %q; got %v", wantPrompt, sddPaths)
	}

	wantSettings := filepath.Join(home, ".pi", "agent", "settings.json")
	if !containsPath(sddPaths, wantSettings) {
		t.Fatalf("PathsForComponent(sdd, pi) missing settings path %q; got %v", wantSettings, sddPaths)
	}

	ctxPaths := PathsForComponent(home, adapter, model.ComponentContext7)
	wantCtx := filepath.Join(home, ".pi", "agent", "extensions", "context7-tools.ts")
	if !containsPath(ctxPaths, wantCtx) {
		t.Fatalf("PathsForComponent(context7, pi) missing %q; got %v", wantCtx, ctxPaths)
	}

	themePaths := PathsForComponent(home, adapter, model.ComponentTheme)
	wantTheme := filepath.Join(home, ".pi", "agent", "themes", "gentleman-kanagawa.json")
	if !containsPath(themePaths, wantTheme) {
		t.Fatalf("PathsForComponent(theme, pi) missing %q; got %v", wantTheme, themePaths)
	}
}

func TestInjectSDDWritesPromptsAndManagedPackageState(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentPi)
	if err != nil {
		t.Fatalf("NewAdapter(pi) error = %v", err)
	}

	result, err := Inject(home, adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("Inject(sdd,pi) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(sdd,pi) changed = false")
	}

	promptPath := filepath.Join(home, ".pi", "agent", "prompts", "sdd-new.md")
	if _, err := os.Stat(promptPath); err != nil {
		t.Fatalf("expected prompt file %q: %v", promptPath, err)
	}

	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected settings file %q: %v", settingsPath, err)
	}
	if !strings.Contains(string(raw), "\"managedPackages\"") || !strings.Contains(string(raw), "\"pi-gentle-ai\"") {
		t.Fatalf("settings missing managed package metadata, got: %s", string(raw))
	}

	second, err := Inject(home, adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("Inject(sdd,pi) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(sdd,pi) second changed = true, want false")
	}
}

func TestInjectNonPiIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("NewAdapter(claude) error = %v", err)
	}

	result, err := Inject(home, adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("Inject(sdd,claude) error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("Inject(sdd,claude) = %+v, want no-op", result)
	}
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}
