package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

var settingsFileReference = regexp.MustCompile(`\{file:([^}]*)\}`)

// SettingsFileReferences resolves file substitutions in settings values, including
// embedded substitutions and arrays. JSONC comments are not runtime values.
// Relative paths are based on the settings document, not the process directory.
func SettingsFileReferences(settingsPath, homeDir string) ([]string, error) {
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read file references in %s: %w", settingsPath, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("read file references in %s: %w", settingsPath, err)
	}
	var refs []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		case string:
			for _, match := range settingsFileReference.FindAllStringSubmatch(value, -1) {
				target := filepath.FromSlash(match[1])
				if strings.HasPrefix(target, "~/") || strings.HasPrefix(target, `~\`) {
					target = filepath.Join(homeDir, target[2:])
				} else if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(settingsPath), target)
				}
				refs = append(refs, filepath.Clean(target))
			}
		}
	}
	visit(root)
	slices.Sort(refs)
	return slices.Compact(refs), nil
}

// ValidateSettingsFileReferences rejects dangling or non-file substitutions.
func ValidateSettingsFileReferences(settingsPath, homeDir string) error {
	refs, err := SettingsFileReferences(settingsPath, homeDir)
	if err != nil {
		return err
	}
	for _, target := range refs {
		info, err := os.Stat(target)
		if err == nil && info.IsDir() {
			err = fmt.Errorf("target is a directory")
		}
		if err != nil {
			return fmt.Errorf("invalid file reference in %s: %s: %w; restore the referenced file or correct its {file:...} reference before retrying", settingsPath, target, err)
		}
	}
	return nil
}
