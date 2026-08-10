package vscode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type Adapter struct {
	lookPath func(string) (string, error)
}

func NewAdapter() *Adapter {
	return &Adapter{
		lookPath: exec.LookPath,
	}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID {
	return model.AgentVSCodeCopilot
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	binaryPath, err := a.lookPath("code")
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return false, "", "", false, err
		}
		return false, "", "", false, nil
	}
	extensionsDir := filepath.Join(homeDir, ".vscode", "extensions")
	entries, err := os.ReadDir(extensionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, binaryPath, extensionsDir, false, nil
		}
		return false, "", extensionsDir, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() && (entry.Name() == "github.copilot" || strings.HasPrefix(entry.Name(), "github.copilot-")) {
			return true, binaryPath, extensionsDir, true, nil
		}
	}
	return false, binaryPath, extensionsDir, false, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentVSCodeCopilot)
}

func (a *Adapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, AgentNotInstallableError{Agent: model.AgentVSCodeCopilot}
}

// --- Config paths ---
// VS Code Copilot reads .instructions.md files from the VS Code User prompts folder.
// Skills are loaded from ~/.copilot/skills/ (global), .github/skills/ (workspace),
// ~/.claude/skills/, and .claude/skills/. We target ~/.copilot/skills/ for global reach.

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".copilot")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(a.vscodeUserDir(homeDir), "prompts")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.SystemPromptDir(homeDir), "gentle-ai.instructions.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	// Skills under ~/.copilot/skills/ — VS Code Copilot global skills directory.
	return filepath.Join(homeDir, ".copilot", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.vscodeUserDir(homeDir), "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyInstructionsFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.vscodeUserDir(homeDir), "mcp.json")
}

// vscodeUserDir returns the OS-specific VS Code User config directory.
//
// Environment overrides (XDG_CONFIG_HOME, APPDATA) are honored only when
// homeDir is the real user home: a caller that passes a custom installation
// root (sandboxed installs, tests) must stay contained inside that root, so
// ambient environment can never redirect a write outside it.
func (a *Adapter) vscodeUserDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Code", "User")
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); filepath.IsAbs(appData) && isRealUserHome(homeDir) {
			return filepath.Join(appData, "Code", "User")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Code", "User")
	default:
		if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(xdgConfigHome) && isRealUserHome(homeDir) {
			return filepath.Join(xdgConfigHome, "Code", "User")
		}
		return filepath.Join(homeDir, ".config", "Code", "User")
	}
}

// isRealUserHome reports whether homeDir is the current user's actual home
// directory — the only case where process-wide environment overrides may
// legitimately steer config resolution away from homeDir.
func isRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
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

// AgentNotInstallableError is returned when InstallCommand is called on a desktop-only agent.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return "agent " + string(e.Agent) + " is a desktop app and cannot be installed via CLI"
}
