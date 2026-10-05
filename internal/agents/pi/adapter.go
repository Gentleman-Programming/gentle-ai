// Package pi provides Pi CLI agent integration.
package pi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// pi-mcp-adapter is retired: Pi >= 0.99.0 ships built-in MCP support that
// reads mcp.json, and any installed extension registering /mcp (such as
// pi-mcp-adapter) replaces that built-in support. Gentle AI therefore never
// installs the adapter and removes it wherever it finds it.
const (
	retiredPiMCPAdapterPackage    = "npm:pi-mcp-adapter"
	retiredPiMCPAdapterDependency = "pi-mcp-adapter"
)

const (
	piGentleEngramPackageSource = "npm:gentle-engram"
	piAppendSystemFile          = "APPEND_SYSTEM.md"
	piMCPConfigFile             = "mcp.json"
	piMCPAdapterConfigFile      = "mcp-adapter.json"
	piMCPServersKey             = "mcpServers"
	piSettingsFile              = "settings.json"
	piNPMDirectory              = "npm"
	piNPMPackageFile            = "package.json"
)

var legacyPiSubagentPackageIdentities = map[string]struct{}{
	"npm:pi-subagents":          {},
	"vendor/pi-subagents":       {},
	"vendor/pi-subagents-fixed": {},
}

// Retired Pi companion packages: their replacement ships inside gentle-pi,
// so every install or update drops them from the user's settings and Pi
// uninstalls them on its next package sync.
// Gentle Agents (the subagent_* tools) ships in gentle-pi 2.5.0; a settings
// file that pins an older gentle-pi keeps the retired subagents package.
const gentleAgentsGentlePiVersion = "2.5.0"

// gentle-pi ships the first-party ask_user_question tool since gentle-pi
// f2d9d073 (gentle-pi#1274). Pi tool names are exclusive, so keeping
// npm:@juicesharp/rpiv-ask-user-question installed alongside it makes Pi
// fail to load with `Tool "ask_user_question" conflicts with ...`.
var retiredPiPackageIdentities = map[string]struct{}{
	"npm:@juicesharp/rpiv-todo":              {},
	"npm:pi-subagents-j0k3r":                 {},
	"npm:@juicesharp/rpiv-ask-user-question": {},
}

var managedPackageSources = []string{
	"npm:gentle-pi",
	piGentleEngramPackageSource,
	"npm:pi-web-access",
	"npm:pi-btw",
}

// ManagedPackageSources returns every Pi package source installed by this
// adapter. Callers receive a copy so package ownership remains adapter-owned.
func ManagedPackageSources() []string {
	return slices.Clone(managedPackageSources)
}

// UninstallPackageSources returns every Pi package source an uninstall should
// remove: the managed sources plus the retired pi-mcp-adapter, which older
// Gentle AI releases installed and which may still be present.
func UninstallPackageSources() []string {
	return append(ManagedPackageSources(), retiredPiMCPAdapterPackage)
}

var piWalkDir = filepath.WalkDir

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
// override instead of assuming the default agent directory. When an agent
// directory override is active, an isolated manifest path is derived to prevent
// cross-contamination with the default global Pi manifest.
func CodeGraphPaths(homeDir string) CodeGraphPathSet {
	agentDir := AgentConfigPath(homeDir)
	manifest := filepath.Join(homeDir, ".gentle-ai", "pi-codegraph.json")
	if defaultDir := filepath.Join(ConfigPath(homeDir), "agent"); filepath.Clean(agentDir) != filepath.Clean(defaultDir) {
		sum := sha256.Sum256([]byte(filepath.Clean(agentDir)))
		manifest = filepath.Join(homeDir, ".gentle-ai", fmt.Sprintf("pi-codegraph-%x.json", sum[:8]))
	}
	return CodeGraphPathSet{
		AgentDir:  agentDir,
		MCPConfig: filepath.Join(agentDir, piMCPConfigFile),
		Manifest:  manifest,
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

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentPi)
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	commands := make([][]string, 0, len(managedPackageSources)+1)
	for _, source := range ManagedPackageSources() {
		commands = append(commands, []string{"pi", "install", source})
		if source == piGentleEngramPackageSource {
			commands = append(commands, a.engramInitCommand())
		}
	}
	return commands, nil
}

