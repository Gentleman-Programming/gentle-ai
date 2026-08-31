package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"strings"
)

// baseAdvanceIdentity is the fixed identity of the wrapper commits merge-tree
// needs; fixed dates make the same advance mint the same object every time.
var baseAdvanceIdentity = []string{
	"GIT_AUTHOR_NAME=gentle-ai", "GIT_AUTHOR_EMAIL=gentle-ai@localhost", "GIT_AUTHOR_DATE=@0 +0000",
	"GIT_COMMITTER_NAME=gentle-ai", "GIT_COMMITTER_EMAIL=gentle-ai@localhost", "GIT_COMMITTER_DATE=@0 +0000",
}

// defaultBranchRefs is the offline resolution order for the default branch;
// the first one that resolves is the base and nothing else ever is.
var defaultBranchRefs = []string{"refs/remotes/origin/HEAD", "refs/heads/main", "refs/heads/master"}

// HeadCommit resolves HEAD to its full commit ID; an unborn HEAD resolves to "".
func (builder SnapshotBuilder) HeadCommit(ctx context.Context) (string, error) {
	output, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	var commandErr *GitCommandError
	if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
		return "", nil
	}
	return strings.TrimSpace(string(output)), err
}

// AuthoredChangedLines counts the lines the candidate changed against the
// snapshot's base tree, excluding the base advance that entered HEAD after
// sinceCommit (#2536): the default-branch commits HEAD gained since then, by
// merge, fast-forward, or rebase. The advance is re-applied onto the base
// tree as a three-way merge, so the remaining diff is what the candidate
// authored on top of it; a path the merge cannot resolve keeps its charge
// against the original base tree. Only the default branch is a base, so no
// other ref, including one at the attempt's own commits, reduces the charge.
// The exclusion is best effort: an empty sinceCommit, no resolvable default
// branch, or any failure while measuring (an object store refusing the
// wrapper objects included) charges the plain base-to-candidate diff.
func (builder SnapshotBuilder) AuthoredChangedLines(ctx context.Context, snapshot Snapshot, sinceCommit string) (int, error) {
	if sinceCommit != "" {
		if lines, measured := builder.baseAdvancedChangedLines(ctx, snapshot, sinceCommit); measured {
			return lines, nil
		}
	}
	return builder.ChangedLines(ctx, snapshot)
}

func (builder SnapshotBuilder) baseAdvancedChangedLines(ctx context.Context, snapshot Snapshot, sinceCommit string) (int, bool) {
	tip := builder.defaultBranchAdvanceTip(ctx, sinceCommit)
	if tip == "" {
		return 0, false
	}
	base, conflicted, err := builder.advanceBase(ctx, snapshot.BaseTree, sinceCommit, tip)
	if err != nil {
		return 0, false
	}
	paths, err := builder.changedPaths(ctx, base, snapshot.CandidateTree)
	if err != nil {
		return 0, false
	}
	stats, err := builder.DiffStats(ctx, Snapshot{BaseTree: base, CandidateTree: snapshot.CandidateTree, Paths: paths})
	if err != nil {
		return 0, false
	}
	if len(conflicted) != 0 {
		beginStats, err := builder.DiffStats(ctx, snapshot)
		if err != nil {
			return 0, false
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
	lines, err := CountChangedLines(stats)
	return lines, err == nil
}

// defaultBranchAdvanceTip is the newest default-branch commit inside HEAD,
// provided it entered HEAD after sinceCommit; otherwise "". When HEAD is the
// default branch itself there is no base to advance from: its commits are the
// attempt's own.
func (builder SnapshotBuilder) defaultBranchAdvanceTip(ctx context.Context, sinceCommit string) string {
	headOutput, _ := runGit(ctx, builder.Repo, nil, nil, "symbolic-ref", "--quiet", "HEAD")
	headRef := strings.TrimSpace(string(headOutput))
	for _, ref := range defaultBranchRefs {
		if _, err := runGit(ctx, builder.Repo, nil, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
			continue
		}
		if target, err := runGit(ctx, builder.Repo, nil, nil, "symbolic-ref", "--quiet", ref); err == nil {
			ref = strings.TrimSpace(string(target))
		}
		if headRef != "" && strings.TrimPrefix(headRef, "refs/heads/") == ref[strings.LastIndex(ref, "/")+1:] {
			return ""
		}
		output, err := runGit(ctx, builder.Repo, nil, nil, "merge-base", "HEAD", ref)
		if err != nil {
			return ""
		}
		tip := strings.TrimSpace(string(output))
		if _, err := runGit(ctx, builder.Repo, nil, nil, "merge-base", "--is-ancestor", tip, sinceCommit); err == nil {
			return ""
		}
		return tip
	}
	return ""
}

// advanceBase re-applies the base advance onto base: the changes from the
// merge base of sinceCommit and tip up to tip, merged three-way onto base. It
// returns the advanced tree and the paths the merge left unresolved.
func (builder SnapshotBuilder) advanceBase(ctx context.Context, base, sinceCommit, tip string) (string, map[string]struct{}, error) {
	mergeBase, err := runGit(ctx, builder.Repo, nil, nil, "merge-base", sinceCommit, tip)
	if err != nil {
		return "", nil, err
	}
	// merge-tree merges commits: the base tree travels inside a wrapper commit
	// whose parent is the merge base, so merge-tree finds the three-way base.
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
	unresolved := make(map[string]struct{}, len(parts)-1)
	for _, part := range parts[1:] {
		if len(part) == 0 {
			continue
		}
		logicalPath, err := normalizeLogicalPath(string(part))
		if err != nil {
			return "", nil, err
		}
		unresolved[logicalPath] = struct{}{}
	}
	return strings.TrimSpace(string(parts[0])), unresolved, nil
}
