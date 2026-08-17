package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

var themeOverlayJSON = []byte("{\n  \"theme\": \"gentleman\"\n}\n")

const openCodeThemeAsset = "opencode/themes/gentleman.json"

type claudeTheme struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Overrides map[string]string `json:"overrides"`
}

var gentlemanClaudeTheme = claudeTheme{
	Name: "gentleman",
	Base: "dark",
	Overrides: map[string]string{
		"diffAdded":                 "#3F4A2D",
		"diffRemoved":               "#5C3838",
		"diffAddedWord":             "#76946A",
		"diffRemovedWord":           "#C34043",
		"chromeYellow":              "#DCA561",
		"briefLabelYou":             "#DCA561",
		"rainbow_yellow":            "#DCA561",
		"yellow_FOR_SUBAGENTS_ONLY": "#DCA561",
	},
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if adapter.Agent() != model.AgentOpenCode {
		return InjectionResult{}, nil
	}

	configDir := adapter.GlobalConfigDir(homeDir)
	tuiPath := filepath.Join(configDir, "tui.json")
	themePath := filepath.Join(configDir, "themes", "gentleman.json")
	themeContent, err := assets.Read(openCodeThemeAsset)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("read bundled OpenCode theme: %w", err)
	}

	themeResult, err := filemerge.WriteFileAtomic(themePath, []byte(themeContent), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write OpenCode theme: %w", err)
	}
	tuiResult, err := mergeJSONFile(tuiPath, themeOverlayJSON)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("merge OpenCode tui theme: %w", err)
	}

	return InjectionResult{
		Changed: themeResult.Changed || tuiResult.Changed,
		Files:   []string{tuiPath, themePath},
	}, nil
}

func InjectClaudeTheme(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if adapter.Agent() != model.AgentClaudeCode {
		return InjectionResult{}, nil
	}

	themePath := filepath.Join(homeDir, ".claude", "themes", "gentleman.json")
	content, err := json.MarshalIndent(gentlemanClaudeTheme, "", "  ")
	if err != nil {
		return InjectionResult{}, err
	}
	content = append(content, '\n')

	writeResult, err := filemerge.WriteFileAtomic(themePath, content, 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{themePath}}, nil
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
