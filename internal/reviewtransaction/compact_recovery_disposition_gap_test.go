package reviewtransaction

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSanctionedRecoveryExitNamesMaintainerPathForInteriorReconciliationAnomaly
// is issue #2422. Wave 7 S3a retired `review reconcile-authority`, the only
// exit reconciliation's two anomaly classes (unchanged_target,
// malformed_recovery_authorization) ever had, and an INTERIOR successor -- one
// that itself has a successor recovering from it -- was left with no exit.
// `review abandon` now accepts it too and abandons its successors with it, so
// the exit is a runnable abandon, never a prose dead end.
func TestSanctionedRecoveryExitNamesMaintainerPathForInteriorReconciliationAnomaly(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)

	// L1: an escalated predecessor, exactly like forgedRecoveryPair's own.
	predecessor := correctedCompactTestState(t, repo, "chain-l1")
	predecessor.State = StateEscalated
	predecessorStore, err := CompactAuthoritativeStore(ctx, repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord := writeCompactFixtureRecord(t, predecessorStore, predecessor)

	// L2 recovers from L1 without the candidate content changing at all --
	// the unchanged_target anomaly class -- with an exact, contract-bound
	// authorization (never the pre-contract free-form text that would
	// instead classify as malformed_recovery_authorization).
	l2State := newCompactTestState(t, repo, "chain-l2")
	l2State.Generation = predecessor.Generation + 1
	l2Store, err := CompactAuthoritativeStore(ctx, repo, l2State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	l2State, _ = startReviewingCompactFixture(t, repo, l2State)
	l2State.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: predecessorRecord.Revision,
		Disposition: RecoveryEscalated, Reason: "retry", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(
			predecessor.LineageID, predecessorRecord.Revision, l2State.InitialSnapshot.Identity, "maintainer@example.com", "retry"),
	}
	l2Record := writeCompactFixtureRecord(t, l2Store, l2State)

	// L3 recovers from L2, making L2 an INTERIOR node of the chain: abandon's
	// own read-only prediction refuses any lineage another lineage recovers
	// from (compact_abandon.go), independent of that further edge's own
	// validity -- exactly the topology the #2422 field report describes.
	l3State := newCompactTestState(t, repo, "chain-l3")
	l3State.Generation = l2State.Generation + 1
	l3Store, err := CompactAuthoritativeStore(ctx, repo, l3State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	l3State, _ = startReviewingCompactFixture(t, repo, l3State)
	l3State.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: l2State.LineageID, PredecessorRevision: l2Record.Revision,
		Disposition: RecoveryEscalated, Reason: "retry", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(
			l2State.LineageID, l2Record.Revision, l3State.InitialSnapshot.Identity, "maintainer@example.com", "retry"),
	}
	writeCompactFixtureRecord(t, l3Store, l3State)

	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	var l2Edge *CompactRecoveryEdgeInspection
	for index := range report.Edges {
		if report.Edges[index].SuccessorLineageID == l2State.LineageID {
			l2Edge = &report.Edges[index]
		}
	}
	if l2Edge == nil || l2Edge.Valid || !compactRecoveryDispositionGapContainsString(l2Edge.AnomalyClasses, compactRecoveryEdgeUnchangedTarget) {
		t.Fatalf("l2 edge = %#v, want an invalid unchanged_target edge", report.Edges)
	}

	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, l2State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if !eligibility.Eligible {
		t.Fatalf("l2 abandon eligibility = %#v, want eligible: abandonment cascades to its successor l3", eligibility)
	}

	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil {
		t.Fatal(err)
	}
	var l2Exit *CompactRecoverySanctionedExit
	for index := range exits {
		if exits[index].SuccessorLineageID == l2State.LineageID {
			l2Exit = &exits[index]
		}
	}
	if l2Exit == nil {
		t.Fatalf("exits = %#v, want an exit entry for l2", exits)
	}
	if l2Exit.Operation != CompactRecoveryEdgeExitAbandon || l2Exit.Blocked != "" {
		t.Fatalf("l2 exit = %#v, want the runnable abandon exit", *l2Exit)
	}

	// Driving the named exit abandons l2 AND its successor l3, and leaves the
	// escalated predecessor l1 alone.
	committed, err := abandonAuthorized(t, ctx, repo, CompactAbandonRequest{
		LineageID: l2State.LineageID, Reason: CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
	})
	if err != nil || len(committed.Cascaded) != 1 || committed.Cascaded[0].LineageID != l3State.LineageID {
		t.Fatalf("interior abandonment = %#v, %v", committed, err)
	}
	after, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil || len(after.Edges) != 0 || !after.Valid {
		t.Fatalf("post-abandon inspection = %#v, %v", after, err)
	}
	if _, err := os.Stat(predecessorStore.Dir); err != nil {
		t.Fatalf("the predecessor did not survive: %v", err)
	}
}

func compactRecoveryDispositionGapContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
