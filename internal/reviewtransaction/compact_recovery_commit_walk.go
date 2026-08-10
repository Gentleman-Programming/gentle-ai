package reviewtransaction

// compactLinearCommitProof, linearCommitsBetween, and pathUnion are generic
// Git commit-walk / path-set utilities relocated here from the now-deleted
// compact_chain.go (Wave 5 Slice 5, pre-PR chain composition deletion):
// unlike EvaluateCompactPrePRChain and its own composition-authorization
// machinery (deleted along with that file), compact_recovery_binding.go
// genuinely still needs these — recovery-binding compatibility derivation is
// unrelated to pre-PR receipt composition, it only happened to share this
// code because both walk a linear commit range and union path sets. Names
// dropped their "PrePRChain" prefix accordingly; bodies are unchanged.

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type compactLinearCommitProof struct {
	Commit     string `json:"commit"`
	Parent     string `json:"parent"`
	ParentTree string `json:"parent_tree"`
	Tree       string `json:"tree"`
}

func linearCommitsBetween(ctx context.Context, repo, baseCommit, headCommit string) ([]compactLinearCommitProof, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--reverse", "--parents", baseCommit+".."+headCommit)
	if err != nil {
		return nil, fmt.Errorf("inspect compact receipt publication commits: %w", err)
	}
	previousCommit := baseCommit
	previousTree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, baseCommit)
	if err != nil {
		return nil, err
	}
	commits := []compactLinearCommitProof{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || fields[1] != previousCommit {
			return nil, errors.New("compact receipt composition requires one exact linear publication ancestry")
		}
		tree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, fields[0])
		if err != nil {
			return nil, err
		}
		commits = append(commits, compactLinearCommitProof{Commit: fields[0], Parent: previousCommit, ParentTree: previousTree, Tree: tree})
		previousCommit, previousTree = fields[0], tree
	}
	if len(commits) == 0 || previousCommit != headCommit {
		return nil, errors.New("compact receipt publication history does not reach live HEAD")
	}
	return commits, nil
}

func pathUnion(groups ...[]string) ([]string, error) {
	unique := map[string]struct{}{}
	for _, group := range groups {
		for _, path := range group {
			unique[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}
	return canonicalPaths(paths)
}
