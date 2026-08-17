package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestInjectSkipsNonOpenCodeAdapter(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	before := []byte("{\n  \"theme\": \"existing-theme\"\n}\n")
	if err := os.WriteFile(settingsPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("Inject() = %#v, want no-op for non-OpenCode adapter", result)
	}
	if after, err := os.ReadFile(settingsPath); err != nil || string(after) != string(before) {
		t.Fatalf("Claude settings changed: content=%q err=%v", after, err)
	}
}

func TestInjectInstallsOpenCodeThemeAndMergesTUIConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) error = %v", err)
	}
	tuiPath := filepath.Join(configDir, "tui.json")
	if err := os.WriteFile(tuiPath, []byte("{\n  // preserve valid JSONC fields\n  \"$schema\": \"https://opencode.ai/tui.json\",\n  \"plugin\": [\"user-plugin\"],\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tui) error = %v", err)
	}
	opencodePath := filepath.Join(configDir, "opencode.json")
	opencodeBefore := []byte("{\n  \"model\": \"user/model\"\n}\n")
	if err := os.WriteFile(opencodePath, opencodeBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(opencode) error = %v", err)
	}

	first, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	second, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	themePath := filepath.Join(configDir, "themes", "gentleman.json")
	if !first.Changed || second.Changed {
		t.Fatalf("changed first/second = %v/%v, want true/false", first.Changed, second.Changed)
	}
	if len(first.Files) != 2 || first.Files[0] != tuiPath || first.Files[1] != themePath {
		t.Fatalf("files = %#v, want [%q %q]", first.Files, tuiPath, themePath)
	}

	tuiData, err := os.ReadFile(tuiPath)
	if err != nil {
		t.Fatalf("ReadFile(tui) error = %v", err)
	}
	var tuiConfig struct {
		Schema string   `json:"$schema"`
		Plugin []string `json:"plugin"`
		Theme  string   `json:"theme"`
	}
	if err := json.Unmarshal(tuiData, &tuiConfig); err != nil {
		t.Fatalf("Unmarshal(tui) error = %v", err)
	}
	if tuiConfig.Schema != "https://opencode.ai/tui.json" || len(tuiConfig.Plugin) != 1 || tuiConfig.Plugin[0] != "user-plugin" || tuiConfig.Theme != "gentleman" {
		t.Fatalf("merged tui config = %#v", tuiConfig)
	}
	if after, err := os.ReadFile(opencodePath); err != nil || string(after) != string(opencodeBefore) {
		t.Fatalf("opencode.json changed: content=%q err=%v", after, err)
	}

	themeData, err := os.ReadFile(themePath)
	if err != nil {
		t.Fatalf("ReadFile(theme) error = %v", err)
	}
	var bundled struct {
		Schema string                       `json:"$schema"`
		Defs   map[string]string            `json:"defs"`
		Theme  map[string]map[string]string `json:"theme"`
	}
	if err := json.Unmarshal(themeData, &bundled); err != nil {
		t.Fatalf("Unmarshal(theme) error = %v", err)
	}
	if bundled.Schema != "https://opencode.ai/theme.json" {
		t.Fatalf("theme schema = %q", bundled.Schema)
	}
	for key, want := range map[string]string{"bg": "#06080f", "text": "#F3F6F9", "blue": "#7FB4CA", "green": "#B7CC85", "red": "#CB7C94"} {
		if got := bundled.Defs[key]; got != want {
			t.Fatalf("defs[%q] = %q, want %q", key, got, want)
		}
	}
	for role, want := range map[string]string{"primary": "blue", "background": "bg", "text": "text", "success": "green", "error": "red", "syntaxFunction": "syntaxFunction"} {
		if got := bundled.Theme[role]; got["dark"] != want || got["light"] != want {
			t.Fatalf("theme[%q] = %#v, want %q for dark and light", role, got, want)
		}
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

	if root.Name != "gentleman" || root.Base != "dark" {
		t.Fatalf("theme identity = %q/%q, want gentleman/dark", root.Name, root.Base)
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
