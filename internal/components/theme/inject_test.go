package theme

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestInjectPreservesExistingThemeInAdapterSettings(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\n  \"permissions\": {\n    \"allow\": [\"Bash(go test ./...)\"]\n  },\n  \"theme\": \"kanagawa\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("Inject() changed = true, want false (preserving user theme)")
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
	if root.Theme != "kanagawa" {
		t.Fatalf("theme = %q, want kanagawa preserved", root.Theme)
	}
	if got := root.Permissions["allow"]; len(got) != 1 || got[0] != "Bash(go test ./...)" {
		t.Fatalf("permissions.allow = %#v, want preserved existing permission", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "gentleman.json")); !os.IsNotExist(err) {
		t.Fatalf("Inject() should not write Claude custom theme file; stat error = %v", err)
	}
}

func TestInjectDoesNotForceThemeWhenEmpty(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("Inject() changed = true, want false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		var root struct {
			Theme string `json:"theme"`
		}
		_ = json.Unmarshal(data, &root)
		if root.Theme == "gentleman" {
			t.Fatalf("Inject() must not force gentleman theme")
		}
	}
}

func TestInjectVisualThemesIsIdempotentForClaude(t *testing.T) {
	home := t.TempDir()

	first, err := InjectVisualThemes(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectVisualThemes() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("InjectVisualThemes() first changed = false")
	}

	second, err := InjectVisualThemes(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectVisualThemes() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("InjectVisualThemes() second changed = true")
	}

	path := filepath.Join(home, ".claude", "themes", "axiom.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected Claude theme file %q: %v", path, err)
	}
}

func TestInjectVisualThemesSkipsUnsupportedAdapter(t *testing.T) {
	home := t.TempDir()
	adapter, _ := agents.NewAdapter(model.AgentGeminiCLI)

	result, err := InjectVisualThemes(home, adapter)
	if err != nil {
		t.Fatalf("InjectVisualThemes() error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("InjectVisualThemes() = %#v, want no-op for unsupported adapter", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "axiom.json")); !os.IsNotExist(err) {
		t.Fatalf("InjectVisualThemes() should not write Claude files for Gemini; stat error = %v", err)
	}
}

func TestInjectVisualThemesPreservesAxiomClaudeTheme(t *testing.T) {
	home := t.TempDir()

	result, err := InjectVisualThemes(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectVisualThemes() error = %v", err)
	}

	themePath := filepath.Join(home, ".claude", "themes", "axiom.json")
	if len(result.Files) != 2 || result.Files[0] != themePath {
		t.Fatalf("files = %#v, want Axiom first at %q", result.Files, themePath)
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

	if root.Name != "axiom" || root.Base != "dark" {
		t.Fatalf("theme identity = %q/%q, want axiom/dark", root.Name, root.Base)
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

func TestInjectVisualThemesWritesExpectedAssets(t *testing.T) {
	home, xdg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, adapter := range []agents.Adapter{claudeAdapter(), opencodeAdapter()} {
		settings := adapter.SettingsPath(home)
		if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settings, []byte(`{"theme":"user-selected"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		first, err := InjectVisualThemes(home, adapter)
		if err != nil || !first.Changed {
			t.Fatalf("first injection = %#v, %v", first, err)
		}
		if second, err := InjectVisualThemes(home, adapter); err != nil || second.Changed {
			t.Fatalf("second injection = %#v, %v", second, err)
		}
		if got, err := os.ReadFile(settings); err != nil || string(got) != `{"theme":"user-selected"}` {
			t.Fatalf("settings changed = %q, %v", got, err)
		}
	}
	for _, tt := range []struct {
		path  string
		value any
	}{
		{filepath.Join(home, ".claude", "themes", "axiom.json"), axiomClaudeTheme},
		{filepath.Join(home, ".claude", "themes", "axiom-dark.json"), axiomDarkClaudeTheme},
		{filepath.Join(xdg, "opencode", "themes", "axiom.json"), axiomOpenCodeTheme},
		{filepath.Join(xdg, "opencode", "themes", "axiom-dark.json"), axiomDarkOpenCodeTheme},
	} {
		got, err := os.ReadFile(tt.path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", tt.path, err)
		}
		expected, err := json.MarshalIndent(tt.value, "", "  ")
		if err != nil {
			t.Fatalf("Marshal(%s): %v", tt.path, err)
		}
		expected = append(expected, '\n')
		if !bytes.Equal(got, expected) {
			t.Fatalf("%s bytes mismatch: got %s, want %s", tt.path, string(got), string(expected))
		}
	}

	for _, forbidden := range []string{
		filepath.Join(home, ".claude", "themes", "gentleman.json"),
		filepath.Join(home, ".claude", "themes", "gentleman-cute.json"),
		filepath.Join(xdg, "opencode", "themes", "gentleman.json"),
		filepath.Join(xdg, "opencode", "themes", "gentleman-cute.json"),
	} {
		if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
			t.Fatalf("forbidden legacy theme file should not exist: %s", forbidden)
		}
	}
}
