package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

type Definition struct {
	ID          model.OpenCodeCommunityPluginID
	Name        string
	PackageName string
	RepoURL     string
	Owner       string
	Repo        string
	Description string
}

type Result struct {
	Changed bool
	Files   []string
}

// Retained only to identify registrations owned by older installations during
// explicit CLI uninstall. External plugins are never installable or updatable.
var legacyStatuslineDefinition = Definition{
	ID:          model.OpenCodePluginSubAgentStatusline,
	Name:        "Sub-agent Statusline",
	PackageName: "opencode-subagent-statusline",
	RepoURL:     "https://github.com/Joaquinvesapa/sub-agent-statusline",
	Owner:       "Joaquinvesapa",
	Repo:        "sub-agent-statusline",
	Description: "OpenCode sidebar/statusline for sub-agent activity",
}

// Retain only for identifying registrations owned by older installations during
// uninstall. It is not offered in the installation or update catalogs.
var legacySDDEngramDefinition = Definition{
	ID:          model.OpenCodePluginSDDEngramManage,
	Name:        "SDD Engram Manager",
	PackageName: "opencode-sdd-engram-manage",
	RepoURL:     "https://github.com/j0k3r-dev-rgl/sdd-engram-plugin",
	Owner:       "j0k3r-dev-rgl",
	Repo:        "sdd-engram-plugin",
	Description: "OpenCode TUI for SDD profiles and Engram memories",
}

const gentleLogoBundleFile = "gentle-logo.js"

// gentleLogoPluginFile is the T1-era TSX artifact name, still referenced by
// the uninstall path until the uninstaller learns the T2 layout.
const gentleLogoPluginFile = "gentle-logo.tsx"

// OpenCode configuration file names and registration entries, mirroring the
// cortex-ia reference installer (issue #5364): V2 loads cli.json plus the
// managed opencode.jsonc plugins array with a relative directory package;
// V1 loads tui.jsonc/tui.json with a relative bundle path.
const (
	gentleLogoBridgeDir   = "gentle-logo"
	gentleLogoV1Entry     = "./tui-plugins/gentle-logo.js"
	gentleLogoV2Entry     = "./tui-plugins/gentle-logo"
	cliConfigName         = "cli.json"
	tuiJSONCName          = "tui.jsonc"
	tuiJSONName           = "tui.json"
	opencodeJSONCName     = "opencode.jsonc"
	opencodeJSONName      = "opencode.json"
	cliSchemaV2           = "https://opencode.ai/v2/cli.json"
	tuiSchemaV1           = "https://opencode.ai/tui.json"
	opencodeSchemaV2      = "https://opencode.ai/config.json"
	bridgeTUIFileName     = "tui.js"
	bridgeManifestName    = "package.json"
	bridgeTUIFileContent  = "// OpenCode v2 directory bridge for the gentle-logo TUI plugin\nexport * from \"../gentle-logo.js\";\nexport { default } from \"../gentle-logo.js\";\n"
	bridgeManifestContent = "{\n  \"name\": \"gentle-logo-tui\",\n  \"private\": true,\n  \"type\": \"module\",\n  \"exports\": {\n    \".\": \"./tui.js\",\n    \"./tui\": \"./tui.js\"\n  }\n}\n"
)

// openCodeGeneration is the OpenCode configuration generation that owns the
// config directory. Durable artifacts OpenCode itself wrote (cli.json is V2;
// tui.jsonc/tui.json are V1) win over the version probe, and the product
// default is V2, mirroring the cortex-ia reference installer.
type openCodeGeneration int

const (
	generationV1 openCodeGeneration = iota
	generationV2
)

