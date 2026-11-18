package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// userPruneRemove removes one file. Tests replace it to exercise failures.
var userPruneRemove = os.Remove

func userNonRuntimeFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range userNonRuntimeSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// userPruneProtectedDir reports whether the pruner must not enter a
// directory. rel is the slash-separated path below node_modules.
//
//   - typescript is protected wherever it is installed: the compiler reads its
//     own lib.*.d.ts files at run time.
//   - gentle-pi is protected at the top level and directly under a scope
//     (@scope/gentle-pi): verification compares it byte for byte against the
//     Main snapshot.
func userPruneProtectedDir(rel string) bool {
	parts := strings.Split(rel, "/")
	last := parts[len(parts)-1]
	if last == "typescript" {
		return true
	}
	if last != "gentle-pi" {
		return false
	}
	return len(parts) == 1 || (len(parts) == 2 && strings.HasPrefix(parts[0], "@"))
}

// userPruneNonRuntimeFiles removes declaration files and source maps below
// modules (a node_modules directory) and returns how many it removed.
//
// It is best effort: it never follows or removes symlinks or junctions, never
// enters a protected package tree, treats a missing root as nothing to do, and
// joins per-file failures instead of stopping, so a partial prune still leaves
// a working tree. It stops early with the context error when ctx is done.
func userPruneNonRuntimeFiles(ctx context.Context, modules string) (int, error) {
	if _, err := os.Lstat(modules); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	removed := 0
	var failures []error
	walkErr := filepath.WalkDir(modules, func(path string, entry fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			failures = append(failures, err)
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path == modules {
				return nil
			}
			rel, relErr := filepath.Rel(modules, path)
			if relErr == nil && userPruneProtectedDir(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !userNonRuntimeFile(entry.Name()) {
			return nil
		}
		if err := userPruneRemove(path); err != nil {
			failures = append(failures, err)
			return nil
		}
		removed++
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
			return removed, walkErr
		}
		failures = append(failures, walkErr)
	}
	if len(failures) > 0 {
		return removed, fmt.Errorf("prune non-runtime files under %q: %w", modules, errors.Join(failures...))
	}
	return removed, nil
}

// userPrunePrefix is the install step: it prunes stage/prefix/node_modules
// unless the SDK component was reused (it arrives already pruned). A prune
// failure is reported to log as a warning and never fails the install; only a
// done context is returned, because the install cannot continue then. The
// elapsed time is always logged so a slow prune can be diagnosed.
func userPrunePrefix(ctx context.Context, stage string, reused bool, log io.Writer) error {
	if reused {
		return nil
	}
	start := time.Now()
	removed, err := userPruneNonRuntimeFiles(ctx, filepath.Join(stage, "prefix", "node_modules"))
	elapsed := time.Since(start).Round(time.Millisecond)
	if ctxErr := ctx.Err(); ctxErr != nil {
		fmt.Fprintf(log, "Windows install phase: prefix prune cancelled after %d files in %s\n", removed, elapsed)
		return ctxErr
	}
	if err != nil {
		fmt.Fprintf(log, "Windows install warning: prefix prune incomplete after %d files in %s: %v\n", removed, elapsed, err)
		return nil
	}
	fmt.Fprintf(log, "Windows install phase: prefix pruned %d non-runtime files in %s\n", removed, elapsed)
	return nil
}
