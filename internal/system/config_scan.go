package system

import (
	"os"
	"path/filepath"
	"runtime"
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
		{Agent: "kilocode", Path: filepath.Join(homeDir, ".config", "kilo")},
		{Agent: "gemini-cli", Path: filepath.Join(homeDir, ".gemini")},
		{Agent: "cursor", Path: filepath.Join(homeDir, ".cursor")},
		{Agent: "vscode-copilot", Path: vscodeCopilotGlobalConfigDir(homeDir)},
		{Agent: "codex", Path: filepath.Join(homeDir, ".codex")},
		{Agent: "antigravity", Path: filepath.Join(homeDir, ".gemini", "antigravity-cli")},
		{Agent: "windsurf", Path: filepath.Join(homeDir, ".codeium", "windsurf")},
		{Agent: "kimi", Path: filepath.Join(homeDir, ".kimi")},
		{Agent: "qwen-code", Path: filepath.Join(homeDir, ".qwen")},
		{Agent: "kiro-ide", Path: filepath.Join(homeDir, ".kiro")},
		{Agent: "openclaw", Path: filepath.Join(homeDir, ".openclaw")},
		{Agent: "pi", Path: filepath.Join(homeDir, ".pi")},
		{Agent: "trae-ide", Path: filepath.Join(homeDir, ".trae")},
		{Agent: "hermes", Path: hermesConfigDir(homeDir)},
	}
}

// hermesConfigDir mirrors the Hermes adapter's home resolution
// (internal/agents/hermes ResolveHome): HERMES_HOME first, then
// %LOCALAPPDATA%\hermes on Windows, then ~/.hermes. Duplicated here because
// importing the agents package would create an import cycle
// (system ← agents ← system).
//
// Environment overrides are honored only when homeDir is the real user home
// so sandboxed callers stay contained inside the root they pass in.
func hermesConfigDir(homeDir string) string {
	if hermesHome := strings.TrimSpace(os.Getenv("HERMES_HOME")); filepath.IsAbs(hermesHome) && isHermesRealUserHome(homeDir) {
		return hermesHome
	}
	if runtime.GOOS == "windows" {
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); filepath.IsAbs(localAppData) && isHermesRealUserHome(homeDir) {
			return filepath.Join(localAppData, "hermes")
		}
		return filepath.Join(homeDir, "AppData", "Local", "hermes")
	}
	return filepath.Join(homeDir, ".hermes")
}

func isHermesRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
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
		info, err := os.Stat(states[idx].Path)
		if err != nil {
			continue
		}

		states[idx].Exists = true
		states[idx].IsDirectory = info.IsDir()
	}

	return states
}
