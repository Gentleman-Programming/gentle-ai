package reviewtransaction

// Wave 5 (Gate Cutover) Slice 7, task 8.2: StateInvalidated /
// InvalidationEvidence stay parse-only (design decision 2) -- a pre-cutover
// invalidated record (written before this slice's deletion, simulated here
// directly with the package's own write primitives, exactly mirroring what
// the deleted InvalidateApprovedCompactAuthority produced) still loads and
// its evidence still reads back exactly, even though nothing can write a
// NEW one via any gate or compact path anymore
// (TestNoGateWritesAuthority_CallAbsenceGuard, gate_write_guard_test.go,
// proves that by call-absence).

import (
	"context"
	"testing"
)

// TestPreCutoverInvalidatedRecordsStayReadable is task 8.2. This claim is
// unaffected by whether InvalidateApprovedCompactAuthority still exists --
// StateInvalidated/InvalidationEvidence were already parse-only historical
// vocabulary before this slice (design decision 2's own framing: "stay
// parse-only... so historical records still render"), so this is a
// regression guard proving the invariant continues to hold, not a claim
// this slice's deletion newly enables it.
func TestPreCutoverInvalidatedRecordsStayReadable(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := "compact-invalidated-readonly"
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	_, receipt := persistApprovedCompactState(t, repo, state)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	loadedApproved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a pre-cutover invalidated record: exactly the complete shape
	// compactInvalidationDenialBound (deleted) used to guarantee before
	// InvalidateApprovedCompactAuthority (deleted) persisted it via
	// writeAtomic directly (never through Replace's own closed operation
	// vocabulary, which never recognized "review/invalidate-approved" as a
	// distinct named transition).
	const invalidationReason = "pre-cutover simulated invalidation"
	next := loadedApproved.State
	next.State = StateInvalidated
	next.InvalidationReason = invalidationReason
	next.InvalidationEvidence = &CompactInvalidationEvidence{
		Gate: GatePostApply, Reason: invalidationReason,
		Context: GateContext{
			Gate: GatePostApply, LineageID: lineage, Generation: loadedApproved.State.Generation,
			StoreRevision: loadedApproved.Revision, GenesisRevision: loadedApproved.Revision,
			ChainIdentity: loadedApproved.Revision, BundleDigest: loadedApproved.Revision,
			BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
			FixDeltaHash: receipt.FixDeltaHash, PolicyHash: receipt.PolicyHash,
			LedgerHash: loadedApproved.State.LedgerHash(), EvidenceHash: receipt.EvidenceHash,
		},
	}
	_, payload, err := makeCompactRecord(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("pre-cutover invalidated record no longer loads: %v", err)
	}
	if loaded.State.State != StateInvalidated || loaded.State.InvalidationReason != invalidationReason {
		t.Fatalf("pre-cutover invalidated record did not parse back exactly: %#v", loaded.State)
	}
	if loaded.State.InvalidationEvidence == nil || loaded.State.InvalidationEvidence.Gate != GatePostApply ||
		loaded.State.InvalidationEvidence.Reason != invalidationReason {
		t.Fatalf("pre-cutover invalidation evidence did not parse back exactly: %#v", loaded.State.InvalidationEvidence)
	}

	// Reading it again is idempotent: no write, no revision change.
	reread, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reread.Revision != loaded.Revision {
		t.Fatalf("reading the pre-cutover invalidated record changed its revision: before=%q after=%q", loaded.Revision, reread.Revision)
	}
}
