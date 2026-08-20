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

// SelectedAgentIDs returns the agent IDs the user explicitly chose at install
// time, as persisted in ~/.gentle-ai/state.json (installed_agents).
//
// This is the single authority for "which agents did the user pick". An empty
// result means no selection was ever persisted — users who installed before
// state persistence existed — and callers must fall back to filesystem
// discovery rather than treating it as "no agents".
func SelectedAgentIDs(homeDir string) []model.AgentID {
	s, err := state.Read(homeDir)
	if err != nil || len(s.InstalledAgents) == 0 {
		return nil
	}

	ids := make([]model.AgentID, 0, len(s.InstalledAgents))
	for _, a := range s.InstalledAgents {
		ids = append(ids, model.AgentID(a))
	}
	return ids
}

// DiscoverSelected returns the installed agents the user actually selected.
//
// It is the discovery entry point for every code path that WRITES managed
// files. DiscoverInstalled alone answers "what exists on this machine", which
// is not the same question: an IDE the user never chose in Gentle AI still has
// a config directory, and writing into it violates the install-time selection
// (issue #3473).
//
// The persisted selection is intersected with filesystem discovery, so this
// keeps both guarantees at once: never touch an unselected agent, and never
// create a config directory for an agent that is not installed.
//
// When no selection was persisted, the filesystem result is returned unchanged
// for backward compatibility with pre-state installs.
func DiscoverSelected(reg *Registry, homeDir string) []InstalledAgent {
	installed := DiscoverInstalled(reg, homeDir)

	selected := SelectedAgentIDs(homeDir)
	if len(selected) == 0 {
		return installed
	}

	allowed := make(map[model.AgentID]struct{}, len(selected))
	for _, id := range selected {
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
