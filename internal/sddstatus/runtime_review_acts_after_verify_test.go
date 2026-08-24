//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"path/filepath"
	"testing"
)

// Receipt-driven review acts AFTER implementation and verification, on the
// finished result. That is not an aspiration bolted on here: this package
// already implements it twice.
//
//   applyReviewOfferRouting (status.go) fires in exactly one window — verify
//   has passed and the kill switch is on — and with the switch off it "returns
//   before doing anything at all [...] which is what makes the absence
//   structural".
//
//   resolveNextRecommended (status.go) never routes to review before verify.
//   `resolve-review` appears only once apply is done, a verify report exists,
//   and verification did not pass.
//
// A third place disagreed with both: Finish refused to close a PASSING
// IMPLEMENTATION attempt whenever a review binding covered the pre-attempt
// bytes. That is review acting during implementation, before any verification
// has run, and it is what #1993 hit — the corrected candidate passed every
// gate the maintainer authorized, and the settle refused because the binding
// covered the target from before the correction.
//
// Removing it takes nothing away from delivery. The delivery gates
// (post-apply, pre-commit, pre-push, pre-pr, release) re-derive their verdict
// from the candidate actually being delivered, so an unreviewed candidate is
// still refused there — after SDD finishes, which is the whole point.

// TestPassingImplementationSettlesWhileABindingCoversOlderBytes is #1993.
func TestPassingImplementationSettlesWhileABindingCoversOlderBytes(t *testing.T) {
	change := "review-after-verify"
	fixture := newRuntimeUnchangedBindingFixture(t, change)

	// The attempt does what an implementation attempt does: it changes the
	// candidate. The binding now covers bytes that no longer exist.
	write(t, filepath.Join(fixture.store.Repo, "openspec", "changes", change, "tasks.md"),
		"- [x] 1.1 Done\n# the maintainer-authorized correction\n")

	status, err := fixture.store.Finish(context.Background(), fixture.finishRequest(change+"-settle"))
	if err != nil {
		t.Fatalf("a passing implementation settle was refused because a review binding covers older bytes: %v\n\nReview acts after implementation and verification; the delivery gates enforce on the finished candidate (#1993)", err)
	}
	if last := status.Attempts[len(status.Attempts)-1]; last.Outcome != AttemptPassed {
		t.Fatalf("settled attempt = %#v, want a passed outcome", last)
	}
	// The binding is still recorded. Independence means review stops deciding,
	// not that its authority stops being tracked.
	if status.Binding == nil {
		t.Fatal("the review binding was dropped from the ledger; it must stay recorded, it just must not gate")
	}
}

// TestSwitchedOffReviewLeavesNoResidueOnTheLedger is the kill-switch half, and
// it is a straight contradiction of this package's own written guarantee:
// "While it is off receipt-driven development does not exist, so it must have
// no implications."
//
// A binding created before the operator turned review off kept blocking the
// unmanaged correction path, because the guard demanded the switch be off AND
// no binding exist. So the switch made review inert only for changes that had
// never used it. Turning it off after the fact bought nothing.
func TestSwitchedOffReviewLeavesNoResidueOnTheLedger(t *testing.T) {
	ctx := context.Background()
	change := "switch-off-residue"
	fixture := newRuntimeUnchangedBindingFixture(t, change)
	bound, err := fixture.store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if bound.Binding == nil {
		t.Fatal("fixture did not produce a review binding")
	}

	// The operator turns review off. From here it must have no implications.
	store := fixture.store
	store.ReviewDisabled = true

	// Fail a verification under the disabled switch, then correct it. This is
	// the ordinary unmanaged correction, and the leftover binding must not
	// stand in its way.
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: bound.Revision, RequestID: change + "-fail", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: change + "-correct", WorkUnit: change,
		EvidenceGoal: "verify unchanged approved candidate", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(store.Repo, "openspec", "changes", change, "tasks.md"),
		"- [x] 1.1 Done\n# the correction\n")

	if _, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: correction.Revision, RequestID: change + "-correct-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "the correction passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	}); err != nil {
		t.Fatalf("a leftover review binding blocked the unmanaged correction while the kill switch is OFF: %v\n\nThe package's own contract says that while it is off receipt-driven development does not exist, so it must have no implications", err)
	}
}
