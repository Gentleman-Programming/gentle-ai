package theme

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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
	if root.Theme != "gentleman" {
		t.Fatalf("theme = %q, want gentleman", root.Theme)
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
	if root.Theme != "gentleman" {
		t.Fatalf("theme = %q, want gentleman", root.Theme)
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

	path := filepath.Join(home, ".claude", "themes", "gentleman.json")
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
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "gentleman.json")); !os.IsNotExist(err) {
		t.Fatalf("InjectVisualThemes() should not write Claude files for Gemini; stat error = %v", err)
	}
}

func TestInjectVisualThemesPreservesGentlemanClaudeTheme(t *testing.T) {
	home := t.TempDir()

	result, err := InjectVisualThemes(home, claudeAdapter())
	if err != nil {
		t.Fatalf("InjectVisualThemes() error = %v", err)
	}

	themePath := filepath.Join(home, ".claude", "themes", "gentleman.json")
	if len(result.Files) != 2 || result.Files[0] != themePath {
		t.Fatalf("files = %#v, want Gentleman first at %q", result.Files, themePath)
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

const (
	gentlemanClaudeFixture   = `{"name":"gentleman","base":"dark","overrides":{"diffAdded":"#3F4A2D","diffRemoved":"#5C3838","diffAddedWord":"#76946A","diffRemovedWord":"#C34043","chromeYellow":"#DCA561","briefLabelYou":"#DCA561","rainbow_yellow":"#DCA561","yellow_FOR_SUBAGENTS_ONLY":"#DCA561"}}`
	cuteClaudeFixture        = `{"name":"Gentleman Cute","base":"dark","overrides":{"claude":"#E35FA6","claudeShimmer":"#FF81CC","text":"#F6EFF3","inactive":"#A78E9B","subtle":"#76616B","suggestion":"#FFB1DD","permission":"#E35FA6","promptBorder":"#E35FA6","planMode":"#C49BFF","autoAccept":"#FF81CC","bashBorder":"#E0C27A","remember":"#E0C27A","success":"#D2CBD0","merged":"#D2CBD0","error":"#FF718F","warning":"#F2B86D","diffAdded":"#1A2430","diffRemoved":"#2D151F","diffAddedWord":"#2D4F6A","diffRemovedWord":"#7A2948","userMessageBackground":"#241822","userMessageBackgroundHover":"#342230","selectionBg":"#563040","memoryBackgroundColor":"#1A1218","bashMessageBackgroundColor":"#151316"}}`
	gentlemanOpenCodeFixture = `{"$schema":"https://opencode.ai/theme.json","theme":{"background":"none","backgroundPanel":"#06080f","backgroundElement":"#06080f","text":"#F3F6F9","textMuted":"#5C6170","primary":"#7FB4CA","secondary":"#A3B5D6","accent":"#E0C15A","error":"#CB7C94","warning":"#DEBA87","success":"#B7CC85","info":"#7FB4CA","border":"#313342","borderActive":"#7FB4CA","borderSubtle":"#232A40","diffAdded":"#B7CC85","diffRemoved":"#CB7C94","diffContext":"#5C6170","diffHunkHeader":"#8394A3","diffHighlightAdded":"#D1E8A9","diffHighlightRemoved":"#DE8FA8","diffAddedBg":"#1a2e1a","diffRemovedBg":"#2e1a1a","diffContextBg":"#0d0f14","diffLineNumber":"#8394A3","diffAddedLineNumberBg":"#1a2e1a","diffRemovedLineNumberBg":"#2e1a1a","markdownText":"#F3F6F9","markdownHeading":"#B5B2D0","markdownLink":"#7FB4CA","markdownLinkText":"#79B8EA","markdownCode":"#B7CC85","markdownBlockQuote":"#DEBA87","markdownEmph":"#7CB9DD","markdownStrong":"#DEBA87","markdownHorizontalRule":"#5C6170","markdownListItem":"#7FB4CA","markdownListEnumeration":"#A3B5D6","markdownImage":"#7FB4CA","markdownImageText":"#79B8EA","markdownCodeBlock":"#F3F6F9","syntaxComment":"#8394A3","syntaxKeyword":"#C99AD6","syntaxFunction":"#B99BF2","syntaxVariable":"#F3F6F9","syntaxString":"#DFBD76","syntaxNumber":"#A4DAA7","syntaxType":"#8FB8DD","syntaxOperator":"#DEBA87","syntaxPunctuation":"#96A2B0"}}`
	cuteOpenCodeFixture      = `{"$schema":"https://opencode.ai/theme.json","theme":{"background":"none","backgroundPanel":"#1A1218","backgroundElement":"#241822","text":"#F6EFF3","textMuted":"#A78E9B","primary":"#E35FA6","secondary":"#D7A0B8","accent":"#E0C27A","error":"#FF718F","warning":"#F2B86D","success":"#D2CBD0","info":"#D7A0B8","border":"#342230","borderActive":"#FF81CC","borderSubtle":"#241822","diffAdded":"#A9C7EE","diffRemoved":"#FF718F","diffContext":"#A78E9B","diffHunkHeader":"#D7A0B8","diffHighlightAdded":"#A9C7EE","diffHighlightRemoved":"#FF718F","diffAddedBg":"#151316","diffRemovedBg":"#261019","diffContextBg":"#1A1218","diffLineNumber":"#76616B","diffAddedLineNumberBg":"#151316","diffRemovedLineNumberBg":"#261019","markdownText":"#F6EFF3","markdownHeading":"#E0C27A","markdownLink":"#E35FA6","markdownLinkText":"#E35FA6","markdownCode":"#E0C27A","markdownBlockQuote":"#A78E9B","markdownEmph":"#D7A0B8","markdownStrong":"#E0C27A","markdownHorizontalRule":"#A78E9B","markdownListItem":"#E35FA6","markdownListEnumeration":"#D7A0B8","markdownImage":"#E35FA6","markdownImageText":"#E35FA6","markdownCodeBlock":"#F6EFF3","syntaxComment":"#A78E9B","syntaxKeyword":"#C15995","syntaxFunction":"#D7A0B8","syntaxVariable":"#F6EFF3","syntaxString":"#E0C27A","syntaxNumber":"#D7A0B8","syntaxType":"#A9C7EE","syntaxOperator":"#E35FA6","syntaxPunctuation":"#A78E9B"}}`
)

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
		path, fixture string
		value         any
	}{
		{filepath.Join(home, ".claude", "themes", "gentleman.json"), gentlemanClaudeFixture, &claudeTheme{}},
		{filepath.Join(home, ".claude", "themes", "gentleman-cute.json"), cuteClaudeFixture, &claudeTheme{}},
		{filepath.Join(xdg, "opencode", "themes", "gentleman.json"), gentlemanOpenCodeFixture, &openCodeTheme{}},
		{filepath.Join(xdg, "opencode", "themes", "gentleman-cute.json"), cuteOpenCodeFixture, &openCodeTheme{}},
	} {
		got, err := os.ReadFile(tt.path)
		if err != nil || !bytes.Equal(got, indentedJSON(t, tt.fixture, tt.value)) {
			t.Fatalf("%s bytes mismatch: %v", tt.path, err)
		}
	}
	cute := gentlemanCuteClaudeTheme
	for _, key := range []string{"background", "backgroundFullscreen", "backgroundUser", "backgroundHover", "backgroundSelection", "backgroundMemory", "backgroundBash", "shimmer"} {
		if _, exists := cute.Overrides[key]; exists {
			t.Fatalf("Claude Cute contains invalid override %q", key)
		}
	}
}

func indentedJSON(t *testing.T, fixture string, value any) []byte {
	t.Helper()
	if err := json.Unmarshal([]byte(fixture), value); err != nil {
		t.Fatal(err)
	}
	result, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(result, '\n')
}
