package system

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/copilotconfig"
)

// ConfigState records the filesystem presence of an agent's global config directory.
// All known registry agents are always represented — Exists=false for absent dirs.
// This contract is consumed by the TUI detection screen and install/validate flows.
type ConfigState struct {
	Agent       string
	Path        string
	Exists      bool
	IsDirectory bool
}

// knownAgentConfigDirs enumerates the per-agent config roots used by ScanConfigs
// for presence scanning as (agentID, path) pairs. This is a compatibility shim
// that mirrors the adapter registry's full set without importing the agents
// package (which would create an import cycle: system ← agents ← system).
//
// Most entries mirror Adapter.GlobalConfigDir(). Kiro is an intentional
// exception: we scan `~/.kiro` (managed artifacts root) instead of
// `%APPDATA%/kiro/User` (settings root) due to Kiro's split-root layout.
// Kimi is resolved through kimiConfigState: it prefers the current
// kimi-code v0.11+ root `~/.kimi-code` when it exists as a directory and
// falls back to the legacy `~/.kimi` root.
//
// When a new agent is added to the registry, its entry must also be added here
// until the import cycle is resolved and ScanConfigs can delegate directly to
// agents.DiscoverInstalled.
func knownAgentConfigDirs(homeDir string) []ConfigState {
	return []ConfigState{
		{Agent: "claude-code", Path: filepath.Join(homeDir, ".claude")},
		{Agent: "opencode", Path: filepath.Join(homeDir, ".config", "opencode")},
		{Agent: "kilocode", Path: filepath.Join(homeDir, ".config", "kilo")},
		{Agent: "gemini-cli", Path: filepath.Join(homeDir, ".gemini")},
		{Agent: "cursor", Path: filepath.Join(homeDir, ".cursor")},
		{Agent: "vscode-copilot", Path: vscodeCopilotGlobalConfigDir(homeDir)},
		{Agent: "codex", Path: filepath.Join(homeDir, ".codex")},
		{Agent: "antigravity", Path: filepath.Join(homeDir, ".gemini", "antigravity-cli")},
		{Agent: "windsurf", Path: filepath.Join(homeDir, ".codeium", "windsurf")},
		{Agent: "kimi", Path: kimiConfigDir(homeDir)},
		{Agent: "qwen-code", Path: filepath.Join(homeDir, ".qwen")},
		{Agent: "kiro-ide", Path: filepath.Join(homeDir, ".kiro")},
		{Agent: "openclaw", Path: filepath.Join(homeDir, ".openclaw")},
		{Agent: "pi", Path: filepath.Join(homeDir, ".pi")},
		{Agent: "trae-ide", Path: filepath.Join(homeDir, ".trae")},
		{Agent: "hermes", Path: filepath.Join(homeDir, ".hermes")},
		{Agent: "conductor", Path: filepath.Join(homeDir, ".conductor")},
		{Agent: "github-copilot-cli", Path: copilotconfig.Root(homeDir)},
	}
}

// kimiConfigStat is the stat seam used to resolve the Kimi config layout.
var kimiConfigStat = os.Stat

// kimiConfigDir returns the Kimi config root for homeDir, preferring the
// current kimi-code v0.11+ root `~/.kimi-code` when it exists as a directory
// and falling back to the legacy `~/.kimi` root. A plain file named
// `.kimi-code` is not a v0.11+ layout and is ignored, like absence.
//
// Only ENOENT/ENOTDIR (both reported by os.IsNotExist) mean the current root
// is absent. Any other stat failure leaves the layout undeterminable: the
// preferred current root is returned so ScanConfigs reports it as absent
// (its stat fails too) instead of silently selecting the legacy layout.
func kimiConfigDir(homeDir string) string {
	current := filepath.Join(homeDir, ".kimi-code")
	if info, err := kimiConfigStat(current); err != nil {
		if !os.IsNotExist(err) {
			return current
		}
	} else if info.IsDir() {
		return current
	}
	return filepath.Join(homeDir, ".kimi")
}

// vscodeCopilotGlobalConfigDir returns ~/.vscode to report VS Code Copilot root.
func vscodeCopilotGlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".vscode")
}

func isCopilotExtensionDir(name string) bool {
	return name == "github.copilot" || (strings.HasPrefix(name, "github.copilot-") && !strings.HasPrefix(name, "github.copilot-chat"))
}

// hasVSCodeCopilotExtension checks for github.copilot extension under .vscode/extensions.
func hasVSCodeCopilotExtension(homeDir string) (bool, bool) {
	entries, _ := os.ReadDir(filepath.Join(homeDir, ".vscode", "extensions"))
	for _, entry := range entries {
		if entry.IsDir() && isCopilotExtensionDir(entry.Name()) {
			return true, true
		}
	}
	return false, false
}

// ScanConfigs returns the presence state of every known managed agent's global
// configuration directory.
func ScanConfigs(homeDir string) []ConfigState {
	states := knownAgentConfigDirs(homeDir)

	for idx := range states {
		if states[idx].Agent == "vscode-copilot" {
			exists, isDir := hasVSCodeCopilotExtension(homeDir)
			states[idx].Exists = exists
			states[idx].IsDirectory = isDir
			continue
		}

		info, err := os.Stat(states[idx].Path)
		if err != nil {
			continue
		}

		states[idx].Exists = true
		states[idx].IsDirectory = info.IsDir()
	}

	return states
}
