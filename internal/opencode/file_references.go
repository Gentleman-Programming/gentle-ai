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

var settingsFileReference = regexp.MustCompile(`\{file:([^}]+)\}`)

// The stat boundary is replaceable by tests so hostile paths are never probed.
var statSettingsFileReference = os.Stat

// ValidateLocalFileReference rejects Windows UNC, device, and NT object-manager
// (\??\) namespaces without touching the filesystem. Check raw paths before
// cleaning, which can erase a leading double separator, and check resolved
// paths before probing them.
func ValidateLocalFileReference(target string) error {
	normalized := strings.ReplaceAll(target, `\`, "/")
	if strings.HasPrefix(normalized, "//") || normalized == "/??" || strings.HasPrefix(normalized, "/??/") {
		return fmt.Errorf("remote or device paths are not supported")
	}
	return nil
}

func invalidSettingsFileReference(settingsPath, target string, err error) error {
	return fmt.Errorf("invalid file reference in %s: %s: %w; restore the referenced file or correct its {file:...} reference before retrying", settingsPath, target, err)
}

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
	var visit func(any) error
	visit = func(value any) error {
		switch value := value.(type) {
		case map[string]any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		case string:
			for _, match := range settingsFileReference.FindAllStringSubmatch(value, -1) {
				if err := ValidateLocalFileReference(match[1]); err != nil {
					return invalidSettingsFileReference(settingsPath, match[1], err)
				}
				target := filepath.FromSlash(match[1])
				if strings.HasPrefix(target, "~/") || strings.HasPrefix(target, `~\`) {
					if err := ValidateLocalFileReference(homeDir); err != nil {
						return invalidSettingsFileReference(settingsPath, homeDir, err)
					}
					target = filepath.Join(homeDir, target[2:])
				} else if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(settingsPath), target)
				}
				if err := ValidateLocalFileReference(target); err != nil {
					return invalidSettingsFileReference(settingsPath, target, err)
				}
				refs = append(refs, filepath.Clean(target))
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
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
		info, err := statSettingsFileReference(target)
		if err == nil && info.IsDir() {
			err = fmt.Errorf("target is a directory")
		}
		if err != nil {
			return invalidSettingsFileReference(settingsPath, target, err)
		}
	}
	return nil
}
