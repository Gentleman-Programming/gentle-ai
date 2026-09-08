package reviewtransaction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// newUnachievableLensFixtureState builds a StateReviewing authority that
// selects the full canonical four-lens set, regardless of what the fixture
// repository would actually classify -- RiskHigh only requires the declared
// selection to match the canonical order (validateSelectedLenses), so a
// forced high tier keeps this fixture small while still exercising more than
// one selected order.
func newUnachievableLensFixtureState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("1"), RiskLevel: RiskHigh, SelectedLenses: append([]string{}, supportedLenses...),
		OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// TestRecordUnachievableLensAttemptReplaysAndRefusesMismatch is the
// reviewtransaction-level regression for issue #3442: a host that reports one
// bound selected lens slot unachievable gets that exact answer back on exact
// replay (restart-safety), a contradictory second declaration for the same
// slot is refused rather than silently overwritten, and a slot that already
// holds an admitted result cannot be retroactively declared unachievable.
func TestRecordUnachievableLensAttemptReplaysAndRefusesMismatch(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "unachievable-lens-attempt-replay")
	state, store := startReviewingCompactAuthority(t, repo, state)

	request := CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"),
		Reason: "relay_transport_bound_exceeded", Detail: "elapsed 42s against a 30s bound",
	}
	recorded, err := store.RecordUnachievableLensAttempt(context.Background(), request)
	if err != nil || !recorded {
		t.Fatalf("record first unachievable attempt: recorded=%t err=%v", recorded, err)
	}

	// Restart-safety: a fresh store handle over the same lineage returns the
	// same answer on an exact replay without mutating the record.
	restarted, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err = restarted.RecordUnachievableLensAttempt(context.Background(), request)
	if err != nil || recorded {
		t.Fatalf("exact replay after restart: recorded=%t err=%v", recorded, err)
	}
	afterReplay, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterReplay.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("unachievable attempt count after replay = %d, want 1", len(afterReplay.State.UnachievableLensAttempts))
	}
	beforeMismatch := afterReplay.Revision

	// A contradictory declaration for the SAME slot (different subject hash)
	// must never silently overwrite the frozen answer.
	mismatch := request
	mismatch.SubjectHash = hash("b")
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), mismatch); err == nil {
		t.Fatal("a mismatched unachievable declaration for the same slot was not refused")
	}
	afterMismatch, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterMismatch.Revision != beforeMismatch || len(afterMismatch.State.UnachievableLensAttempts) != 1 {
		t.Fatal("a refused mismatched declaration must not mutate the authority")
	}

	// A different slot may still be declared unachievable independently.
	second := request
	second.Lens, second.SelectedOrder, second.SubjectHash = state.SelectedLenses[1], 1, hash("c")
	if recorded, err := store.RecordUnachievableLensAttempt(context.Background(), second); err != nil || !recorded {
		t.Fatalf("record second unachievable attempt on a distinct slot: recorded=%t err=%v", recorded, err)
	}
	afterSecond, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterSecond.State.UnachievableLensAttempts) != 2 {
		t.Fatalf("unachievable attempt count = %d, want 2", len(afterSecond.State.UnachievableLensAttempts))
	}
	if err := afterSecond.State.Validate(); err != nil {
		t.Fatalf("state carrying two unachievable attempts must validate: %v", err)
	}

	// A slot that already holds an admitted result cannot be retroactively
	// declared unachievable: the admitted result already answers it.
	captureCompactLens(t, store, afterSecond.State, 2)
	admittedRecord, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	afterAdmission := CompactUnachievableLensAttemptRequest{
		ExpectedRevision: admittedRecord.State.CapturePhaseRevision, TargetIdentity: admittedRecord.State.InitialSnapshot.Identity,
		Lens: admittedRecord.State.SelectedLenses[2], SelectedOrder: 2, SubjectHash: hash("d"), Reason: "relay_transport_bound_exceeded",
	}
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), afterAdmission); err == nil {
		t.Fatal("declaring an already-admitted slot unachievable was not refused")
	}
}

