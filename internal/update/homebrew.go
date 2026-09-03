package update

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var homebrewPackageInstalled = defaultHomebrewPackageInstalled
var homebrewOwnershipDetector = DetectHomebrewOwnership

type commandRunner func(string, ...string) *exec.Cmd
type pathResolver func(string) (string, error)

func defaultHomebrewPackageInstalled(toolName string) bool {
	kind, err := homebrewOwnershipDetector(toolName)
	return err == nil && kind != HomebrewNone
}

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

// pathWithinPrefix reports whether path is owned by the Homebrew installation
// rooted at prefix. A path counts as Homebrew-owned when:
//  1. path itself (after symlink resolution) lives under prefix, OR
//  2. path is a symlink that lives under binRoot (e.g. <brew-root>/bin/<tool>
//     → an external target). Homebrew placed the symlink during
//     `brew install`; `brew upgrade` will replace it.
//
// binRoot is optional. When empty, only rule 1 applies. When set, it is
// resolved together with prefix so platform-level symlinks (e.g. macOS
// /var → /private/var) don't desync the comparison.
func pathWithinPrefix(path, prefix, binRoot string) bool {
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err == nil {
		prefix = resolvedPrefix
	}
	prefix = filepath.Clean(prefix)

	cleanPath := filepath.Clean(path)

	// Rule 1: path (as written or as the resolved symlink target) lives in
	// the resolved prefix.
	if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+string(filepath.Separator)) {
		return true
	}
	if resolvedPath, err := filepath.EvalSymlinks(cleanPath); err == nil {
		resolvedPath = filepath.Clean(resolvedPath)
		if resolvedPath == prefix || strings.HasPrefix(resolvedPath, prefix+string(filepath.Separator)) {
			return true
		}
	}

	// Rule 2: the symlink itself lives under <brew-root>/bin. Resolving the
	// symlink's parent puts both sides in the same resolved namespace, which
	// matters on macOS where /var/folders resolves to /private/var/folders.
	if binRoot != "" {
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
		// Also accept the symlink path itself (un-resolved) as a quick
		// fallback when the parent path has no resolvable intermediate
		// symlinks.
		if parent := filepath.Dir(cleanPath); parent == binRoot || strings.HasPrefix(parent, binRoot+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

type HomebrewOwnership string

const (
	HomebrewNone    HomebrewOwnership = "none"
	HomebrewFormula HomebrewOwnership = "formula"
	HomebrewCask    HomebrewOwnership = "cask"
)

type outputRunner func(string, ...string) ([]byte, error)

// DetectHomebrewOwnership determines which kind of Homebrew artifact (formula
// or cask), if any, owns the active executable for toolName.
//
// A Homebrew "formula" is an entry in /opt/homebrew/Library/Taps/.../Formula
// that installs to <brew-root>/opt/<tool>/<version>/. A "cask" is the legacy
// distribution path that installs to <brew-root>/Caskroom/<tool>/<version>/.
// `brew install <tool>` materializes both: it drops the binary in the
// artifact-specific directory AND a symlink at <brew-root>/bin/<tool> that
// points to it. The symlink is what `command -v <tool>` returns.
//
// Ownership is decided by checking whether the active executable lives inside
// either the artifact-specific directory OR <brew-root>/bin. The second arm
// matters when a developer redirects the symlink to a local `go install` build
// (e.g. /opt/homebrew/bin/gentle-ai → /Users/<name>/go/bin/gentle-ai) — the
// symlink still belongs to Homebrew because brew placed it, and `brew upgrade`
// will replace it. Treating this as "not Homebrew-owned" would break
// `gentle-ai update` for every developer using gentle-ai via brew alongside a
// source build.
func DetectHomebrewOwnership(toolName string) (HomebrewOwnership, error) {
	return detectHomebrewOwnershipWith(func(name string, args ...string) ([]byte, error) {
		return execCommand(name, args...).Output()
	}, lookPath, toolName)
}

func detectHomebrewOwnershipWith(run outputRunner, resolvePath pathResolver, toolName string) (HomebrewOwnership, error) {
	toolName = strings.TrimSpace(toolName)
	formulaOutput, err := run("brew", "list", "--formula", "--full-name")
	if err != nil {
		return HomebrewNone, fmt.Errorf("list Homebrew formulae: %w", err)
	}
	caskOutput, err := run("brew", "list", "--cask", "--full-name")
	if err != nil {
		return HomebrewNone, fmt.Errorf("list Homebrew casks: %w", err)
	}
	formula := matchingHomebrewArtifact(string(formulaOutput), toolName)
	cask := matchingHomebrewArtifact(string(caskOutput), toolName)
	if formula == "" && cask == "" {
		return HomebrewNone, nil
	}
	activePath, err := resolvePath(toolName)
	if err != nil {
		return HomebrewNone, fmt.Errorf("resolve active executable %q: %w", toolName, err)
	}
	if strings.TrimSpace(activePath) == "" {
		return HomebrewNone, fmt.Errorf("resolve active executable %q: empty path", toolName)
	}

	brewRootOutput, err := run("brew", "--prefix")
	if err != nil {
		return HomebrewNone, fmt.Errorf("resolve Homebrew root: %w", err)
	}
	brewRoot := strings.TrimSpace(string(brewRootOutput))
	if brewRoot == "" {
		return HomebrewNone, fmt.Errorf("resolve Homebrew root: empty output")
	}
	brewBinRoot := filepath.Join(brewRoot, "bin")

	artifacts := map[HomebrewOwnership]string{HomebrewFormula: formula, HomebrewCask: cask}
	match := HomebrewNone
	for kind, artifact := range artifacts {
		if artifact == "" {
			continue
		}
		args := []string{"--prefix"}
		if kind == HomebrewFormula {
			args = append(args, artifact)
		}
		output, err := run("brew", args...)
		if err != nil {
			return HomebrewNone, fmt.Errorf("resolve Homebrew %s prefix for %q: %w", kind, artifact, err)
		}
		root := strings.TrimSpace(string(output))
		if root == "" {
			return HomebrewNone, fmt.Errorf("resolve Homebrew %s prefix for %q: empty output", kind, artifact)
		}
		if kind == HomebrewCask {
			root = filepath.Join(root, "Caskroom", filepath.Base(artifact))
		}
		owned, err := pathWithinResolvedPrefix(activePath, root, brewBinRoot)
		if err != nil {
			return HomebrewNone, fmt.Errorf("verify Homebrew %s ownership for %q: %w", kind, toolName, err)
		}
		if owned && match != HomebrewNone {
			return HomebrewNone, fmt.Errorf("active executable %q matches both Homebrew formula and cask paths", activePath)
		}
		if owned {
			match = kind
		}
	}
	if match == HomebrewNone {
		return HomebrewNone, fmt.Errorf("active executable %q is outside installed Homebrew paths", activePath)
	}
	return match, nil
}

func matchingHomebrewArtifact(output, toolName string) string {
	for _, line := range strings.Split(output, "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == toolName || strings.HasSuffix(candidate, "/"+toolName) {
			return candidate
		}
	}
	return ""
}

// pathWithinResolvedPrefix is the error-returning sibling of pathWithinPrefix.
// See pathWithinPrefix for the rationale on accepting Homebrew-placed symlinks
// even when their targets are external. It only surfaces EvalSymlinks failures
// for the prefix itself; the active path is checked as-given first, then
// followed when that fails.
func pathWithinResolvedPrefix(path, prefix, binRoot string) (bool, error) {
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return false, err
	}
	resolvedPrefix = filepath.Clean(resolvedPrefix)

	cleanPath := filepath.Clean(path)
	if cleanPath == resolvedPrefix || strings.HasPrefix(cleanPath, resolvedPrefix+string(filepath.Separator)) {
		return true, nil
	}

	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		resolvedPath = filepath.Clean(resolvedPath)
		if resolvedPath == resolvedPrefix || strings.HasPrefix(resolvedPath, resolvedPrefix+string(filepath.Separator)) {
			return true, nil
		}
	} else {
		// Surface a stable error so the caller can distinguish "not within
		// prefix" from "could not resolve symlink", but only after the cheap
		// path-prefix check has already failed.
		return false, fmt.Errorf("resolve symlink for %q: %w", path, err)
	}

	// Rule 2 fallback: the symlink itself lives under <brew-root>/bin.
	if binRoot != "" {
		resolvedBin, binErr := filepath.EvalSymlinks(binRoot)
		if binErr == nil {
			binRoot = resolvedBin
		}
		binRoot = filepath.Clean(binRoot)
		if resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(cleanPath)); parentErr == nil {
			resolvedParent = filepath.Clean(resolvedParent)
			if resolvedParent == binRoot || strings.HasPrefix(resolvedParent, binRoot+string(filepath.Separator)) {
				return true, nil
			}
		}
		if parent := filepath.Dir(cleanPath); parent == binRoot || strings.HasPrefix(parent, binRoot+string(filepath.Separator)) {
			return true, nil
		}
	}

	return false, nil
}
