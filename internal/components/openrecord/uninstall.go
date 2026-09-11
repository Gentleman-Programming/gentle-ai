package openrecord

import "path/filepath"

// UninstallTargets reports every path under skillDir that gentle-ai's own
// fan-out is authoritative for: exactly this manifest's entries plus the
// manifest itself — never a hardcoded skill list, since gentle-ai does not
// decide what openrecord ships, only what its own fan-out wrote here. It
// returns nil when no manifest is present, so a caller that ranges over the
// result never removes anything on an empty scope.
func UninstallTargets(skillDir string) []string {
	entries := EmittedPaths(skillDir)
	if len(entries) == 0 {
		return nil
	}
	targets := make([]string, 0, len(entries)+1)
	for _, emittedPath := range entries {
		targets = append(targets, filepath.Join(skillDir, filepath.FromSlash(emittedPath)))
	}
	return append(targets, filepath.Join(skillDir, ManifestName))
}