func (a *Adapter) engramInitCommand() []string {
	return []string{"npm", "exec", "--yes", "--package", "gentle-engram@latest", "--", "pi-engram", "init"}
}

// GlobalConfigDir returns Pi's global config directory: always
// homeDir/.pi, matching every other installed agent's config root.
// PI_CODING_AGENT_DIR never moves this parent root — it only relocates
// Pi's agent-owned paths, resolved separately through AgentConfigPath.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ConfigPath(homeDir)
}

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
	return filepath.Join(AgentConfigPath(homeDir), piMCPConfigFile)
}

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}

func (a *Adapter) OutputStyleDir(string) string { return "" }

func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}

func (a *Adapter) CommandsDir(string) string { return "" }

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

func (a *Adapter) SubAgentsDir(string) string { return "" }

func (a *Adapter) EmbeddedSubAgentsDir() string { return "" }

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

// ConfigPath returns Pi's global config directory path. It always stays
// under homeDir/.pi, even when PI_CODING_AGENT_DIR is set: Pi's own
// precedence only overrides the agent directory, not this parent.
func ConfigPath(homeDir string) string { return filepath.Join(homeDir, ".pi") }

// AgentConfigPath returns Pi's current agent-owned config directory path. It
// honors PI_CODING_AGENT_DIR when set and non-blank, matching Pi's own
// runtime override, so gentle-ai's install and sync operations target the
// same directory Pi itself reads and writes (for example gentle-shell's
// isolated `~/.gentle-shell/agent` home). A "~" or "~/..." form always
// expands against homeDir and stays contained there; an absolute or
// cwd-relative form is honored only when homeDir is isRealUserHome, so a
// caller resolving paths for a different home (a sandbox, a test temp dir)
// can never be redirected outside it by ambient environment. Falls back to
// homeDir/.pi/agent otherwise.
func AgentConfigPath(homeDir string) string {
	if override := piCodingAgentDirOverride(); override != "" {
		return resolvePiAgentDirOverride(override, homeDir)
	}
	return filepath.Join(ConfigPath(homeDir), "agent")
}

// isRealUserHome reports whether homeDir is the current user's actual home
// directory — the only case where PI_CODING_AGENT_DIR's absolute or
// cwd-relative forms may legitimately redirect Pi's agent-owned paths away
// from homeDir. Mirrors the equivalent guard other relocatable adapters
// (vscode, windsurf, kiro, trae) already apply to their own override
// variables.
func isRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
}

// piCodingAgentDirOverride returns the trimmed PI_CODING_AGENT_DIR value, or
// "" when it is unset or blank.
func piCodingAgentDirOverride() string {
	return strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
}

// resolveAbsPath resolves a relative path against the process's current
// working directory. It is a package-level var so tests can simulate the
// (extremely rare) failure of the underlying os.Getwd call.
var resolveAbsPath = filepath.Abs

// resolvePiAgentDirOverride resolves a PI_CODING_AGENT_DIR value the same way
// Pi itself does: a leading "~/" (or a bare "~") expands against homeDir, an
// absolute path is used as-is, and a relative path resolves against the
// process's current working directory. Those last two forms escape homeDir
// entirely, so they are honored only when homeDir isRealUserHome; a caller
// resolving paths for a different home falls back to the default agent
// directory instead, exactly like an unset override. If cwd resolution
// fails, it likewise falls back to the default agent directory instead of
// returning the raw relative string, which would silently resolve to
// something else entirely once passed to filepath.Join by a caller.
func resolvePiAgentDirOverride(override, homeDir string) string {
	defaultDir := filepath.Join(ConfigPath(homeDir), "agent")
	switch {
	case override == "~":
		return homeDir
	case strings.HasPrefix(override, "~/"):
		return filepath.Join(homeDir, strings.TrimPrefix(override, "~/"))
	case !isRealUserHome(homeDir):
		return defaultDir
	case filepath.IsAbs(override):
		return filepath.Clean(override)
	default:
		if abs, err := resolveAbsPath(override); err == nil {
			return abs
		}
		return defaultDir
	}
}

