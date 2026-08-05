// Package reviewtransaction — candidate identity resolver (Wave 1 Slice 2;
// promoted out of the shadow gate in Wave 3 Slice 1, design decision 2).
// This file used to be shadow_identity.go, part of the read-only shadow of
// the target RDD relation model
// (docs/architecture/rdd-root-simplification-design.md). Promotion means it
// now serves the live ReviewCore (Wave 3 Slice 3+) directly — the Wave 1
// shadow observer that used to independently resolve candidate identity
// from a selector (shadowSelector/shadowCandidateIdentity and everything
// they called) retired in Wave 7 S2a, along with that whole resolver
// subsystem, since FreezeCandidateIdentity (review_core.go) never used it —
// it already holds a Snapshot and calls only shadowChangedPathsModesDigest
// below. It reuses live production primitives (OpenRepositoryIdentityLease,
// SnapshotBuilder) rather than restating their logic, and must still never
// mutate authority state, a Store, or a CompactState — see
// candidate_readonly_guard_test.go for the AST guard that enforces this.
//
// CandidateIdentity is the only symbol this slice exports (design decision
// 1).
package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CandidateIdentity is the canonical shadow candidate identity computed from
// any of the four Wave 1 selector variants (workspace, staged,
// committed-range, workspace-overlay). It contains exactly the five fields
// the design specifies — repository_id, base_tree, candidate_tree,
// changed_paths_modes_digest, policy_hash — and nothing selector-specific.
type CandidateIdentity struct {
	// RepositoryID is the exact RepositoryIdentity.RepositoryRef resolved by
	// OpenRepositoryIdentityLease. It is never independently derived.
	RepositoryID string
	// BaseTree and CandidateTree are the resolved Git tree object IDs for
	// the candidate's base and candidate sides.
	BaseTree      string
	CandidateTree string
	// ChangedPathsModesDigest covers both the changed paths and their Git
	// file modes (old and new), so a mode-only change is a measurable
	// divergence class distinct from a pure path-set change.
	ChangedPathsModesDigest string
	// PolicyHash is CompactState.PolicyHash / Receipt.PolicyHash for the
	// resolved candidate. It is "unknown" — never fabricated — whenever the
	// caller has no live policy hash to supply.
	PolicyHash string
}

// shadowChangedPathsModesDigest hashes the changed paths together with their
// Git file modes (old and new). digestPaths (snapshot.go) already covers the
// paths-only digest that Snapshot.PathsDigest carries; this digest records
// modes too, so a mode-only drift between two otherwise-identical
// resolutions is a measurable divergence class instead of silently
// collapsing into the paths-only digest.
func shadowChangedPathsModesDigest(ctx context.Context, repo string, paths []string, baseTree, candidateTree string) (string, error) {
	raw, err := runGitIsolated(ctx, repo, nil, nil,
		"diff", "--raw", "-z", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none",
		baseTree, candidateTree, "--",
	)
	if err != nil {
		return "", err
	}
	modesByPath, err := parseRawDiffModes(raw)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.paths-modes/v1\x00"))
	for _, path := range paths {
		modes, ok := modesByPath[path]
		if !ok {
			return "", fmt.Errorf("candidate identity: path %q missing from raw tree diff", path) // refusal:by-design world-action: a Git raw-diff parsing data-consistency invariant; FreezeCandidateIdentity's own caller (ReviewCore.start) surfaces this as a real start refusal — there is no operator command that repairs a raw-diff parsing mismatch, only re-deriving the snapshot from a consistent tree pair
		}
		writeLengthPrefixed(hash, []byte(path))
		writeLengthPrefixed(hash, []byte(modes.status))
		writeLengthPrefixed(hash, []byte(modes.oldMode))
		writeLengthPrefixed(hash, []byte(modes.newMode))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
