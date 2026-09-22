package kickoff

import (
	"context"
	"fmt"
)

// AncestryChecker is the one-function port VerifyIntegrationEvidence needs
// to confirm a pr_merged commit is a real local ancestor of the branch it
// claims to have merged into. It exists so this leaf package never imports
// internal/reviewtransaction for a single boolean question: the real
// implementation is backed by
// (reviewtransaction.SnapshotBuilder).RevisionIsAncestor (phase 19),
// injected by the CLI adapter (phase 21). This package's own tests exercise
// the four evidence rules against a double instead of the real executor.
type AncestryChecker interface {
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
}

// Evidence is the integration-evidence declaration an operator attaches to
// a decision on the "integration" gate (design.md S5.2, D-13): which class
// of evidence it is, the reference that supports it, and — for pr_merged
// only — the branch that reference must be a local ancestor of. Verified is
// VerifyIntegrationEvidence's own output field; a caller never sets it.
type Evidence struct {
	Kind     EvidenceKind
	Ref      string
	BaseRef  string
	Verified bool
}

// VerifyIntegrationEvidence classifies one Evidence declaration honestly
// (D-13, T-11): Axiom can prove a pr_merged commit is a local ancestor of
// its claimed base branch, and it does, via checker; it cannot prove a
// deployment or an attestation actually happened, so both of those kinds
// are always reported Verified: false, distinguishing "declared" from
// "checked" instead of inventing a verdict Axiom cannot back up.
//
// No path through this function ever returns Verified: true without
// checker having reported the ancestry itself, successfully, first: a
// checker error propagates unchanged (wrapped, never swallowed), and a
// confirmed non-ancestor becomes a named rejection that identifies both
// revisions compared — neither path ever reaches Verified: true.
func VerifyIntegrationEvidence(ctx context.Context, ev Evidence, checker AncestryChecker) (Evidence, error) {
	if ev.Kind != EvidencePRMerged {
		ev.Verified = false
		return ev, nil
	}
	isAncestor, err := checker.IsAncestor(ctx, ev.Ref, ev.BaseRef)
	if err != nil {
		return Evidence{}, fmt.Errorf("comprobar que %q es ancestro de %q: %w", ev.Ref, ev.BaseRef, err)
	}
	if !isAncestor {
		return Evidence{}, fmt.Errorf("el commit %q no es ancestro de %q; el registro de integracion no se anexa sin evidencia confirmada", ev.Ref, ev.BaseRef)
	}
	ev.Verified = true
	return ev, nil
}
