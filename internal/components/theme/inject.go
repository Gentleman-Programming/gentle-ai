package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
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
	return InjectionResult{}, nil
}

// InjectVisualThemes is retained for legacy callers, but Axiom no longer
// installs visual themes. Existing files are handled by guarded cleanup.
func InjectVisualThemes(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	return InjectionResult{}, nil
}

// IsManagedVisualTheme proves that a file still contains the exact bytes an
// earlier Axiom release generated. Names alone are insufficient provenance.
func IsManagedVisualTheme(path string, adapter agents.Adapter, content []byte) bool {
	var values []any
	switch adapter.Agent() {
	case model.AgentClaudeCode:
		values = []any{axiomClaudeTheme, axiomDarkClaudeTheme}
	case model.AgentOpenCode:
		values = []any{axiomOpenCodeTheme, axiomDarkOpenCodeTheme}
	default:
		return false
	}
	var index int
	switch filepath.Base(path) {
	case "axiom.json":
		index = 0
	case "axiom-dark.json":
		index = 1
	default:
		return false
	}
	expected, err := json.MarshalIndent(values[index], "", "  ")
	return err == nil && string(content) == string(append(expected, '\n'))
}

// ManagedVisualThemePaths returns only regular files still identical to old
// Axiom assets. Sync snapshots these paths before attempting their removal.
func ManagedVisualThemePaths(homeDir string, adapter agents.Adapter) ([]string, error) {
	var owned []string
	for _, path := range VisualThemePaths(homeDir, adapter) {
		managed, err := IsManagedVisualThemeFile(path, adapter)
		if err != nil {
			return nil, err
		}
		if managed {
			owned = append(owned, path)
		}
	}
	return owned, nil
}

// IsManagedVisualThemeFile requires both exact content and a link-free path.
// This is shared by sync and uninstall so neither can follow a linked theme dir.
func IsManagedVisualThemeFile(path string, adapter agents.Adapter) (bool, error) {
	linked, err := linkedThemeAncestor(path)
	if err != nil {
		return false, fmt.Errorf("inspect theme ancestors for %q: %w", path, err)
	}
	if linked {
		return false, nil
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect legacy theme %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read legacy theme %q: %w", path, err)
	}
	return IsManagedVisualTheme(path, adapter, content), nil
}

// linkedThemeAncestor rejects symlinks and Windows junctions before a managed
// asset is read or removed. Go reports junctions as non-directories to Lstat.
func linkedThemeAncestor(path string) (bool, error) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return false, nil // The theme path cannot exist below a missing ancestor.
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			return false, nil
		}
	}
}

// RetireManagedVisualThemes rechecks ownership immediately before deletion.
// Modified assets and user-selected settings are left untouched.
func RetireManagedVisualThemes(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	paths, err := ManagedVisualThemePaths(homeDir, adapter)
	if err != nil {
		return InjectionResult{}, err
	}
	result := InjectionResult{}
	for _, path := range paths {
		managed, err := IsManagedVisualThemeFile(path, adapter)
		if err != nil {
			return InjectionResult{}, err
		}
		if !managed {
			continue
		}
		if err := os.Remove(path); err != nil {
			return InjectionResult{}, fmt.Errorf("remove managed visual theme %q: %w", path, err)
		}
		result.Files = append(result.Files, path)
	}
	result.Changed = len(result.Files) > 0
	return result, nil
}

// VisualThemePaths returns paths used by earlier releases, for guarded cleanup.
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