// TestRecordUnachievableLensAttemptRequiresReviewingBinding proves the
// declaration is bound like every other capture: a stale revision, wrong
// target, or wrong lens name for the given order is refused rather than
// silently applied to the current authority.
func TestRecordUnachievableLensAttemptRequiresReviewingBinding(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "unachievable-lens-attempt-binding")
	state, store := startReviewingCompactAuthority(t, repo, state)

	base := CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}

	staleRevision := base
	staleRevision.ExpectedRevision = hash("9")
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), staleRevision); err == nil {
		t.Fatal("a stale expected-revision was not refused")
	}

	wrongTarget := base
	wrongTarget.TargetIdentity = hash("9")
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), wrongTarget); err == nil {
		t.Fatal("a mismatched target identity was not refused")
	}

	wrongLens := base
	wrongLens.Lens = state.SelectedLenses[1]
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), wrongLens); err == nil {
		t.Fatal("a lens name that does not match its declared order was not refused")
	}

	missingReason := base
	missingReason.Reason = "  "
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), missingReason); err == nil {
		t.Fatal("an empty reason was not refused")
	}

	if _, err := store.RecordUnachievableLensAttempt(context.Background(), base); err != nil {
		t.Fatalf("a correctly bound declaration was refused: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("unachievable attempt count = %d, want 1 (rejected attempts must never mutate the authority)", len(record.State.UnachievableLensAttempts))
	}
}

// TestWithdrawUnachievableLensAttemptIsIdempotentAndRestoresTheSlot is the
// reviewtransaction-level regression for the resilience gap the native
// review found: declaring a slot unachievable must not be a one-way trip
// out of the lineage for a host that mistook a transient failure for a
// deterministic one. This proves withdrawal removes exactly the declared
// slot, restart-safety holds for the removal itself, an exact replay of the
// withdrawal (on the now-absent entry) succeeds without mutation, and a
// slot that has since been genuinely admitted refuses withdrawal instead of
// silently discarding the now-stale bookkeeping underneath a real result.
func TestWithdrawUnachievableLensAttemptIsIdempotentAndRestoresTheSlot(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "unachievable-lens-attempt-withdraw")
	state, store := startReviewingCompactAuthority(t, repo, state)

	declare := CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), declare); err != nil {
		t.Fatal(err)
	}
	second := declare
	second.Lens, second.SelectedOrder, second.SubjectHash = state.SelectedLenses[1], 1, hash("b")
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	withdrawal := CompactUnachievableLensWithdrawalRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity, SubjectHash: declare.SubjectHash,
	}
	removed, err := store.WithdrawUnachievableLensAttempt(context.Background(), withdrawal)
	if err != nil || !removed {
		t.Fatalf("withdraw the first declared slot: removed=%t err=%v", removed, err)
	}
	afterWithdraw, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterWithdraw.State.UnachievableLensAttempts) != 1 || afterWithdraw.State.UnachievableLensAttempts[0].SubjectHash != second.SubjectHash {
		t.Fatalf("withdraw removed the wrong entry, or the wrong count remains: %#v", afterWithdraw.State.UnachievableLensAttempts)
	}

	// Restart-safety: exact replay of the same withdrawal after a fresh
	// store handle succeeds without mutation -- the entry is already gone.
	restarted, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	beforeReplay, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	removed, err = restarted.WithdrawUnachievableLensAttempt(context.Background(), withdrawal)
	if err != nil || removed {
		t.Fatalf("idempotent replay of an absent withdrawal: removed=%t err=%v", removed, err)
	}
	afterReplay, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterReplay.Revision != beforeReplay.Revision {
		t.Fatal("an idempotent no-op withdrawal must not mutate the authority")
	}

	// Withdrawing a SubjectHash that was never declared is the same safe
	// no-op, never an error.
	removed, err = store.WithdrawUnachievableLensAttempt(context.Background(), CompactUnachievableLensWithdrawalRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity, SubjectHash: hash("f"),
	})
	if err != nil || removed {
		t.Fatalf("withdrawing a never-declared subject hash: removed=%t err=%v", removed, err)
	}

	// A slot that now holds a real admitted result refuses withdrawal of its
	// stale declaration instead of silently discarding bookkeeping under a
	// resolved result.
	captureCompactLens(t, store, afterReplay.State, 1)
	if _, err := store.WithdrawUnachievableLensAttempt(context.Background(), CompactUnachievableLensWithdrawalRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity, SubjectHash: second.SubjectHash,
	}); err == nil {
		t.Fatal("withdrawing an already-admitted slot's declaration was not refused")
	}
}

