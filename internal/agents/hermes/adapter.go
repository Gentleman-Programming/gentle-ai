package hermes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	lookPath func(string) (string, error)
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: LookPathOverride,
		statPath: defaultStat,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentHermes
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ResolveHome(homeDir)

	binaryPath, err := a.lookPath("hermes")
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
	return capabilitymanifest.MustForAgent(model.AgentHermes)
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: a.Agent()}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ResolveHome(homeDir)
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return ResolveHome(homeDir)
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(ResolveHome(homeDir), "SOUL.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(ResolveHome(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(ResolveHome(homeDir), "config.yaml")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoYAML
}

// --- MCP ---

// MCPConfigPath returns the path to the Hermes config.yaml (effective home,
// see ResolveHome) for both context7 and engram. Hermes stores all MCP
// servers in a single YAML config file, so the serverName argument is
// intentionally ignored.
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(ResolveHome(homeDir), "config.yaml")
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

func (a *Adapter) CommandsDir(_ string) string {
	return ""
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

// ConfigPath returns the path to ~/.hermes, the Hermes global config
// directory on Linux and macOS. On Windows, and whenever HERMES_HOME is set,
// use ResolveHome instead: it implements the native Hermes home resolution
// order (HERMES_HOME, then %LOCALAPPDATA%\hermes on Windows).
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".hermes")
}

// ResolveHome returns the effective Hermes home directory, following the
// native Hermes agent specification:
//
//  1. HERMES_HOME, when set to an absolute path.
//  2. %LOCALAPPDATA%\hermes on native Windows.
//  3. $HOME/.hermes on Linux and macOS.
//
// Environment overrides are honored only when homeDir is the real user home:
// a caller that passes a custom installation root (sandboxed installs,
// tests) must stay contained inside that root, so ambient environment can
// never redirect a write outside it.
func ResolveHome(homeDir string) string {
	if hermesHome := strings.TrimSpace(os.Getenv("HERMES_HOME")); filepath.IsAbs(hermesHome) && isRealUserHome(homeDir) {
		return hermesHome
	}
	if runtime.GOOS == "windows" {
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); filepath.IsAbs(localAppData) && isRealUserHome(homeDir) {
			return filepath.Join(localAppData, "hermes")
		}
		return filepath.Join(homeDir, "AppData", "Local", "hermes")
	}
	return filepath.Join(homeDir, ".hermes")
}

// isRealUserHome reports whether homeDir is the current user's actual home
// directory — the only case where process-wide environment overrides may
// legitimately steer config resolution away from homeDir.
func isRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
}

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q must be installed manually before Gentle AI can configure it", e.Agent)
}
