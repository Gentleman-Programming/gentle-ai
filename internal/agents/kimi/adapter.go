// Package kimi provides Kimi Code CLI agent integration.
//
// Integration Note:
// This adapter targets the modern Kimi Code CLI (v0.18.0+, TypeScript/npm-based).
// Legacy Python/uv-based kimi-cli is no longer supported; users should migrate
// via `kimi migrate` before running gentle-ai install.
package kimi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Kimi Code CLI.
type Adapter struct {
	lookPath    func(string) (string, error)
	statPath    func(string) statResult
	pathExists  func(string) bool
	userHomeDir func() (string, error)
	resolver    installcmd.Resolver
}

// NewAdapter creates a new Kimi adapter instance.
func NewAdapter() *Adapter {
	return &Adapter{
		lookPath:    LookPathOverride,
		statPath:    defaultStat,
		pathExists:  defaultPathExists,
		userHomeDir: os.UserHomeDir,
		resolver:    installcmd.NewResolver(),
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentKimi
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.findKimi()
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

// findKimi searches for kimi in PATH and official fallback locations.
func (a *Adapter) findKimi() (string, error) {
	if path, err := a.lookPath("kimi"); err == nil {
		return path, nil
	}

	home, err := a.userHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("kimi not found in PATH and home directory is unavailable")
	}

	fallbacks := []string{
		filepath.Join(home, ".local", "bin", binaryName()),
		filepath.Join(home, "bin", binaryName()),
	}
	if runtime.GOOS == "windows" {
		fallbacks = append(fallbacks,
			filepath.Join(home, "AppData", "Local", "Microsoft", "WinGet", "Links", "kimi.exe"),
			filepath.Join(home, "AppData", "Roaming", "npm", "kimi.cmd"),
			filepath.Join(home, "AppData", "Roaming", "npm", "kimi.exe"),
		)
	}

	for _, fb := range fallbacks {
		if a.pathExists(fb) {
			return fb, nil
		}
	}

	return "", fmt.Errorf("kimi not found in PATH or official install locations")
}

// --- Installation ---

func (a *Adapter) SupportsAutoInstall() bool {
	return true
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
	return filepath.Join(homeDir, ".kimi-code")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "AGENTS.md")
}

// SkillsDir returns the Kimi-native skills directory path.
//
// Kimi Code CLI v0.18.0+ discovers skills in ~/.kimi-code/skills natively.
// We also register the generic ~/.config/agents/skills path in the skill
// registry so cross-agent skills are shared.
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "config.toml")
}

func (a *Adapter) CommandsDir(string) string {
	return ""
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyFileReplace
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(homeDir, ".kimi-code", "mcp.json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return false
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return false
}

func (a *Adapter) SupportsSkills() bool {
	return true
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return true
}

func (a *Adapter) SupportsMCP() bool {
	return true
}

// --- Sub-agent support (optional interface) ---
//
// Kimi Code CLI has built-in subagents (coder, explore, plan) that are
// native to the tool. Gentle AI SDD agents are injected via the AGENTS.md
// system prompt and skill registry, not via separate YAML specs.

func (a *Adapter) SupportsSubAgents() bool {
	return true
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "kimi/agents"
}

func (a *Adapter) PostInstallMessage(homeDir string) string {
	agentsPath := filepath.Join(homeDir, ".kimi-code", "AGENTS.md")
	skillsRoot := filepath.Join(homeDir, ".kimi-code", "skills")
	mcpPath := filepath.Join(homeDir, ".kimi-code", "mcp.json")

	return fmt.Sprintf(`Kimi Code CLI configured!

System prompt:
  "%s"

Skills root:
  "%s"

MCP config:
  "%s"

Native SDD entrypoints (via skills):
  /skill:sdd-init
  /skill:sdd-explore
  /skill:sdd-propose
  /skill:sdd-spec
  /skill:sdd-design
  /skill:sdd-tasks
  /skill:sdd-apply
  /skill:sdd-verify
  /skill:sdd-archive
  /skill:sdd-onboard`, agentsPath, skillsRoot, mcpPath)
}

// --- Helpers ---

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}

func defaultPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ConfigPath returns the configuration directory path.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code")
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

// BootstrapTemplate ensures the base AGENTS.md template exists in the agent's
// config directory. It is used by the installation pipeline to guarantee that
// modular components (SDD, Engram) can be included even if the Persona
// component is not installed.
func (a *Adapter) BootstrapTemplate(homeDir string) error {
	kimiDir := a.GlobalConfigDir(homeDir)
	if err := os.MkdirAll(kimiDir, 0o755); err != nil {
		return fmt.Errorf("create kimi config dir: %w", err)
	}

	skeletonPath := a.SystemPromptFile(homeDir)

	// We write the skeleton only if AGENTS.md does not already exist so we
	// do not clobber user-authored instructions on a re-install.
	if _, err := os.Stat(skeletonPath); os.IsNotExist(err) {
		content := assets.MustRead("kimi/AGENTS.md")
		if _, err := filemerge.WriteFileAtomic(skeletonPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write AGENTS.md skeleton: %w", err)
		}
	}

	// Kimi Code CLI expects config.toml to exist.
	configPath := a.SettingsPath(homeDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, err := filemerge.WriteFileAtomic(configPath, []byte("# Kimi Code Config\n"), 0o644); err != nil {
			return err
		}
	}

	return nil
}