// TestCompleteReviewClearsUnachievableLensAttempts proves the declaration
// ledger never outlives the reviewing phase it was bound to: once the review
// closes (every selected slot ends up admitted, so any earlier declaration
// for one of them is moot), CompleteReview clears it and the resulting state
// still validates.
func TestCompleteReviewClearsUnachievableLensAttempts(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "unachievable-lens-attempt-complete-review")
	state, store := startReviewingCompactAuthority(t, repo, state)

	if _, err := store.RecordUnachievableLensAttempt(context.Background(), CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("unachievable attempt count = %d, want 1", len(record.State.UnachievableLensAttempts))
	}

	// The host later succeeds on retry for every selected lens, so the review
	// can close for real -- the earlier declaration is now moot.
	captured := record.State
	for order := range captured.SelectedLenses {
		captureCompactLens(t, store, captured, order)
		reloaded, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		captured = reloaded.State
	}
	view, err := captured.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := captured.CompleteReview(CompactReviewInput{LensResults: view.LensResults}); err != nil {
		t.Fatalf("CompleteReview: %v", err)
	}
	if len(captured.UnachievableLensAttempts) != 0 {
		t.Fatalf("CompleteReview left %d unachievable attempts behind, want 0", len(captured.UnachievableLensAttempts))
	}
	if err := captured.Validate(); err != nil {
		t.Fatalf("state after CompleteReview must validate: %v", err)
	}
}

// TestInvalidateRefusesThenSucceedsAcrossAnUnachievableDeclaration is the
// regression for the native review's centralization finding, on the
// invalidate exit specifically: Invalidate()'s own pristine gate
// (compactPristineReviewing, which already requires an empty
// UnachievableLensAttempts ledger) makes it structurally impossible to leave
// StateReviewing with a standing declaration through this path -- it
// refuses instead. Once the declaration is properly withdrawn, invalidate
// succeeds and setCompactStateExit's centralized clearing still applies
// (the ledger was already empty, so this proves the invariant holds, not
// that it was needed here); the persisted, reloaded record validates.
func TestInvalidateRefusesThenSucceedsAcrossAnUnachievableDeclaration(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "invalidate-unachievable")
	state, store := startReviewingCompactAuthority(t, repo, state)

	declare := CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}
	if _, err := store.RecordUnachievableLensAttempt(context.Background(), declare); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	standing := record.State
	if err := standing.Invalidate("operator abandoned"); err == nil {
		t.Fatal("invalidate accepted a reviewing authority with a standing unachievable declaration")
	}

	if _, err := store.WithdrawUnachievableLensAttempt(context.Background(), CompactUnachievableLensWithdrawalRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity, SubjectHash: declare.SubjectHash,
	}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	working := record.State
	if err := working.Invalidate("operator abandoned"); err != nil {
		t.Fatalf("invalidate a genuinely pristine reviewing authority: %v", err)
	}
	if len(working.UnachievableLensAttempts) != 0 {
		t.Fatalf("invalidated in-memory state carries %d unachievable lens attempts, want 0", len(working.UnachievableLensAttempts))
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", working); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("load the invalidated record: %v", err)
	}
	if reloaded.State.State != StateInvalidated || len(reloaded.State.UnachievableLensAttempts) != 0 {
		t.Fatalf("reloaded invalidated record = %#v", reloaded.State)
	}
	if err := reloaded.State.Validate(); err != nil {
		t.Fatalf("invalidated record with an empty ledger must validate: %v", err)
	}
}

