// Package pi provides Pi CLI agent integration.
package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

const (
	piMCPAdapterPackage         = "npm:pi-mcp-adapter"
	piMCPAdapterPackageSpec     = "npm:pi-mcp-adapter"
	piMCPAdapterDependency      = "pi-mcp-adapter"
	piMCPAdapterVersion         = "2.6.0"
	piMCPAdapterVersionRange    = "^2.6.0"
	piAppendSystemFile          = "APPEND_SYSTEM.md"
	piEngramMCPConfigFile       = "mcp.json"
	piSettingsFile              = "settings.json"
	piNPMDirectory              = "npm"
	piNPMPackageFile            = "package.json"
	piSubagentsJ0k3rPackageSpec = "npm:pi-subagents-j0k3r"
)

var legacyPiSubagentPackageIdentities = map[string]struct{}{
	"npm:pi-subagents":          {},
	"vendor/pi-subagents":       {},
	"vendor/pi-subagents-fixed": {},
}

var piWalkDir = filepath.WalkDir

func piSubagentsInstallCommand(system.PlatformProfile) []string {
	return []string{"pi", "install", piSubagentsJ0k3rPackageSpec}
}

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Pi.
type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
}

// CodeGraphPathSet declares the Pi paths owned or inspected by Gentle AI's
// optional CodeGraph integration. It intentionally contains no gentle-pi path.
type CodeGraphPathSet struct {
	AgentDir  string
	MCPConfig string
	Manifest  string
}

// CodeGraphPaths resolves PI_CODING_AGENT_DIR when set, matching Pi's runtime
// override instead of assuming the default agent directory.
func CodeGraphPaths(homeDir string) CodeGraphPathSet {
	agentDir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if agentDir == "" {
		agentDir = AgentConfigPath(homeDir)
	}
	return CodeGraphPathSet{
		AgentDir:  agentDir,
		MCPConfig: filepath.Join(agentDir, piEngramMCPConfigFile),
		Manifest:  filepath.Join(homeDir, ".gentle-ai", "pi-codegraph.json"),
	}
}

// CodeGraphChild is an effective Pi child definition. PackageOwned signals
// callers to create an overlay rather than mutate package content.
type CodeGraphChild struct {
	Name         string
	Source       string
	Target       string
	PackageOwned bool
}

// EffectiveCodeGraphMCPPath resolves the MCP configuration Pi will apply for a
// workspace. Later Pi discovery locations override an earlier CodeGraph server;
// malformed or unreadable participating configuration fails closed.
func EffectiveCodeGraphMCPPath(homeDir, workspaceDir string) (string, error) {
	paths := CodeGraphPaths(homeDir)
	candidates := []string{
		filepath.Join(homeDir, ".config", "mcp", "mcp.json"),
		paths.MCPConfig,
	}
	if workspaceDir != "" {
		candidates = append(candidates,
			filepath.Join(workspaceDir, ".mcp.json"),
			filepath.Join(workspaceDir, ".pi", "mcp.json"),
		)
	}

	effective := paths.MCPConfig
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read effective Pi MCP config %q: %w", path, err)
		}
		root := map[string]any{}
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse effective Pi MCP config %q: %w", path, err)
		}
		servers, ok := root["mcpServers"].(map[string]any)
		if !ok && root["mcpServers"] != nil {
			return "", fmt.Errorf("parse effective Pi MCP config %q: mcpServers must be an object", path)
		}
		if _, configured := servers["codegraph"]; configured {
			effective = path
		}
	}
	return effective, nil
}

