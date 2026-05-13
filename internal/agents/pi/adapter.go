// Package pi provides Pi CLI agent integration.
package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

const (
	piMCPAdapterPackage      = "npm:pi-mcp-adapter"
	piMCPAdapterPackageSpec  = "npm:pi-mcp-adapter"
	piMCPAdapterDependency   = "pi-mcp-adapter"
	piMCPAdapterVersion      = "2.6.0"
	piMCPAdapterVersionRange = "^2.6.0"
	piEngramMCPActiveServer  = "engram"
	piEngramMCPConfigFile    = "mcp.json"
	piSettingsFile           = "settings.json"
	piNPMDirectory           = "npm"
	piNPMPackageFile         = "package.json"
)

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Pi.
type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
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
	configPath := ConfigPath(homeDir)
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

func (a *Adapter) InstallCommand(system.PlatformProfile) ([][]string, error) {
	return [][]string{
		{"pi", "install", "npm:gentle-pi"},
		{"pi", "install", "npm:gentle-engram"},
		{"pi", "install", "npm:pi-mcp-adapter"},
		{"npm", "exec", "--yes", "--package", "gentle-engram@" + versions.GentleEngram, "--", "pi-engram", "init"},
		{"pi", "install", "npm:pi-subagents"},
		{"pi", "install", "npm:pi-intercom"},
		{"pi", "install", "npm:@juicesharp/rpiv-ask-user-question"},
		{"pi", "install", "npm:pi-web-access"},
		{"pi", "install", "npm:pi-lens"},
		{"pi", "install", "npm:@juicesharp/rpiv-todo"},
		{"pi", "install", "npm:pi-btw"},
	}, nil
}

func (a *Adapter) GlobalConfigDir(homeDir string) string { return ConfigPath(homeDir) }

func (a *Adapter) SystemPromptDir(string) string { return "" }

func (a *Adapter) SystemPromptFile(string) string { return "" }

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

func (a *Adapter) SupportsSystemPrompt() bool { return false }

func (a *Adapter) SupportsMCP() bool { return true }

// ConfigPath returns Pi's global config directory path.
func ConfigPath(homeDir string) string { return filepath.Join(homeDir, ".pi") }

// AgentConfigPath returns Pi's current agent-owned config directory path.
func AgentConfigPath(homeDir string) string { return filepath.Join(ConfigPath(homeDir), "agent") }

// ProvisionEngramMCP declares and wires pi-mcp-adapter so Pi exposes the
// Engram MCP server through /mcp. It is invoked by ComponentEngram; keeping it
// here lets Pi own the exact config shape without teaching the generic Engram
// injector about Pi internals.
func (a *Adapter) ProvisionEngramMCP(homeDir string) (bool, []string, error) {
	paths := []string{
		a.SettingsPath(homeDir),
		filepath.Join(ConfigPath(homeDir), piNPMDirectory, piNPMPackageFile),
		a.MCPConfigPath(homeDir, piEngramMCPActiveServer),
	}
	overlays := [][]byte{
		nil,
		mustJSON(map[string]any{
			"dependencies": map[string]any{
				piMCPAdapterDependency: piMCPAdapterVersionRange,
			},
		}),
		mustJSON(map[string]any{
			"activeMCP": piEngramMCPActiveServer,
			"mcpServers": map[string]any{
				piEngramMCPActiveServer: map[string]any{
					"__replace__": map[string]any{
						"command":     "engram",
						"args":        []string{"mcp", "--tools=agent"},
						"directTools": true,
					},
				},
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
		if piPackageIdentity(pkg) == piMCPAdapterPackage {
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
	return source
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