// ProvisionEngramMCP prepares Pi's MCP runtime for the Engram component. Pi
// Engram itself is native-only: gentle-engram registers its tools directly,
// not through MCP, so this never adds an engram server to mcp.json (and never
// removes a user-owned one). Pi >= 0.99.0 runs other MCP servers from
// <agentDir>/mcp.json through its built-in MCP support. It is invoked by
// ComponentEngram on install and sync; keeping it here lets Pi own its config
// shape without teaching the generic Engram injector about Pi internals. It:
//
//  1. retires what would shadow built-in MCP: the pi-mcp-adapter package in
//     settings.json (plus the legacy and retired companion packages) and the
//     pi-mcp-adapter dependency in <agentDir>/npm/package.json;
//  2. merges the mcpServers entries of a legacy <agentDir>/mcp-adapter.json
//     (the file pi-mcp-adapter 3.x read) into mcp.json, creating mcp.json
//     only when there is a server to migrate. Entries already in mcp.json win;
//     mcp-adapter.json is never modified or removed.
//
// Malformed JSON in any file it must read is reported, never overwritten.
// Missing settings and npm manifests are never created, and files with
// nothing to change are left byte-identical. The returned paths are the files
// actually rewritten.
func (a *Adapter) ProvisionEngramMCP(homeDir string) (bool, []string, error) {
	settingsPath := a.SettingsPath(homeDir)
	// Pi's npm manifest lives at <agentDir>/npm/package.json
	// (package-manager.ts:2033), not under GlobalConfigDir's ~/.pi root.
	npmPackagePath := filepath.Join(AgentConfigPath(homeDir), piNPMDirectory, piNPMPackageFile)

	var paths []string
	for _, step := range []struct {
		path  string
		apply func(string) (filemerge.WriteResult, error)
	}{
		{settingsPath, prunePiSettingsFile},
		{npmPackagePath, prunePiNPMPackageFile},
		{a.MCPConfigPath(homeDir, ""), migratePiMCPAdapterServers},
	} {
		write, err := step.apply(step.path)
		if err != nil {
			return false, nil, err
		}
		if write.Changed {
			paths = append(paths, step.path)
		}
	}

	return len(paths) > 0, paths, nil
}

// prunePiSettingsFile drops retired packages from an existing settings.json.
// A missing file, a file without packages, or one with nothing to drop is
// left untouched.
func prunePiSettingsFile(path string) (filemerge.WriteResult, error) {
	settings, exists, err := readExistingPiJSONObject(path)
	if err != nil || !exists {
		return filemerge.WriteResult{}, err
	}
	existing, ok := settings["packages"]
	if !ok {
		return filemerge.WriteResult{}, nil
	}

	retained := retainPiPackages(existing)
	if current, isSlice := existing.([]any); isSlice && reflect.DeepEqual(current, retained) {
		return filemerge.WriteResult{}, nil
	}
	settings["packages"] = retained

	return writePiJSONObject(path, settings)
}

// prunePiNPMPackageFile removes the retired pi-mcp-adapter dependency from an
// existing <agentDir>/npm/package.json, leaving every other key in place.
func prunePiNPMPackageFile(path string) (filemerge.WriteResult, error) {
	manifest, exists, err := readExistingPiJSONObject(path)
	if err != nil || !exists {
		return filemerge.WriteResult{}, err
	}
	dependencies, ok := manifest["dependencies"].(map[string]any)
	if !ok {
		return filemerge.WriteResult{}, nil
	}
	if _, present := dependencies[retiredPiMCPAdapterDependency]; !present {
		return filemerge.WriteResult{}, nil
	}
	delete(dependencies, retiredPiMCPAdapterDependency)

	return writePiJSONObject(path, manifest)
}

