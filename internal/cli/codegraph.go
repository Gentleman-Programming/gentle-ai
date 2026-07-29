package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	codeGraphGitTopLevel = defaultCodeGraphGitTopLevel
	codeGraphInit        = func(name string, args ...string) error { return exec.Command(name, args...).Run() }
	codeGraphUserHomeDir = os.UserHomeDir
	codeGraphTempDir     = os.TempDir
)

// RunCodeGraph exposes the safe, Gentle-AI-owned initialization boundary used
// by generated agent guidance. It intentionally accepts no raw CodeGraph args.
func RunCodeGraph(args []string, stdout io.Writer) error {
	if len(args) != 3 || args[0] != "init" || args[1] != "--cwd" || strings.TrimSpace(args[2]) == "" {
		return fmt.Errorf("usage: gentle-ai codegraph init --cwd <project-root>")
	}
	root, err := canonicalCodeGraphProjectRoot(args[2])
	if err != nil {
		return err
	}
	if err := codeGraphInit("codegraph", "init", root); err != nil {
		return fmt.Errorf("initialize CodeGraph at %q: %w", root, err)
	}
	_, _ = fmt.Fprintf(stdout, "CodeGraph initialized: %s\n", root)
	return nil
}

func canonicalCodeGraphProjectRoot(candidate string) (string, error) {
	canonicalCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve CodeGraph root %q: %w", candidate, err)
	}
	canonicalCandidate, err = filepath.Abs(canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("resolve CodeGraph root %q: %w", candidate, err)
	}
	if isUnsafeCodeGraphRoot(canonicalCandidate) {
		return "", fmt.Errorf("unsafe CodeGraph root %q", candidate)
	}
	gitRoot, err := codeGraphGitTopLevel(canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("CodeGraph root %q is not a recognized project: %w", candidate, err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(strings.TrimSpace(gitRoot))
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", gitRoot, err)
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", gitRoot, err)
	}
	if canonicalRoot != canonicalCandidate || isUnsafeCodeGraphRoot(canonicalRoot) {
		return "", fmt.Errorf("unsafe CodeGraph root %q", candidate)
	}
	return canonicalRoot, nil
}

func isUnsafeCodeGraphRoot(root string) bool {
	if volume := filepath.VolumeName(root); root == volume+string(filepath.Separator) {
		return true
	}
	home := canonicalCodeGraphRestrictedPath(codeGraphHomeDir())
	if home != "" && root == home {
		return true
	}
	temp := canonicalCodeGraphRestrictedPath(codeGraphTempDir())
	if temp != "" && pathIsWithin(temp, root) {
		return true
	}
	return false
}

func canonicalCodeGraphRestrictedPath(path string) string {
	if path == "" {
		return ""
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		canonical = path
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return ""
	}
	return canonical
}

func codeGraphHomeDir() string {
	home, err := codeGraphUserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func pathIsWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func defaultCodeGraphGitTopLevel(path string) (string, error) {
	output, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
