package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pristineReviewingFixture persists one pristine reviewing compact lineage
// through the real store write path (repository evidence validated at start).
func pristineReviewingFixture(t *testing.T, repo, lineage string) (CompactRecord, CompactStore) {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record, store
}

// TestStalePristineReviewingLineageHasNoSanctionedExit reproduces the #1441
// deadlock on the pre-abandon remedy surface: once HEAD advances past a
// pristine reviewing lineage, review/invalidate refuses because the live
// snapshot no longer matches, reclaim refuses because the entry holds
// review-state.json, and reconcile-authority refuses because the entry is not
// a recovery successor. The lineage is permanently unretirable.
func TestStalePristineReviewingLineageHasNoSanctionedExit(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\naccidental change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-stale-pristine")

	// HEAD advances; the frozen current-changes snapshot goes stale.
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "advance past the accidental lineage")

	invalidated := record.State
	if err := invalidated.Invalidate("obsolete accidental lineage"); err != nil {
		t.Fatalf("in-memory invalidation: %v", err)
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", invalidated); err == nil ||
		!strings.Contains(err.Error(), "live repository snapshot no longer matches the reviewing authority") {
		t.Fatalf("stale pristine invalidate error = %v", err)
	}

	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "abandon-stale-pristine", Reason: "retire accidental lineage", Actor: "maintainer@example.com",
	}); err == nil || !strings.Contains(err.Error(), "holds authoritative artifact") {
		t.Fatalf("reclaim refusal = %v", err)
	}

	if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, CompactReconcileRequest{
		PredecessorLineageID: "abandon-unrelated-predecessor", ExpectedPredecessorRevision: record.Revision,
		SuccessorLineageID: "abandon-stale-pristine", ExpectedSuccessorRevision: record.Revision,
		Reason: "retire accidental lineage", Actor: "maintainer@example.com",
		MaintainerAuthorization: "irrelevant",
	}); err == nil || !strings.Contains(err.Error(), "is not a recovery successor") {
		t.Fatalf("reconcile-authority refusal = %v", err)
	}
}

// TestPristineInvalidatedLineagePoisonsInventory reproduces the second #1441
// deadlock: a successfully invalidated pristine lineage with no recovery
// successor maps to AuthorityStatusInvalid, forcing Complete=false on the
// whole inventory forever, with reclaim and reconcile both refusing to
// retire it.
func TestPristineInvalidatedLineagePoisonsInventory(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\naccidental change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-pristine-invalidated")
	invalidated := record.State
	if err := invalidated.Invalidate("obsolete accidental lineage"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", invalidated); err != nil {
		t.Fatalf("live-matching invalidate: %v", err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative ||
		!hasAuthorityInventoryStatus(report.Entries, "abandon-pristine-invalidated", AuthorityStatusInvalid) {
		t.Fatalf("invalidated lineage inventory = %#v", report)
	}

	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "abandon-pristine-invalidated", Reason: "retire invalidated lineage", Actor: "maintainer@example.com",
	}); err == nil || !strings.Contains(err.Error(), "holds authoritative artifact") {
		t.Fatalf("reclaim refusal = %v", err)
	}
}

// approvedCompactFixture persists one terminal approved compact lineage with
// its authoritative receipt.
func approvedCompactFixture(t *testing.T, repo, lineage string) (CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record := writeCompactFixtureRecord(t, store, state)
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return record, store
}

