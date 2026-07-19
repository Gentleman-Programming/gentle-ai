package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// poisonedRecoveryFixture persists an escalated predecessor with its receipt
// and a recovery successor whose sole structural anomaly is an unchanged
// target. mutate, when non-nil, adjusts the successor before it is persisted.
func poisonedRecoveryFixture(t *testing.T, repo string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, "reconcile-predecessor")
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
	successorState := newCompactTestState(t, repo, "reconcile-successor")
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry terminal validator", Actor: "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(state.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity, "maintainer@example.com", "retry terminal validator"),
	}
	if mutate != nil {
		mutate(&successorState)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, predecessorStore, successor, successorStore
}

func writeCompactFixtureRecord(t *testing.T, store CompactStore, state CompactState) CompactRecord {
	t.Helper()
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return record
}

func reconcileFixtureRequest(predecessor, successor CompactRecord) CompactReconcileRequest {
	request := CompactReconcileRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: successor.State.LineageID, ExpectedSuccessorRevision: successor.Revision,
		Reason: "quarantine invalid unchanged-target recovery edge", Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason)
	return request
}

func combinedRecoveryFixture(t *testing.T, repo string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	return poisonedRecoveryFixture(t, repo, func(state *CompactState) {
		state.Recovery.MaintainerAuthorization = preContractFixtureAuthorization
		if mutate != nil {
			mutate(state)
		}
	})
}

func combinedReconcileFixtureRequest(predecessor, successor CompactRecord) CompactReconcileRequest {
	request := reconcileFixtureRequest(predecessor, successor)
	request.Reason = "quarantine combined recovery anomalies"
	request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason) + "\nanomalies=unchanged_target,malformed_recovery_authorization"
	return request
}

func TestReconcileInvalidRecoveryEdgeQuarantinesSuccessorAndRestoresAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, predecessorStore, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "target has not changed") {
		t.Fatalf("poisoned graph leaves error = %v", err)
	}
	before, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Complete || before.Authoritative || !hasAuthorityInventoryStatus(before.Entries, successor.State.LineageID, AuthorityStatusInvalid) {
		t.Fatalf("poisoned inventory = %#v", before)
	}
	predecessorStateBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	predecessorReceiptBefore, err := os.ReadFile(predecessorStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	successorPayload, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	request := reconcileFixtureRequest(predecessor, successor)
	record, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("reconcile invalid recovery edge: %v", err)
	}
	if record.Schema != CompactReclaimRecordSchema || record.Status != CompactReclaimCommitted ||
		record.LineageID != successor.State.LineageID || record.SourcePath != successorStore.Dir ||
		record.Reason != request.Reason || record.Actor != request.Actor || record.ReclaimedAt.IsZero() {
		t.Fatalf("reconcile record = %#v", record)
	}
	if record.InvalidRecoveryEdge == nil || record.InvalidRecoveryEdge.PredecessorLineageID != predecessor.State.LineageID ||
		record.InvalidRecoveryEdge.PredecessorRevision != predecessor.Revision ||
		record.InvalidRecoveryEdge.SuccessorRevision != successor.Revision ||
		!strings.Contains(record.InvalidRecoveryEdge.ValidationError, "target has not changed") {
		t.Fatalf("reconcile edge proof = %#v", record.InvalidRecoveryEdge)
	}
	if len(record.Residue) != 1 || record.Residue[0] != "review-state.json" {
		t.Fatalf("reconcile residue manifest = %#v", record.Residue)
	}
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(record.QuarantinePath, filepath.Join(root, "quarantine")+string(os.PathSeparator)) {
		t.Fatalf("quarantine path %q is outside the quarantine root", record.QuarantinePath)
	}
	if _, statErr := os.Stat(successorStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("reconciled successor entry still present: %v", statErr)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, successorPayload) {
		t.Fatalf("quarantined successor bytes = %q, %v", moved, err)
	}
	payload, err := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatalf("read persisted reconcile audit record: %v", err)
	}
	var persisted CompactReclaimRecord
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("parse persisted reconcile audit record: %v", err)
	}
	if persisted.Status != CompactReclaimCommitted || persisted.InvalidRecoveryEdge == nil ||
		persisted.InvalidRecoveryEdge.SuccessorRevision != successor.Revision ||
		!strings.Contains(persisted.InvalidRecoveryEdge.ValidationError, "target has not changed") {
		t.Fatalf("persisted reconcile record = %#v", persisted)
	}
	predecessorStateAfter, _ := os.ReadFile(predecessorStore.StatePath())
	predecessorReceiptAfter, _ := os.ReadFile(predecessorStore.ReceiptPath())
	if !bytes.Equal(predecessorStateBefore, predecessorStateAfter) || !bytes.Equal(predecessorReceiptBefore, predecessorReceiptAfter) {
		t.Fatal("reconcile changed predecessor state or receipt bytes")
	}

	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != predecessor.State.LineageID {
		t.Fatalf("post-reconcile leaves = %#v, %v", leaves, err)
	}
	after, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-reconcile inventory = %#v", after)
	}
	if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil {
		t.Fatal("reconcile replayed after the successor entry was quarantined")
	}
	writeSnapshotFile(t, repo, "tracked.txt", "reconciled target\n")
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "reconcile-fresh")})
	if err != nil || started.Action != CompactStartRecover || started.Record.State.LineageID != predecessor.State.LineageID {
		t.Fatalf("post-reconcile lineage-less start = %#v, %v", started, err)
	}
}

