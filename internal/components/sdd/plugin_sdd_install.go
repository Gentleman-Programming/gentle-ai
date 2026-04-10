package sdd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// pluginSddDirName is the name of the plugin directory as it lives in assets
// and as it will be installed under ~/.config/opencode/plugins/.
const pluginSddDirName = "plugin-sdd-opencode"

// pluginSddRegistryKey is the key used to register the plugin in tui.json.
const pluginSddRegistryKey = "./plugins/" + pluginSddDirName + "/"

// tuiJSONSchema is the $schema value written to tui.json.
const tuiJSONSchema = "https://opencode.ai/tui.json"

// opencodePackageRequirements lists the dependencies that must be present in
// ~/.config/opencode/package.json for the plugin to load correctly.
var opencodePackageRequirements = map[string]string{
	"@opencode-ai/plugin":    "1.4.2",
	"unique-names-generator": "^4.7.1",
}

// opencodePluginMinVersion is the minimum supported version for the runtime
// plugin API dependency.
const opencodePluginMinVersion = "1.4.2"

// PluginInstallResult holds the outcome of InstallSddPlugin.
type PluginInstallResult struct {
	// FilesChanged is the number of files that were created or updated.
	FilesChanged int
	// AlreadyInstalled is true when the plugin was already present and
	// tui.json already registered it — nothing was written.
	AlreadyInstalled bool
	// PackageWarning is non-empty when ~/.config/opencode/package.json is
	// missing a required dependency. The plugin is still installed but the
	// user should run `bun install` (or npm install) in that directory.
	PackageWarning string
}

// InstallSddPlugin copies the plugin-sdd-opencode directory from embedded
// assets into ~/.config/opencode/plugins/, writes tui.json with the plugin
// registration, and validates that ~/.config/opencode/package.json contains
// the required dependencies.
//
// homeDir is the user home directory (os.UserHomeDir()).
// The function is idempotent: re-running it only writes files that changed.
func InstallSddPlugin(homeDir string) (PluginInstallResult, error) {
	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
	pluginsDir := filepath.Join(opencodeDir, "plugins")
	destDir := filepath.Join(pluginsDir, pluginSddDirName)

	// 1. Copy all embedded plugin files recursively.
	filesChanged, err := copyEmbeddedPlugin(destDir)
	if err != nil {
		return PluginInstallResult{}, fmt.Errorf("copy plugin-sdd-opencode: %w", err)
	}

	// 2. Write / update tui.json.
	tuiChanged, err := writeTUIJSON(opencodeDir)
	if err != nil {
		return PluginInstallResult{}, fmt.Errorf("write tui.json: %w", err)
	}
	if tuiChanged {
		filesChanged++
	}

	// 3. Ensure package.json has required deps — non-fatal, produces a warning only.
	warning := ensureOpencodePackageJSON(opencodeDir)

	alreadyInstalled := filesChanged == 0 && !tuiChanged
	return PluginInstallResult{
		FilesChanged:     filesChanged,
		AlreadyInstalled: alreadyInstalled,
		PackageWarning:   warning,
	}, nil
}

// copyEmbeddedPlugin removes any existing plugin directory at destDir and
// copies all files from the embedded asset tree fresh. This ensures stale
// files from a previous version are never left behind.
// Returns the number of files written.
func copyEmbeddedPlugin(destDir string) (int, error) {
	// Remove existing directory so stale files from older versions are cleaned up.
	if _, err := os.Stat(destDir); err == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return 0, fmt.Errorf("remove existing plugin directory %q: %w", destDir, err)
		}
	}

	assetRoot := "opencode/plugins/" + pluginSddDirName
	changed := 0

	err := fs.WalkDir(assets.FS, assetRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		// Compute relative path from asset root.
		rel, err := filepath.Rel(assetRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path %q: %w", path, err)
		}

		content, err := assets.Read(path)
		if err != nil {
			return fmt.Errorf("read embedded asset %q: %w", path, err)
		}

		dest := filepath.Join(destDir, rel)
		result, err := filemerge.WriteFileAtomic(dest, []byte(content), 0o644)
		if err != nil {
			return fmt.Errorf("write %q: %w", dest, err)
		}
		if result.Changed {
			changed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return changed, nil
}

// writeTUIJSON writes ~/.config/opencode/tui.json with the plugin-sdd-opencode
// entry. If the file already exists and already contains the entry, it is left
// unchanged (idempotent). Other existing plugin entries are preserved,
// including tuple-style entries like [path, options].
func writeTUIJSON(opencodeDir string) (bool, error) {
	tuiPath := filepath.Join(opencodeDir, "tui.json")

	raw := map[string]json.RawMessage{}
	plugins := []any{}
	schema := tuiJSONSchema

	// Read existing tui.json if present.
	if data, err := os.ReadFile(tuiPath); err == nil {
		// Best-effort parse — if it fails we start fresh with the correct schema.
		_ = json.Unmarshal(data, &raw)

		if schemaRaw, ok := raw["$schema"]; ok {
			_ = json.Unmarshal(schemaRaw, &schema)
			if schema == "" {
				schema = tuiJSONSchema
			}
		}

		if pluginsRaw, ok := raw["plugin"]; ok {
			_ = json.Unmarshal(pluginsRaw, &plugins)
		}
	}

	// Check if the plugin is already registered.
	if pluginEntryExists(plugins, pluginSddRegistryKey) {
		return false, nil // already present, nothing to do
	}

	// Append the plugin entry.
	plugins = append(plugins, pluginSddRegistryKey)

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return false, fmt.Errorf("marshal tui.json schema: %w", err)
	}
	raw["$schema"] = schemaJSON

	pluginsJSON, err := json.Marshal(plugins)
	if err != nil {
		return false, fmt.Errorf("marshal tui.json plugins: %w", err)
	}
	raw["plugin"] = pluginsJSON

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal tui.json: %w", err)
	}
	out = append(out, '\n')

	result, err := filemerge.WriteFileAtomic(tuiPath, out, 0o644)
	if err != nil {
		return false, err
	}

	return result.Changed, nil
}

