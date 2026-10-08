package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectOpenCodeGeneration applies the durable-artifact-first cascade. The
// probed major only breaks ties for an otherwise empty config directory.
func detectOpenCodeGeneration(configDir string, major opencode.RuntimeMajor) openCodeGeneration {
	if fileExists(filepath.Join(configDir, cliConfigName)) {
		return generationV2
	}
	if fileExists(filepath.Join(configDir, tuiJSONCName)) || fileExists(filepath.Join(configDir, tuiJSONName)) {
		return generationV1
	}
	if major == opencode.RuntimeV1 {
		return generationV1
	}
	return generationV2
}

// v1TUIConfigPath mirrors the V1 cascade: an existing tui.jsonc wins, then
// tui.json, and a fresh V1 home materializes tui.jsonc.
func v1TUIConfigPath(configDir string) string {
	jsonc := filepath.Join(configDir, tuiJSONCName)
	if fileExists(jsonc) {
		return jsonc
	}
	jsonPath := filepath.Join(configDir, tuiJSONName)
	if fileExists(jsonPath) {
		return jsonPath
	}
	return jsonc
}

// managedOpenCodeConfigPath follows the MCP manager's load precedence: JSONC
// is loaded after JSON and therefore owns conflicting keys when both exist.
func managedOpenCodeConfigPath(configDir string) string {
	jsonc := filepath.Join(configDir, opencodeJSONCName)
	if fileExists(jsonc) {
		return jsonc
	}
	jsonPath := filepath.Join(configDir, opencodeJSONName)
	if fileExists(jsonPath) {
		return jsonPath
	}
	return jsonc
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
	pluginDir := filepath.Join(configDir, "tui-plugins")
	files := []string{filepath.Join(pluginDir, gentleLogoBundleFile)}
	if detectOpenCodeGeneration(configDir, major) == generationV2 {
		bridgeDir := filepath.Join(pluginDir, gentleLogoBridgeDir)
		files = append(files,
			filepath.Join(bridgeDir, bridgeTUIFileName),
			filepath.Join(bridgeDir, bridgeManifestName),
			filepath.Join(configDir, cliConfigName),
			managedOpenCodeConfigPath(configDir),
		)
		if tuiPath := filepath.Join(configDir, tuiJSONName); fileExists(tuiPath) {
			files = append(files, tuiPath)
		}
		return files, nil
	}
	files = append(files, v1TUIConfigPath(configDir))
	if primary := v1TUIConfigPath(configDir); filepath.Base(primary) == tuiJSONCName {
		if jsonPath := filepath.Join(configDir, tuiJSONName); fileExists(jsonPath) {
			files = append(files, jsonPath)
		}
	}
	return files, nil
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

func DefinitionFor(id model.OpenCodeCommunityPluginID) (Definition, bool) {
	if id == model.OpenCodePluginSDDEngramManage {
		return legacySDDEngramDefinition, true
	}
	if id == model.OpenCodePluginSubAgentStatusline {
		return legacyStatuslineDefinition, true
	}
	return Definition{}, false
}

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
	generation := detectOpenCodeGeneration(configDir, major)
	paths := managedLogoPaths(configDir, generation)

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

		cliChanged, err := upsertPluginRegistration(filepath.Join(configDir, cliConfigName), gentleLogoV2Entry, cliSchemaV2, "plugins")
		if err != nil {
			return Result{}, rollback(fmt.Errorf("register gentle-logo in cli.json: %w", err))
		}
		changed = changed || cliChanged

		opencodeChanged, err := unionOpenCodePlugins(managedOpenCodeConfigPath(configDir))
		if err != nil {
			return Result{}, rollback(fmt.Errorf("union gentle-logo into the managed OpenCode config: %w", err))
		}
		changed = changed || opencodeChanged

		tuiRetired, err := retireGentleLogoEntries(filepath.Join(configDir, tuiJSONName))
		if err != nil {
			return Result{}, rollback(fmt.Errorf("retire stale gentle-logo registration in tui.json: %w", err))
		}
		changed = changed || tuiRetired
		return Result{Changed: changed, Files: paths}, nil
	}

	registeredChanged, err := upsertV1PluginRegistration(configDir)
	if err != nil {
		return Result{}, rollback(fmt.Errorf("register gentle-logo in the OpenCode TUI config: %w", err))
	}
	return Result{Changed: changed || registeredChanged, Files: paths}, nil
}