// TestAbandonSucceedsAcrossAnUnachievableDeclarationAndQuarantinedRecordValidates
// is the abandon half of the same regression. Unlike Invalidate,
// AbandonPristineCompactStore has no pristine precondition (it only refuses
// a terminal Approved/Escalated/Invalidated authority) and never rewrites
// CompactState.State at all -- it quarantines the exact persisted bytes via
// a directory rename (reclaimQuarantineResidue = os.Rename), never
// re-serializing them. So a standing declaration is neither cleared nor
// corrupting: the quarantined record keeps State=StateReviewing with its
// ledger intact, which remains a valid combination, and it must still load
// and validate from its quarantined location.
func TestAbandonSucceedsAcrossAnUnachievableDeclarationAndQuarantinedRecordValidates(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "abandon-unachievable")
	state, store := startReviewingCompactAuthority(t, repo, state)

	if _, err := store.RecordUnachievableLensAttempt(context.Background(), CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("unachievable attempt count = %d, want 1", len(record.State.UnachievableLensAttempts))
	}

	committed, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record))
	if err != nil {
		t.Fatalf("abandon a reviewing lineage with a standing unachievable declaration: %v", err)
	}

	quarantinedPayload, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "review-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	quarantinedRecord, err := parseCompactRecord(quarantinedPayload, state.LineageID)
	if err != nil {
		t.Fatalf("quarantined record must still load and validate: %v", err)
	}
	if quarantinedRecord.State.State != StateReviewing || len(quarantinedRecord.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("quarantined record = %#v, want the standing declaration preserved under its original reviewing state", quarantinedRecord.State)
	}
	if err := quarantinedRecord.State.Validate(); err != nil {
		t.Fatalf("quarantined record with a standing declaration must validate: %v", err)
	}
}

// TestParseCompactRecordDropsStaleUnachievableLensAttemptsOnLoad proves the
// load-time tolerance (issue #3442): a record written before
// setCompactStateExit centralized the clearing of UnachievableLensAttempts
// can carry a declaration left standing after the phase moved on. Loading it
// must recover -- dropping the stale entries with a diagnostic note -- not
// refuse forever. The fabricated record is built and checksummed exactly
// like a real one (makeCompactRecord), only its in-memory State field is
// hand-set to bypass Invalidate()'s own pristine gate, simulating exactly
// the shape a pre-fix exit could have left on disk.
func TestParseCompactRecordDropsStaleUnachievableLensAttemptsOnLoad(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newUnachievableLensFixtureState(t, repo, "legacy-stuck-unachievable")
	state, store := startReviewingCompactAuthority(t, repo, state)

	if _, err := store.RecordUnachievableLensAttempt(context.Background(), CompactUnachievableLensAttemptRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		Lens: state.SelectedLenses[0], SelectedOrder: 0, SubjectHash: hash("a"), Reason: "relay_transport_bound_exceeded",
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	stuck := record.State
	stuck.State = StateInvalidated
	stuck.InvalidationReason = "legacy exit predates centralized clearing"
	_, payload, err := makeCompactRecord(stuck)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	var notes []string
	original := compactStaleUnachievableLensAttemptsNote
	compactStaleUnachievableLensAttemptsNote = func(lineageID string, dropState State, dropped int) {
		notes = append(notes, fmt.Sprintf("%s|%s|%d", lineageID, dropState, dropped))
	}
	t.Cleanup(func() { compactStaleUnachievableLensAttemptsNote = original })

	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("a legacy stuck record must load, not refuse: %v", err)
	}
	if len(reloaded.State.UnachievableLensAttempts) != 0 {
		t.Fatalf("stale unachievable lens attempts were not dropped: %#v", reloaded.State.UnachievableLensAttempts)
	}
	if err := reloaded.State.Validate(); err != nil {
		t.Fatalf("the tolerant reloaded record must validate: %v", err)
	}
	wantNote := fmt.Sprintf("%s|%s|1", stuck.LineageID, StateInvalidated)
	if len(notes) != 1 || notes[0] != wantNote {
		t.Fatalf("debug note = %v, want exactly [%q]", notes, wantNote)
	}
}
