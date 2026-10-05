package reviewtransaction

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// PrepareReviewRepositoryRoot is the review lifecycle entry boundary for a
// workspace that may not yet have local Git metadata. It reuses an existing
// containing worktree and initializes Git only after both hardened discovery
// and an isolated probe prove the workspace is genuinely unversioned.
//
// SnapshotBuilder and every lower-level repository boundary remain read-only:
// callers that need lifecycle preparation must opt in at this boundary.
func PrepareReviewRepositoryRoot(ctx context.Context, workspace string) (string, error) {
	builder := SnapshotBuilder{Repo: workspace}
	root, resolveErr := builder.ResolveRepositoryRoot(ctx)
	if resolveErr == nil {
		return root, nil
	}
	if !reviewRootResolutionReportsNoRepository(resolveErr) {
		return "", resolveErr
	}

	workspace, pathErr := canonicalRepositoryPath(workspace)
	if pathErr != nil {
		return "", pathErr
	}
	info, statErr := os.Stat(workspace)
	if statErr != nil || !info.IsDir() {
		return "", resolveErr
	}
	metadataPresent, metadataErr := reviewWorkspaceHasGitMetadata(workspace)
	if metadataErr != nil || metadataPresent {
		return "", resolveErr
	}

	if _, probeErr := runGitIsolated(ctx, workspace, nil, nil, "rev-parse", "--is-inside-work-tree"); !reviewIsolatedProbeReportsNoRepository(probeErr) {
		return "", resolveErr
	}
	if _, initErr := runGitIsolated(ctx, workspace, nil, nil, "init", "--quiet"); initErr != nil {
		return "", initErr
	}
	return (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(ctx)
}

// ReviewRootResolutionReportsNoRepository reports the bounded Git discovery
// failure for a cwd outside any repository. It lets higher-level command
// surfaces decide whether repository identity was truly required instead of
// matching localized Git stderr text.
func ReviewRootResolutionReportsNoRepository(err error) bool {
	return reviewRootResolutionReportsNoRepository(err)
}

func reviewRootResolutionReportsNoRepository(err error) bool {
	return reviewGitCommandExited128(err, "rev-parse", "--show-toplevel")
}

func reviewIsolatedProbeReportsNoRepository(err error) bool {
	return reviewGitCommandExited128(err, "rev-parse", "--is-inside-work-tree")
}

func reviewGitCommandExited128(err error, args ...string) bool {
	var commandErr *GitCommandError
	if !errors.As(err, &commandErr) || commandErr.ExitCode != 128 || len(commandErr.Args) != len(args) {
		return false
	}
	for index, arg := range args {
		if commandErr.Args[index] != arg {
			return false
		}
	}
	return true
}

// reviewWorkspaceHasGitMetadata refuses initialization whenever a .git entry
// already exists in the requested directory or one of its parents. A valid
// containing worktree returned before this scan; reaching it here means the
// existing metadata is unusable and must be left untouched.
func reviewWorkspaceHasGitMetadata(workspace string) (bool, error) {
	for directory := workspace; ; directory = filepath.Dir(directory) {
		_, err := os.Lstat(filepath.Join(directory, ".git"))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return false, nil
		}
	}
}
