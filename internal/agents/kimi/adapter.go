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
	"regexp"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
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
	configPath, _, err := a.configRoot(homeDir)
	if err != nil {
		return false, "", "", false, err
	}

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

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentKimi)
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}
	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---
//
// All path methods resolve through configRoot: the current ~/.kimi-code root
// (kimi-code v0.11+) is preferred when it exists as a directory, and the
// legacy ~/.kimi root is used otherwise. The path accessors have no error
// channel (the agents.Adapter interface returns plain strings), so they use
// configRootForPaths: an unexpected stat failure there deterministically
// selects the legacy root. Detect and BootstrapTemplate are the callers with
// error channels, and they surface the error instead of falling back.

// configRoot resolves the Kimi config root for homeDir using the adapter's
// testable stat seam.
func (a *Adapter) configRoot(homeDir string) (string, ConfigLayout, error) {
	statPath := a.statPath
	if statPath == nil {
		statPath = defaultStat
	}
	return resolveConfigRoot(statPath, homeDir)
}

// configRootForPaths is the configRoot variant for the string-only path
// accessors: an unexpected stat failure (layout undeterminable) falls back to
// the legacy root instead of silently assuming the current layout. Callers
// that can surface errors (Detect, BootstrapTemplate) use configRoot directly.
func (a *Adapter) configRootForPaths(homeDir string) (string, ConfigLayout) {
	root, layout, err := a.configRoot(homeDir)
	if err != nil {
		return filepath.Join(homeDir, LegacyConfigDirName), LayoutLegacy
	}
	return root, layout
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	root, _ := a.configRootForPaths(homeDir)
	return root
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return a.GlobalConfigDir(homeDir)
}

// SystemPromptFile returns the system prompt hub file Kimi reads. The legacy
// Python/uv CLI reads KIMI.md from its config root; the current kimi-code
// v0.11+ layout reads AGENTS.md from ~/.kimi-code.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	root, layout := a.configRootForPaths(homeDir)
	if layout == LayoutCurrent {
		return filepath.Join(root, "AGENTS.md")
	}
	return filepath.Join(root, "KIMI.md")
}

// SkillsDir returns the skills directory path for homeDir.
//
// Kimi Code CLI supports native Agent Skills. For the current kimi-code
// v0.11+ layout (config root ~/.kimi-code) the native per-brand skills
// directory ~/.kimi-code/skills is used. For the legacy Python/uv layout the
// generic shared skills directory is kept:
//   - generic shared skills: ~/.config/agents/skills and ~/.agents/skills
//
// We intentionally use ~/.config/agents/skills for legacy installs as a
// cross-agent shared convention. Kimi discovers this directory natively as
// part of its generic skills group (the docs mark this path as
// "recommended").
//
// See: https://moonshotai.github.io/kimi-cli/en/customization/skills.html
func (a *Adapter) SkillsDir(homeDir string) string {
	root, layout := a.configRootForPaths(homeDir)
	if layout == LayoutCurrent {
		return filepath.Join(root, "skills")
	}
	return filepath.Join(homeDir, ".config", "agents", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "config.toml")
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
	return filepath.Join(a.GlobalConfigDir(homeDir), "mcp.json")
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

func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}

func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

// --- Sub-agent support (optional interface) ---
//
// Kimi uses YAML-based agent specs with separate .md system prompts.

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

// SubAgentsDir returns the YAML agents directory. Only the legacy Python/uv
// layout discovers YAML agent specs (kimi --agent-file); the current
// kimi-code v0.11+ layout retired that format upstream, so YAML agent
// installations always target the legacy ~/.kimi/agents directory even when
// the current layout is preferred. The current layout never receives agent
// files it cannot load.
func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, LegacyConfigDirName, "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "kimi/agents"
}

