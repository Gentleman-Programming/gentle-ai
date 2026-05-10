// Package kimi provides Kimi Code CLI agent integration.
//
// Integration Note:
// This adapter natively relies on Astral's `uv` package manager 
// (`uv tool install kimi-cli`) to securely download and run Kimi CLI, 
// avoiding upstream's pipe-to-shell bootstrap scripts.
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
			filepath.Join(home, "AppData", "Roaming", "uv", "bin", "kimi.exe"),
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
	return filepath.Join(homeDir, ".kimi")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".kimi", "KIMI.md")
}

// SkillsDir returns the shared skills directory path.
//
// Kimi Code CLI supports native Agent Skills. It recognizes both:
//   - native brand-specific skills: ~/.kimi/skills
//   - generic shared skills: ~/.config/agents/skills and ~/.agents/skills
//
// We intentionally use ~/.config/agents/skills here as a cross-agent shared
// convention. Kimi will discover this directory natively as part of its
// generic skills group (the docs mark this path as "recommended").
//
// See: https://moonshotai.github.io/kimi-cli/en/customization/skills.html
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "agents", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ".kimi", "config.toml")
}

func (a *Adapter) CommandsDir(string) string {
	return ""
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyJinjaModules
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(homeDir, ".kimi", "mcp.json")
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
// Kimi uses YAML-based agent specs with separate .md system prompts.
// The SDD component copies all files from the embedded agents directory.

func (a *Adapter) SupportsSubAgents() bool {
	return true
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".kimi", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "kimi/agents"
}

func (a *Adapter) PostInstallMessage(homeDir string) string {
	gentlemanYaml := filepath.Join(homeDir, ".kimi", "agents", "gentleman.yaml")
	skillsRoot := filepath.Join(homeDir, ".config", "agents", "skills")

	return fmt.Sprintf(`Kimi Code configured!

Usage:
  kimi --agent-file "%s"

Native SDD entrypoints:
  /skill:sdd-init
  /skill:sdd-explore
  /skill:sdd-propose
  /skill:sdd-spec
  /skill:sdd-design
  /skill:sdd-tasks
  /skill:sdd-apply
  /skill:sdd-verify
  /skill:sdd-archive

Skills root:
  "%s"`, gentlemanYaml, skillsRoot)
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
	return filepath.Join(homeDir, ".kimi")
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

// BootstrapTemplate ensures the base KIMI.md template exists in the agent's config directory.
// It is used by the installation pipeline to guarantee that modular components 
// (SDD, Engram) can be included even if the Persona component is not installed.
func (a *Adapter) BootstrapTemplate(homeDir string) error {
	kimiDir := a.GlobalConfigDir(homeDir)
	if err := os.MkdirAll(kimiDir, 0o755); err != nil {
		return fmt.Errorf("create kimi config dir: %w", err)
	}

	skeletonPath := a.SystemPromptFile(homeDir)
	
	// We always write the skeleton to ensure any missing includes are restored.
	// Since KIMI.md is the 'router' for modular Jinja components, it should 
	// remain managed by the framework.
	content := assets.MustRead("kimi/KIMI.md")
	if _, err := filemerge.WriteFileAtomic(skeletonPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write KIMI.md skeleton: %w", err)
	}

	// Kimi considers config.toml a required file. We create an empty one if
	// it's missing to satisfy verification during a minimalist install.
	configPath := a.SettingsPath(homeDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, err := filemerge.WriteFileAtomic(configPath, []byte("# Kimi Code Config\n"), 0o644); err != nil {
			return err
		}
	}

	return nil
}