func TestReconcileCombinedRecoveryAnomaliesQuarantinesWithBothProofsAndRestoresAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, predecessorStore, successor, successorStore := combinedRecoveryFixture(t, repo, nil)
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "target has not changed") {
		t.Fatalf("combined-anomaly graph leaves error = %v", err)
	}
	predecessorStateBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	successorPayload, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	request := combinedReconcileFixtureRequest(predecessor, successor)
	record, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("reconcile combined recovery anomalies: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.InvalidRecoveryEdge == nil ||
		!strings.Contains(record.InvalidRecoveryEdge.ValidationError, "target has not changed") {
		t.Fatalf("combined unchanged-target proof = %#v", record.InvalidRecoveryEdge)
	}
	recordedDigest := sha256.Sum256([]byte(preContractFixtureAuthorization))
	if record.MalformedRecoveryAuthorization == nil ||
		record.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 != "sha256:"+hex.EncodeToString(recordedDigest[:]) ||
		!strings.Contains(record.MalformedRecoveryAuthorization.ValidationError, "exact maintainer authorization binding") {
		t.Fatalf("combined malformed-authorization proof = %#v", record.MalformedRecoveryAuthorization)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, successorPayload) {
		t.Fatalf("quarantined combined successor bytes = %q, %v", moved, err)
	}
	persistedPayload, err := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted CompactReclaimRecord
	if err := json.Unmarshal(persistedPayload, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.InvalidRecoveryEdge == nil || persisted.MalformedRecoveryAuthorization == nil ||
		persisted.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 != record.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 {
		t.Fatalf("persisted combined audit record = %#v", persisted)
	}
	predecessorStateAfter, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil || !bytes.Equal(predecessorStateBefore, predecessorStateAfter) {
		t.Fatal("combined reconcile changed predecessor authority bytes")
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != predecessor.State.LineageID {
		t.Fatalf("post-combined-reconcile leaves = %#v, %v", leaves, err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "post-combined-repair target\n")
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "reconcile-combined-fresh")})
	if err != nil || started.Action != CompactStartRecover || started.Record.State.LineageID != predecessor.State.LineageID {
		t.Fatalf("post-combined-reconcile start = %#v, %v", started, err)
	}
}