// PostInstallMessage returns launch guidance for the resolved Kimi layout.
// The legacy Python/uv layout keeps YAML agent instructions (--agent-file);
// the current kimi-code v0.11+ layout retired --agent-file upstream, so its
// guidance only points at the native skills root.
func (a *Adapter) PostInstallMessage(homeDir string) string {
	root, layout, err := a.configRoot(homeDir)
	if err != nil {
		// The message is informational; fall back to the legacy guidance
		// rather than failing on an undeterminable layout.
		root, layout = filepath.Join(homeDir, LegacyConfigDirName), LayoutLegacy
	}

	if layout == LayoutCurrent {
		skillsRoot := filepath.Join(root, "skills")
		return fmt.Sprintf(`Kimi Code configured!

Usage:
  kimi --prompt "List skills"

Kimi Code v0.11+ discovers native Agent Skills automatically from the skills root; no agent-file launch step is required.
YAML agent specs (e.g. gentleman) are installed to %s for the legacy Kimi CLI; kimi-code v0.11+ does not load them.

Skills root:
  "%s"`, filepath.Join(homeDir, LegacyConfigDirName, "agents"), skillsRoot)
	}

	gentlemanYaml := filepath.Join(root, "agents", "gentleman.yaml")
	skillsRoot := filepath.Join(homeDir, ".config", "agents", "skills")

	return fmt.Sprintf(`Kimi Code configured!

Usage:
  kimi --agent-file "%s"

Launch the gentleman agent for ODD guidance. Kimi also supports YAML agents in ~/.kimi/agents.

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

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

var kimiIncludePattern = regexp.MustCompile(`(?m)^\s*\{%\s*include\s+"([^"]+)"\s+ignore\s+missing\s*%\}\s*$`)

const currentAgentsHubSection = "kimi-agents-hub"

func removeLegacyPlaceholderSection(content, heading, token string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return content
	}
	sectionEnd := len(content)
	if next := strings.Index(content[start+len(heading):], "\n## "); next >= 0 {
		sectionEnd = start + len(heading) + next + 1
	}
	if !strings.Contains(content[start:sectionEnd], token) {
		return content
	}
	return content[:start] + content[sectionEnd:]
}

func renderCurrentAgentsHub(configDir string, template string) (string, error) {
	var renderErr error
	rendered := kimiIncludePattern.ReplaceAllStringFunc(template, func(match string) string {
		if renderErr != nil {
			return ""
		}

		parts := kimiIncludePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}

		moduleName := parts[1]
		modulePath := filepath.Join(configDir, moduleName)
		content, err := os.ReadFile(modulePath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Sprintf("<!-- kimi-code module %s: missing -->", moduleName)
			}
			renderErr = fmt.Errorf("read kimi-code module %s: %w", moduleName, err)
			return ""
		}

		trimmed := strings.TrimRight(string(content), "\n")
		if strings.TrimSpace(trimmed) == "" {
			return fmt.Sprintf("<!-- kimi-code module %s: empty -->", moduleName)
		}
		return fmt.Sprintf("<!-- kimi-code module %s -->\n%s\n<!-- /kimi-code module %s -->", moduleName, trimmed, moduleName)
	})
	if renderErr != nil {
		return "", renderErr
	}
	rendered = removeLegacyPlaceholderSection(rendered, "## Project Instructions", "${KIMI_AGENTS_MD}")
	rendered = removeLegacyPlaceholderSection(rendered, "## Loaded Skills", "${KIMI_SKILLS}")
	return strings.TrimSpace(rendered) + "\n", nil
}

// BootstrapTemplate ensures the base system prompt hub exists in the agent's
// config directory (KIMI.md for the legacy layout, AGENTS.md for the current
// kimi-code v0.11+ layout). It is used by the installation pipeline to provide
// the managed Kimi prompt even when optional components are not installed.
func (a *Adapter) BootstrapTemplate(homeDir string) error {
	kimiDir, layout, err := a.configRoot(homeDir)
	if err != nil {
		return fmt.Errorf("resolve kimi config dir: %w", err)
	}
	if err := os.MkdirAll(kimiDir, 0o755); err != nil {
		return fmt.Errorf("create kimi config dir: %w", err)
	}

	skeletonPath := a.SystemPromptFile(homeDir)

	// We always write the skeleton to ensure any missing includes are restored.
	// Legacy Kimi reads the Jinja router directly. Current kimi-code reads plain
	// AGENTS.md, so it receives the same template with local modules expanded and
	// marker-bound into the existing file to preserve user-authored instructions.
	content := assets.MustRead("kimi/KIMI.md")
	if layout == LayoutCurrent {
		managedContent, err := renderCurrentAgentsHub(kimiDir, content)
		if err != nil {
			return fmt.Errorf("render current kimi-code agents hub: %w", err)
		}
		existing, err := os.ReadFile(skeletonPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read existing current kimi-code agents hub: %w", err)
		}
		content = filemerge.InjectMarkdownSection(string(existing), currentAgentsHubSection, managedContent)
	}
	if _, err := filemerge.WriteFileAtomic(skeletonPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write system prompt skeleton: %w", err)
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