func abandonFixtureRequest(record CompactRecord) CompactAbandonRequest {
	request := CompactAbandonRequest{
		LineageID: record.State.LineageID, ExpectedRevision: record.Revision,
		Reason: "retire obsolete accidental lineage", Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = compactAbandonAuthorizationBinding(
		request.LineageID, record.Revision, record.State.InitialSnapshot.Identity, request.Actor, request.Reason)
	return request
}

func TestAbandonStalePristineReviewingQuarantinesEntryAndRestoresInventory(t *testing.T) {
	repo := initSnapshotRepo(t)
	approved, approvedStore := approvedCompactFixture(t, repo, "abandon-unrelated-approved")
	writeSnapshotFile(t, repo, "tracked.txt", "base\naccidental change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-stale-pristine")

	// HEAD advances; abandonment must not require the live worktree to match.
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "advance past the accidental lineage")

	approvedStateBefore, err := os.ReadFile(approvedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	approvedReceiptBefore, err := os.ReadFile(approvedStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	pristinePayload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	request := abandonFixtureRequest(record)
	committed, err := AbandonPristineCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("abandon stale pristine reviewing lineage: %v", err)
	}
	if committed.Schema != CompactReclaimRecordSchema || committed.Status != CompactReclaimCommitted ||
		committed.LineageID != record.State.LineageID || committed.SourcePath != store.Dir ||
		committed.Reason != request.Reason || committed.Actor != request.Actor || committed.ReclaimedAt.IsZero() {
		t.Fatalf("abandonment record = %#v", committed)
	}
	if committed.PristineAbandonment == nil || committed.PristineAbandonment.LineageID != record.State.LineageID ||
		committed.PristineAbandonment.Revision != record.Revision ||
		committed.PristineAbandonment.SnapshotIdentity != record.State.InitialSnapshot.Identity ||
		committed.PristineAbandonment.State != StateReviewing || committed.PristineAbandonment.InvalidationReason != "" {
		t.Fatalf("abandonment proof = %#v", committed.PristineAbandonment)
	}
	if len(committed.Residue) != 1 || committed.Residue[0] != "review-state.json" {
		t.Fatalf("abandonment residue manifest = %#v", committed.Residue)
	}
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(committed.QuarantinePath, filepath.Join(root, "quarantine")+string(os.PathSeparator)) {
		t.Fatalf("quarantine path %q is outside the quarantine root", committed.QuarantinePath)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("abandoned entry still present: %v", statErr)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, pristinePayload) {
		t.Fatalf("quarantined pristine bytes = %q, %v", moved, err)
	}
	var persisted CompactReclaimRecord
	persistedPayload, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(persistedPayload, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status != CompactReclaimCommitted || persisted.PristineAbandonment == nil ||
		persisted.PristineAbandonment.Revision != record.Revision {
		t.Fatalf("persisted abandonment record = %#v", persisted)
	}
	approvedStateAfter, _ := os.ReadFile(approvedStore.StatePath())
	approvedReceiptAfter, _ := os.ReadFile(approvedStore.ReceiptPath())
	if !bytes.Equal(approvedStateBefore, approvedStateAfter) || !bytes.Equal(approvedReceiptBefore, approvedReceiptAfter) {
		t.Fatal("abandonment changed unrelated approved state or receipt bytes")
	}

	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != approved.State.LineageID {
		t.Fatalf("post-abandon leaves = %#v, %v", leaves, err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("post-abandon inventory = %#v", report)
	}

	// Exact replay converges on the committed record without a second quarantine.
	replayed, err := AbandonPristineCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("idempotent abandonment replay: %v", err)
	}
	if replayed.Status != CompactReclaimCommitted || replayed.QuarantinePath != committed.QuarantinePath {
		t.Fatalf("replayed abandonment record = %#v", replayed)
	}
	quarantined, err := os.ReadDir(filepath.Join(root, "quarantine"))
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("replay minted quarantine directories = %#v, %v", quarantined, err)
	}

	// A differing request over the quarantined lineage is refused.
	differing := request
	differing.Reason = "different reason"
	differing.MaintainerAuthorization = compactAbandonAuthorizationBinding(
		differing.LineageID, record.Revision, record.State.InitialSnapshot.Identity, differing.Actor, differing.Reason)
	if _, err := AbandonPristineCompactStore(context.Background(), repo, differing); err == nil ||
		!strings.Contains(err.Error(), "no committed abandonment record matches") {
		t.Fatalf("differing abandonment replay = %v", err)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh work\n")
	if _, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "abandon-fresh")}); err != nil {
		t.Fatalf("post-abandon lineage-less start: %v", err)
	}
}

