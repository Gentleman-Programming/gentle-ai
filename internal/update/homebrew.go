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

	activePath, err := resolvePath(toolName)
	if err != nil || activePath == "" {
		return false
	}

	return pathWithinPrefix(activePath, brewPrefix)
}

func pathWithinPrefix(path, prefix string) bool {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolvedPath
	}
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err == nil {
		prefix = resolvedPrefix
	}

	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+string(filepath.Separator))
}

type HomebrewOwnership string

const (
	HomebrewNone    HomebrewOwnership = "none"
	HomebrewFormula HomebrewOwnership = "formula"
	HomebrewCask    HomebrewOwnership = "cask"
)

type outputRunner func(string, ...string) ([]byte, error)

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
		owned, err := pathWithinResolvedPrefix(activePath, root)
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

func pathWithinResolvedPrefix(path, prefix string) (bool, error) {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return false, err
	}
	resolvedPath, resolvedPrefix = filepath.Clean(resolvedPath), filepath.Clean(resolvedPrefix)
	return resolvedPath == resolvedPrefix || strings.HasPrefix(resolvedPath, resolvedPrefix+string(filepath.Separator)), nil
}
