package symlinkguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveExisting(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return "", false, fmt.Errorf("stat file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, true, nil
	}

	root, err := AllowedRoot(path)
	if err != nil {
		return "", false, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("resolve symlink %q: %w", path, err)
		}
		target, readErr := DanglingTarget(path)
		if readErr != nil {
			return "", false, readErr
		}
		return target, false, EnsureWithinRoot(target, root, path)
	}
	if err := EnsureWithinRoot(resolved, root, path); err != nil {
		return "", false, err
	}
	return resolved, true, nil
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