func TestAbandonPristineInvalidatedRestoresInventory(t *testing.T) {
	repo := initSnapshotRepo(t)
	approvedCompactFixture(t, repo, "abandon-unrelated-approved")
	writeSnapshotFile(t, repo, "tracked.txt", "base\naccidental change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-pristine-invalidated")
	invalidated := record.State
	if err := invalidated.Invalidate("obsolete accidental lineage"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", invalidated); err != nil {
		t.Fatalf("live-matching invalidate: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	before, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Complete || before.Authoritative {
		t.Fatalf("pre-abandon poisoned inventory = %#v", before)
	}

	committed, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record))
	if err != nil {
		t.Fatalf("abandon pristine invalidated lineage: %v", err)
	}
	if committed.Status != CompactReclaimCommitted || committed.PristineAbandonment == nil ||
		committed.PristineAbandonment.State != StateInvalidated ||
		committed.PristineAbandonment.InvalidationReason != "obsolete accidental lineage" {
		t.Fatalf("invalidated abandonment proof = %#v", committed)
	}
	after, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-abandon inventory = %#v", after)
	}
}

func TestAbandonRefusesIneligibleTargetsAndBindings(t *testing.T) {
	repo := initSnapshotRepo(t)

	t.Run("terminal approved lineage", func(t *testing.T) {
		record, store := approvedCompactFixture(t, repo, "abandon-terminal-approved")
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `holds "approved" authority`) {
			t.Fatalf("terminal approved refusal = %v", err)
		}
		if _, err := os.Stat(store.StatePath()); err != nil {
			t.Fatalf("refused abandonment moved the terminal entry: %v", err)
		}
	})

	t.Run("terminal escalated lineage", func(t *testing.T) {
		state := correctedCompactTestState(t, repo, "abandon-terminal-escalated")
		state.State = StateEscalated
		store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record := writeCompactFixtureRecord(t, store, state)
		receipt, err := state.Receipt()
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
			t.Fatal(err)
		}
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `holds "escalated" authority`) {
			t.Fatalf("terminal escalated refusal = %v", err)
		}
	})

	t.Run("lineage with captured review data", func(t *testing.T) {
		state, _ := pendingCompactCorrection(t, repo, "abandon-corrected")
		store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record := writeCompactFixtureRecord(t, store, state)
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `holds "correction_required" authority`) {
			t.Fatalf("captured review data refusal = %v", err)
		}
	})

	t.Run("captured reviewer-results artifact", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\ncaptured results change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-captured-results")
		if err := os.MkdirAll(filepath.Join(store.Dir, CompactReviewerResultsDir), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `authoritative artifact "reviewer-results"`) {
			t.Fatalf("captured reviewer-results refusal = %v", err)
		}
	})

	t.Run("finalize journal artifact", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\nfinalize journal change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-finalize-journal")
		if err := os.WriteFile(filepath.Join(store.Dir, "finalize-attempt-journal.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `authoritative artifact "finalize-attempt-journal.json"`) {
			t.Fatalf("finalize journal refusal = %v", err)
		}
	})

	t.Run("interrupted atomic residue", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\natomic residue change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-atomic-residue")
		if err := os.WriteFile(filepath.Join(store.Dir, ".atomic-12345"), []byte("partial\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
			!strings.Contains(err.Error(), `authoritative artifact ".atomic-12345"`) {
			t.Fatalf("atomic residue refusal = %v", err)
		}
	})

	t.Run("stale expected revision and inexact binding", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\ncas change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-cas")
		payload, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		stale := abandonFixtureRequest(record)
		stale.ExpectedRevision = "sha256:" + strings.Repeat("00", 32)
		if _, err := AbandonPristineCompactStore(context.Background(), repo, stale); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale expected revision = %v", err)
		}
		inexact := abandonFixtureRequest(record)
		inexact.MaintainerAuthorization = compactAbandonAuthorizationBinding(
			record.State.LineageID, record.State.InitialSnapshot.Identity, record.Revision, inexact.Actor, inexact.Reason)
		if _, err := AbandonPristineCompactStore(context.Background(), repo, inexact); err == nil ||
			!strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("inexact binding = %v", err)
		}
		missing := abandonFixtureRequest(record)
		missing.Reason = "  "
		if _, err := AbandonPristineCompactStore(context.Background(), repo, missing); err == nil ||
			!strings.Contains(err.Error(), "non-empty reason and actor") {
			t.Fatalf("missing reason = %v", err)
		}
		current, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(current, payload) {
			t.Fatalf("refused abandonment mutated the entry: %v", err)
		}
	})

	t.Run("superseded lineage", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\nsuperseded change\n")
		record, _ := pristineReviewingFixture(t, repo, "abandon-superseded")
		predecessorState := record.State
		if err := predecessorState.Invalidate("superseded by explicit recovery"); err != nil {
			t.Fatal(err)
		}
		predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessorState.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		predecessor := writeCompactFixtureRecord(t, predecessorStore, predecessorState)
		writeSnapshotFile(t, repo, "tracked.txt", "base\nsuperseded successor change\n")
		successorState := newCompactTestState(t, repo, "abandon-superseded-g2")
		successorState.Generation = predecessorState.Generation + 1
		successorState.Recovery = &CompactRecoveryProvenance{
			PredecessorLineageID: predecessorState.LineageID, PredecessorRevision: predecessor.Revision,
			Disposition: RecoveryInvalidated, Reason: "superseded by explicit recovery", Actor: "maintainer@example.com",
			RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessorState.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity, "maintainer@example.com", "superseded by explicit recovery"),
		}
		successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, successorStore, successorState)
		if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(predecessor)); err == nil ||
			!strings.Contains(err.Error(), "superseded by recovery successor") {
			t.Fatalf("superseded refusal = %v", err)
		}
	})

	t.Run("absent lineage without committed record", func(t *testing.T) {
		request := CompactAbandonRequest{
			LineageID: "abandon-never-existed", ExpectedRevision: "sha256:" + strings.Repeat("11", 32),
			Reason: "retire ghost", Actor: "maintainer@example.com",
			MaintainerAuthorization: "irrelevant",
		}
		if _, err := AbandonPristineCompactStore(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "no committed abandonment record matches") {
			t.Fatalf("absent lineage refusal = %v", err)
		}
	})
}

