// Package reviewtransaction — shadow candidate identity resolver (Wave 1,
// Slice 2). This file is part of the read-only shadow of the target RDD
// relation model (docs/architecture/rdd-root-simplification-design.md). It
// reuses live production primitives (OpenRepositoryIdentityLease,
// SnapshotBuilder) rather than restating their logic, and never mutates
// authority state, a Store, or a CompactState. See
// shadow_readonly_guard_test.go for the AST guard that enforces this.
//
// CandidateIdentity is the only symbol this slice exports (design decision
// 1); everything else here stays unexported until the relation algebra
// (Slice 3) and observer (Slice 5) need it.
package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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

// shadowSelectorKind names one of the four Wave 1 selector variants. It
// normalizes into the same CandidateIdentity shape without a distinct
// lifecycle or state machine per kind (Requirement: Selector Normalization).
type shadowSelectorKind string

const (
	shadowSelectorWorkspace        shadowSelectorKind = "workspace"
	shadowSelectorStaged           shadowSelectorKind = "staged"
	shadowSelectorCommittedRange   shadowSelectorKind = "committed-range"
	shadowSelectorWorkspaceOverlay shadowSelectorKind = "workspace-overlay"
)

// shadowPolicyHashSource returns the caller's known policy hash, if any.
// known=false (or a nil source) means "no live policy hash available" —
// shadowCandidateIdentity reports "unknown" rather than fabricating one.
type shadowPolicyHashSource func() (hash string, known bool)

// shadowSelector is the read-only input shadowCandidateIdentity normalizes.
// Only the fields relevant to Kind are meaningful; see the design's selector
// -> live-target mapping table (design.md, "Selector -> canonical tuple
// mapping").
type shadowSelector struct {
	Kind shadowSelectorKind
	Repo string

	// BaseRef targets: staged base-diff, committed-range base-diff, and
	// workspace-overlay (all map onto TargetBaseDiff / TargetBaseWorkspaceOverlay's base_ref).
	BaseRef string
	// Revision targets committed-range's exact-revision form: one full
	// commit ID or an "A..B" range (TargetExactRevision).
	Revision string
	// LedgerIDs targets staged fix-diff (TargetFixDiff); it is only
	// meaningful together with BaseRef.
	LedgerIDs []string
	// IntendedUntracked is forwarded to TargetCurrentChanges /
	// TargetBaseWorkspaceOverlay exactly as SnapshotBuilder.Build requires.
	IntendedUntracked []string

	PolicyHashSource shadowPolicyHashSource
}

// shadowIdentityFailure is the typed, evidence-carrying failure
// shadowCandidateIdentity returns instead of guessing a candidate identity.
// RepositoryID is populated whenever repository identity resolution itself
// succeeded before the failure, so callers can distinguish "we don't know
// which repository" from "we know the repository but not this candidate".
type shadowIdentityFailure struct {
	Kind         shadowSelectorKind
	Repo         string
	RepositoryID string
	Reason       string
}

func (failure *shadowIdentityFailure) Error() string {
	return fmt.Sprintf("shadow candidate identity unresolvable for %s selector: %s", failure.Kind, failure.Reason)
}

// shadowIdentityAmbiguity is returned when a selector legitimately matches
// more than one equally applicable live target. shadowCandidateIdentity
// never picks the most recent or most likely candidate in this case — it
// returns the complete set (Requirement: Deterministic Ambiguity and
// Failure Reporting).
type shadowIdentityAmbiguity struct {
	Kind         shadowSelectorKind
	RepositoryID string
	Candidates   []CandidateIdentity
	Reason       string
}

func (ambiguity *shadowIdentityAmbiguity) Error() string {
	return fmt.Sprintf(
		"shadow candidate identity ambiguous for %s selector: %d equally applicable candidates (%s)",
		ambiguity.Kind, len(ambiguity.Candidates), ambiguity.Reason,
	)
}

