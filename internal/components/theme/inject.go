package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

type claudeTheme struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Overrides map[string]string `json:"overrides"`
}

type openCodeTheme struct {
	Schema string            `json:"$schema"`
	Theme  map[string]string `json:"theme"`
}

const openCodeThemeSchema = "https://opencode.ai/theme.json"

func palette(pairs ...string) map[string]string {
	colors := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		colors[pairs[i]] = pairs[i+1]
	}
	return colors
}

var axiomClaudeTheme = claudeTheme{
	Name: "axiom", Base: "dark",
	Overrides: palette(
		"diffAdded", "#3F4A2D", "diffRemoved", "#5C3838", "diffAddedWord", "#76946A", "diffRemovedWord", "#C34043",
		"chromeYellow", "#DCA561", "briefLabelYou", "#DCA561", "rainbow_yellow", "#DCA561", "yellow_FOR_SUBAGENTS_ONLY", "#DCA561",
	),
}

var axiomDarkClaudeTheme = claudeTheme{
	Name: "Axiom Dark", Base: "dark",
	Overrides: palette(
		"claude", "#6B8AFE", "claudeShimmer", "#8EA7FF", "text", "#F3F6F9", "inactive", "#7B849B", "subtle", "#4B556D", "suggestion", "#8EA7FF",
		"permission", "#6B8AFE", "promptBorder", "#6B8AFE", "planMode", "#80D4FF", "autoAccept", "#6B8AFE", "bashBorder", "#E0C27A",
		"remember", "#E0C27A", "success", "#B4E7C7", "merged", "#B4E7C7", "error", "#FF718F", "warning", "#F2B86D",
		"diffAdded", "#1A2420", "diffRemoved", "#2D151F", "diffAddedWord", "#2D5A45", "diffRemovedWord", "#7A2948",
		"userMessageBackground", "#1E2230", "userMessageBackgroundHover", "#262C3E", "selectionBg", "#38415C", "memoryBackgroundColor", "#151824", "bashMessageBackgroundColor", "#12151E",
	),
}

var axiomOpenCodeTheme = openCodeTheme{
	Schema: openCodeThemeSchema,
	Theme: palette(
		"background", "none", "backgroundPanel", "#06080f", "backgroundElement", "#06080f", "text", "#F3F6F9", "textMuted", "#5C6170",
		"primary", "#7FB4CA", "secondary", "#A3B5D6", "accent", "#E0C15A", "error", "#CB7C94", "warning", "#DEBA87", "success", "#B7CC85", "info", "#7FB4CA",
		"border", "#313342", "borderActive", "#7FB4CA", "borderSubtle", "#232A40", "diffAdded", "#B7CC85", "diffRemoved", "#CB7C94", "diffContext", "#5C6170",
		"diffHunkHeader", "#8394A3", "diffHighlightAdded", "#D1E8A9", "diffHighlightRemoved", "#DE8FA8", "diffAddedBg", "#1a2e1a", "diffRemovedBg", "#2e1a1a", "diffContextBg", "#0d0f14",
		"diffLineNumber", "#8394A3", "diffAddedLineNumberBg", "#1a2e1a", "diffRemovedLineNumberBg", "#2e1a1a", "markdownText", "#F3F6F9", "markdownHeading", "#B5B2D0",
		"markdownLink", "#7FB4CA", "markdownLinkText", "#79B8EA", "markdownCode", "#B7CC85", "markdownBlockQuote", "#DEBA87", "markdownEmph", "#7CB9DD", "markdownStrong", "#DEBA87",
		"markdownHorizontalRule", "#5C6170", "markdownListItem", "#7FB4CA", "markdownListEnumeration", "#A3B5D6", "markdownImage", "#7FB4CA", "markdownImageText", "#79B8EA", "markdownCodeBlock", "#F3F6F9",
		"syntaxComment", "#8394A3", "syntaxKeyword", "#C99AD6", "syntaxFunction", "#B99BF2", "syntaxVariable", "#F3F6F9", "syntaxString", "#DFBD76", "syntaxNumber", "#A4DAA7", "syntaxType", "#8FB8DD", "syntaxOperator", "#DEBA87", "syntaxPunctuation", "#96A2B0",
	),
}

