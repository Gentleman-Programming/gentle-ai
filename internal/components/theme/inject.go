package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

var themeOverlayJSON = []byte("{\n  \"theme\": \"gentleman\"\n}\n")

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

var gentlemanClaudeTheme = claudeTheme{
	Name: "gentleman", Base: "dark",
	Overrides: palette(
		"diffAdded", "#3F4A2D", "diffRemoved", "#5C3838", "diffAddedWord", "#76946A", "diffRemovedWord", "#C34043",
		"chromeYellow", "#DCA561", "briefLabelYou", "#DCA561", "rainbow_yellow", "#DCA561", "yellow_FOR_SUBAGENTS_ONLY", "#DCA561",
	),
}

var gentlemanCuteClaudeTheme = claudeTheme{
	Name: "Gentleman Cute", Base: "dark",
	Overrides: palette(
		"claude", "#E35FA6", "claudeShimmer", "#FF81CC", "text", "#F6EFF3", "inactive", "#A78E9B", "subtle", "#76616B", "suggestion", "#FFB1DD",
		"permission", "#E35FA6", "promptBorder", "#E35FA6", "planMode", "#C49BFF", "autoAccept", "#FF81CC", "bashBorder", "#E0C27A",
		"remember", "#E0C27A", "success", "#D2CBD0", "merged", "#D2CBD0", "error", "#FF718F", "warning", "#F2B86D",
		"diffAdded", "#1A2430", "diffRemoved", "#2D151F", "diffAddedWord", "#2D4F6A", "diffRemovedWord", "#7A2948",
		"userMessageBackground", "#241822", "userMessageBackgroundHover", "#342230", "selectionBg", "#563040", "memoryBackgroundColor", "#1A1218", "bashMessageBackgroundColor", "#151316",
	),
}

var gentlemanOpenCodeTheme = openCodeTheme{
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

var gentlemanCuteOpenCodeTheme = openCodeTheme{
	Schema: openCodeThemeSchema,
	Theme: palette(
		"background", "none", "backgroundPanel", "#1A1218", "backgroundElement", "#241822", "text", "#F6EFF3", "textMuted", "#A78E9B",
		"primary", "#E35FA6", "secondary", "#D7A0B8", "accent", "#E0C27A", "error", "#FF718F", "warning", "#F2B86D", "success", "#D2CBD0", "info", "#D7A0B8",
		"border", "#342230", "borderActive", "#FF81CC", "borderSubtle", "#241822", "diffAdded", "#A9C7EE", "diffRemoved", "#FF718F", "diffContext", "#A78E9B",
		"diffHunkHeader", "#D7A0B8", "diffHighlightAdded", "#A9C7EE", "diffHighlightRemoved", "#FF718F", "diffAddedBg", "#151316", "diffRemovedBg", "#261019", "diffContextBg", "#1A1218",
		"diffLineNumber", "#76616B", "diffAddedLineNumberBg", "#151316", "diffRemovedLineNumberBg", "#261019", "markdownText", "#F6EFF3", "markdownHeading", "#E0C27A",
		"markdownLink", "#E35FA6", "markdownLinkText", "#E35FA6", "markdownCode", "#E0C27A", "markdownBlockQuote", "#A78E9B", "markdownEmph", "#D7A0B8", "markdownStrong", "#E0C27A",
		"markdownHorizontalRule", "#A78E9B", "markdownListItem", "#E35FA6", "markdownListEnumeration", "#D7A0B8", "markdownImage", "#E35FA6", "markdownImageText", "#E35FA6", "markdownCodeBlock", "#F6EFF3",
		"syntaxComment", "#A78E9B", "syntaxKeyword", "#C15995", "syntaxFunction", "#D7A0B8", "syntaxVariable", "#F6EFF3", "syntaxString", "#E0C27A", "syntaxNumber", "#D7A0B8", "syntaxType", "#A9C7EE", "syntaxOperator", "#E35FA6", "syntaxPunctuation", "#A78E9B",
	),
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	writeResult, err := mergeJSONFile(settingsPath, themeOverlayJSON)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
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
		values = []any{gentlemanClaudeTheme, gentlemanCuteClaudeTheme}
	case model.AgentOpenCode:
		values = []any{gentlemanOpenCodeTheme, gentlemanCuteOpenCodeTheme}
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
	return []string{filepath.Join(root, "gentleman.json"), filepath.Join(root, "gentleman-cute.json")}
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
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
