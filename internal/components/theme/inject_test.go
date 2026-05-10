package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestInjectMergesThemeOverlayIntoAdapterSettings(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\n  \"permissions\": {\n    \"allow\": [\"Bash(go test ./...)\"]\n  },\n  \"theme\": \"existing-theme\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	if len(first.Files) != 1 || first.Files[0] != settingsPath {
		t.Fatalf("files = %#v, want only %q", first.Files, settingsPath)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var root struct {
		Permissions map[string][]string `json:"permissions"`
		Theme       string              `json:"theme"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	if root.Theme != "gentleman-kanagawa" {
		t.Fatalf("theme = %q, want gentleman-kanagawa", root.Theme)
	}
	if got := root.Permissions["allow"]; len(got) != 1 || got[0] != "Bash(go test ./...)" {
		t.Fatalf("permissions.allow = %#v, want preserved existing permission", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "gentleman.json")); !os.IsNotExist(err) {
		t.Fatalf("Inject() should not write Claude custom theme file; stat error = %v", err)
	}
}

func TestInjectCreatesAdapterSettingsWhenMissing(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}
	if len(result.Files) != 1 || result.Files[0] != settingsPath {
		t.Fatalf("files = %#v, want only %q", result.Files, settingsPath)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var root struct {
		Theme string `json:"theme"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	if root.Theme != "gentleman-kanagawa" {
		t.Fatalf("theme = %q, want gentleman-kanagawa", root.Theme)
	}
}

func TestInjectClaudeThemeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := InjectClaudeTheme(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectClaudeTheme() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("InjectClaudeTheme() first changed = false")
	}

	second, err := InjectClaudeTheme(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectClaudeTheme() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("InjectClaudeTheme() second changed = true")
	}

	path := filepath.Join(home, ".claude", "themes", "gentleman.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected Claude theme file %q: %v", path, err)
	}
}

func TestInjectClaudeThemeSkipsNonClaudeAdapter(t *testing.T) {
	home := t.TempDir()

	result, err := InjectClaudeTheme(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("InjectClaudeTheme() error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("InjectClaudeTheme() = %#v, want no-op for non-Claude adapter", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "gentleman.json")); !os.IsNotExist(err) {
		t.Fatalf("InjectClaudeTheme() should not write file for OpenCode; stat error = %v", err)
	}
}

func TestInjectClaudeThemeWritesGentlemanThemeFile(t *testing.T) {
	home := t.TempDir()

	result, err := InjectClaudeTheme(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectClaudeTheme() error = %v", err)
	}

	themePath := filepath.Join(home, ".claude", "themes", "gentleman.json")
	if len(result.Files) != 1 || result.Files[0] != themePath {
		t.Fatalf("files = %#v, want only %q", result.Files, themePath)
	}

	data, err := os.ReadFile(themePath)
	if err != nil {
		t.Fatalf("ReadFile(theme) error = %v", err)
	}

	var root struct {
		Name      string            `json:"name"`
		Base      string            `json:"base"`
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(theme) error = %v", err)
	}

	if root.Name != "Gentleman" || root.Base != "dark" {
		t.Fatalf("theme identity = %q/%q, want Gentleman/dark", root.Name, root.Base)
	}
	expected := map[string]string{
		"diffAdded":                 "#3F4A2D",
		"diffRemoved":               "#5C3838",
		"diffAddedWord":             "#76946A",
		"diffRemovedWord":           "#C34043",
		"chromeYellow":              "#DCA561",
		"briefLabelYou":             "#DCA561",
		"rainbow_yellow":            "#DCA561",
		"yellow_FOR_SUBAGENTS_ONLY": "#DCA561",
	}
	for key, want := range expected {
		if root.Overrides[key] != want {
			t.Fatalf("override %s = %q, want %q", key, root.Overrides[key], want)
		}
	}
	for _, forbidden := range []string{"markdown", "syntax", "keyword", "string"} {
		if _, ok := root.Overrides[forbidden]; ok {
			t.Fatalf("theme contains forbidden non-Claude theme key %q", forbidden)
		}
	}
}
