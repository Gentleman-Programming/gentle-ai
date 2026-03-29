package agents

import (
	"os"

	"github.com/gentleman-programming/gentle-ai/internal/model"
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
