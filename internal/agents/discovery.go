package agents

import (
	"context"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// InstalledAgent pairs an agent ID with its resolved config root directory.
// Both fields are guaranteed non-empty when returned from DiscoverInstalled.
type InstalledAgent struct {
	ID        model.AgentID
	ConfigDir string // GlobalConfigDir value (non-empty, exists on disk)
}

// DiscoverInstalled returns agents whose GlobalConfigDir exists on disk.
//
// It iterates over all adapters registered in reg and calls GlobalConfigDir
// for each. Adapters that return an empty string or a path that does not exist
// as a directory on disk are silently excluded.
//
// This is a pure FS check — no subprocess spawning occurs.
// The registry parameter is explicit (not a package global) to keep the
// function TDD-pure: callers and tests inject the exact registry they want.
func DiscoverInstalled(reg *Registry, homeDir string) []InstalledAgent {
	var out []InstalledAgent

	for _, id := range reg.SupportedAgents() {
		adapter, ok := reg.Get(id)
		if !ok {
			continue
		}
		installed, _, _, configFound, err := adapter.Detect(context.Background(), homeDir)
		if err != nil || (!installed && !configFound) {
			continue
		}

		dir := adapter.GlobalConfigDir(homeDir)
		if dir == "" {
			continue
		}

		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		out = append(out, InstalledAgent{ID: id, ConfigDir: dir})
	}

	return out
}

// ConfigRootsForBackup returns deduplicated config root directories for all
// agents in reg whose GlobalConfigDir exists on disk.
//
// The returned slice is never nil (may be empty). Directories are deduplicated
// so that agents sharing a config root contribute only one entry.
func ConfigRootsForBackup(reg *Registry, homeDir string) []string {
	installed := DiscoverInstalled(reg, homeDir)

	seen := make(map[string]struct{}, len(installed))
	dirs := make([]string, 0, len(installed))

	for _, a := range installed {
		if _, ok := seen[a.ConfigDir]; ok {
			continue
		}
		seen[a.ConfigDir] = struct{}{}
		dirs = append(dirs, a.ConfigDir)
	}

	return dirs
}

// SelectionScopeMode describes whether persisted installation state defines the
// managed agent scope or whether callers may use filesystem discovery instead.
type SelectionScopeMode uint8

const (
	// SelectionScopeFilesystemFallback preserves behavior for missing state and
	// incidental state written before installation, such as the update cooldown.
	SelectionScopeFilesystemFallback SelectionScopeMode = iota
	// SelectionScopeConfigured means AgentIDs is authoritative, including when it
	// is empty because the user deliberately saved an empty selection.
	SelectionScopeConfigured
	// SelectionScopeUnavailable means state could not be read and callers must not
	// widen scope through filesystem discovery.
	SelectionScopeUnavailable
)

// SelectionScope is the persisted installation-selection contract shared by
// managed-file writers, backup, and TUI preselection.
type SelectionScope struct {
	Mode     SelectionScopeMode
	AgentIDs []model.AgentID
}

// SelectionScopeFromInstallState classifies a readable install state. A
// non-empty installed_agents list is always authoritative for backward
// compatibility. An empty list becomes authoritative only after installation
// recorded SelectionConfigured; otherwise it may be incidental state created by
// non-install flows such as the update cooldown.
func SelectionScopeFromInstallState(s state.InstallState) SelectionScope {
	ids := make([]model.AgentID, 0, len(s.InstalledAgents))
	for _, id := range s.InstalledAgents {
		ids = append(ids, model.AgentID(id))
	}
	if len(ids) > 0 || s.SelectionConfigured {
		return SelectionScope{Mode: SelectionScopeConfigured, AgentIDs: ids}
	}
	return SelectionScope{Mode: SelectionScopeFilesystemFallback}
}

// ReadSelectionScope reads and classifies the persisted installation selection.
// Missing state is a legacy filesystem-fallback case. Any other read error leaves
// the scope unavailable so callers fail closed instead of managing detected
// agents that were never selected.
func ReadSelectionScope(homeDir string) SelectionScope {
	s, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return SelectionScope{Mode: SelectionScopeFilesystemFallback}
		}
		return SelectionScope{Mode: SelectionScopeUnavailable}
	}
	return SelectionScopeFromInstallState(s)
}

// DiscoverSelected returns the installed agents the user actually selected, and
// is the discovery entry point for every code path that WRITES managed files.
//
// DiscoverInstalled alone answers "what exists on this machine" — a different
// question: an IDE the user never chose still has a config directory, and
// writing into it violates the install-time selection.
// Intersecting the two keeps both guarantees: never touch an unselected agent,
// never create a config directory for an agent that is not installed. Legacy or
// incidental state falls back to the filesystem; unreadable state fails closed.
func DiscoverSelected(reg *Registry, homeDir string) []InstalledAgent {
	scope := ReadSelectionScope(homeDir)
	if scope.Mode == SelectionScopeUnavailable {
		return nil
	}

	installed := DiscoverInstalled(reg, homeDir)
	if scope.Mode == SelectionScopeFilesystemFallback {
		return installed
	}

	allowed := make(map[model.AgentID]struct{}, len(scope.AgentIDs))
	for _, id := range scope.AgentIDs {
		allowed[id] = struct{}{}
	}

	var out []InstalledAgent
	for _, agent := range installed {
		if _, ok := allowed[agent.ID]; ok {
			out = append(out, agent)
		}
	}
	return out
}
