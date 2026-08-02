package reviewtransaction

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// forgedRecoveryPair persists one escalated predecessor with its receipt and a
// changed-target escalated recovery successor whose recorded maintainer
// authorization carries the exact gentle-ai.review-recovery-authorization/v1
// prefix but binds different content: the reason drifted after the fact. It is
// therefore neither the unchanged-target class nor the pre-contract free-form
// class, so reconciliation classifies it as corruption and refuses.
//
// The shape is written straight to store bytes because it is unreachable
// through the CLI: `review recover` and ImportCompactTransport both run
// validateCompactRecoveryEdge at write time and refuse it.
func forgedRecoveryPair(t *testing.T, repo, suffix, target string, mutate ...func(*CompactState)) (CompactRecord, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, "forged-predecessor-"+suffix)
	state.State = StateEscalated
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := writeCompactFixtureRecord(t, predecessorStore, state)
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(predecessorStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", target)
	successorState := newCompactTestState(t, repo, "forged-successor-"+suffix)
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry terminal validator", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(
			state.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity,
			"maintainer@example.com", "retry terminal validator after the incident review"),
	}
	for _, adjust := range mutate {
		adjust(&successorState)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, successor, successorStore
}

// TestForgedRecoveryAuthorizationOnNonPristineSuccessorStaysBlocked pins the
// one shape that remains unrecoverable. Reconciliation refuses it as
// corruption and the abandonment gate refuses to discard captured review
// results, so no advertised operation clears it. Admitting it would mean
// making the corruption class quarantinable, which is a maintainer decision,
// not a repair. What this test does require is that the refusal say so
// precisely and never name a command that would not resolve the block.
func TestForgedRecoveryAuthorizationOnNonPristineSuccessorStaysBlocked(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	predecessor, successor, successorStore := forgedRecoveryPair(t, repo, "captured", "forged captured target\n", func(state *CompactState) {
		results := make([]LensResult, 0, len(state.SelectedLenses))
		for _, lens := range state.SelectedLenses {
			results = append(results, LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed once"}})
		}
		if err := state.CompleteReview(CompactReviewInput{
			LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
		}); err != nil {
			t.Fatal(err)
		}
	})
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Edges) != 1 || !strings.Contains(report.Edges[0].NonReconcilableReason, "corruption") {
		t.Fatalf("blocked inspection = %#v", report.Edges)
	}
	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 1 || exits[0].Operation != "" || !strings.Contains(exits[0].Blocked, "no advertised operation admits this edge") {
		t.Fatalf("blocked exit = %#v", exits)
	}

	_, err = ReconcileInvalidRecoveryEdge(ctx, repo, forgedReconcileRequest(predecessor, successor))
	if err == nil {
		t.Fatal("reconcile admitted a forged authorization on a non-pristine successor")
	}
	refusal := err.Error()
	for _, want := range []string{
		"corruption",
		"No advertised operation admits this edge",
		"captured review or correction data",
		"gentle-ai review inspect-authority",
	} {
		if !strings.Contains(refusal, want) {
			t.Fatalf("blocked refusal does not name %q:\n%s", want, refusal)
		}
	}
	// The refusal must not advertise an abandonment that would then refuse.
	if strings.Contains(refusal, "gentle-ai review abandon") {
		t.Fatalf("blocked refusal names an abandonment the gate rejects:\n%s", refusal)
	}
	if _, abandonErr := InspectCompactPristineAbandonment(ctx, repo, successor.State.LineageID); abandonErr != nil {
		t.Fatal(abandonErr)
	}
	current, err := os.ReadFile(successorStore.StatePath())
	if err != nil || !bytes.Equal(current, persisted) {
		t.Fatalf("a refused reconcile rewrote the corrupt authorization bytes: %v", err)
	}
}

// TestCompactAuthorityRemovalRegressionStillRefuses proves the relaxed graph
// guard is a real refusal and not an unconditional yes. Dropping a defect the
// removed entry carried is permitted; orphaning a successor behind a removed
// predecessor is not, even though the graph was already invalid before.
func TestCompactAuthorityRemovalRegressionStillRefuses(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, successor, _ := forgedRecoveryPair(t, repo, "regression", "forged regression target\n")
	records := map[string]CompactRecord{
		predecessor.State.LineageID: predecessor,
		successor.State.LineageID:   successor,
	}
	// The graph is already invalid, so a whole-graph post-condition would
	// refuse both removals and leave the store unrecoverable.
	if _, err := compactAuthorityLeaves(records, map[string]CompactStore{}); err == nil {
		t.Fatal("fixture graph is unexpectedly valid")
	}
	if err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, successor.State.LineageID)); err != nil {
		t.Fatalf("removing the defective successor was refused: %v", err)
	}
	err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, predecessor.State.LineageID))
	if err == nil || !strings.Contains(err.Error(), "would introduce a new authority graph defect") ||
		!strings.Contains(err.Error(), "dangling predecessor") {
		t.Fatalf("orphaning the successor was admitted: %v", err)
	}
}

