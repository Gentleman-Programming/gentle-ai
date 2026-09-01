package assets

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// SharedSkillDir is the embedded directory holding the skill files that are
// shared across skills rather than owned by any single one.
const SharedSkillDir = "skills/_shared"

// SharedSkillFileNames returns every file embedded under SharedSkillDir as a
// slash-separated path relative to that directory, sorted for deterministic
// ordering. The walk is recursive, so a nested shared file is reported too.
//
// The embedded directory listing is the single source of truth for the shared
// skill inventory. Every deployment path (install, sync, upgrade) derives from
// this function; none of them may keep a literal list, because a literal that
// drifts silently strands a shared file on the build machine.
func SharedSkillFileNames() ([]string, error) {
	var names []string
	err := fs.WalkDir(FS, SharedSkillDir, func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		names = append(names, strings.TrimPrefix(assetPath, SharedSkillDir+"/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list embedded %s: %w", SharedSkillDir, err)
	}

	sort.Strings(names)
	return names, nil
}