// migratePiMCPAdapterServers copies into the mcp.json at path every server of
// a sibling mcp-adapter.json that mcp.json lacks, preserving every other key.
// It never adds a server of its own: Pi Engram is native-only (gentle-engram),
// so an engram entry appears only when the user kept one in mcp-adapter.json.
// With nothing to migrate it reads and writes nothing, so mcp.json is created
// only to hold migrated servers.
func migratePiMCPAdapterServers(path string) (filemerge.WriteResult, error) {
	legacy, legacyExists, err := readExistingPiJSONObject(filepath.Join(filepath.Dir(path), piMCPAdapterConfigFile))
	if err != nil {
		return filemerge.WriteResult{}, err
	}
	legacyServers, isObject := legacy[piMCPServersKey].(map[string]any)
	if !legacyExists || !isObject || len(legacyServers) == 0 {
		return filemerge.WriteResult{}, nil
	}

	config, err := readPiJSONObject(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}
	servers := map[string]any{}
	if existing, present := config[piMCPServersKey]; present && existing != nil {
		object, isObject := existing.(map[string]any)
		if !isObject {
			return filemerge.WriteResult{}, fmt.Errorf("pi mcp config %q: %s must be a JSON object", path, piMCPServersKey)
		}
		servers = object
	}

	changed := false
	for name, server := range legacyServers {
		if _, present := servers[name]; !present {
			servers[name] = server
			changed = true
		}
	}
	if !changed {
		return filemerge.WriteResult{}, nil
	}
	config[piMCPServersKey] = servers

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("create pi agent dir for %q: %w", path, err)
	}
	return writePiJSONObject(path, config)
}

func readExistingPiJSONObject(path string) (map[string]any, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat pi json file %q: %w", path, err)
	}
	object, err := readPiJSONObject(path)
	return object, err == nil, err
}

func writePiJSONObject(path string, object map[string]any) (filemerge.WriteResult, error) {
	encoded, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return filemerge.WriteResult{}, fmt.Errorf("marshal pi json file %q: %w", path, err)
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

	object, err := filemerge.DecodeStrictJSONObject(base)
	if err != nil {
		return nil, fmt.Errorf("unmarshal pi json file %q: %w", path, err)
	}
	if object == nil {
		object = map[string]any{}
	}
	return object, nil
}

// retainPiPackages returns the packages Gentle AI keeps in Pi's settings:
// everything except the retired pi-mcp-adapter, the legacy subagent packages,
// and the retired companion packages.
func retainPiPackages(existing any) []any {
	packages := piPackagesAsSlice(existing)
	filtered := make([]any, 0, len(packages))
	keepSubagents := !gentlePiShipsSubagents(packages)
	for _, pkg := range packages {
		identity := piPackageIdentity(pkg)
		retired := isRetiredPiPackage(identity) && !(keepSubagents && identity == "npm:pi-subagents-j0k3r")
		if identity == retiredPiMCPAdapterPackage || isLegacyPiSubagentPackage(identity) || retired {
			continue
		}
		filtered = append(filtered, pkg)
	}
	return filtered
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
	if strings.HasPrefix(source, retiredPiMCPAdapterPackage+"@") || source == retiredPiMCPAdapterPackage {
		return retiredPiMCPAdapterPackage
	}
	for legacy := range legacyPiSubagentPackageIdentities {
		if source == legacy || strings.HasPrefix(source, legacy+"@") {
			return legacy
		}
	}
	for retired := range retiredPiPackageIdentities {
		if source == retired || strings.HasPrefix(source, retired+"@") {
			return retired
		}
	}
	return source
}

func isLegacyPiSubagentPackage(identity string) bool {
	_, ok := legacyPiSubagentPackageIdentities[identity]
	return ok
}

func gentlePiShipsSubagents(packages []any) bool {
	for _, pkg := range packages {
		source, _ := pkg.(string)
		if !strings.HasPrefix(source, "npm:gentle-pi@") {
			continue
		}
		var major, minor, wantMajor, wantMinor int
		fmt.Sscanf(strings.TrimPrefix(source, "npm:gentle-pi@"), "%d.%d", &major, &minor)
		fmt.Sscanf(gentleAgentsGentlePiVersion, "%d.%d", &wantMajor, &wantMinor)
		return major > wantMajor || (major == wantMajor && minor >= wantMinor)
	}
	return true
}

func isRetiredPiPackage(identity string) bool {
	_, ok := retiredPiPackageIdentities[identity]
	return ok
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}
