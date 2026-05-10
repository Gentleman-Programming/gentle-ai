package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

var themeOverlayJSON = []byte("{\n  \"theme\": \"gentleman-kanagawa\"\n}\n")

type claudeTheme struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Overrides map[string]string `json:"overrides"`
}

var gentlemanClaudeTheme = claudeTheme{
	Name: "Gentleman",
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
