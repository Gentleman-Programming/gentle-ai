package reviewtransaction

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSanctionedRecoveryExitNamesMaintainerPathForInteriorReconciliationAnomaly
// is issue #2422. Wave 7 S3a retired `review reconcile-authority`, the only
// exit reconciliation's two anomaly classes (unchanged_target,
// malformed_recovery_authorization) ever had. A LEAF successor in either
// class still gets `review abandon` today
// (TestForgedRecoveryAuthorizationOnNonTerminalSuccessorHasSanctionedAbandonExit
// proves the same for the disjoint content-mismatch corruption class), but an
// INTERIOR successor -- one that itself has a successor recovering from it,
// so InspectCompactPristineAbandonment's own read-only prediction refuses it
// -- had no exit left at all: SanctionedCompactRecoveryExits fell through to
// prose naming no runnable `gentle-ai` invocation, so the block was terminal
// for a consumer.
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
	if eligibility.Eligible {
		t.Fatalf("l2 abandon eligibility = %#v, want ineligible: it has its own successor l3", eligibility)
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
	if l2Exit.Operation != "" {
		t.Fatalf("l2 exit = %#v, want no automatic operation for an interior reconciliation anomaly", *l2Exit)
	}
	if !strings.Contains(l2Exit.Blocked, "gentle-ai review mode disable") {
		t.Fatalf("l2 exit Blocked = %q, want a literal runnable gentle-ai invocation naming the maintainer path", l2Exit.Blocked)
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