// fileExists reports whether path exists on disk. It returns false only when
// the stat error indicates the path is genuinely missing; any other failure
// (permission, I-O, invalid path) is propagated so callers never treat an
// unreadable location as absence.
func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// detectOpenCodeGeneration applies the durable-artifact-first cascade. The
// probed major only breaks ties for an otherwise empty config directory.
// Stat failures other than absence are propagated: an unreadable config
// directory must not silently select the wrong generation.
func detectOpenCodeGeneration(configDir string, major opencode.RuntimeMajor) (openCodeGeneration, error) {
	cliExists, err := fileExists(filepath.Join(configDir, cliConfigName))
	if err != nil {
		return generationV1, fmt.Errorf("probe %s: %w", filepath.Join(configDir, cliConfigName), err)
	}
	if cliExists {
		return generationV2, nil
	}
	jsoncExists, err := fileExists(filepath.Join(configDir, tuiJSONCName))
	if err != nil {
		return generationV1, fmt.Errorf("probe %s: %w", filepath.Join(configDir, tuiJSONCName), err)
	}
	if jsoncExists {
		return generationV1, nil
	}
	jsonExists, err := fileExists(filepath.Join(configDir, tuiJSONName))
	if err != nil {
		return generationV1, fmt.Errorf("probe %s: %w", filepath.Join(configDir, tuiJSONName), err)
	}
	if jsonExists {
		return generationV1, nil
	}
	if major == opencode.RuntimeV1 {
		return generationV1, nil
	}
	return generationV2, nil
}

// v1TUIConfigPath mirrors the V1 cascade: an existing tui.jsonc wins, then
// tui.json, and a fresh V1 home materializes tui.jsonc.
func v1TUIConfigPath(configDir string) (string, error) {
	jsonc := filepath.Join(configDir, tuiJSONCName)
	jsoncExists, err := fileExists(jsonc)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", jsonc, err)
	}
	if jsoncExists {
		return jsonc, nil
	}
	jsonPath := filepath.Join(configDir, tuiJSONName)
	jsonExists, err := fileExists(jsonPath)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", jsonPath, err)
	}
	if jsonExists {
		return jsonPath, nil
	}
	return jsonc, nil
}

// managedOpenCodeConfigPath follows the MCP manager's load precedence: JSONC
// is loaded after JSON and therefore owns conflicting keys when both exist.
func managedOpenCodeConfigPath(configDir string) (string, error) {
	jsonc := filepath.Join(configDir, opencodeJSONCName)
	jsoncExists, err := fileExists(jsonc)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", jsonc, err)
	}
	if jsoncExists {
		return jsonc, nil
	}
	jsonPath := filepath.Join(configDir, opencodeJSONName)
	jsonExists, err := fileExists(jsonPath)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", jsonPath, err)
	}
	if jsonExists {
		return jsonPath, nil
	}
	return jsonc, nil
}

// ManagedLogoFiles returns the managed gentle-logo file set for the detected
// OpenCode generation: the pre-built bundle, the V2 bridge directory and
// registration files, or the V1 registration file. Callers use it for backup
// and verification target lists, so it mirrors exactly what Install writes.
func ManagedLogoFiles(homeDir string) ([]string, error) {
	major, err := opencode.DetectRuntimeMajor(context.Background())
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(homeDir, ".config", "opencode")
	generation, err := detectOpenCodeGeneration(configDir, major)
	if err != nil {
		return nil, err
	}
	return managedLogoPaths(configDir, generation)
}

// EmbeddedBundle returns the go:embedded pre-built gentle-logo ESM bundle.
// OpenCode never transpiles TSX, so the committed build artifact produced by
// internal/tuiassets is the only plugin payload the installer ships.
func EmbeddedBundle() ([]byte, error) {
	data, err := assets.Read("tui/" + gentleLogoBundleFile)
	if err != nil {
		return nil, fmt.Errorf("read embedded gentle-logo bundle: %w", err)
	}
	return []byte(data), nil
}

// DefinitionFor resolves a community plugin ID to its catalog definition. It
// reports false for unknown or retired IDs; legacy definitions exist only so
// the uninstaller can identify registrations owned by older installations.
func DefinitionFor(id model.OpenCodeCommunityPluginID) (Definition, bool) {
	if id == model.OpenCodePluginSDDEngramManage {
		return legacySDDEngramDefinition, true
	}
	if id == model.OpenCodePluginSubAgentStatusline {
		return legacyStatuslineDefinition, true
	}
	return Definition{}, false
}