func TestReconcileCombinedRecoveryAnomaliesRefusesWithoutMutation(t *testing.T) {
	assertUntouched := func(t *testing.T, repo string, store CompactStore, payload []byte) {
		t.Helper()
		current, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(current, payload) {
			t.Fatalf("refused combined reconcile mutated successor bytes: %v", err)
		}
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "quarantine"))
		if err == nil && len(entries) != 0 {
			t.Fatalf("refused combined reconcile left quarantine entries: %#v", entries)
		}
	}

	t.Run("binding omits combined anomaly set", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := combinedRecoveryFixture(t, repo, nil)
		payload, _ := os.ReadFile(successorStore.StatePath())
		request := combinedReconcileFixtureRequest(predecessor, successor)
		request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
			request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
			request.Actor, request.Reason)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("inexact combined binding error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("declared classification changed", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := combinedRecoveryFixture(t, repo, func(state *CompactState) {
			writeSnapshotFile(t, repo, "tracked.txt", "changed combined target\n")
			changed := newCompactTestState(t, repo, state.LineageID)
			state.InitialSnapshot = changed.InitialSnapshot
			state.CurrentSnapshot = changed.CurrentSnapshot
			state.GenesisPaths = changed.GenesisPaths
		})
		payload, _ := os.ReadFile(successorStore.StatePath())
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, combinedReconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("changed combined classification error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("stale successor revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := combinedRecoveryFixture(t, repo, nil)
		payload, _ := os.ReadFile(successorStore.StatePath())
		request := combinedReconcileFixtureRequest(predecessor, successor)
		request.ExpectedSuccessorRevision = predecessor.Revision
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale combined revision error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("unrelated graph defect", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := combinedRecoveryFixture(t, repo, nil)
		payload, _ := os.ReadFile(successorStore.StatePath())
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		writeReclaimFixtureFile(t, filepath.Join(root, "v2", "reconcile-unrelated", "review-state.json"), "not json\n")
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, combinedReconcileFixtureRequest(predecessor, successor)); err == nil ||
			!strings.Contains(err.Error(), `related compact authority "reconcile-unrelated" does not load`) {
			t.Fatalf("unrelated combined graph defect error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})
}

func TestReconcileInvalidRecoveryEdgeRefusesIneligibleTargetsAndBindings(t *testing.T) {
	assertUntouched := func(t *testing.T, repo string, store CompactStore, payload []byte) {
		t.Helper()
		current, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(current, payload) {
			t.Fatalf("refused reconcile mutated the successor entry: %q, %v", current, err)
		}
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "quarantine"))
		if err == nil && len(entries) != 0 {
			t.Fatalf("refused reconcile left quarantine entries: %#v", entries)
		}
	}

	t.Run("valid recovery edge", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		state := correctedCompactTestState(t, repo, "reconcile-predecessor")
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
		writeSnapshotFile(t, repo, "tracked.txt", "changed reconcile target\n")
		successorState := newCompactTestState(t, repo, "reconcile-successor")
		successorState.Generation = state.Generation + 1
		recovery := CompactRecoveryRequest{
			PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
			Successor: successorState, Disposition: RecoveryEscalated,
			Reason: "retry terminal validator", Actor: "maintainer@example.com",
		}
		recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(state.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
		successor, err := RecoverCompactAuthority(context.Background(), repo, recovery)
		if err != nil {
			t.Fatal(err)
		}
		successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successor.State.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), "validates") {
			t.Fatalf("claimed-invalid valid edge error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("successor names a different predecessor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := reconcileFixtureRequest(predecessor, successor)
		request.PredecessorLineageID = "reconcile-other"
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), `names predecessor "reconcile-predecessor"`) {
			t.Fatalf("mismatched predecessor lineage reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("stale predecessor revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := reconcileFixtureRequest(predecessor, successor)
		request.ExpectedPredecessorRevision = successor.Revision
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) || !strings.Contains(err.Error(), "expected predecessor revision") {
			t.Fatalf("stale predecessor revision reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("predecessor does not load", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, predecessorStore, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(predecessorStore.StatePath(), []byte("not json\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), "load reconcile predecessor") {
			t.Fatalf("unloadable predecessor reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("unrelated authority does not load", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		writeReclaimFixtureFile(t, filepath.Join(root, "v2", "reconcile-unrelated", "review-state.json"), "not json\n")
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), `related compact authority "reconcile-unrelated" does not load`) {
			t.Fatalf("unloadable unrelated authority reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("predecessor has a sibling successor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		sibling := newCompactTestState(t, repo, "reconcile-sibling")
		sibling.Generation = predecessor.State.Generation + 1
		sibling.Recovery = &CompactRecoveryProvenance{
			PredecessorLineageID: predecessor.State.LineageID, PredecessorRevision: predecessor.Revision,
			Disposition: RecoveryEscalated, Reason: "sibling fork", Actor: "maintainer@example.com",
			RecoveredAt:             time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC),
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, sibling.InitialSnapshot.Identity, "maintainer@example.com", "sibling fork"),
		}
		siblingStore, err := CompactAuthoritativeStore(context.Background(), repo, sibling.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, siblingStore, sibling)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), `has another successor "reconcile-sibling"`) {
			t.Fatalf("sibling fork reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("successor has its own successor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		grandchild := newCompactTestState(t, repo, "reconcile-grandchild")
		grandchild.Generation = successor.State.Generation + 1
		grandchild.Recovery = &CompactRecoveryProvenance{
			PredecessorLineageID: successor.State.LineageID, PredecessorRevision: successor.Revision,
			Disposition: RecoveryEscalated, Reason: "descendant of the invalid successor", Actor: "maintainer@example.com",
			RecoveredAt:             time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC),
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(successor.State.LineageID, successor.Revision, grandchild.InitialSnapshot.Identity, "maintainer@example.com", "descendant of the invalid successor"),
		}
		grandchildStore, err := CompactAuthoritativeStore(context.Background(), repo, grandchild.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, grandchildStore, grandchild)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), `has its own successor "reconcile-grandchild"`) {
			t.Fatalf("descendant successor reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("incomplete entry", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		writeReclaimFixtureFile(t, filepath.Join(root, "v2", "reconcile-successor", "stray.tmp"), "stray\n")
		request := CompactReconcileRequest{
			PredecessorLineageID: "reconcile-predecessor", ExpectedPredecessorRevision: hash("1"),
			SuccessorLineageID: "reconcile-successor", ExpectedSuccessorRevision: hash("2"),
			Reason: "residue", Actor: "maintainer",
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "review reclaim") {
			t.Fatalf("incomplete entry reconcile error = %v", err)
		}
	})

	t.Run("non-recovery record", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "reconcile-successor")
		store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record := writeCompactFixtureRecord(t, store, state)
		payload, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := CompactReconcileRequest{
			PredecessorLineageID: "reconcile-predecessor", ExpectedPredecessorRevision: hash("1"),
			SuccessorLineageID: state.LineageID, ExpectedSuccessorRevision: record.Revision,
			Reason: "residue", Actor: "maintainer",
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "not a recovery successor") {
			t.Fatalf("non-recovery reconcile error = %v", err)
		}
		assertUntouched(t, repo, store, payload)
	})

	t.Run("missing authorization", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := reconcileFixtureRequest(predecessor, successor)
		request.MaintainerAuthorization = ""
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("missing authorization reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("authorization bound to different content", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := reconcileFixtureRequest(predecessor, successor)
		request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
			request.PredecessorLineageID, successor.Revision, request.SuccessorLineageID, predecessor.Revision,
			request.Actor, request.Reason)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("mismatched authorization reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("stale successor revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := reconcileFixtureRequest(predecessor, successor)
		request.ExpectedSuccessorRevision = predecessor.Revision
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale successor revision reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("edge invalid outside the unchanged-target class", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, func(state *CompactState) {
			state.Generation = 1
		})
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), "outside the unchanged-target class") {
			t.Fatalf("other-class reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("recorded recovery binding inexact", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, func(state *CompactState) {
			state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
				state.Recovery.PredecessorLineageID, state.Recovery.PredecessorRevision,
				state.InitialSnapshot.Identity, state.Recovery.Actor, "different reason")
		})
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor)); err == nil || !strings.Contains(err.Error(), "sole anomaly") {
			t.Fatalf("inexact recorded binding reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})
}

// preContractRecoveryFixture persists an escalated predecessor with its
// receipt and a changed-target escalated recovery successor whose sole edge
// anomaly is a pre-contract free-form maintainer authorization written before
// the exact gentle-ai.review-recovery-authorization/v1 binding existed.
// mutate, when non-nil, adjusts the successor before it is persisted.
func preContractRecoveryFixture(t *testing.T, repo, authorization string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, "reconcile-predecessor")
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
	writeSnapshotFile(t, repo, "tracked.txt", "pre-contract recovery target\n")
	successorState := newCompactTestState(t, repo, "reconcile-successor")
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry terminal validator", Actor: "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: authorization,
	}
	if mutate != nil {
		mutate(&successorState)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, predecessorStore, successor, successorStore
}

