package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrTreeArtifactMissing = errors.New("tree artifact is missing") // refusal:by-design world-action: the requested artifact must exist in the immutable candidate tree before it can be attested

// ReadTreeBlob returns one canonical regular-file blob from an immutable Git
// tree. Callers supply the byte ceiling because the same tree primitive serves
// several bounded artifact domains.
func ReadTreeBlob(ctx context.Context, repo, tree, logicalPath string, limit int) ([]byte, error) {
	if err := validateTreeBlobAddress(tree, logicalPath, limit); err != nil {
		return nil, err
	}
	entry, err := treeBlobEntry(ctx, repo, tree, logicalPath)
	if err != nil {
		return nil, err
	}
	if entry.mode != "100644" {
		return nil, errors.New("tree artifact is not a canonical regular file") // refusal:by-design world-action: a report attestation requires a regular-file blob in the immutable candidate tree
	}
	return runGitLimited(ctx, repo, nil, nil, limit, "show", tree+":"+logicalPath)
}

// RestoreTreeBlob reconstructs currentTree after replacing logicalPath's blob
// with its exact blob from sourceTree. It neither reads the working tree nor
// modifies the real index.
func RestoreTreeBlob(ctx context.Context, repo, currentTree, sourceTree, logicalPath string) (string, error) {
	if err := validateTreeBlobAddress(currentTree, logicalPath, 1); err != nil || !validGitTree(sourceTree) {
		return "", errors.New("invalid tree blob restoration address") // refusal:by-design world-action: malformed immutable tree identities cannot be safely reconstructed
	}
	current, err := treeBlobEntry(ctx, repo, currentTree, logicalPath)
	if err != nil {
		return "", err
	}
	source, err := treeBlobEntry(ctx, repo, sourceTree, logicalPath)
	if err != nil {
		return "", err
	}
	if current.mode != "100644" || source.mode != "100644" || current.mode != source.mode {
		return "", errors.New("tree artifact is not an unchanged canonical regular file") // refusal:by-design world-action: a report-only exception cannot restore a changed mode or non-regular artifact
	}

	gitDir, err := resolveGitDirectory(ctx, repo, "--git-dir")
	if err != nil {
		return "", err
	}
	index, err := os.CreateTemp(gitDir, ".gentle-ai-tree-restore-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	defer os.Remove(indexPath)
	if err := index.Close(); err != nil {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := runGit(ctx, repo, env, nil, "read-tree", currentTree); err != nil {
		return "", err
	}
	if _, err := runGit(ctx, repo, env, nil, "update-index", "--add", "--cacheinfo", source.mode+","+source.object+","+logicalPath); err != nil {
		return "", err
	}
	output, err := runGit(ctx, repo, env, nil, "write-tree")
	if err != nil {
		return "", err
	}
	rebuilt := strings.TrimSpace(string(output))
	if !validGitTree(rebuilt) {
		return "", errors.New("rebuilt tree is invalid") // refusal:by-design world-action: invalid Git output cannot prove a receipt-tree reconstruction
	}
	return rebuilt, nil
}

type treeArtifactBlob struct {
	mode   string
	object string
}

func validateTreeBlobAddress(tree, logicalPath string, limit int) error {
	if !validGitTree(tree) || limit < 1 {
		return errors.New("invalid tree blob address") // refusal:by-design world-action: malformed immutable tree identities cannot be addressed safely
	}
	normalized, err := normalizeLogicalPath(logicalPath)
	if err != nil || normalized != logicalPath || strings.ContainsRune(logicalPath, ':') {
		return errors.New("tree blob path is not canonical") // refusal:by-design world-action: only one canonical path can be safely addressed in a Git tree
	}
	return nil
}

func treeBlobEntry(ctx context.Context, repo, tree, logicalPath string) (treeArtifactBlob, error) {
	entries, err := listTreeEntries(ctx, repo, tree)
	if err != nil {
		return treeArtifactBlob{}, err
	}
	record, ok := entries[logicalPath]
	if !ok {
		return treeArtifactBlob{}, ErrTreeArtifactMissing
	}
	record = bytes.TrimSuffix(record, []byte{0})
	tab := bytes.IndexByte(record, '\t')
	if tab < 0 || string(record[tab+1:]) != logicalPath {
		return treeArtifactBlob{}, errors.New("tree artifact entry is malformed") // refusal:by-design world-action: malformed Git tree metadata cannot prove artifact identity
	}
	fields := strings.Fields(string(record[:tab]))
	if len(fields) != 3 || fields[1] != "blob" || !validGitTree(fields[2]) {
		return treeArtifactBlob{}, fmt.Errorf("tree artifact %q is not a blob", logicalPath) // refusal:by-design world-action: a non-blob tree entry cannot supply immutable report bytes
	}
	return treeArtifactBlob{mode: fields[0], object: fields[2]}, nil
}