// Install ships the gentle-logo TUI plugin and registers it with the OpenCode
// generation that owns the config directory. Every other community plugin ID
// is refused: external plugins are never installable. The runtime probe is a
// fail-closed pre-flight, so an unresolvable runtime blocks the install.
func Install(homeDir string, id model.OpenCodeCommunityPluginID) (Result, error) {
	// Legacy lookup is uninstall-only: refuse before probing or mutating anything.
	if id != model.OpenCodePluginGentleLogo {
		return Result{}, fmt.Errorf("OpenCode community plugin %q is not installable; existing configuration preserved", id)
	}
	// Runtime detection remains a fail-closed pre-flight: an unresolvable
	// runtime still blocks every managed asset, including the logo.
	major, err := opencode.DetectRuntimeMajor(context.Background())
	if err != nil {
		return Result{}, err
	}

	return installGentleLogo(homeDir, major)
}

// installGentleLogo ships the pre-built bundle and registers it with the
// OpenCode generation that owns the config directory (issue #5364): V2 gets
// the bridge directory package registered in cli.json and the managed
// opencode.jsonc plugins union; V1 gets the relative bundle path in
// tui.json(c). Every touched file is captured first so a failure compensates
// as one recoverable operation (#1678), including the landed-with-error
// window of WriteFileAtomic (#1676).
func installGentleLogo(homeDir string, major opencode.RuntimeMajor) (Result, error) {
	bundle, err := EmbeddedBundle()
	if err != nil {
		return Result{}, err
	}
	configDir := filepath.Join(homeDir, ".config", "opencode")
	pluginDir := filepath.Join(configDir, "tui-plugins")
	generation, err := detectOpenCodeGeneration(configDir, major)
	if err != nil {
		return Result{}, err
	}
	paths, err := managedLogoPaths(configDir, generation)
	if err != nil {
		return Result{}, err
	}

	priors := make(map[string]priorFile, len(paths))
	for _, path := range paths {
		prior, err := capturePriorFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("capture prior Gentle Logo TUI plugin state for %s: %w", path, err)
		}
		priors[path] = prior
	}
	rollback := func(cause error) error {
		joined := []error{cause}
		for path, prior := range priors {
			if restoreErr := prior.restore(path); restoreErr != nil {
				joined = append(joined, fmt.Errorf("roll back %s, the previous state could not be restored: %w", path, restoreErr))
			}
		}
		if len(joined) == 1 {
			return cause
		}
		return errors.Join(joined...)
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return Result{}, rollback(fmt.Errorf("create OpenCode tui-plugins directory: %w", err))
	}
	bundleWrite, err := writeFileAtomicFn(filepath.Join(pluginDir, gentleLogoBundleFile), bundle, 0o644)
	if err != nil {
		return Result{}, rollback(fmt.Errorf("write gentle-logo bundle: %w", err))
	}
	changed := bundleWrite.Changed

	if generation == generationV2 {
		bridgeDir := filepath.Join(pluginDir, gentleLogoBridgeDir)
		if err := os.MkdirAll(bridgeDir, 0o755); err != nil {
			return Result{}, rollback(fmt.Errorf("create gentle-logo bridge directory: %w", err))
		}
		tuiChanged, err := writeIfChanged(filepath.Join(bridgeDir, bridgeTUIFileName), []byte(bridgeTUIFileContent))
		if err != nil {
			return Result{}, rollback(fmt.Errorf("write gentle-logo bridge tui.js: %w", err))
		}
		manifestChanged, err := writeIfChanged(filepath.Join(bridgeDir, bridgeManifestName), []byte(bridgeManifestContent))
		if err != nil {
			return Result{}, rollback(fmt.Errorf("write gentle-logo bridge package.json: %w", err))
		}
		changed = changed || tuiChanged || manifestChanged

		cliChanged, err := upsertPluginRegistration(filepath.Join(configDir, cliConfigName), gentleLogoV2Entry, cliSchemaV2, "plugins", pluginDir)
		if err != nil {
			return Result{}, rollback(fmt.Errorf("register gentle-logo in cli.json: %w", err))
		}
		changed = changed || cliChanged

		opencodeConfigPath, err := managedOpenCodeConfigPath(configDir)
		if err != nil {
			return Result{}, rollback(err)
		}
		opencodeChanged, err := unionOpenCodePlugins(opencodeConfigPath, pluginDir)
		if err != nil {
			return Result{}, rollback(fmt.Errorf("union gentle-logo into the managed OpenCode config: %w", err))
		}
		changed = changed || opencodeChanged

		tuiRetired, err := retireGentleLogoEntries(filepath.Join(configDir, tuiJSONName), pluginDir)
		if err != nil {
			return Result{}, rollback(fmt.Errorf("retire stale gentle-logo registration in tui.json: %w", err))
		}
		changed = changed || tuiRetired
		return Result{Changed: changed, Files: paths}, nil
	}

	registeredChanged, err := upsertV1PluginRegistration(configDir, pluginDir)
	if err != nil {
		return Result{}, rollback(fmt.Errorf("register gentle-logo in the OpenCode TUI config: %w", err))
	}
	return Result{Changed: changed || registeredChanged, Files: paths}, nil
}