func forgedReconcileRequest(predecessor, successor CompactRecord) CompactReconcileRequest {
	request := CompactReconcileRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: successor.State.LineageID, ExpectedSuccessorRevision: successor.Revision,
		Reason: "quarantine forged recovery authorization", Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason)
	return request
}

// TestForgedRecoveryAuthorizationDeadEndHasRunnableExit drives a compact-v2
// store holding two independent forged-authorization recovery edges from
// non-authoritative back to authoritative by running only what the product's
// own refusals name. Two edges are load-bearing: with a whole-graph
// post-condition each repair refuses citing the other, so neither can go first.
func TestForgedRecoveryAuthorizationDeadEndHasRunnableExit(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	predecessorAlpha, successorAlpha, successorStoreAlpha := forgedRecoveryPair(t, repo, "alpha", "forged alpha target\n")
	predecessorBravo, successorBravo, successorStoreBravo := forgedRecoveryPair(t, repo, "bravo", "forged bravo target\n")

	alphaBytes, err := os.ReadFile(successorStoreAlpha.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	bravoBytes, err := os.ReadFile(successorStoreBravo.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	before, err := InventoryAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Authoritative ||
		!hasAuthorityInventoryStatus(before.Entries, successorAlpha.State.LineageID, AuthorityStatusInvalid) ||
		!hasAuthorityInventoryStatus(before.Entries, successorBravo.State.LineageID, AuthorityStatusInvalid) {
		t.Fatalf("forged inventory = %#v", before)
	}

	// 1. The inspection surface publishes the diagnosis it already computed.
	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || report.Totals.InvalidEdges != 2 {
		t.Fatalf("forged inspection totals = %#v", report.Totals)
	}
	for _, edge := range report.Edges {
		if !strings.Contains(edge.NonReconcilableReason, "corruption") {
			t.Fatalf("edge %q publishes no non-reconcilable reason: %#v", edge.SuccessorLineageID, edge)
		}
	}
	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 2 {
		t.Fatalf("sanctioned exits = %#v", exits)
	}
	for _, exit := range exits {
		if exit.Operation != CompactRecoveryEdgeExitAbandon || exit.Blocked != "" {
			t.Fatalf("edge %q publishes no runnable exit: %#v", exit.SuccessorLineageID, exit)
		}
	}

	// 2. Reconciliation still refuses — the edge really is corruption — but the
	//    refusal now names a continuation that resolves the block.
	_, err = ReconcileInvalidRecoveryEdge(ctx, repo, forgedReconcileRequest(predecessorAlpha, successorAlpha))
	if err == nil {
		t.Fatal("reconcile admitted a forged recovery authorization")
	}
	refusal := err.Error()
	for _, want := range []string{
		"corruption",
		"gentle-ai review abandon",
		"--lineage \"" + successorAlpha.State.LineageID + "\"",
		"--expected-revision \"" + successorAlpha.Revision + "\"",
		CompactAbandonAuthorizationSchema,
	} {
		if !strings.Contains(refusal, want) {
			t.Fatalf("reconcile refusal does not name %q:\n%s", want, refusal)
		}
	}

	// 3. Run only what the refusal named, on each edge in turn.
	for _, successor := range []CompactRecord{successorAlpha, successorBravo} {
		eligibility, err := InspectCompactPristineAbandonment(ctx, repo, successor.State.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		if !eligibility.Eligible {
			t.Fatalf("successor %q is not abandonable; the named continuation does not run", successor.State.LineageID)
		}
		request := CompactAbandonRequest{
			LineageID: successor.State.LineageID, ExpectedRevision: eligibility.Revision,
			Reason: "quarantine forged recovery authorization", Actor: "maintainer@example.com",
		}
		request.MaintainerAuthorization = RenderCompactAbandonAuthorization(
			request.LineageID, eligibility.Revision, eligibility.SnapshotIdentity, request.Actor, request.Reason)
		record, err := AbandonPristineCompactStore(ctx, repo, request)
		if err != nil {
			t.Fatalf("abandon %q: %v", successor.State.LineageID, err)
		}
		if record.Status != CompactReclaimCommitted {
			t.Fatalf("abandon %q record = %#v", successor.State.LineageID, record)
		}
		moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", compactStateFileName))
		if err != nil {
			t.Fatalf("read quarantined residue for %q: %v", successor.State.LineageID, err)
		}
		want := alphaBytes
		if successor.State.LineageID == successorBravo.State.LineageID {
			want = bravoBytes
		}
		if !bytes.Equal(moved, want) {
			t.Fatalf("quarantine did not byte-preserve the corrupt authorization for %q", successor.State.LineageID)
		}
	}

	// 4. The store is authoritative again, and both predecessors survive.
	after, err := InventoryAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-abandon inventory = %#v", after)
	}
	leaves, err := CompactAuthorityLeaves(ctx, repo)
	if err != nil || len(leaves) != 2 {
		t.Fatalf("post-abandon leaves = %#v, %v", leaves, err)
	}
	for _, predecessor := range []CompactRecord{predecessorAlpha, predecessorBravo} {
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(successorStoreAlpha.Dir), predecessor.State.LineageID)); statErr != nil {
			t.Fatalf("predecessor %q did not survive: %v", predecessor.State.LineageID, statErr)
		}
	}
}
