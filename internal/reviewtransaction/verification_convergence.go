package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
)

var ErrVerificationSubjectMutated = errors.New(
	"verification subject mutated during final verification",
)

// VerificationSubjectMutationError preserves both immutable snapshot
// identities when a functional verifier changes its own subject. Callers must
// stop instead of accepting the result or silently planning another pass.
type VerificationSubjectMutationError struct {
	Expected VerificationSubject
	Observed VerificationSubject
}

func (err *VerificationSubjectMutationError) Error() string {
	if err == nil {
		return ErrVerificationSubjectMutated.Error()
	}
	return fmt.Sprintf(
		"%v: expected %s, observed %s",
		ErrVerificationSubjectMutated,
		err.Expected.SnapshotIdentity,
		err.Observed.SnapshotIdentity,
	)
}

func (err *VerificationSubjectMutationError) Unwrap() error {
	return ErrVerificationSubjectMutated
}

// ResnapshotVerificationSubject rebuilds the exact mutable candidate after
// final functional verification and compares every Snapshot field. The
// expected snapshot supplies only frozen target inputs; no caller-provided
// equality claim is trusted.
func ResnapshotVerificationSubject(
	ctx context.Context,
	repo string,
	expected Snapshot,
) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, errors.New("verification resnapshot context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	builder := SnapshotBuilder{Repo: repo}
	if err := builder.ValidateEvidence(ctx, expected); err != nil {
		return Snapshot{}, fmt.Errorf(
			"validate expected verification snapshot: %w",
			err,
		)
	}
	expectedSubject, err := VerificationSubjectFromSnapshot(expected)
	if err != nil {
		return Snapshot{}, err
	}
	target := Target{
		Kind:              expected.Kind,
		Projection:        expected.Projection,
		IntendedUntracked: append([]string{}, expected.IntendedUntracked...),
		LedgerIDs:         append([]string{}, expected.LedgerIDs...),
	}
	switch expected.Kind {
	case TargetCurrentChanges:
	case TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetFixDiff:
		target.BaseRef = expected.BaseTree
	default:
		return Snapshot{}, fmt.Errorf(
			"unsupported mutable verification subject kind %q",
			expected.Kind,
		)
	}
	live, err := builder.BuildStoredSnapshot(ctx, target)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resnapshot verification subject: %w", err)
	}
	if SnapshotsEqualExact(expected, live) {
		return live, nil
	}
	observedSubject, subjectErr := VerificationSubjectFromSnapshot(live)
	if subjectErr != nil {
		return Snapshot{}, subjectErr
	}
	return live, &VerificationSubjectMutationError{
		Expected: expectedSubject,
		Observed: observedSubject,
	}
}
