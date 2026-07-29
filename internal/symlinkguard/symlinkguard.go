package symlinkguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		root, err := AllowedRoot(candidate)
		if err != nil {
			return "", false, err
		}
		if err := EnsureWithinRoot(target, root, candidate); err != nil {
			return "", false, err
		}
		return resolvePath(filepath.Join(append([]string{target}, parts[i+1:]...)...), original, depth+1)
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
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		homeAbs, absErr := filepath.Abs(home)
		if absErr != nil {
			return "", fmt.Errorf("resolve home directory %q: %w", home, absErr)
		}
		if WithinRoot(pathAbs, homeAbs) {
			return filepath.Clean(homeAbs), nil
		}
	}
	return filepath.Clean(filepath.Dir(pathAbs)), nil
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
	if !WithinRoot(pathAbs, root) {
		return fmt.Errorf("symlink %q resolves outside allowed root %q: %q", original, root, pathAbs)
	}
	return nil
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
