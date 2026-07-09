package update

import (
	"os/exec"
	"path/filepath"
	"strings"
)

var homebrewPackageInstalled = defaultHomebrewPackageInstalled

type commandRunner func(string, ...string) *exec.Cmd
type pathResolver func(string) (string, error)

func defaultHomebrewPackageInstalled(toolName string) bool {
	return homebrewPackageInstalledWith(execCommand, lookPath, toolName)
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
