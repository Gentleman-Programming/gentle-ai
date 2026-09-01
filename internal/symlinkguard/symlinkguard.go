package symlinkguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

const maxSymlinkDepth = 255

func ResolveExisting(path string) (string, bool, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("resolve absolute path %q: %w", path, err)
	}
	return resolvePath(filepath.Clean(pathAbs), filepath.Clean(pathAbs), 0)
}

func resolvePath(path, original string, depth int) (string, bool, error) {
	return resolvePathInternal(path, original, depth, true)
}

func resolvePathInternal(path, original string, depth int, enforceAllowedRoot bool) (string, bool, error) {
	if depth > maxSymlinkDepth {
		return "", false, fmt.Errorf("resolve symlink %q: too many links", original)
	}

	current, parts := pathRootAndParts(path)
	for i, part := range parts {
		candidate := filepath.Join(current, part)
		info, err := os.Lstat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.Join(append([]string{candidate}, parts[i+1:]...)...), false, nil
			}
			return "", false, fmt.Errorf("stat file %q: %w", candidate, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}

		target, err := DanglingTarget(candidate)
		if err != nil {
			return "", false, err
		}
		var root string
		if enforceAllowedRoot {
			root, err = AllowedRoot(candidate)
			if err != nil {
				return "", false, err
			}
		}
		resolved, exists, err := resolvePathInternal(filepath.Join(append([]string{target}, parts[i+1:]...)...), original, depth+1, enforceAllowedRoot)
		if err != nil {
			return "", false, err
		}
		if enforceAllowedRoot {
			if err := EnsureWithinRoot(resolved, root, candidate); err != nil {
				return "", false, err
			}
		}
		return resolved, exists, nil
	}
	return current, true, nil
}

func pathRootAndParts(path string) (string, []string) {
	volume := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, volume)
	root := volume
	if filepath.IsAbs(path) {
		root += string(filepath.Separator)
		rest = strings.TrimLeft(rest, string(filepath.Separator))
	}
	if rest == "" {
		return root, nil
	}
	return root, strings.Split(rest, string(filepath.Separator))
}

func AllowedRoot(path string) (string, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %q: %w", path, err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(pathAbs))
	if err != nil {
		return "", fmt.Errorf("resolve parent directory %q: %w", filepath.Dir(pathAbs), err)
	}
	physicalPath := filepath.Join(parent, filepath.Base(pathAbs))

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		homeAbs, absErr := filepath.Abs(home)
		if absErr != nil {
			return "", fmt.Errorf("resolve home directory %q: %w", home, absErr)
		}
		if homeResolved, resolveErr := filepath.EvalSymlinks(homeAbs); resolveErr == nil {
			pathInHome, pathErr := isWithinRootIdentity(pathAbs, homeResolved)
			physicalPathInHome, physicalPathErr := isWithinRootIdentity(physicalPath, homeResolved)
			if (pathErr == nil && pathInHome) || (physicalPathErr == nil && physicalPathInHome) {
				return filepath.Clean(homeResolved), nil
			}
		}
	}
	return filepath.Clean(parent), nil
}

func DanglingTarget(path string) (string, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", fmt.Errorf("read symlink %q: %w", path, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve symlink target %q: %w", target, err)
	}
	return filepath.Clean(targetAbs), nil
}

func EnsureWithinRoot(path, root, original string) error {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path %q: %w", path, err)
	}
	within, err := isWithinRootIdentity(pathAbs, root)
	if err != nil {
		return fmt.Errorf("resolve path %q for containment: %w", path, err)
	}
	if !within {
		return fmt.Errorf("symlink %q resolves outside allowed root %q: %q", original, root, pathAbs)
	}
	return nil
}

func isWithinRootIdentity(path, root string) (bool, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	if WithinRoot(pathAbs, rootAbs) {
		return true, nil
	}

	pathResolved, err := canonicalPath(pathAbs)
	if err != nil {
		return false, err
	}
	rootResolved, err := canonicalPath(rootAbs)
	if err != nil {
		return false, err
	}
	return pathidentity.Contains(rootResolved, pathResolved), nil
}

// SafeRemovalPath validates a path against an explicit managed root and
// returns the physical path that can be removed safely. A final symlink is
// returned unchanged so callers remove the link rather than its target.
// Symlinked ancestors are canonicalized and are allowed only when they remain
// inside root.
func SafeRemovalPath(path, root string) (string, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve removal path %q: %w", path, err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve removal root %q: %w", root, err)
	}
	rootResolved, err := filepath.EvalSymlinks(filepath.Clean(rootAbs))
	if err != nil {
		return "", fmt.Errorf("resolve removal root %q: %w", root, err)
	}

	resolved, _, err := resolvePathInternal(filepath.Clean(pathAbs), filepath.Clean(pathAbs), 0, false)
	if err != nil {
		return "", err
	}
	physical, err := canonicalPath(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve removal path %q: %w", path, err)
	}
	within, err := isWithinRootIdentity(physical, rootResolved)
	if err != nil {
		return "", fmt.Errorf("resolve removal path %q for containment: %w", path, err)
	}
	if !within {
		return "", fmt.Errorf("path %q resolves outside allowed root %q: %q", path, rootResolved, physical)
	}

	info, err := os.Lstat(filepath.Clean(pathAbs))
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return filepath.Clean(pathAbs), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat removal path %q: %w", path, err)
	}
	return physical, nil
}

func canonicalPath(path string) (string, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(pathAbs)
	missing := make([]string, 0)

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func WithinRoot(path, root string) bool {
	pathClean := filepath.Clean(path)
	rootClean := filepath.Clean(root)
	rel, err := filepath.Rel(rootClean, pathClean)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