// managedLogoPaths lists every file an install of the given generation
// writes or touches, in the same order ManagedLogoFiles reports them.
func managedLogoPaths(configDir string, generation openCodeGeneration) ([]string, error) {
	pluginDir := filepath.Join(configDir, "tui-plugins")
	paths := []string{filepath.Join(pluginDir, gentleLogoBundleFile)}
	if generation == generationV2 {
		bridgeDir := filepath.Join(pluginDir, gentleLogoBridgeDir)
		paths = append(paths,
			filepath.Join(bridgeDir, bridgeTUIFileName),
			filepath.Join(bridgeDir, bridgeManifestName),
			filepath.Join(configDir, cliConfigName),
		)
		opencodeConfigPath, err := managedOpenCodeConfigPath(configDir)
		if err != nil {
			return nil, err
		}
		paths = append(paths, opencodeConfigPath)
		tuiPath := filepath.Join(configDir, tuiJSONName)
		tuiExists, err := fileExists(tuiPath)
		if err != nil {
			return nil, err
		}
		if tuiExists {
			paths = append(paths, tuiPath)
		}
		return paths, nil
	}
	primary, err := v1TUIConfigPath(configDir)
	if err != nil {
		return nil, err
	}
	paths = append(paths, primary)
	if filepath.Base(primary) == tuiJSONCName {
		jsonPath := filepath.Join(configDir, tuiJSONName)
		jsonExists, err := fileExists(jsonPath)
		if err != nil {
			return nil, err
		}
		if jsonExists {
			paths = append(paths, jsonPath)
		}
	}
	return paths, nil
}

// writeIfChanged publishes content through the atomic write path only when
// the bytes differ, so idempotent installs report no change.
func writeIfChanged(path string, content []byte) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	wr, err := writeFileAtomicFn(path, content, 0o644)
	if err != nil {
		return wr.Changed, err
	}
	return wr.Changed, nil
}

// isGentleLogoEntry reports whether a configured plugin entry is owned by the
// gentle-logo installer: one of the known relative entries, or a path that
// resolves directly inside the given managed tui-plugins directory to one of
// the managed artifact names (including the T1-era absolute .tsx path).
// Entries elsewhere that merely share a basename or suffix are user-owned and
// must be preserved.
func isGentleLogoEntry(value, pluginDir string) bool {
	if value == gentleLogoV1Entry || value == gentleLogoV2Entry {
		return true
	}
	// Normalize separators and case before comparing directories: the
	// T1-era installer wrote a Windows absolute path with backslashes.
	cleaned := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(value, "\\", "/")))
	if !strings.EqualFold(filepath.Dir(cleaned), filepath.Clean(pluginDir)) {
		return false
	}
	switch strings.ToLower(filepath.Base(cleaned)) {
	case "gentle-logo", "gentle-logo.js", "gentle-logo.tsx":
		return true
	}
	return false
}

// pluginEntries collects the plugin entries configured under both the V1
// "plugin" key and the V2 "plugins" key, refusing non-array values.
func pluginEntries(root map[string]any) ([]any, error) {
	var entries []any
	for _, key := range []string{"plugin", "plugins"} {
		configured, exists := root[key]
		if !exists || configured == nil {
			continue
		}
		values, ok := configured.([]any)
		if !ok {
			return nil, fmt.Errorf("OpenCode config %q must be an array", key)
		}
		entries = append(entries, values...)
	}
	return entries, nil
}

