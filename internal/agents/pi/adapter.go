package pi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

var LookPathOverride = exec.LookPath

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for pi.
//
// pi stores user-level resources under ~/.pi/agent/ (AGENTS.md, skills,
// settings). gentle-ai integrates with those filesystem primitives directly.
//
// MCP injection is intentionally disabled for pi in this first-class adapter.
// Context7/Engram are expected to be handled by pi-native extensions/packages.
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
	return model.AgentPi
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := filepath.Join(homeDir, ".pi", "agent")

	binaryPath, err := a.lookPath("pi")
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

func (a *Adapter) SupportsAutoInstall() bool {
	return false
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentPi}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "AGENTS.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoSettings
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(homeDir, ".pi", "agent", "settings.json")
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

func (a *Adapter) CommandsDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSkills() bool {
	return true
}

func (a *Adapter) SupportsSystemPrompt() bool {
	return true
}

func (a *Adapter) SupportsMCP() bool {
	return false
}

// --- Extended resources (optional interface) ---

func (a *Adapter) SupportsExtensions() bool {
	return true
}

func (a *Adapter) ExtensionsDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "extensions")
}

func (a *Adapter) SupportsPromptTemplates() bool {
	return true
}

func (a *Adapter) PromptsDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "prompts")
}

func (a *Adapter) SupportsThemes() bool {
	return true
}

func (a *Adapter) ThemesDir(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "themes")
}

func (a *Adapter) SupportsPackages() bool {
	return false
}

func (a *Adapter) PackagesStatePath(homeDir string) string {
	return filepath.Join(homeDir, ".pi", "agent", "settings.json")
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}

// AgentNotInstallableError is returned when pi cannot be installed automatically.
//
// gentle-ai currently treats pi as manual-install for safety: users should
// install/upgrade pi via its official distribution channel.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q cannot be auto-installed; install pi first", e.Agent)
}