// managedLogoPaths lists every file an install of the given generation
// writes or touches, in the same order ManagedLogoFiles reports them.
func managedLogoPaths(configDir string, generation openCodeGeneration) []string {
	pluginDir := filepath.Join(configDir, "tui-plugins")
	paths := []string{filepath.Join(pluginDir, gentleLogoBundleFile)}
	if generation == generationV2 {
		bridgeDir := filepath.Join(pluginDir, gentleLogoBridgeDir)
		paths = append(paths,
			filepath.Join(bridgeDir, bridgeTUIFileName),
			filepath.Join(bridgeDir, bridgeManifestName),
			filepath.Join(configDir, cliConfigName),
			managedOpenCodeConfigPath(configDir),
		)
		if tuiPath := filepath.Join(configDir, tuiJSONName); fileExists(tuiPath) {
			paths = append(paths, tuiPath)
		}
		return paths
	}
	primary := v1TUIConfigPath(configDir)
	paths = append(paths, primary)
	if filepath.Base(primary) == tuiJSONCName {
		if jsonPath := filepath.Join(configDir, tuiJSONName); fileExists(jsonPath) {
			paths = append(paths, jsonPath)
		}
	}
	return paths
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
// gentle-logo installer: the V1/V2 relative entries or any path whose base
// name is one of the managed artifacts (including the T1-era .tsx file).
func isGentleLogoEntry(value string) bool {
	if value == gentleLogoV1Entry || value == gentleLogoV2Entry {
		return true
	}
	normalized := strings.ReplaceAll(filepath.ToSlash(value), "\\", "/")
	base := normalized[strings.LastIndex(normalized, "/")+1:]
	switch base {
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

func filterGentleLogoEntries(entries []any) []any {
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		if value, ok := entry.(string); ok && isGentleLogoEntry(value) {
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
func upsertPluginRegistration(path, entry, schema, pluginsKey string) (bool, error) {
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
	entries = filterGentleLogoEntries(entries)
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
// survive and exactly one gentle-logo entry remains (cortex-ia parity).
func unionOpenCodePlugins(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		if strings.HasSuffix(path, ".jsonc") {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
		// Plain opencode.json follows the shared settings-merge tolerance:
		// the installer backup already snapshotted the file, and the merge
		// below proceeds from an empty base rather than aborting the install.
		root = map[string]any{}
	}
	configured, exists := root["plugins"]
	if exists && configured != nil {
		values, ok := configured.([]any)
		if !ok {
			return false, fmt.Errorf("OpenCode config plugins must be an array")
		}
		root["plugins"] = filterGentleLogoEntries(values)
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
func upsertV1PluginRegistration(configDir string) (bool, error) {
	primary := v1TUIConfigPath(configDir)
	primaryChanged, err := upsertPluginRegistration(primary, gentleLogoV1Entry, tuiSchemaV1, "")
	if err != nil {
		return false, err
	}
	secondaryChanged := false
	if filepath.Base(primary) == tuiJSONCName {
		secondary := filepath.Join(configDir, tuiJSONName)
		if fileExists(secondary) {
			secondaryChanged, err = upsertPluginRegistration(secondary, gentleLogoV1Entry, tuiSchemaV1, "")
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
func retireGentleLogoEntries(path string) (bool, error) {
	if !fileExists(path) {
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
	kept := filterGentleLogoEntries(entries)
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

func ensureTUIPlugin(path, pkg string) (bool, error) {
	root := map[string]any{"$schema": "https://opencode.ai/tui.json"}
	if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("parse OpenCode TUI config %q: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read OpenCode TUI config %q: %w", path, err)
	}

	plugins := stringSlice(root["plugin"])
	for _, existing := range plugins {
		if existing == pkg {
			return false, nil
		}
	}
	plugins = append(plugins, pkg)
	root["plugin"] = plugins

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	wr, err := filemerge.WriteFileAtomic(path, out, 0o644)
	if err != nil {
		// WriteFileAtomic can publish the replacement and still return an error
		// (#1676), so report wr.Changed truthfully alongside the error instead
		// of pretending nothing happened.
		return wr.Changed, err
	}
	return wr.Changed, nil
}

// removeTUIPlugin is the uninstall-side mirror of ensureTUIPlugin. It removes
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
