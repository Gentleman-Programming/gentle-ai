package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EngramExtensionStatus struct {
	Configured    bool
	ExtensionPath string
	Source        string
	PackageName   string
	Message       string
}

func ValidateEngramExtension(paths Paths) (EngramExtensionStatus, error) {
	data, err := os.ReadFile(paths.SettingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return EngramExtensionStatus{}, fmt.Errorf("missing PI→Engram extension contract at %s (MCP/Context7 are out of scope for this iteration)", paths.SettingsPath)
		}
		return EngramExtensionStatus{}, fmt.Errorf("read PI settings %q: %w", paths.SettingsPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return EngramExtensionStatus{}, fmt.Errorf("parse PI settings %q: %w", paths.SettingsPath, err)
	}
	if status, ok, err := validateExplicitContract(root, paths.SettingsPath); ok || err != nil {
		return status, err
	}

	status, ok, err := validatePackagesContract(root, paths)
	if ok || err != nil {
		return status, err
	}

	return EngramExtensionStatus{}, fmt.Errorf("missing PI→Engram extension contract in %s (expected engram.extension.path or packages[] entry like ../../Documents/repos/pi-engram or npm:pi-engram)", paths.SettingsPath)
}

func validateExplicitContract(root map[string]any, settingsPath string) (EngramExtensionStatus, bool, error) {
	engramRaw, ok := root["engram"].(map[string]any)
	if !ok {
		return EngramExtensionStatus{}, false, nil
	}

	extensionRaw, ok := engramRaw["extension"].(map[string]any)
	if !ok {
		return EngramExtensionStatus{}, false, nil
	}

	pathRaw, pathOK := extensionRaw["path"]
	pathValue, isString := pathRaw.(string)
	if !pathOK || !isString {
		return EngramExtensionStatus{}, true, fmt.Errorf("malformed PI→Engram extension contract in %s (engram.extension.path must be a non-empty string)", settingsPath)
	}

	extensionPath := strings.TrimSpace(pathValue)
	if extensionPath == "" {
		return EngramExtensionStatus{}, true, fmt.Errorf("missing PI→Engram extension contract in %s (expected engram.extension.path)", settingsPath)
	}

	enabled, ok := extensionRaw["enabled"].(bool)
	if !ok || !enabled {
		return EngramExtensionStatus{}, true, fmt.Errorf("malformed PI→Engram extension contract in %s (engram.extension.enabled must be true)", settingsPath)
	}

	return EngramExtensionStatus{
		Configured:    true,
		ExtensionPath: extensionPath,
		Source:        "explicit",
		Message:       "PI Engram extension contract validated",
	}, true, nil
}

func validatePackagesContract(root map[string]any, paths Paths) (EngramExtensionStatus, bool, error) {
	packagesRaw, exists := root["packages"]
	if !exists {
		return EngramExtensionStatus{}, false, nil
	}

	packages, ok := packagesRaw.([]any)
	if !ok {
		return EngramExtensionStatus{}, true, fmt.Errorf("malformed PI→Engram extension contract in %s (packages must be an array)", paths.SettingsPath)
	}

	for _, item := range packages {
		entry, ok := item.(string)
		if !ok {
			continue
		}

		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if pkgName, match := normalizeNpmEngramPackage(entry); match {
			return EngramExtensionStatus{
				Configured:    true,
				ExtensionPath: "npm:" + pkgName,
				Source:        "package-npm",
				PackageName:   pkgName,
				Message:       "PI Engram extension resolved from packages[] npm entry",
			}, true, nil
		}

		if isLocalPathLike(entry) && looksLikeEngramPath(entry) {
			resolved := resolvePackagePath(paths.Root, entry)
			if _, err := os.Stat(resolved); err != nil {
				if os.IsNotExist(err) {
					return EngramExtensionStatus{}, true, fmt.Errorf("missing PI→Engram extension contract in %s (packages[] local entry %q resolves to %q, but it does not exist or is not readable)", paths.SettingsPath, entry, resolved)
				}
				return EngramExtensionStatus{}, true, fmt.Errorf("read PI→Engram extension local path %q: %w", resolved, err)
			}
			return EngramExtensionStatus{
				Configured:    true,
				ExtensionPath: resolved,
				Source:        "package-local",
				PackageName:   filepath.Base(filepath.Clean(resolved)),
				Message:       "PI Engram extension resolved from packages[] local path",
			}, true, nil
		}
	}

	return EngramExtensionStatus{}, false, nil
}

func normalizeNpmEngramPackage(entry string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(entry), "npm:") {
		return "", false
	}

	pkgName := strings.TrimSpace(entry[len("npm:"):])
	if pkgName == "" {
		return "", false
	}

	if !looksLikeEngramPackageName(pkgName) {
		return "", false
	}

	return pkgName, true
}

func looksLikeEngramPackageName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}

	return strings.Contains(lower, "pi-engram") || (strings.Contains(lower, "engram") && strings.Contains(lower, "pi"))
}

func looksLikeEngramPath(pathValue string) bool {
	base := strings.ToLower(filepath.Base(filepath.Clean(pathValue)))
	return strings.Contains(base, "pi-engram") || (strings.Contains(base, "engram") && strings.Contains(base, "pi"))
}

func isLocalPathLike(entry string) bool {
	if filepath.IsAbs(entry) {
		return true
	}

	if strings.HasPrefix(entry, "./") || strings.HasPrefix(entry, "../") || strings.HasPrefix(entry, "~/") {
		return true
	}

	return strings.Contains(entry, "/") || strings.Contains(entry, `\\`)
}

func resolvePackagePath(piRoot string, entry string) string {
	if filepath.IsAbs(entry) {
		return filepath.Clean(entry)
	}

	if strings.HasPrefix(entry, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil && strings.TrimSpace(homeDir) != "" {
			return filepath.Clean(filepath.Join(homeDir, strings.TrimPrefix(entry, "~/")))
		}
		return filepath.Clean(entry)
	}

	return filepath.Clean(filepath.Join(piRoot, entry))
}
