// Package conductor provides Conductor workspace-orchestrator integration.
//
// Conductor is a desktop workspace orchestrator that inherits Claude Code
// configuration for the workspaces it manages. Gentle AI therefore treats it
// as a detection/catalog-only integration: it never writes Conductor-specific
// skills, MCP configuration, or system prompt files. All write-capability
// claims are false by construction and projected from the canonical
// capability manifest.
package conductor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

// Adapter implements agents.Adapter for Conductor. It is intentionally thin:
// detection and identity only, with every write capability disabled.
type Adapter struct {
	statPath func(string) statResult
}

func NewAdapter() *Adapter {
	return &Adapter{
		statPath: defaultStat,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentConductor
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

// Detect reports the presence of the ~/.conductor directory, Conductor's
// global configuration root. Conductor ships as a desktop application with
// no PATH binary, so directory presence is the sole installation signal.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			return false, "", configPath, false, nil
		}
		return false, "", "", false, stat.err
	}

	return stat.isDir, "", configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentConductor)
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: a.Agent()}
}

// --- Config paths ---

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ConfigPath(homeDir)
}

// The remaining path methods stay empty: Gentle AI performs no writes for
// Conductor, so no write target is advertised and no caller can resolve one.

func (a *Adapter) SystemPromptDir(_ string) string {
	return ""
}

func (a *Adapter) SystemPromptFile(_ string) string {
	return ""
}

func (a *Adapter) SkillsDir(_ string) string {
	return ""
}

func (a *Adapter) SettingsPath(_ string) string {
	return ""
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategySeparateMCPFiles
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(_ string, _ string) string {
	return ""
}

// --- Optional capabilities ---

// Every capability projection below reads the canonical capability manifest,
// which claims no features for Conductor.

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

// ConfigPath returns the path to ~/.conductor, the Conductor global
// configuration directory that holds inherited Claude Code workspace state.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".conductor")
}

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q must be installed manually before Gentle AI can configure it", e.Agent)
}
