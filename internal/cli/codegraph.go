package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

var (
	codeGraphGitTopLevel = defaultCodeGraphGitTopLevel
	codeGraphInit        = func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		system.EnsureCommandDir(cmd)
		return cmd.Run()
	}
	codeGraphUserHomeDir = os.UserHomeDir
	codeGraphTempDir     = os.TempDir
	codeGraphWarnings    = io.Writer(os.Stderr)
)

// RunCodeGraph exposes the safe, Gentle-AI-owned initialization boundary used
// by generated agent guidance. It intentionally accepts no raw CodeGraph args.
func RunCodeGraph(args []string, stdout io.Writer) error {
	if len(args) == 0 ||
		(len(args) == 1 && (args[0] == "init" || args[0] == "--help" || args[0] == "-h")) ||
		(len(args) == 2 && args[0] == "init" && (args[1] == "--help" || args[1] == "-h")) {
		_, _ = io.WriteString(stdout, codeGraphHelp)
		return nil
	}
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
	if err := excludeCodeGraphIndexLocally(root); err != nil {
		_, _ = fmt.Fprintf(codeGraphWarnings, "WARNING: could not exclude .codegraph/ from git locally: %v\n", err)
	}
	_, _ = fmt.Fprintf(stdout, "CodeGraph initialized: %s\n", root)
	return nil
}

const codeGraphExcludeEntry = ".codegraph/"

// excludeCodeGraphIndexLocally keeps the index out of git status. CodeGraph's
// own .codegraph/.gitignore un-ignores itself, so the directory would
// otherwise stay untracked and leak into diffs. Only the worktree's local
// exclude file changes; tracked files and .gitignore are never edited.
func excludeCodeGraphIndexLocally(root string) error {
	output, err := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree", "--show-toplevel", "--git-path", "info/exclude").Output()
	if err != nil {
		return fmt.Errorf("resolve git exclude file: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 3 || lines[0] != "true" {
		// refusal:by-design world-action: only a warning; initialization continues and there is no worktree whose exclude file could change.
		return fmt.Errorf("%q is not a git work tree", root)
	}
	if topLevel, err := filepath.EvalSymlinks(lines[1]); err != nil || topLevel != root {
		// refusal:by-design world-action: only a warning; initialization continues and an enclosing repository's exclude file must not change.
		return fmt.Errorf("%q is not the git work tree root", root)
	}
	exclude := lines[2]
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(root, exclude)
	}

	content, err := os.ReadFile(exclude)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(content), "\n") {
		switch strings.TrimSpace(line) {
		case ".codegraph", ".codegraph/", "/.codegraph", "/.codegraph/":
			return nil
		}
	}
	entry := codeGraphExcludeEntry + "\n"
	if len(content) > 0 && content[len(content)-1] != '\n' {
		entry = "\n" + entry
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(entry); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

const codeGraphHelp = `gentle-ai codegraph — Initialize a project's CodeGraph index

USAGE
  gentle-ai codegraph init --cwd <project-root>

OPTIONS
  --cwd <project-root>  Existing Git project directory (required for initialization)
  --help, -h            Show this help without initializing an index

Running 'gentle-ai codegraph' or 'gentle-ai codegraph init' without arguments shows this help.
Only 'init --cwd <project-root>' is supported; other CodeGraph commands are not forwarded.
Initialization adds .codegraph/ to the worktree's local Git exclude file (never .gitignore)
so the index stays out of git status; a failure there is reported and initialization continues.
`

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