// shadowCandidateIdentity resolves one selector into a CandidateIdentity. It
// performs no mutation: repository identity comes only from
// OpenRepositoryIdentityLease, and the candidate/base trees come only from
// SnapshotBuilder.Build, both already read-only. Repeated resolution of the
// same selector is idempotent.
func shadowCandidateIdentity(ctx context.Context, selector shadowSelector) (CandidateIdentity, error) {
	if err := ctx.Err(); err != nil {
		return CandidateIdentity{}, err
	}

	lease, err := OpenRepositoryIdentityLease(ctx, selector.Repo)
	if err != nil {
		return CandidateIdentity{}, &shadowIdentityFailure{
			Kind: selector.Kind, Repo: selector.Repo,
			Reason: fmt.Sprintf("resolve repository identity: %v", err),
		}
	}
	repositoryID := lease.Identity().RepositoryRef

	if reason := shadowSelectorAmbiguityReason(selector); reason != "" {
		ambiguity := &shadowIdentityAmbiguity{Kind: selector.Kind, RepositoryID: repositoryID, Reason: reason}
		for _, target := range shadowAmbiguousTargets(selector) {
			candidate, buildErr := shadowResolveTarget(ctx, selector.Repo, repositoryID, target, selector.PolicyHashSource)
			if buildErr != nil {
				return CandidateIdentity{}, &shadowIdentityFailure{
					Kind: selector.Kind, Repo: selector.Repo, RepositoryID: repositoryID,
					Reason: fmt.Sprintf("gather ambiguous candidate evidence: %v", buildErr),
				}
			}
			ambiguity.Candidates = append(ambiguity.Candidates, candidate)
		}
		return CandidateIdentity{}, ambiguity
	}

	target, err := shadowSelectorTarget(selector)
	if err != nil {
		return CandidateIdentity{}, &shadowIdentityFailure{
			Kind: selector.Kind, Repo: selector.Repo, RepositoryID: repositoryID, Reason: err.Error(),
		}
	}
	candidate, err := shadowResolveTarget(ctx, selector.Repo, repositoryID, target, selector.PolicyHashSource)
	if err != nil {
		return CandidateIdentity{}, &shadowIdentityFailure{
			Kind: selector.Kind, Repo: selector.Repo, RepositoryID: repositoryID, Reason: err.Error(),
		}
	}
	return candidate, nil
}

// shadowSelectorAmbiguityReason reports why a selector matches more than one
// equally applicable live target, or "" when it is unambiguous. A selector
// is ambiguous exactly when the caller populated more than one of the
// mutually exclusive fields that would each independently select a live
// target — the resolver refuses to silently prioritize one over another.
func shadowSelectorAmbiguityReason(selector shadowSelector) string {
	switch selector.Kind {
	case shadowSelectorStaged:
		if strings.TrimSpace(selector.BaseRef) != "" && len(selector.LedgerIDs) > 0 {
			return "staged selector supplied both base_ref (base-diff) and ledger_ids (fix-diff): two equally applicable live targets"
		}
	case shadowSelectorCommittedRange:
		if strings.TrimSpace(selector.Revision) != "" && strings.TrimSpace(selector.BaseRef) != "" {
			return "committed-range selector supplied both revision (exact-revision) and base_ref (base-diff): two equally applicable live targets"
		}
	}
	return ""
}

// shadowAmbiguousTargets enumerates the equally applicable live targets for
// an ambiguous selector, in the same order shadowSelectorAmbiguityReason
// reported them.
func shadowAmbiguousTargets(selector shadowSelector) []Target {
	switch selector.Kind {
	case shadowSelectorStaged:
		return []Target{
			{Kind: TargetBaseDiff, Projection: ProjectionStaged, BaseRef: selector.BaseRef},
			{
				Kind: TargetFixDiff, Projection: ProjectionStaged, BaseRef: selector.BaseRef,
				LedgerIDs: selector.LedgerIDs, IntendedUntracked: nonNilShadowPaths(selector.IntendedUntracked),
			},
		}
	case shadowSelectorCommittedRange:
		return []Target{
			{Kind: TargetExactRevision, Revision: selector.Revision},
			{Kind: TargetBaseDiff, BaseRef: selector.BaseRef},
		}
	default:
		return nil
	}
}

