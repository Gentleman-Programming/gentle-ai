package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	config "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
	resolver installcmd.Resolver
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
		resolver: installcmd.NewResolver(),
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentOpenCode
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.lookPath("opencode")
	installed := err == nil

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return installed, binaryPath, configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return installed, binaryPath, configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentOpenCode)
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}

	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ConfigPath(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return ConfigPath(homeDir)
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "AGENTS.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	configDir := ConfigPath(homeDir)
	jsoncPath := filepath.Join(configDir, "opencode.jsonc")
	if regularFileExists(jsoncPath) {
		return jsoncPath
	}
	return filepath.Join(configDir, "opencode.json")
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyFileReplace
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoSettings
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, serverName string) string {
	// OpenCode merges into opencode.json, but this provides the path
	// for components that use the separate-file strategy fallback.
	return a.SettingsPath(homeDir)
}

// EffectiveCodeGraphWiring validates OpenCode's effective MCP entry while
// keeping OpenCode's JSON/JSONC schema behind the adapter boundary.
func (a *Adapter) EffectiveCodeGraphWiring(homeDir string) (string, bool) {
	for _, path := range a.codeGraphSettingsPaths(homeDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		root, err := filemerge.UnmarshalJSONObject(data)
		if err != nil {
			continue
		}
		entry, native := config.MCPEntry(root, "codegraph")
		if entry == nil {
			continue
		}
		if native {
			if disabled, exists := entry["disabled"]; exists && disabled != false {
				return "", false
			}
			// V2 does not interpret legacy enabled inside a native server.
			entry = map[string]any{"type": entry["type"], "command": entry["command"]}
		}
		if isEffectiveCodeGraphEntry(entry) {
			return path, true
		}
		return "", false
	}
	return "", false
}

func (a *Adapter) codeGraphSettingsPaths(homeDir string) []string {
	path := a.SettingsPath(homeDir)
	paths := []string{path}
	if strings.HasSuffix(path, ".jsonc") {
		paths = append(paths, strings.TrimSuffix(path, ".jsonc")+".json")
	} else if strings.HasSuffix(path, ".json") {
		paths = append(paths, strings.TrimSuffix(path, ".json")+".jsonc")
	}
	return paths
}

// NativeCodeGraphWiringError explains native conditions a legacy installer cannot
// repair. Call after effective-wiring detection; absent native entries need the installer.
func (a *Adapter) NativeCodeGraphWiringError(homeDir string) error {
	for _, path := range a.codeGraphSettingsPaths(homeDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		root, err := filemerge.UnmarshalJSONObject(data)
		if err != nil {
			continue
		}
		entry, native := config.MCPEntry(root, "codegraph")
		if entry == nil {
			continue
		}
		if !native {
			return nil
		}
		reason := ""
		disabled, exists := entry["disabled"]
		switch {
		case exists && disabled != false:
			reason = "disabled must be false (or absent); enable or remove this entry to complete sync"
		case entry["type"] != "local":
			reason = "type must be local"
		case !isCodeGraphCommand(entry["command"]):
			reason = "command must be a string array with only supported arguments"
		}
		if reason != "" {
			return fmt.Errorf("edit %s at mcp.servers.codegraph: %s; expected type local and command [codegraph serve --mcp], optionally --no-watch and --path <directory> (-p <directory>), with a bare codegraph or absolute executable path; the legacy CodeGraph installer cannot repair this native entry", path, reason)
		}
		return nil
	}
	return nil
}

func isEffectiveCodeGraphEntry(value any) bool {
	entry, ok := value.(map[string]any)
	if !ok || entry["type"] != "local" {
		return false
	}
	if enabled, exists := entry["enabled"]; exists && enabled != true {
		return false
	}
	return isCodeGraphCommand(entry["command"])
}

func isCodeGraphCommand(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) < 3 {
		return false
	}
	command := make([]string, len(values))
	for i, value := range values {
		command[i], ok = value.(string)
		if !ok {
			return false
		}
	}
	executable := command[0]
	if executable != "codegraph" {
		// Recognize Windows absolute paths even when inspecting config on POSIX.
		normalized := strings.ReplaceAll(executable, `\`, "/")
		driveAbsolute := len(normalized) > 3 && ((normalized[0] >= 'A' && normalized[0] <= 'Z') || (normalized[0] >= 'a' && normalized[0] <= 'z')) && normalized[1:3] == ":/"
		basename := strings.ToLower(normalized[strings.LastIndex(normalized, "/")+1:])
		if !(path.IsAbs(executable) || filepath.IsAbs(executable) || driveAbsolute || strings.HasPrefix(normalized, "//")) || (basename != "codegraph" && basename != "codegraph.exe" && basename != "codegraph.cmd") {
			return false
		}
	}
	if command[1] != "serve" {
		return false
	}
	mcp := false
	for i := 2; i < len(command); i++ {
		switch command[i] {
		case "--mcp":
			mcp = true
		case "--no-watch":
		case "--path", "-p":
			i++
			if i >= len(command) || command[i] == "" || strings.HasPrefix(command[i], "-") {
				return false
			}
		default:
			if !strings.HasPrefix(command[i], "--path=") || strings.TrimPrefix(command[i], "--path=") == "" {
				return false
			}
		}
	}
	return mcp
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}

func (a *Adapter) CommandsDir(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "commands")
}

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

func (a *Adapter) SubAgentsDir(_ string) string {
	return ""
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return ""
}

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
