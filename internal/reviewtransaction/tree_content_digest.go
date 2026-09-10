package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TreeContentDigest is a stable identity for a tree's content with named paths
// excluded from it.
//
// It exists because a Git tree OID cannot identify the content an artifact
// stored inside that same tree describes. Writing the artifact changes the
// tree, so an artifact naming its own tree names a hash fixed point: the value
// it would have to contain depends on containing it. Excluding the artifact's
// own path makes the identity stable across writing it, and lets a later
// reader recompute the same value from the tree the artifact now lives in.
//
// The digest covers each remaining entry's mode, object, and path, so a
// content change, a mode change, an addition, or a removal all move it.
func TreeContentDigest(ctx context.Context, repo, tree string, excluded []string) (string, error) {
	if !validGitTree(tree) {
		return "", errors.New("tree content digest requires a valid tree identity") // refusal:by-design world-action: a malformed tree identity names no content to digest
	}
	skip, err := canonicalPaths(excluded)
	if err != nil {
		return "", err
	}
	entries, err := listTreeEntries(ctx, repo, tree)
	if err != nil {
		return "", err
	}
	records := make([]string, 0, len(entries))
	for logicalPath, record := range entries {
		if stringIndex(skip, logicalPath) >= 0 {
			continue
		}
		fields := strings.Fields(strings.SplitN(strings.TrimRight(string(record), "\x00"), "\t", 2)[0])
		if len(fields) != 3 {
			return "", errors.New("tree entry is malformed") // refusal:by-design world-action: malformed Git tree metadata cannot prove content identity, and no operator command repairs the object store from here
		}
		// Tree records are skipped because their content is already implied by
		// the paths beneath them; including them would let an empty
		// intermediate directory move the digest with no content change.
		//
		// Gitlinks are NOT skipped. A submodule pointer is content this tree
		// owns: moving it changes what the candidate is, and a digest that
		// ignored it would keep attesting a candidate that had changed.
		if fields[1] == "tree" {
			continue
		}
		// The path is length-prefixed rather than delimited. Git permits a
		// newline in a path, and a plain separator would let two different
		// candidates render identical records.
		records = append(records, fmt.Sprintf("%s %s %d:%s", fields[0], fields[2], len(logicalPath), logicalPath))
	}
	sort.Strings(records)
	sum := sha256.Sum256([]byte(strings.Join(records, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
