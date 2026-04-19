package piresources

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

const managedPackageKey = "pi-gentle-ai"

var managedPackageOverlayJSON = []byte(`{
  "gentleAI": {
    "managedPackages": {
      "pi-gentle-ai": {
        "managedBy": "gentle-ai",
        "source": "internal-assets"
      }
    }
  }
}
`)

type InjectionResult struct {
	Changed bool
	Files   []string
}

func extendedPiAdapter(adapter agents.Adapter) (agents.ExtendedResourceAdapter, bool) {
	if adapter == nil || adapter.Agent() != model.AgentPi {
		return nil, false
	}
	ext, ok := adapter.(agents.ExtendedResourceAdapter)
	if !ok {
		return nil, false
	}
	return ext, true
}

// PathsForComponent returns the expected pi-native managed resource paths for
// a component. Non-pi adapters return nil.
func PathsForComponent(homeDir string, adapter agents.Adapter, component model.ComponentID) []string {
	ext, ok := extendedPiAdapter(adapter)
	if !ok {
		return nil
	}

	switch component {
	case model.ComponentEngram:
		if !ext.SupportsExtensions() {
			return nil
		}
		return []string{filepath.Join(ext.ExtensionsDir(homeDir), "engram-tools.ts")}
	case model.ComponentContext7:
		if !ext.SupportsExtensions() {
			return nil
		}
		return []string{filepath.Join(ext.ExtensionsDir(homeDir), "context7-tools.ts")}
	case model.ComponentSDD:
		if !ext.SupportsPromptTemplates() {
			return nil
		}
		paths := promptPaths(ext.PromptsDir(homeDir))
		if settings := adapter.SettingsPath(homeDir); settings != "" {
			paths = append(paths, settings)
		}
		return paths
	case model.ComponentTheme:
		if !ext.SupportsThemes() {
			return nil
		}
		return []string{filepath.Join(ext.ThemesDir(homeDir), "gentleman-kanagawa.json")}
	default:
		return nil
	}
}

func promptPaths(promptDir string) []string {
	entries, err := assets.FS.ReadDir("pi/prompts")
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(promptDir, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

// Inject writes pi-native resources for the given component.
// Non-pi adapters are a no-op.
func Inject(homeDir string, adapter agents.Adapter, component model.ComponentID) (InjectionResult, error) {
	ext, ok := extendedPiAdapter(adapter)
	if !ok {
		return InjectionResult{}, nil
	}

	result := InjectionResult{Files: []string{}}

	switch component {
	case model.ComponentEngram:
		if !ext.SupportsExtensions() {
			return result, nil
		}
		changed, path, err := writeAssetFile("pi/extensions/engram-tools.ts", filepath.Join(ext.ExtensionsDir(homeDir), "engram-tools.ts"))
		if err != nil {
			return InjectionResult{}, err
		}
		result.Changed = result.Changed || changed
		result.Files = append(result.Files, path)
		return result, nil

	case model.ComponentContext7:
		if !ext.SupportsExtensions() {
			return result, nil
		}
		changed, path, err := writeAssetFile("pi/extensions/context7-tools.ts", filepath.Join(ext.ExtensionsDir(homeDir), "context7-tools.ts"))
		if err != nil {
			return InjectionResult{}, err
		}
		result.Changed = result.Changed || changed
		result.Files = append(result.Files, path)
		return result, nil

	case model.ComponentSDD:
		if ext.SupportsPromptTemplates() {
			changed, files, err := writeAssetDir("pi/prompts", ext.PromptsDir(homeDir))
			if err != nil {
				return InjectionResult{}, err
			}
			result.Changed = result.Changed || changed
			result.Files = append(result.Files, files...)
		}
		if settingsPath := adapter.SettingsPath(homeDir); settingsPath != "" {
			writeResult, err := mergeJSONFile(settingsPath, managedPackageOverlayJSON)
			if err != nil {
				return InjectionResult{}, err
			}
			result.Changed = result.Changed || writeResult.Changed
			result.Files = append(result.Files, settingsPath)
		}
		return result, nil

	case model.ComponentTheme:
		if !ext.SupportsThemes() {
			return result, nil
		}
		changed, path, err := writeAssetFile("pi/themes/gentleman-kanagawa.json", filepath.Join(ext.ThemesDir(homeDir), "gentleman-kanagawa.json"))
		if err != nil {
			return InjectionResult{}, err
		}
		result.Changed = result.Changed || changed
		result.Files = append(result.Files, path)
		return result, nil
	}

	return result, nil
}

func writeAssetDir(assetDir, outDir string) (bool, []string, error) {
	entries, err := assets.FS.ReadDir(assetDir)
	if err != nil {
		return false, nil, fmt.Errorf("read embedded dir %q: %w", assetDir, err)
	}
	changed := false
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		assetPath := filepath.Join(assetDir, entry.Name())
		outPath := filepath.Join(outDir, entry.Name())
		fileChanged, _, err := writeAssetFile(assetPath, outPath)
		if err != nil {
			return false, nil, err
		}
		changed = changed || fileChanged
		files = append(files, outPath)
	}
	sort.Strings(files)
	return changed, files, nil
}

func writeAssetFile(assetPath, outPath string) (bool, string, error) {
	content, err := assets.Read(assetPath)
	if err != nil {
		return false, "", fmt.Errorf("read embedded asset %q: %w", assetPath, err)
	}
	result, err := filemerge.WriteFileAtomic(outPath, []byte(content), 0o644)
	if err != nil {
		return false, "", err
	}
	return result.Changed, outPath, nil
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