// filterGentleLogoEntries drops every entry owned by the gentle-logo
// installer for the given managed tui-plugins directory, keeping all other
// entries untouched.
func filterGentleLogoEntries(entries []any, pluginDir string) []any {
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		if value, ok := entry.(string); ok && isGentleLogoEntry(value, pluginDir) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// mergeAndWriteConfig merges the overlay into the existing JSON/JSONC
// document (JSONC comments outside touched values are preserved) and
// publishes it through the atomic write path.
func mergeAndWriteConfig(path string, raw []byte, overlay map[string]any) (bool, error) {
	if strings.HasSuffix(path, ".jsonc") && len(bytes.TrimSpace(raw)) == 0 {
		// Seed an empty JSONC object so the comment-preserving rewrite
		// produces the same bytes on the first and every later install;
		// merging into an empty base would normalize to plain JSON and
		// reformat on the next run, breaking idempotency.
		raw = []byte("{}")
	}
	overlayJSON, err := json.Marshal(overlay)
	if err != nil {
		return false, err
	}
	merged, err := filemerge.MergeJSONObjectsForPath(path, raw, overlayJSON)
	if err != nil {
		return false, err
	}
	wr, err := writeFileAtomicFn(path, merged, 0o644)
	if err != nil {
		return wr.Changed, err
	}
	return wr.Changed, nil
}

// upsertPluginRegistration ensures the config file's plugin list contains
// exactly one occurrence of entry, preserving every other entry. An empty
// pluginsKey auto-detects the existing key (defaulting to V1's "plugin");
// a non-empty key is forced, as cli.json always uses the V2 "plugins" key.
// pluginDir scopes the gentle-logo entry ownership check.
func upsertPluginRegistration(path, entry, schema, pluginsKey, pluginDir string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	entries, err := pluginEntries(root)
	if err != nil {
		return false, err
	}
	entries = filterGentleLogoEntries(entries, pluginDir)
	found := false
	for _, configured := range entries {
		if value, ok := configured.(string); ok && value == entry {
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}
	if pluginsKey == "" {
		pluginsKey = "plugin"
		if _, exists := root["plugins"]; exists {
			pluginsKey = "plugins"
		}
	}
	return mergeAndWriteConfig(path, raw, map[string]any{"$schema": schema, pluginsKey: entries})
}

// unionOpenCodePlugins applies the durable V2 channel: the managed
// opencode.jsonc plugins array is rebuilt as a union so third-party entries
// survive and exactly one gentle-logo entry remains (cortex-ia parity). An
// unparseable config surfaces its parse error and is never rewritten from an
// empty base.
func unionOpenCodePlugins(path, pluginDir string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	configured, exists := root["plugins"]
	if exists && configured != nil {
		values, ok := configured.([]any)
		if !ok {
			return false, fmt.Errorf("OpenCode config plugins must be an array")
		}
		root["plugins"] = filterGentleLogoEntries(values, pluginDir)
	}
	return mergeAndWriteConfig(path, raw, map[string]any{"$schema": opencodeSchemaV2, "plugins": appendPluginEntry(root, gentleLogoV2Entry)})
}

// appendPluginEntry appends entry to the plugins array exactly once. It
// expects gentle-logo entries to have been filtered already.
func appendPluginEntry(root map[string]any, entry string) []any {
	entries, _ := root["plugins"].([]any)
	for _, configured := range entries {
		if value, ok := configured.(string); ok && value == entry {
			return entries
		}
	}
	return append(entries, entry)
}

// upsertV1PluginRegistration registers the relative bundle path in the V1
// tui.jsonc/tui.json cascade: an existing tui.jsonc wins and tui.json, when
// it also exists, is kept in sync as a secondary target.
func upsertV1PluginRegistration(configDir, pluginDir string) (bool, error) {
	primary, err := v1TUIConfigPath(configDir)
	if err != nil {
		return false, err
	}
	primaryChanged, err := upsertPluginRegistration(primary, gentleLogoV1Entry, tuiSchemaV1, "", pluginDir)
	if err != nil {
		return false, err
	}
	secondaryChanged := false
	if filepath.Base(primary) == tuiJSONCName {
		secondary := filepath.Join(configDir, tuiJSONName)
		secondaryExists, err := fileExists(secondary)
		if err != nil {
			return false, err
		}
		if secondaryExists {
			secondaryChanged, err = upsertPluginRegistration(secondary, gentleLogoV1Entry, tuiSchemaV1, "", pluginDir)
			if err != nil {
				return false, err
			}
		}
	}
	return primaryChanged || secondaryChanged, nil
}

// retireGentleLogoEntries removes every gentle-logo registration from the
// V1-era tui.json without touching the user's other entries. It is a no-op
// when the file does not exist or holds no gentle-logo entries.
func retireGentleLogoEntries(path, pluginDir string) (bool, error) {
	pathExists, err := fileExists(path)
	if err != nil {
		return false, err
	}
	if !pathExists {
		return false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	entries, err := pluginEntries(root)
	if err != nil {
		return false, err
	}
	kept := filterGentleLogoEntries(entries, pluginDir)
	if len(kept) == len(entries) {
		return false, nil
	}
	pluginKey := "plugins"
	if _, exists := root["plugins"]; !exists {
		pluginKey = "plugin"
	}
	overlay := map[string]any{pluginKey: kept}
	if schema, exists := root["$schema"]; exists {
		overlay["$schema"] = schema
	}
	return mergeAndWriteConfig(path, raw, overlay)
}

// writeFileAtomicFn is the source-write seam for installGentleLogo, so tests
// can exercise the landed-with-error window of WriteFileAtomic (#1676).
var writeFileAtomicFn = filemerge.WriteFileAtomic

// priorFile captures the prior on-disk state of a file so a multi-step install
// can compensate as one recoverable operation (#1678): a newly created file is
// removed and a pre-existing file is restored byte-exactly, including its mode.
type priorFile struct {
	existed bool
	data    []byte
	mode    os.FileMode
}

// capturePriorFile snapshots the current on-disk state of path so a
// multi-step install can compensate as one recoverable operation (#1678).
func capturePriorFile(path string) (priorFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return priorFile{}, nil
		}
		return priorFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return priorFile{}, err
	}
	return priorFile{existed: true, data: data, mode: info.Mode()}, nil
}

// restore puts back the captured bytes through the same durable write path used
// by installs, or removes the file when it did not previously exist. Removing an
// already-absent file is not an error.
func (p priorFile) restore(path string) error {
	if !p.existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if _, err := filemerge.WriteFileAtomicMode(path, p.data, p.mode.Perm()); err != nil {
		return err
	}
	return nil
}

// removeTUIPlugin is the uninstall-side companion of the installer's
// registration writers. It removes
// every occurrence of pkg from tui.json's plugin[] list. It returns the exact
// replacement bytes without writing so the caller can perform a guarded write.
// If the file is missing or pkg is not present, it returns (false, nil, nil).
func removeTUIPlugin(path, pkg string) (bool, []byte, error) {
	root := map[string]any{"$schema": "https://opencode.ai/tui.json"}
	data, readErr := os.ReadFile(path)
	switch {
	case readErr == nil && len(bytes.TrimSpace(data)) > 0:
		if err := json.Unmarshal(data, &root); err != nil {
			return false, nil, fmt.Errorf("parse OpenCode TUI config %q: %w", path, err)
		}
	case readErr != nil && !os.IsNotExist(readErr):
		return false, nil, fmt.Errorf("read OpenCode TUI config %q: %w", path, readErr)
	}

	plugins := stringSlice(root["plugin"])
	kept := make([]string, 0, len(plugins))
	changedAny := false
	for _, existing := range plugins {
		if existing == pkg {
			changedAny = true
			continue
		}
		kept = append(kept, existing)
	}
	if !changedAny {
		return false, nil, nil
	}
	root["plugin"] = kept

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, nil, err
	}
	out = append(out, '\n')
	return true, out, nil
}

// stringSlice narrows a decoded JSON value to its non-empty string items.
// Non-array values decode to nil; non-string and blank items are dropped.
func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
