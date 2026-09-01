package claude

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
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
	return model.AgentClaudeCode
}

func (a *Adapter) Tier() model.SupportTier {
	return model.TierFull
}

// --- Detection ---

func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	configPath := ConfigPath(homeDir)

	binaryPath, err := a.lookPath("claude")
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
	return capabilitymanifest.MustForAgent(model.AgentClaudeCode)
}

func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}

	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---

// UserConfigPath returns ~/.claude.json, the only user-scope file Claude Code
// reads MCP servers from; it also carries the OAuth session — never reset it.
func UserConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude.json")
}

// MergeUserConfig merges overlayJSON into ~/.claude.json: an unparsable base
// aborts instead of being reset to {}, the file always ends at 0600, and a
// base that moves underneath the merge is re-read and retried (issue #1868).
func MergeUserConfig(homeDir string, overlayJSON []byte) (filemerge.WriteResult, string, error) {
	configPath := UserConfigPath(homeDir)
	const maxAttempts = 4
	for attempt := 1; ; attempt++ {
		raw, err := readUserConfigBase(configPath)
		if err != nil {
			return filemerge.WriteResult{}, configPath, err
		}
		if _, parseErr := filemerge.UnmarshalJSONObject(raw); parseErr != nil {
			return filemerge.WriteResult{}, configPath, fmt.Errorf("refusing to modify %q: it holds the Claude Code session and could not be parsed as JSON: %w", configPath, parseErr)
		}
		merged, err := filemerge.MergeJSONObjects(raw, overlayJSON)
		if err != nil {
			return filemerge.WriteResult{}, configPath, err
		}
		current, err := readUserConfigBase(configPath)
		if err != nil {
			return filemerge.WriteResult{}, configPath, err
		}
		if !bytes.Equal(current, raw) {
			if attempt < maxAttempts {
				continue
			}
			return filemerge.WriteResult{}, configPath, fmt.Errorf("gave up merging into %q after %d attempts: the file kept changing underneath the merge", configPath, maxAttempts)
		}
		writeResult, err := filemerge.WriteFileAtomic(configPath, merged, 0o600)
		if err != nil {
			return filemerge.WriteResult{}, configPath, err
		}
		// WriteFileAtomic skips byte-identical writes (and their mode);
		// the OAuth-bearing file must end at 0600 regardless.
		if chmodErr := os.Chmod(configPath, 0o600); chmodErr != nil {
			return writeResult, configPath, fmt.Errorf("tighten mode of %q: %w", configPath, chmodErr)
		}
		return writeResult, configPath, nil
	}
}

func readUserConfigBase(configPath string) ([]byte, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %q: %w", configPath, err)
	}
	return raw, nil
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "CLAUDE.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategySeparateMCPFiles
}

// --- MCP ---

func (a *Adapter) MCPConfigPath(homeDir string, serverName string) string {
	return filepath.Join(homeDir, ".claude", "mcp", serverName+".json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}

func (a *Adapter) OutputStyleDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "output-styles")
}

func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}

func (a *Adapter) CommandsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "commands")
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

// --- Sub-agent support ---
//
// Claude Code loads agent files from ~/.claude/agents/*.md. Each file carries
// frontmatter (name, description, tools, model) and a prompt body. The SDD
// component copies the embedded set at install time, resolving the
// {{CLAUDE_MODEL}} placeholder in each file against the user's model
// assignments so the per-phase model contract is enforced at the agent layer
// rather than relying on orchestrator prose.

func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(homeDir, ".claude", "agents")
}

func (a *Adapter) EmbeddedSubAgentsDir() string {
	return "claude/agents"
}

// ClaudeModelID resolves a ClaudeModelAlias to the string Claude Code accepts
// in the `model:` frontmatter field of a sub-agent file. Claude Code uses the
// aliases ("fable", "opus", "sonnet", "haiku") verbatim, so this is an identity over
// alias.String(). Implemented as a method so the SDD injector's
// claudeModelResolver type assertion fires for this adapter.
func (a *Adapter) ClaudeModelID(alias model.ClaudeModelAlias) string {
	return alias.String()
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}

	return statResult{isDir: info.IsDir()}
}
