package shellinstaller

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// userNonRuntimeSuffixes are dependency files that are never loaded at run
// time: TypeScript declaration files and source maps. A managed Gentle Shell
// prefix carries tens of thousands of files and every one of them is scanned
// by real-time antivirus on first use, so removing the dead ones shortens the
// first launch without any exclusion.
var userNonRuntimeSuffixes = []string{
	".d.ts", ".d.mts", ".d.cts",
	".js.map", ".mjs.map", ".cjs.map", ".css.map",
	".d.ts.map", ".d.mts.map", ".d.cts.map",
}

// userPrunePreservedTop names the top-level package trees the pruner never
// enters. gentle-pi is compared byte for byte against the Main snapshot during
// verification, and typescript reads its own lib.*.d.ts files at run time.
var userPrunePreservedTop = map[string]bool{"gentle-pi": true, "typescript": true}

func userNonRuntimeFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range userNonRuntimeSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// userPruneNonRuntimeFiles removes declaration files and source maps below
// modules (a node_modules directory) and returns how many it removed.
//
// It is best effort: it never follows or removes symlinks, never enters the
// top-level gentle-pi and typescript trees, treats a missing root as nothing to do, and joins
// per-file failures instead of stopping, so a partial prune still leaves a
// working tree.
func userPruneNonRuntimeFiles(modules string) (int, error) {
	if _, err := os.Lstat(modules); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	removed := 0
	var failures []error
	walkErr := filepath.WalkDir(modules, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			failures = append(failures, err)
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != modules && userPrunePreservedTop[entry.Name()] && filepath.Dir(path) == modules {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !userNonRuntimeFile(entry.Name()) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			failures = append(failures, err)
			return nil
		}
		removed++
		return nil
	})
	if walkErr != nil {
		failures = append(failures, walkErr)
	}
	if len(failures) > 0 {
		return removed, fmt.Errorf("prune non-runtime files under %q: %w", modules, errors.Join(failures...))
	}
	return removed, nil
}