var axiomDarkOpenCodeTheme = openCodeTheme{
	Schema: openCodeThemeSchema,
	Theme: palette(
		"background", "none", "backgroundPanel", "#0D1117", "backgroundElement", "#161B22", "text", "#F0F6FC", "textMuted", "#8B949E",
		"primary", "#58A6FF", "secondary", "#79C0FF", "accent", "#58A6FF", "error", "#FF7B72", "warning", "#D29922", "success", "#3FB950", "info", "#58A6FF",
		"border", "#30363D", "borderActive", "#58A6FF", "borderSubtle", "#21262D", "diffAdded", "#3FB950", "diffRemoved", "#FF7B72", "diffContext", "#8B949E",
		"diffHunkHeader", "#79C0FF", "diffHighlightAdded", "#3FB950", "diffHighlightRemoved", "#FF7B72", "diffAddedBg", "#033A16", "diffRemovedBg", "#67060C", "diffContextBg", "#0D1117",
		"diffLineNumber", "#6E7681", "diffAddedLineNumberBg", "#033A16", "diffRemovedLineNumberBg", "#67060C", "markdownText", "#F0F6FC", "markdownHeading", "#58A6FF",
		"markdownLink", "#58A6FF", "markdownLinkText", "#58A6FF", "markdownCode", "#E3B341", "markdownBlockQuote", "#8B949E", "markdownEmph", "#79C0FF", "markdownStrong", "#E3B341",
		"markdownHorizontalRule", "#30363D", "markdownListItem", "#58A6FF", "markdownListEnumeration", "#79C0FF", "markdownImage", "#58A6FF", "markdownImageText", "#58A6FF", "markdownCodeBlock", "#F0F6FC",
		"syntaxComment", "#8B949E", "syntaxKeyword", "#FF7B72", "syntaxFunction", "#D2A8FF", "syntaxVariable", "#FFA657", "syntaxString", "#A5D6FF", "syntaxNumber", "#79C0FF", "syntaxType", "#FFA657", "syntaxOperator", "#FF7B72", "syntaxPunctuation", "#8B949E",
	),
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	// Per REQ-09.2: Axiom is non-intrusive and preserves the developer's theme preferences.
	// It does NOT inject "theme" into settings.json during sync or install.
	return InjectionResult{Changed: false, Files: []string{settingsPath}}, nil
}

// InjectVisualThemes writes the managed visual theme assets without selecting one
// in an agent's active settings.
func InjectVisualThemes(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	paths := VisualThemePaths(homeDir, adapter)
	if len(paths) == 0 {
		return InjectionResult{}, nil
	}

	var values []any
	switch adapter.Agent() {
	case model.AgentClaudeCode:
		values = []any{axiomClaudeTheme, axiomDarkClaudeTheme}
	case model.AgentOpenCode:
		values = []any{axiomOpenCodeTheme, axiomDarkOpenCodeTheme}
	}

	result := InjectionResult{Files: make([]string, 0, len(paths))}
	for i, path := range paths {
		content, err := json.MarshalIndent(values[i], "", "  ")
		if err != nil {
			return InjectionResult{}, fmt.Errorf("marshal visual theme %q: %w", filepath.Base(path), err)
		}
		content = append(content, '\n')
		writeResult, err := filemerge.WriteFileAtomic(path, content, 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		result.Changed = result.Changed || writeResult.Changed
		result.Files = append(result.Files, path)
	}
	return result, nil
}

// VisualThemePaths returns the installer-owned visual theme assets for an adapter.
func VisualThemePaths(homeDir string, adapter agents.Adapter) []string {
	var root string
	switch adapter.Agent() {
	case model.AgentClaudeCode:
		root = filepath.Join(adapter.GlobalConfigDir(homeDir), "themes")
	case model.AgentOpenCode:
		root = filepath.Join(filepath.Dir(adapter.SettingsPath(homeDir)), "themes")
	default:
		return nil
	}
	return []string{filepath.Join(root, "axiom.json"), filepath.Join(root, "axiom-dark.json")}
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjectsForPath(path, baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
}

var osReadFile = func(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read json file %q: %w", path, err)
	}

	return content, nil
}