func pluginEntryExists(entries []any, target string) bool {
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			if v == target {
				return true
			}
		case []any:
			if len(v) > 0 {
				if path, ok := v[0].(string); ok && path == target {
					return true
				}
			}
		}
	}
	return false
}

// opencodePackageJSON is the shape of ~/.config/opencode/package.json.
// We use map[string]json.RawMessage for the top level so we can round-trip
// unknown fields (scripts, engines, etc.) without losing them.
type opencodePackageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// ensureOpencodePackageJSON reads ~/.config/opencode/package.json, adds any
// required dependencies that are missing, and writes the file back.
// If the file does not exist it is created with the required deps.
// Existing entries with equal or newer versions are left untouched.
// Returns a non-empty warning string only when the file could not be written
// and the caller should inform the user to run `bun install` manually.
func ensureOpencodePackageJSON(opencodeDir string) string {
	pkgPath := filepath.Join(opencodeDir, "package.json")

	// raw holds the full JSON so we preserve all unknown keys on write.
	raw := map[string]json.RawMessage{}

	data, err := os.ReadFile(pkgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf("Could not read ~/.config/opencode/package.json: %v", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return "Could not parse ~/.config/opencode/package.json — verify it manually."
		}
	}

	// Decode the dependencies section (create if absent).
	deps := map[string]string{}
	if v, ok := raw["dependencies"]; ok {
		_ = json.Unmarshal(v, &deps)
	}

	// Add missing entries and enforce a minimum version for @opencode-ai/plugin.
	added := []string{}
	updated := []string{}
	for pkg, requiredVer := range opencodePackageRequirements {
		currentVer, exists := deps[pkg]
		if !exists {
			deps[pkg] = requiredVer
			added = append(added, pkg)
			continue
		}

		if pkg == "@opencode-ai/plugin" && isVersionBelowMinimum(currentVer, opencodePluginMinVersion) {
			deps[pkg] = requiredVer
			updated = append(updated, pkg)
		}
	}

	if len(added) == 0 && len(updated) == 0 {
		return "" // nothing to do
	}

	// Re-encode the dependencies section.
	depsJSON, err := json.Marshal(deps)
	if err != nil {
		return fmt.Sprintf("Could not encode dependencies for package.json: %v", err)
	}
	raw["dependencies"] = depsJSON

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Sprintf("Could not marshal package.json: %v", err)
	}
	out = append(out, '\n')

	result, err := filemerge.WriteFileAtomic(pkgPath, out, 0o644)
	if err != nil {
		return fmt.Sprintf(
			"Could not write ~/.config/opencode/package.json: %v\n"+
				"Run manually: cd %s && bun add %s",
			err, opencodeDir, joinDeps(added),
		)
	}

	if result.Changed {
		actions := []string{}
		if len(added) > 0 {
			actions = append(actions, "added missing deps: "+joinDeps(added))
		}
		if len(updated) > 0 {
			actions = append(actions, "updated deps to minimum supported version: "+joinDeps(updated))
		}
		return fmt.Sprintf(
			"Updated package.json (%s)\n"+
				"Run: cd %s && bun install",
			joinDeps(actions), opencodeDir,
		)
	}

	return ""
}

func joinDeps(deps []string) string {
	result := ""
	for i, d := range deps {
		if i > 0 {
			result += " "
		}
		result += d
	}
	return result
}

func isVersionBelowMinimum(current, minimum string) bool {
	currentParts, okCurrent := parseSemverPrefix(current)
	minimumParts, okMinimum := parseSemverPrefix(minimum)
	if !okMinimum {
		return false
	}
	if !okCurrent {
		// Unknown/non-semver range like "latest" or workspace aliases:
		// preserve it instead of guessing.
		return false
	}

	for i := 0; i < 3; i++ {
		if currentParts[i] < minimumParts[i] {
			return true
		}
		if currentParts[i] > minimumParts[i] {
			return false
		}
	}
	return false
}

func parseSemverPrefix(v string) ([3]int, bool) {
	var result [3]int
	v = strings.TrimSpace(v)
	if v == "" {
		return result, false
	}

	for len(v) > 0 {
		switch v[0] {
		case '^', '~', 'v', 'V', '>', '<', '=', ' ':
			v = v[1:]
		default:
			goto cleaned
		}
	}

cleaned:
	if v == "" {
		return result, false
	}

	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return result, false
	}

	for i := 0; i < 3; i++ {
		num := 0
		if parts[i] == "" {
			return result, false
		}
		for _, r := range parts[i] {
			if r < '0' || r > '9' {
				break
			}
			num = num*10 + int(r-'0')
		}
		result[i] = num
	}

	return result, true
}
