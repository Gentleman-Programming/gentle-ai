package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func resolveBindingChangeRoot(ctx context.Context, root, workspace, change string) (string, error) {
	// Both operands are canonicalized the same way before any containment or
	// equality decision below. Resolving only the workspace was 1773 boundary
	// 1: on macOS the same repository spelled through /var and through
	// /private/var compared unequal, and the planning workspace was reported
	// outside its own repository.
	workspace, err := canonicalBindingPath(workspace)
	if err != nil {
		return "", err
	}
	root, err = canonicalBindingPath(root)
	if err != nil {
		return "", err
	}
	if !pathWithinBindingRoot(root, workspace) {
		return "", errors.New("planning workspace is outside selected repository")
	}

	planningRoot := ""
	for current := workspace; pathWithinBindingRoot(root, current); current = filepath.Dir(current) {
		openspecRoot := filepath.Join(current, "openspec")
		info, statErr := os.Stat(openspecRoot)
		if statErr == nil {
			if !info.IsDir() {
				return "", errors.New("selected OpenSpec root is not a directory")
			}
			resolved, resolveErr := filepath.EvalSymlinks(openspecRoot)
			if resolveErr != nil {
				return "", resolveErr
			}
			resolved = filepath.Clean(resolved)
			if !pathWithinBindingRoot(root, resolved) {
				return "", errors.New("selected OpenSpec root resolves outside repository")
			}
			if resolved != filepath.Clean(openspecRoot) {
				return "", errors.New("selected OpenSpec root uses a symlinked path")
			}
			planningRoot = current
			break
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		if pathidentity.SameDirectory(current, root) {
			break
		}
	}
	if planningRoot == "" {
		return "", errors.New("selected OpenSpec change does not exist")
	}
	candidate := filepath.Join(planningRoot, "openspec", "changes", change)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("selected OpenSpec change does not exist")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("selected OpenSpec change is not a directory")
	}
	selected, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	selected = filepath.Clean(selected)
	if !pathWithinBindingRoot(root, selected) {
		return "", errors.New("selected OpenSpec change resolves outside repository")
	}
	if selected != filepath.Clean(candidate) {
		return "", errors.New("selected OpenSpec change uses a symlinked path")
	}

	matches, err := bindingChangeRoots(ctx, root, change)
	if err != nil {
		return "", err
	}
	if len(matches) != 1 || matches[0] != selected {
		return "", errors.New("selected OpenSpec change is ambiguous within repository")
	}
	return selected, nil
}

func bindingChangeRoots(ctx context.Context, root, change string) ([]string, error) {
	paths, err := (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverTrackedAndUnignoredPaths(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	matches := []string{}
	for _, logicalPath := range paths {
		parts := strings.Split(logicalPath, "/")
		if parts[len(parts)-1] == "openspec" {
			parts = append(parts, "changes", change)
		}
		for index := 0; index+2 < len(parts); index++ {
			if parts[index] != "openspec" || parts[index+1] != "changes" || parts[index+2] != change {
				continue
			}
			rootPath := strings.Join(parts[:index+3], "/")
			if _, duplicate := seen[rootPath]; duplicate {
				break
			}
			seen[rootPath] = struct{}{}
			candidate := filepath.Join(root, filepath.FromSlash(rootPath))
			info, statErr := os.Lstat(candidate)
			if os.IsNotExist(statErr) {
				break
			}
			if statErr != nil {
				return nil, statErr
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				matches = append(matches, filepath.Clean(candidate))
			}
			break
		}
	}
	return matches, nil
}

// canonicalBindingPath is the single canonicalization every binding path goes
// through before it is compared with another. Having one of these, used on
// both operands, is what keeps a second spelling of one repository from
// looking like a different repository.
func canonicalBindingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// pathWithinBindingRoot defers containment to the filesystem identity policy
// in internal/pathidentity, so alternate spellings that the operating system
// resolves to one directory -- symlinked ancestors, case-insensitive volumes,
// Unicode-equivalent names -- are one directory here too. Callers still
// resolve a candidate with filepath.EvalSymlinks before asking, because this
// answers "is it inside", never "did it get there through a symlink".
func pathWithinBindingRoot(root, path string) bool {
	return pathidentity.Contains(root, path)
}