const preContractFixtureAuthorization = "maintainer approved incident retry per the 2.1.6 runbook"

func preContractReconcileRequest(predecessor, successor CompactRecord) CompactReconcileRequest {
	request := CompactReconcileRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: successor.State.LineageID, ExpectedSuccessorRevision: successor.Revision,
		Reason: "quarantine pre-contract recovery authorization", Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason)
	return request
}

func TestReconcilePreContractAuthorizationRepairsRecoveryDeadlock(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, predecessorStore, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)

	// The historical free-form authorization bricks whole-graph validation.
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil ||
		!strings.Contains(err.Error(), "invalid compact authority graph: escalated recovery requires an exact maintainer authorization binding") {
		t.Fatalf("poisoned graph leaves error = %v", err)
	}
	before, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Complete || before.Authoritative || !hasAuthorityInventoryStatus(before.Entries, successor.State.LineageID, AuthorityStatusInvalid) {
		t.Fatalf("poisoned inventory = %#v", before)
	}
	// START fails closed for any fresh target.
	writeSnapshotFile(t, repo, "tracked.txt", "fresh start target\n")
	if _, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "reconcile-fresh")}); err == nil ||
		!strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("poisoned start error = %v", err)
	}
	// RECOVER deadlocks: publishing an exactly bound successor still reaches
	// whole-graph validation before the successor can be written.
	writeSnapshotFile(t, repo, "tracked.txt", "recover repair target\n")
	deadlocked := newCompactTestState(t, repo, "reconcile-repair")
	deadlocked.Generation = predecessor.State.Generation + 1
	deadlockedRecovery := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: deadlocked, Disposition: RecoveryEscalated,
		Reason: "retry terminal validator", Actor: "maintainer@example.com",
	}
	deadlockedRecovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		predecessor.State.LineageID, predecessor.Revision, deadlocked.InitialSnapshot.Identity,
		deadlockedRecovery.Actor, deadlockedRecovery.Reason)
	if _, err := RecoverCompactAuthority(context.Background(), repo, deadlockedRecovery); err == nil ||
		!strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("poisoned recover error = %v", err)
	}

	predecessorStateBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	predecessorReceiptBefore, err := os.ReadFile(predecessorStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	successorPayload, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	request := preContractReconcileRequest(predecessor, successor)
	record, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("reconcile pre-contract authorization: %v", err)
	}
	if record.Schema != CompactReclaimRecordSchema || record.Status != CompactReclaimCommitted ||
		record.LineageID != successor.State.LineageID || record.SourcePath != successorStore.Dir ||
		record.Reason != request.Reason || record.Actor != request.Actor || record.ReclaimedAt.IsZero() {
		t.Fatalf("pre-contract reconcile record = %#v", record)
	}
	if record.InvalidRecoveryEdge != nil {
		t.Fatalf("pre-contract reconcile carries the unchanged-target proof class: %#v", record.InvalidRecoveryEdge)
	}
	recordedDigest := sha256.Sum256([]byte(preContractFixtureAuthorization))
	if record.MalformedRecoveryAuthorization == nil ||
		record.MalformedRecoveryAuthorization.PredecessorLineageID != predecessor.State.LineageID ||
		record.MalformedRecoveryAuthorization.PredecessorRevision != predecessor.Revision ||
		record.MalformedRecoveryAuthorization.SuccessorRevision != successor.Revision ||
		record.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 != "sha256:"+hex.EncodeToString(recordedDigest[:]) ||
		!strings.Contains(record.MalformedRecoveryAuthorization.ValidationError, "exact maintainer authorization binding") {
		t.Fatalf("pre-contract reconcile proof = %#v", record.MalformedRecoveryAuthorization)
	}
	if len(record.Residue) != 1 || record.Residue[0] != "review-state.json" {
		t.Fatalf("pre-contract reconcile residue manifest = %#v", record.Residue)
	}
	persistedPayload, err := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatalf("read persisted pre-contract audit record: %v", err)
	}
	var persisted CompactReclaimRecord
	if err := json.Unmarshal(persistedPayload, &persisted); err != nil {
		t.Fatalf("parse persisted pre-contract audit record: %v", err)
	}
	if persisted.Status != CompactReclaimCommitted || persisted.InvalidRecoveryEdge != nil ||
		persisted.MalformedRecoveryAuthorization == nil ||
		persisted.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 != record.MalformedRecoveryAuthorization.RecordedAuthorizationSHA256 ||
		!strings.Contains(persisted.MalformedRecoveryAuthorization.ValidationError, "exact maintainer authorization binding") {
		t.Fatalf("persisted pre-contract audit record = %#v", persisted)
	}
	if _, statErr := os.Stat(successorStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("reconciled successor entry still present: %v", statErr)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, successorPayload) {
		t.Fatalf("quarantined successor bytes = %q, %v", moved, err)
	}
	predecessorStateAfter, _ := os.ReadFile(predecessorStore.StatePath())
	predecessorReceiptAfter, _ := os.ReadFile(predecessorStore.ReceiptPath())
	if !bytes.Equal(predecessorStateBefore, predecessorStateAfter) || !bytes.Equal(predecessorReceiptBefore, predecessorReceiptAfter) {
		t.Fatal("pre-contract reconcile changed predecessor state or receipt bytes")
	}

	// STATUS, START, and RECOVER all work again.
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != predecessor.State.LineageID {
		t.Fatalf("post-reconcile leaves = %#v, %v", leaves, err)
	}
	after, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-reconcile inventory = %#v", after)
	}
	if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil {
		t.Fatal("pre-contract reconcile replayed after the successor entry was quarantined")
	}
	writeSnapshotFile(t, repo, "tracked.txt", "post-repair target\n")
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "reconcile-post-repair")})
	if err != nil || started.Action != CompactStartRecover || started.Record.State.LineageID != predecessor.State.LineageID {
		t.Fatalf("post-reconcile lineage-less start = %#v, %v", started, err)
	}
	repair := newCompactTestState(t, repo, "reconcile-repair")
	repair.Generation = predecessor.State.Generation + 1
	recovery := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: repair, Disposition: RecoveryEscalated,
		Reason: "retry terminal validator", Actor: "maintainer@example.com",
	}
	recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		predecessor.State.LineageID, predecessor.Revision, repair.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
	published, err := RecoverCompactAuthority(context.Background(), repo, recovery)
	if err != nil {
		t.Fatalf("post-reconcile recover: %v", err)
	}
	leaves, err = CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != published.State.LineageID {
		t.Fatalf("post-recover leaves = %#v, %v", leaves, err)
	}
}

