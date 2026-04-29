package pi

import (
	"context"
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

// Adapter implements agents.Adapter for PI Coding Agent.
//
// Phase 1 scope is an explicit boundary with conservative defaults:
// - Config paths stay isolated in ~/.pi/agent, with legacy detection only
// - Auto-install is disabled until install contracts are validated
// - Profiles/model picker/generated multi are explicitly unsupported
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

func (a *Adapter) Agent() model.AgentID {
	return model.AgentPiCodingAgent
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	paths := ResolvePaths(homeDir, os.Getenv)
	configPath := paths.Root

	binaryPath, err := a.lookPath("pi")
	installed := err == nil

	configFound, err := a.detectConfigPresence(paths)
	if err != nil {
		return false, "", "", false, err
	}

	return installed, binaryPath, configPath, configFound, nil
}

func (a *Adapter) detectConfigPresence(paths Paths) (bool, error) {
	for _, candidate := range detectionConfigCandidates(paths) {
		stat := a.statPath(candidate)
		if stat.err == nil {
			return stat.isDir, nil
		}

		if os.IsNotExist(stat.err) {
			continue
		}

		return false, stat.err
	}

	return false, nil
}

func (a *Adapter) SupportsAutoInstall() bool {
	return false
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentPiCodingAgent}
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return ResolvePaths(homeDir, os.Getenv).Root
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return a.GlobalConfigDir(homeDir)
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.SystemPromptDir(homeDir), "AGENTS.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return ResolvePaths(homeDir, os.Getenv).SettingsPath
}

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyFileReplace
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMergeIntoSettings
}

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return a.SettingsPath(homeDir)
}

func (a *Adapter) SupportsOutputStyles() bool {
	return false
}

func (a *Adapter) OutputStyleDir(_ string) string {
	return ""
}

func (a *Adapter) SupportsSlashCommands() bool {
	return false
}

func (a *Adapter) CommandsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "commands")
}

func (a *Adapter) SupportsSubAgents() bool {
	return false
}

func (a *Adapter) SubAgentsDir(_ string) string {
	return ""
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return ""
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

func (a *Adapter) SupportsProfiles() bool {
	return false
}

func (a *Adapter) SupportsModelPicker() bool {
	return false
}

func (a *Adapter) SupportsGeneratedMultiProfiles() bool {
	return false
}

type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " auto-install is disabled until PI install contracts are validated"
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
