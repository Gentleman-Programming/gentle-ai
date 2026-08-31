package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
)

// baseAdvanceIdentity is the fixed identity of the throwaway commits that
// wrap a tree for merge-tree. Fixed dates keep the wrapper content-addressed:
// the same base advance always mints the same object instead of new garbage.
var baseAdvanceIdentity = []string{
	"GIT_AUTHOR_NAME=gentle-ai", "GIT_AUTHOR_EMAIL=gentle-ai@localhost", "GIT_AUTHOR_DATE=@0 +0000",
	"GIT_COMMITTER_NAME=gentle-ai", "GIT_COMMITTER_EMAIL=gentle-ai@localhost", "GIT_COMMITTER_DATE=@0 +0000",
}

// HeadCommit resolves HEAD to its full commit ID. An unborn HEAD resolves to
// the empty string rather than an error.
func (builder SnapshotBuilder) HeadCommit(ctx context.Context) (string, error) {
	output, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		if gitExitCode(err) == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// AuthoredChangedLines counts the lines the candidate changed against the
// snapshot's base tree, excluding a base advance that entered HEAD after
// sinceCommit (#2536). A base advance is what another branch contributed:
// commits that entered HEAD since sinceCommit and are also reachable from a
// branch other than the one being worked on, whether they arrived by merge,
// fast-forward, or rebase. Each such contribution is re-applied onto the base
// tree as a three-way merge, so the diff that remains is what the candidate
// authored on top of the advanced base. A path the merge cannot resolve keeps
// its charge against the original base tree. An empty sinceCommit charges the
// plain base-to-candidate diff.
func (builder SnapshotBuilder) AuthoredChangedLines(ctx context.Context, snapshot Snapshot, sinceCommit string) (int, error) {
	if sinceCommit == "" {
		return builder.ChangedLines(ctx, snapshot)
	}
	tips, err := builder.baseAdvanceTips(ctx, sinceCommit)
	if err != nil {
		return 0, err
	}
	base := snapshot.BaseTree
	conflicted := map[string]struct{}{}
	for _, tip := range tips {
		advanced, unresolved, err := builder.advanceBase(ctx, base, sinceCommit, tip)
		if err != nil {
			return 0, err
		}
		base = advanced
		for _, path := range unresolved {
			conflicted[path] = struct{}{}
		}
	}
	if base == snapshot.BaseTree {
		return builder.ChangedLines(ctx, snapshot)
	}
	paths, err := builder.changedPaths(ctx, base, snapshot.CandidateTree)
	if err != nil {
		return 0, err
	}
	stats, err := builder.DiffStats(ctx, Snapshot{BaseTree: base, CandidateTree: snapshot.CandidateTree, Paths: paths})
	if err != nil {
		return 0, err
	}
	if len(conflicted) != 0 {
		beginStats, err := builder.DiffStats(ctx, snapshot)
		if err != nil {
			return 0, err
		}
		merged := make([]DiffStat, 0, len(stats)+len(conflicted))
		for _, stat := range stats {
			if _, unresolved := conflicted[stat.Path]; !unresolved {
				merged = append(merged, stat)
			}
		}
		for _, stat := range beginStats {
			if _, unresolved := conflicted[stat.Path]; unresolved {
				merged = append(merged, stat)
			}
		}
		stats = merged
	}
	return CountChangedLines(stats)
}

// baseAdvanceTips finds, for every branch other than the one HEAD is on (and
// its remote counterparts, which carry the same authored work once pushed),
// the newest commit of that branch inside HEAD, and keeps the independent
// ones that entered HEAD after sinceCommit.
func (builder SnapshotBuilder) baseAdvanceTips(ctx context.Context, sinceCommit string) ([]string, error) {
	enteredOutput, err := runGit(ctx, builder.Repo, nil, nil, "rev-list", sinceCommit+"..HEAD")
	if err != nil {
		return nil, err
	}
	entered := strings.Fields(string(enteredOutput))
	if len(entered) == 0 {
		return nil, nil
	}
	branchOutput, _ := runGit(ctx, builder.Repo, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch := strings.TrimSpace(string(branchOutput))
	refsOutput, err := runGit(ctx, builder.Repo, nil, nil, "for-each-ref", "--format=%(refname)", "--no-merged="+sinceCommit, "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	tips := make([]string, 0, 4)
	for _, ref := range strings.Fields(string(refsOutput)) {
		if branch != "" && (ref == "refs/heads/"+branch || strings.HasPrefix(ref, "refs/remotes/") && strings.HasSuffix(ref, "/"+branch)) {
			continue
		}
		output, err := runGit(ctx, builder.Repo, nil, nil, "merge-base", "HEAD", ref)
		if err != nil {
			if gitExitCode(err) == 1 {
				continue
			}
			return nil, err
		}
		tip := strings.TrimSpace(string(output))
		if slices.Contains(entered, tip) && !slices.Contains(tips, tip) {
			tips = append(tips, tip)
		}
	}
	if len(tips) < 2 {
		return tips, nil
	}
	output, err := runGit(ctx, builder.Repo, nil, nil, append([]string{"merge-base", "--independent"}, tips...)...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

// advanceBase re-applies one base advance onto base: the changes from the
// merge base of sinceCommit and tip up to tip, merged three-way onto base. It
// returns the advanced tree and the paths the merge left unresolved. Unrelated
// histories have nothing to re-apply and return base unchanged.
func (builder SnapshotBuilder) advanceBase(ctx context.Context, base, sinceCommit, tip string) (string, []string, error) {
	mergeBase, err := runGit(ctx, builder.Repo, nil, nil, "merge-base", sinceCommit, tip)
	if err != nil {
		if gitExitCode(err) == 1 {
			return base, nil, nil
		}
		return "", nil, err
	}
	// merge-tree merges commits, so the base tree travels inside a wrapper
	// commit whose one parent is the merge base; that parent is what makes
	// merge-tree find the same three-way base a live merge would.
	wrapper, err := runGit(ctx, builder.Repo, baseAdvanceIdentity, nil, "commit-tree", base, "-p", strings.TrimSpace(string(mergeBase)), "-m", "gentle-ai attempt base advance")
	if err != nil {
		return "", nil, err
	}
	output, err := runGit(ctx, builder.Repo, nil, nil, "merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", strings.TrimSpace(string(wrapper)), tip)
	if err != nil {
		var commandErr *GitCommandError
		if !errors.As(err, &commandErr) || commandErr.ExitCode != 1 {
			return "", nil, err
		}
		output = []byte(commandErr.Stdout)
	}
	parts := bytes.Split(output, []byte{0})
	tree := strings.TrimSpace(string(parts[0]))
	unresolved := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		if len(part) == 0 {
			continue
		}
		logicalPath, err := normalizeLogicalPath(string(part))
		if err != nil {
			return "", nil, err
		}
		unresolved = append(unresolved, logicalPath)
	}
	return tree, unresolved, nil
}

func gitExitCode(err error) int {
	var commandErr *GitCommandError
	if errors.As(err, &commandErr) {
		return commandErr.ExitCode
	}
	return -1
}
