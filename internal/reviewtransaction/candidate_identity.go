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
