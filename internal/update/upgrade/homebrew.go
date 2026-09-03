package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

var homebrewPackageInstalled = defaultHomebrewPackageInstalled
var homebrewOwnershipDetector = update.DetectHomebrewOwnership

type commandRunner func(string, ...string) *exec.Cmd
type pathResolver func(string) (string, error)

func defaultHomebrewPackageInstalled(toolName string) bool {
	kind, err := homebrewOwnershipDetector(toolName)
	return err == nil && kind != update.HomebrewNone
}

// homebrewPackageInstalledWith reports whether the active executable
// for toolName is owned by Homebrew for the upgrade executor's
// purposes. It mirrors the detection helper in internal/update but
// keeps its own copy so the upgrade package can be tested without
// pulling the full update surface. The shape and rules match
// internal/update.homebrewPackageInstalledWith so the two callers
// always agree on ownership for the same machine.
func homebrewPackageInstalledWith(run commandRunner, resolvePath pathResolver, toolName string) bool {
	if toolName == "" {
		return false
	}
	if run("brew", "list", "--formula", toolName).Run() != nil {
		return false
	}

	brewPrefixOutput, err := run("brew", "--prefix", toolName).Output()
	if err != nil {
		return false
	}
	brewPrefix := strings.TrimSpace(string(brewPrefixOutput))
	if brewPrefix == "" {
		return false
	}
	brewRootOutput, err := run("brew", "--prefix").Output()
	if err != nil {
		return false
	}
	brewRoot := strings.TrimSpace(string(brewRootOutput))
	if brewRoot == "" {
		return false
	}
	brewBinRoot := filepath.Join(brewRoot, "bin")

	activePath, err := resolvePath(toolName)
	if err != nil || activePath == "" {
		return false
	}

	return pathWithinPrefix(activePath, brewPrefix, brewBinRoot)
}

// pathWithinPrefix mirrors internal/update.pathWithinPrefix. A Homebrew-created
// symlink in <brew-root>/bin/<tool> is accepted as Homebrew-owned even when its
// target lies outside the Homebrew prefix, because brew install placed the
// symlink and brew upgrade will replace it. See internal/update/homebrew.go for
// the full rationale.
//
// A regular file under <brew-root>/bin is never accepted: a user-managed
// binary dropped there must not be classified as Homebrew-owned, otherwise
// `gentle-ai upgrade` would hand it to `brew upgrade` for replacement. This
// guard is checked before Rule 1 because the prefix argument can, in theory,
// coincide with the bin directory; the symlink check below is the canonical
// path and would also reject, but doing the check first gives a clearer
// negative result.
func pathWithinPrefix(path, prefix, binRoot string) bool {
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err == nil {
		prefix = resolvedPrefix
	}
	prefix = filepath.Clean(prefix)

	cleanPath := filepath.Clean(path)

	// Pre-Rule-1 guard: a regular file under <brew-root>/bin is never
	// Homebrew-owned, even if some of the prefix arguments below would
	// otherwise match. Reject before any prefix check.
	if binRoot != "" && !isBrewOwnedSymlink(cleanPath) {
		resolvedBin, binErr := filepath.EvalSymlinks(binRoot)
		if binErr == nil {
			binRoot = resolvedBin
		}
		binRoot = filepath.Clean(binRoot)
		if resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(cleanPath)); parentErr == nil {
			resolvedParent = filepath.Clean(resolvedParent)
			if resolvedParent == binRoot || strings.HasPrefix(resolvedParent, binRoot+string(filepath.Separator)) {
				return false
			}
		}
		if parent := filepath.Dir(cleanPath); parent == binRoot || strings.HasPrefix(parent, binRoot+string(filepath.Separator)) {
			return false
		}
	}

	if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+string(filepath.Separator)) {
		return true
	}
	if resolvedPath, err := filepath.EvalSymlinks(cleanPath); err == nil {
		resolvedPath = filepath.Clean(resolvedPath)
		if resolvedPath == prefix || strings.HasPrefix(resolvedPath, prefix+string(filepath.Separator)) {
			return true
		}
	}

	// Rule 2 fallback: a symlink whose parent lives under <brew-root>/bin.
	// The symlink restriction prevents regular files dropped into
	// <brew-root>/bin by the user from being treated as Homebrew-owned.
	if binRoot != "" && isBrewOwnedSymlink(cleanPath) {
		resolvedBin, err := filepath.EvalSymlinks(binRoot)
		if err == nil {
			binRoot = resolvedBin
		}
		binRoot = filepath.Clean(binRoot)
		if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(cleanPath)); err == nil {
			resolvedParent = filepath.Clean(resolvedParent)
			if resolvedParent == binRoot || strings.HasPrefix(resolvedParent, binRoot+string(filepath.Separator)) {
				return true
			}
		}
		if parent := filepath.Dir(cleanPath); parent == binRoot || strings.HasPrefix(parent, binRoot+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// isBrewOwnedSymlink mirrors the helper in internal/update so the upgrade
// predicate applies the same symlink-only restriction to <brew-root>/bin.
func isBrewOwnedSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