// DiscoverCodeGraphChildren resolves Pi's user then project child directories.
// Later directories override an earlier child with the same normalized name.
func DiscoverCodeGraphChildren(homeDir, workspaceDir string) ([]CodeGraphChild, error) {
	paths := CodeGraphPaths(homeDir)
	dirs := []string{
		filepath.Join(paths.AgentDir, "node_modules"),
		filepath.Join(paths.AgentDir, "agents"),
		filepath.Join(paths.AgentDir, "subagents"),
	}
	if workspaceDir != "" {
		dirs = append(dirs,
			filepath.Join(workspaceDir, ".pi", "agents"),
			filepath.Join(workspaceDir, ".pi", "subagents"),
		)
	}
	byName := map[string]CodeGraphChild{}
	for _, dir := range dirs {
		candidates, err := piChildFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, source := range candidates {
			name := normalizeCodeGraphChildIdentity(strings.TrimSuffix(filepath.Base(source), ".md"))
			packageOwned := strings.Contains(filepath.ToSlash(source), "/node_modules/")
			target := source
			if packageOwned {
				target = filepath.Join(paths.AgentDir, "subagents", filepath.Base(source))
			}
			byName[name] = CodeGraphChild{Name: name, Source: source, Target: target, PackageOwned: packageOwned}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	children := make([]CodeGraphChild, 0, len(names))
	for _, name := range names {
		children = append(children, byName[name])
	}
	return children, nil
}

// normalizeCodeGraphChildIdentity mirrors Pi's case-insensitive runtime child
// identity so a later project definition deterministically shadows its user
// counterpart even when the filenames differ only by case or surrounding space.
func normalizeCodeGraphChildIdentity(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func piChildFiles(dir string) ([]string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("read Pi child directory %q: %w", dir, err)
	}
	files := []string{}
	err := piWalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		parent := filepath.Base(filepath.Dir(path))
		if parent == "agents" || parent == "subagents" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read Pi child directory %q: %w", dir, err)
	}
	slices.Sort(files)
	return files, nil
}

// NewAdapter creates a Pi adapter instance.
func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: exec.LookPath,
		statPath: defaultStat,
	}
}

func (a *Adapter) Agent() model.AgentID { return model.AgentPi }

func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := AgentConfigPath(homeDir)
	binaryPath, err := a.lookPath("pi")
	installed := err == nil && binaryPath != ""

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return installed, binaryPath, configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return installed, binaryPath, configPath, stat.isDir, nil
}

func (a *Adapter) SupportsAutoInstall() bool { return true }

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return [][]string{
		{"pi", "install", "npm:gentle-pi"},
		{"pi", "install", "npm:gentle-engram"},
		{"pi", "install", "npm:pi-mcp-adapter"},
		a.engramInitCommand(),
		piSubagentsInstallCommand(profile),
		{"pi", "install", "npm:@juicesharp/rpiv-ask-user-question"},
		{"pi", "install", "npm:pi-web-access"},
		{"pi", "install", "npm:@juicesharp/rpiv-todo"},
		{"pi", "install", "npm:pi-btw"},
	}, nil
}

func (a *Adapter) engramInitCommand() []string {
	if _, err := a.lookPath("pnpm"); err == nil {
		return []string{"pnpm", "dlx", "gentle-engram@latest", "pi-engram", "init"}
	}
	return []string{"npm", "exec", "--yes", "--package", "gentle-engram@latest", "--", "pi-engram", "init"}
}

func (a *Adapter) GlobalConfigDir(homeDir string) string { return ConfigPath(homeDir) }

func (a *Adapter) SystemPromptDir(homeDir string) string { return AgentConfigPath(homeDir) }

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(AgentConfigPath(homeDir), piAppendSystemFile)
}

func (a *Adapter) SkillsDir(string) string { return "" }

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(AgentConfigPath(homeDir), piSettingsFile)
}

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyAppendToFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy { return model.StrategyMCPConfigFile }

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(AgentConfigPath(homeDir), piEngramMCPConfigFile)
}

func (a *Adapter) SupportsOutputStyles() bool { return false }

func (a *Adapter) OutputStyleDir(string) string { return "" }

func (a *Adapter) SupportsSlashCommands() bool { return false }

func (a *Adapter) CommandsDir(string) string { return "" }

func (a *Adapter) SupportsSubAgents() bool { return false }

func (a *Adapter) SubAgentsDir(string) string { return "" }

func (a *Adapter) EmbeddedSubAgentsDir() string { return "" }

func (a *Adapter) SupportsSkills() bool { return false }

func (a *Adapter) SupportsSystemPrompt() bool { return true }

func (a *Adapter) SupportsMCP() bool { return true }

// ConfigPath returns Pi's global config directory path.
func ConfigPath(homeDir string) string { return filepath.Join(homeDir, ".pi") }

// AgentConfigPath returns Pi's current agent-owned config directory path.
func AgentConfigPath(homeDir string) string { return filepath.Join(ConfigPath(homeDir), "agent") }