// shadowSelectorTarget maps one unambiguous selector onto the live Target it
// describes, per the design's selector -> canonical tuple mapping table.
func shadowSelectorTarget(selector shadowSelector) (Target, error) {
	switch selector.Kind {
	case shadowSelectorWorkspace:
		return Target{
			Kind: TargetCurrentChanges, Projection: ProjectionWorkspace,
			IntendedUntracked: nonNilShadowPaths(selector.IntendedUntracked),
		}, nil

	case shadowSelectorStaged:
		hasBaseRef := strings.TrimSpace(selector.BaseRef) != ""
		hasLedger := len(selector.LedgerIDs) > 0
		switch {
		case hasBaseRef && hasLedger:
			// shadowSelectorAmbiguityReason already routes this shape through
			// the ambiguity path before this function is ever called.
			return Target{}, errors.New("staged selector is ambiguous") // refusal:by-design world-action: unreachable in practice -- shadowSelectorAmbiguityReason diverts this exact input shape to the ambiguity path before shadowSelectorTarget runs; reaching here is a caller-order invariant break inside the shadow-only observation path, swallowed by ObserveShadowRelation and never a live block
		case hasBaseRef:
			return Target{Kind: TargetBaseDiff, Projection: ProjectionStaged, BaseRef: selector.BaseRef}, nil
		case hasLedger:
			return Target{}, errors.New("staged fix-diff selector requires base_ref") // refusal:by-design world-action: defensive selector-construction invariant on the shadow-only observation path; the error is swallowed by ObserveShadowRelation's recover and surfaces only as an advisory stderr diagnostic, never a live gate block
		default:
			return Target{
				Kind: TargetCurrentChanges, Projection: ProjectionStaged,
				IntendedUntracked: nonNilShadowPaths(selector.IntendedUntracked),
			}, nil
		}

	case shadowSelectorCommittedRange:
		hasRevision := strings.TrimSpace(selector.Revision) != ""
		hasBaseRef := strings.TrimSpace(selector.BaseRef) != ""
		switch {
		case hasRevision && hasBaseRef:
			return Target{}, errors.New("committed-range selector is ambiguous") // refusal:by-design world-action: unreachable in practice -- shadowSelectorAmbiguityReason diverts this exact input shape to the ambiguity path before shadowSelectorTarget runs; reaching here is a caller-order invariant break inside the shadow-only observation path, swallowed by ObserveShadowRelation and never a live block
		case hasRevision:
			return Target{Kind: TargetExactRevision, Revision: selector.Revision}, nil
		case hasBaseRef:
			return Target{Kind: TargetBaseDiff, BaseRef: selector.BaseRef}, nil
		default:
			return Target{}, errors.New("committed-range selector requires revision or base_ref") // refusal:by-design world-action: defensive selector-construction invariant on the shadow-only observation path; the error is swallowed by ObserveShadowRelation's recover and surfaces only as an advisory stderr diagnostic, never a live gate block
		}

	case shadowSelectorWorkspaceOverlay:
		if strings.TrimSpace(selector.BaseRef) == "" {
			return Target{}, errors.New("workspace-overlay selector requires base_ref") // refusal:by-design world-action: defensive selector-construction invariant on the shadow-only observation path; the error is swallowed by ObserveShadowRelation's recover and surfaces only as an advisory stderr diagnostic, never a live gate block
		}
		return Target{
			Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionWorkspace, BaseRef: selector.BaseRef,
			IntendedUntracked: nonNilShadowPaths(selector.IntendedUntracked),
		}, nil

	default:
		return Target{}, fmt.Errorf("unsupported shadow selector kind %q", selector.Kind) // refusal:by-design world-action: shadowSelectorKind is a closed 4-value enum declared in this file; an unrecognized value can only reach here through a future caller bug, and the error is swallowed by ObserveShadowRelation, never blocking a live decision
	}
}

// shadowResolveTarget builds one CandidateIdentity for an already-resolved
// repository identity and live Target. It is the single place that touches
// SnapshotBuilder, so both the unambiguous path and the ambiguity-evidence
// path share identical resolution semantics.
func shadowResolveTarget(
	ctx context.Context,
	repo, repositoryID string,
	target Target,
	policyHashSource shadowPolicyHashSource,
) (CandidateIdentity, error) {
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, target)
	if err != nil {
		return CandidateIdentity{}, err
	}
	modesDigest, err := shadowChangedPathsModesDigest(ctx, repo, snapshot.Paths, snapshot.BaseTree, snapshot.CandidateTree)
	if err != nil {
		return CandidateIdentity{}, err
	}
	return CandidateIdentity{
		RepositoryID:            repositoryID,
		BaseTree:                snapshot.BaseTree,
		CandidateTree:           snapshot.CandidateTree,
		ChangedPathsModesDigest: modesDigest,
		PolicyHash:              shadowPolicyHash(policyHashSource),
	}, nil
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
			return "", fmt.Errorf("shadow candidate identity: path %q missing from raw tree diff", path) // refusal:by-design world-action: a Git raw-diff parsing data-consistency invariant on the shadow-only path; the error is swallowed by ObserveShadowRelation and surfaces only as an advisory stderr diagnostic, never a live gate block
		}
		writeLengthPrefixed(hash, []byte(path))
		writeLengthPrefixed(hash, []byte(modes.status))
		writeLengthPrefixed(hash, []byte(modes.oldMode))
		writeLengthPrefixed(hash, []byte(modes.newMode))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// shadowPolicyHash reports the caller's known policy hash, or "unknown" —
// never a fabricated value — when none is available.
func shadowPolicyHash(source shadowPolicyHashSource) string {
	if source == nil {
		return "unknown"
	}
	hash, known := source()
	if !known || strings.TrimSpace(hash) == "" {
		return "unknown"
	}
	return hash
}

// nonNilShadowPaths returns paths unchanged, or an empty non-nil slice when
// paths is nil. SnapshotBuilder.Build requires IntendedUntracked to be
// non-nil for TargetCurrentChanges, TargetFixDiff, and
// TargetBaseWorkspaceOverlay even when there is nothing to list.
func nonNilShadowPaths(paths []string) []string {
	if paths == nil {
		return []string{}
	}
	return paths
}