func TestAbandonCrashBeforeRenameLeavesPreparedRecordAndRetrySucceeds(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\ninterrupted change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-interrupted")
	request := abandonFixtureRequest(record)

	original := reclaimQuarantineResidue
	reclaimQuarantineResidue = func(string, string) error {
		return errors.New("simulated crash before residue rename")
	}
	t.Cleanup(func() { reclaimQuarantineResidue = original })
	prepared, err := AbandonPristineCompactStore(context.Background(), repo, request)
	if err == nil {
		t.Fatal("interrupted abandonment reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" || prepared.PristineAbandonment == nil {
		t.Fatalf("rename-failure record = %#v", prepared)
	}
	if _, statErr := os.Stat(store.StatePath()); statErr != nil {
		t.Fatalf("interrupted abandonment mutated the source entry: %v", statErr)
	}

	reclaimQuarantineResidue = original
	committed, err := AbandonPristineCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("retry after interrupted abandonment: %v", err)
	}
	if committed.Status != CompactReclaimCommitted || committed.QuarantinePath == prepared.QuarantinePath {
		t.Fatalf("retry abandonment record = %#v", committed)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("retried abandonment left the source entry behind: %v", statErr)
	}
}

// TestAbandonCommitRewriteFailureRefusesReplayAgainstPreparedOrphan exercises
// the renamed-but-uncommitted crash window: the entry left v2/ but the audit
// record never reached committed. The replay gate must refuse that orphaned
// prepared state instead of silently reporting the abandonment as committed.
func TestAbandonCommitRewriteFailureRefusesReplayAgainstPreparedOrphan(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\ncommit rewrite change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-commit-rewrite")
	pristinePayload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	request := abandonFixtureRequest(record)

	original := persistReclaimRecord
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted {
			return errors.New("simulated crash before committed rewrite")
		}
		return original(record)
	}
	t.Cleanup(func() { persistReclaimRecord = original })
	prepared, err := AbandonPristineCompactStore(context.Background(), repo, request)
	if err == nil {
		t.Fatal("failed committed rewrite reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" || prepared.PristineAbandonment == nil {
		t.Fatalf("committed-rewrite failure record = %#v", prepared)
	}
	if !strings.Contains(err.Error(), prepared.QuarantinePath) {
		t.Fatalf("committed-rewrite failure hides the quarantine location: %v", err)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("committed-rewrite failure left the source entry behind: %v", statErr)
	}
	moved, readErr := os.ReadFile(filepath.Join(prepared.QuarantinePath, "residue", "review-state.json"))
	if readErr != nil || !bytes.Equal(moved, pristinePayload) {
		t.Fatalf("quarantined pristine bytes = %q, %v", moved, readErr)
	}
	var orphan CompactReclaimRecord
	orphanPayload, err := os.ReadFile(filepath.Join(prepared.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(orphanPayload, &orphan); err != nil {
		t.Fatal(err)
	}
	if orphan.Status != CompactReclaimPrepared || orphan.PristineAbandonment == nil {
		t.Fatalf("orphan abandonment record = %#v", orphan)
	}

	// The entry is gone but only a prepared record exists: an exact replay of
	// the same request must refuse and point at the quarantine root for manual
	// reconciliation, never converge on the uncommitted orphan.
	persistReclaimRecord = original
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, replayErr := AbandonPristineCompactStore(context.Background(), repo, request); replayErr == nil ||
		!strings.Contains(replayErr.Error(), "no committed abandonment record matches") ||
		!strings.Contains(replayErr.Error(), filepath.Join(root, "quarantine")) {
		t.Fatalf("replay against prepared orphan = %v", replayErr)
	}
}

// TestAbandonRefusesCorruptedNonPristineInvalidatedRecord feeds a hand-crafted
// invalidated record that retains captured review data — a shape Invalidate
// can never produce — directly into the store and asserts abandonment fails
// closed without mutating the entry. The refusal fires at the load boundary
// (parseCompactRecord re-runs CompactState.Validate, whose invalidated case
// re-proves the pristine projection); the in-branch recheck in
// AbandonPristineCompactStore is layered defense behind that same predicate.
func TestAbandonRefusesCorruptedNonPristineInvalidatedRecord(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "abandon-corrupted-invalidated")
	// Hand-craft the corruption: flip to invalidated while retaining captured
	// review data, a shape Invalidate can never produce because it requires
	// pristineness before the transition. FollowUps survive every structural
	// validator that runs before the invalidated pristineness re-derivation,
	// so this exercises exactly that class.
	state.FollowUps = []FollowUp{{Observation: "captured review observation", ProofRefs: []string{"proof"}}}
	state.State, state.InvalidationReason = StateInvalidated, "hand-crafted corruption"
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record := writeCompactFixtureRecord(t, store, state)
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record)); err == nil ||
		!strings.Contains(err.Error(), "pristine reviewing authority") {
		t.Fatalf("corrupted invalidated refusal = %v", err)
	}
	current, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused abandonment mutated the corrupted entry: %v", err)
	}
	if _, statErr := os.Stat(store.Dir); statErr != nil {
		t.Fatalf("refused abandonment moved the corrupted entry: %v", statErr)
	}
}
