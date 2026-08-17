package skillregistry

import (
	"os"
	"path/filepath"
)

// SkipReason classifies why `skill-registry refresh` must not initialize a
// registry at the resolved working directory. Startup hooks (OpenCode plugin,
// Codex/Claude SessionStart hooks) run the refresh from whatever directory
// the host resolved — which for a brand-new non-project directory can be "/",
// the user's home directory, or a markerless scratch folder. Initializing
// there either fails loudly (mkdir /.atl on a read-only root) or pollutes a
// directory that is not a project. The guard mirrors the CodeGraph principle:
// never initialize in the filesystem root, $HOME, or non-project folders.
type SkipReason string

const (
	// SkipNone means the directory is a project root and refresh proceeds.
	SkipNone SkipReason = ""
	// SkipFilesystemRoot: cwd resolved to the filesystem root. Always refused,
	// even if markers somehow exist there.
	SkipFilesystemRoot SkipReason = "filesystem-root"
	// SkipHomeDirectory: cwd is the user's home directory. Always refused,
	// even with an existing .atl, so a stray home registry never keeps
	// startup hooks writing into $HOME.
	SkipHomeDirectory SkipReason = "home-directory"
	// SkipNoProjectMarker: cwd carries no project marker (no .git, no
	// existing .atl, no project skills workspace). Refresh skips without
	// creating anything.
	SkipNoProjectMarker SkipReason = "no-project-marker"
)

// RefreshSkip reports whether a refresh at cwd must be skipped and why.
// It never writes to the filesystem.
func RefreshSkip(cwd, home string) SkipReason {
	cwd = filepath.Clean(cwd)
	home = filepath.Clean(home)
	if cwd == filepath.Dir(cwd) {
		// "/" on POSIX, a volume root on Windows.
		return SkipFilesystemRoot
	}
	if cwd == home {
		return SkipHomeDirectory
	}
	if hasProjectMarker(cwd) {
		return SkipNone
	}
	return SkipNoProjectMarker
}

// hasProjectMarker reports whether cwd looks like a project root: a Git
// checkout (.git directory, or the .git file used by worktrees and
// submodules), an already-initialized registry (.atl), or any workspace
// skill source directory this registry indexes (ProjectSkillDirs).
func hasProjectMarker(cwd string) bool {
	if _, err := os.Lstat(filepath.Join(cwd, ".git")); err == nil {
		return true
	}
	if dirExists(filepath.Join(cwd, ".atl")) {
		return true
	}
	for _, dir := range ProjectSkillDirs(cwd) {
		if dirExists(dir) {
			return true
		}
	}
	return false
}