func TestReconcilePreContractAuthorizationRefusesIneligibleEdges(t *testing.T) {
	assertUntouched := func(t *testing.T, repo string, store CompactStore, payload []byte) {
		t.Helper()
		current, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(current, payload) {
			t.Fatalf("refused pre-contract reconcile mutated the successor entry: %q, %v", current, err)
		}
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "quarantine"))
		if err == nil && len(entries) != 0 {
			t.Fatalf("refused pre-contract reconcile left quarantine entries: %#v", entries)
		}
	}

	t.Run("structurally inconsistent edge is corruption, not legacy format", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, func(state *CompactState) {
			state.Generation = 1
		})
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, preContractReconcileRequest(predecessor, successor)); err == nil ||
			!strings.Contains(err.Error(), "outside the unchanged-target class and the pre-contract authorization class") ||
			!strings.Contains(err.Error(), "generation must follow predecessor") {
			t.Fatalf("structurally inconsistent pre-contract reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("v1 binding bound to different content is corruption", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, "placeholder", func(state *CompactState) {
			state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
				state.Recovery.PredecessorLineageID, state.Recovery.PredecessorRevision,
				state.InitialSnapshot.Identity, state.Recovery.Actor, "different reason")
		})
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, preContractReconcileRequest(predecessor, successor)); err == nil ||
			!strings.Contains(err.Error(), "corruption, not a pre-contract authorization") {
			t.Fatalf("wrong-content v1 binding reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("malformed successor has its own successor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		writeSnapshotFile(t, repo, "tracked.txt", "grandchild target\n")
		grandchild := newCompactTestState(t, repo, "reconcile-grandchild")
		grandchild.Generation = successor.State.Generation + 1
		grandchild.Recovery = &CompactRecoveryProvenance{
			PredecessorLineageID: successor.State.LineageID, PredecessorRevision: successor.Revision,
			Disposition: RecoveryEscalated, Reason: "descendant of the malformed successor", Actor: "maintainer@example.com",
			RecoveredAt:             time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC),
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(successor.State.LineageID, successor.Revision, grandchild.InitialSnapshot.Identity, "maintainer@example.com", "descendant of the malformed successor"),
		}
		grandchildStore, err := CompactAuthoritativeStore(context.Background(), repo, grandchild.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, grandchildStore, grandchild)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, preContractReconcileRequest(predecessor, successor)); err == nil ||
			!strings.Contains(err.Error(), `has its own successor "reconcile-grandchild"`) {
			t.Fatalf("descendant pre-contract reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("malformed edge is not the sole graph anomaly", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		writeReclaimFixtureFile(t, filepath.Join(root, "v2", "reconcile-unrelated", "review-state.json"), "not json\n")
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, preContractReconcileRequest(predecessor, successor)); err == nil ||
			!strings.Contains(err.Error(), `related compact authority "reconcile-unrelated" does not load`) {
			t.Fatalf("unrelated-anomaly pre-contract reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("missing repair authorization", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := preContractReconcileRequest(predecessor, successor)
		request.MaintainerAuthorization = ""
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("missing repair authorization reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})

	t.Run("repair authorization bound to different content", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor, _, successor, successorStore := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
		payload, err := os.ReadFile(successorStore.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		request := preContractReconcileRequest(predecessor, successor)
		request.MaintainerAuthorization = compactReconcileAuthorizationBinding(
			request.PredecessorLineageID, successor.Revision, request.SuccessorLineageID, predecessor.Revision,
			request.Actor, request.Reason)
		if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("mismatched repair authorization reconcile error = %v", err)
		}
		assertUntouched(t, repo, successorStore, payload)
	})
}

func TestReconcileCrashBeforeRenameLeavesPreparedOrphanAndRetrySucceeds(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	original := reclaimQuarantineResidue
	reclaimQuarantineResidue = func(string, string) error {
		return errors.New("simulated crash before residue rename")
	}
	t.Cleanup(func() { reclaimQuarantineResidue = original })
	request := reconcileFixtureRequest(predecessor, successor)
	prepared, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request)
	if err == nil {
		t.Fatal("interrupted reconcile reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" || prepared.InvalidRecoveryEdge == nil {
		t.Fatalf("rename-failure reconcile record = %#v", prepared)
	}
	if !strings.Contains(err.Error(), prepared.QuarantinePath) {
		t.Fatalf("rename failure hides the prepared quarantine location: %v", err)
	}
	if _, statErr := os.Stat(successorStore.StatePath()); statErr != nil {
		t.Fatalf("interrupted reconcile mutated the successor entry: %v", statErr)
	}
	var orphan CompactReclaimRecord
	orphanPayload, err := os.ReadFile(filepath.Join(prepared.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(orphanPayload, &orphan); err != nil {
		t.Fatal(err)
	}
	if orphan.Status != CompactReclaimPrepared || orphan.InvalidRecoveryEdge == nil ||
		!strings.Contains(orphan.InvalidRecoveryEdge.ValidationError, "target has not changed") {
		t.Fatalf("orphan reconcile record = %#v", orphan)
	}
	if _, err := os.Stat(filepath.Join(prepared.QuarantinePath, "residue")); !os.IsNotExist(err) {
		t.Fatalf("orphan quarantine claims residue it never received: %v", err)
	}

	reclaimQuarantineResidue = original
	record, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("retry after interrupted reconcile: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.QuarantinePath == prepared.QuarantinePath {
		t.Fatalf("retry reconcile record = %#v", record)
	}
	if _, err := os.Stat(successorStore.Dir); !os.IsNotExist(err) {
		t.Fatalf("retried reconcile left the successor entry behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "quarantine")); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileCommitRewriteFailureReturnsPreparedRecordWithQuarantine(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, _, successor, successorStore := poisonedRecoveryFixture(t, repo, nil)
	successorPayload, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	original := persistReclaimRecord
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted {
			return errors.New("simulated crash before committed rewrite")
		}
		return original(record)
	}
	t.Cleanup(func() { persistReclaimRecord = original })
	record, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor))
	if err == nil {
		t.Fatal("failed committed rewrite reported success")
	}
	if record.Status != CompactReclaimPrepared || record.QuarantinePath == "" || record.InvalidRecoveryEdge == nil {
		t.Fatalf("committed-rewrite failure reconcile record = %#v", record)
	}
	if !strings.Contains(err.Error(), record.QuarantinePath) {
		t.Fatalf("committed-rewrite failure hides the quarantine location: %v", err)
	}
	if _, statErr := os.Stat(successorStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("committed-rewrite failure left the successor entry behind: %v", statErr)
	}
	moved, readErr := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "review-state.json"))
	if readErr != nil || !bytes.Equal(moved, successorPayload) {
		t.Fatalf("quarantined successor bytes = %q, %v", moved, readErr)
	}

	// The successor entry is gone, so a retry must refuse and point the
	// operator at the quarantine root where the prepared record lives.
	persistReclaimRecord = original
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	_, retryErr := ReconcileInvalidRecoveryEdge(context.Background(), repo, reconcileFixtureRequest(predecessor, successor))
	if !errors.Is(retryErr, os.ErrNotExist) || !strings.Contains(retryErr.Error(), filepath.Join(root, "quarantine")) ||
		!strings.Contains(retryErr.Error(), "reclaim-record.json") {
		t.Fatalf("retry after quarantined successor = %v", retryErr)
	}
}
