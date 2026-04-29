package system

import (
	"os"
	"path/filepath"
	"strings"
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
//
// When a new agent is added to the registry, its entry must also be added here
// until the import cycle is resolved and ScanConfigs can delegate directly to
// agents.DiscoverInstalled.
func knownAgentConfigDirs(homeDir string) []ConfigState {
	return []ConfigState{
		{Agent: "claude-code", Path: filepath.Join(homeDir, ".claude")},
		{Agent: "opencode", Path: filepath.Join(homeDir, ".config", "opencode")},
		piConfigState(homeDir),
		{Agent: "kilocode", Path: filepath.Join(homeDir, ".config", "kilo")},
		{Agent: "gemini-cli", Path: filepath.Join(homeDir, ".gemini")},
		{Agent: "cursor", Path: filepath.Join(homeDir, ".cursor")},
		{Agent: "vscode-copilot", Path: vscodeCopilotGlobalConfigDir(homeDir)},
		{Agent: "codex", Path: filepath.Join(homeDir, ".codex")},
		{Agent: "antigravity", Path: filepath.Join(homeDir, ".gemini", "antigravity")},
		{Agent: "windsurf", Path: filepath.Join(homeDir, ".codeium", "windsurf")},
		{Agent: "kimi", Path: filepath.Join(homeDir, ".kimi")},
		{Agent: "qwen-code", Path: filepath.Join(homeDir, ".qwen")},
		{Agent: "kiro-ide", Path: filepath.Join(homeDir, ".kiro")},
	}
}

func piConfigState(homeDir string) ConfigState {
	piRoot, piLegacy := resolvePiPaths(homeDir)
	state := ConfigState{Agent: "pi-coding-agent", Path: piRoot}

	for _, candidate := range []string{piRoot, piLegacy} {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}

		state.Exists = true
		state.IsDirectory = info.IsDir()
		return state
	}

	return state
}

func resolvePiPaths(homeDir string) (root string, legacy string) {
	root = strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if root == "" {
		root = filepath.Join(homeDir, ".pi", "agent")
	}

	legacy = filepath.Join(homeDir, ".config", "pi-coding-agent")
	return root, legacy
}

// vscodeCopilotGlobalConfigDir returns ~/.copilot, the GlobalConfigDir used by
// the vscode-copilot adapter across all platforms. The vscode adapter's
// SystemPromptDir and SettingsPath are OS-dependent, but GlobalConfigDir is not.
func vscodeCopilotGlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".copilot")
}

// ScanConfigs returns the presence state of every known managed agent's global
// This is a compatibility shim: it preserves the ConfigState contract for TUI
// and validation callers while the canonical discovery (agents.DiscoverInstalled)
// is used by sync and upgrade flows. Full delegation is deferred until the
// system ← agents import cycle is resolved (follow-up change).
func ScanConfigs(homeDir string) []ConfigState {
	states := knownAgentConfigDirs(homeDir)

	for idx := range states {
		if states[idx].Agent == "pi-coding-agent" && states[idx].Exists {
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