// ProvisionEngramMCP declares pi-mcp-adapter in Pi's settings.json and
// package.json. It is invoked by ComponentEngram; keeping it here lets Pi
// own the exact config shape without teaching the generic Engram injector
// about Pi internals.
//
// mcp.json is NOT written here. pi-engram init (invoked by InstallCommand)
// is the sole writer of that file and owns its schema.
func (a *Adapter) ProvisionEngramMCP(homeDir string) (bool, []string, error) {
	paths := []string{
		a.SettingsPath(homeDir),
		filepath.Join(ConfigPath(homeDir), piSettingsFile),
		filepath.Join(ConfigPath(homeDir), piNPMDirectory, piNPMPackageFile),
	}
	overlays := [][]byte{
		nil,
		nil,
		mustJSON(map[string]any{
			"dependencies": map[string]any{
				piMCPAdapterDependency: piMCPAdapterVersionRange,
			},
		}),
	}

	changed := false
	for i, path := range paths {
		var write filemerge.WriteResult
		var err error
		if i == 0 {
			write, err = mergePiSettingsFile(path)
		} else {
			write, err = mergePiJSONFile(path, overlays[i])
		}
		if err != nil {
			return false, nil, err
		}
		changed = changed || write.Changed
	}

	return changed, paths, nil
}

func mergePiSettingsFile(path string) (filemerge.WriteResult, error) {
	settings, err := readPiJSONObject(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	settings["packages"] = appendPiPackage(settings["packages"], piMCPAdapterPackageSpec)

	// If we are writing the global settings.json (~/.pi/settings.json),
	// sync packages from agent settings (~/.pi/agent/settings.json) to make them discoverable.
	dir := filepath.Dir(path)
	if filepath.Base(dir) == ".pi" {
		agentSettingsPath := filepath.Join(dir, "agent", "settings.json")
		if agentSettings, err := readPiJSONObject(agentSettingsPath); err == nil {
			if agentPkgs, ok := agentSettings["packages"].([]any); ok && len(agentPkgs) > 0 {
				for _, pkg := range agentPkgs {
					if pkgStr, ok := pkg.(string); ok {
						settings["packages"] = appendPiPackage(settings["packages"], pkgStr)
					}
				}
			}
		}
	}

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("marshal pi settings %q: %w", path, err)
	}
	return filemerge.WriteFileAtomic(path, append(encoded, '\n'), 0o644)
}

func readPiJSONObject(path string) (map[string]any, error) {
	base, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read pi json file %q: %w", path, err)
		}
		return map[string]any{}, nil
	}

	var object map[string]any
	if err := json.Unmarshal(base, &object); err != nil {
		return nil, fmt.Errorf("unmarshal pi json file %q: %w", path, err)
	}
	if object == nil {
		object = map[string]any{}
	}
	return object, nil
}

func appendPiPackage(existing any, desired string) []any {
	packages := piPackagesAsSlice(existing)
	filtered := make([]any, 0, len(packages)+1)
	for _, pkg := range packages {
		identity := piPackageIdentity(pkg)
		if identity == piMCPAdapterPackage || isLegacyPiSubagentPackage(identity) {
			continue
		}
		filtered = append(filtered, pkg)
	}
	return append(filtered, desired)
}

func piPackagesAsSlice(existing any) []any {
	switch value := existing.(type) {
	case []any:
		return value
	case []string:
		packages := make([]any, 0, len(value))
		for _, item := range value {
			packages = append(packages, item)
		}
		return packages
	case map[string]any:
		packages := make([]any, 0, len(value))
		for source, version := range value {
			versionString, _ := version.(string)
			if versionString != "" && strings.HasPrefix(source, "npm:") && !strings.Contains(strings.TrimPrefix(source, "npm:"), "@") {
				packages = append(packages, source+"@"+versionString)
				continue
			}
			packages = append(packages, source)
		}
		return packages
	default:
		return nil
	}
}

func piPackageIdentity(pkg any) string {
	source, ok := pkg.(string)
	if !ok {
		object, isObject := pkg.(map[string]any)
		if !isObject {
			return ""
		}
		source, _ = object["source"].(string)
	}
	if strings.HasPrefix(source, piMCPAdapterPackage+"@") || source == piMCPAdapterPackage {
		return piMCPAdapterPackage
	}
	for legacy := range legacyPiSubagentPackageIdentities {
		if source == legacy || strings.HasPrefix(source, legacy+"@") {
			return legacy
		}
	}
	return source
}

func isLegacyPiSubagentPackage(identity string) bool {
	_, ok := legacyPiSubagentPackageIdentities[identity]
	return ok
}

func mergePiJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	base, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return filemerge.WriteResult{}, fmt.Errorf("read pi json file %q: %w", path, err)
		}
		base = nil
	}

	merged, err := filemerge.MergeJSONObjects(base, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
}

func mustJSON(value map[string]any) []byte {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(encoded, '\n')
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}
