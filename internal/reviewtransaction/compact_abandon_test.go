package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// pristineReviewingFixture persists one pristine reviewing compact lineage
// through the real store write path (repository evidence validated at start).
func pristineReviewingFixture(t *testing.T, repo, lineage string) (CompactRecord, CompactStore) {
	t.Helper()
	_, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, lineage))
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record, store
}

// TestStalePristineReviewingLineageHasNoSanctionedExit reproduces the #1441
// deadlock on the pre-abandon remedy surface: once HEAD advances past a
// pristine reviewing lineage, review/invalidate refuses because the live
// snapshot no longer matches, and reclaim refuses because the entry holds
// review-state.json. (A third refusal used to be checked here:
// reconcile-authority, because the entry is not a recovery successor --
// that verb and its provider retired in Wave 7 S3a/S3b, so there is one
// fewer surface to try, not one more exit.) The lineage is permanently
// unretirable.
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
}

// TestPristineInvalidatedLineageRemainsAuditable proves that a successfully
// invalidated pristine lineage remains complete inventory authority without
// becoming usable delivery authority.
func TestPristineInvalidatedLineageRemainsAuditable(t *testing.T) {
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
	if !report.Complete || !report.Authoritative || report.Status != AuthorityStatusInvalidated ||
		!hasAuthorityInventoryStatus(report.Entries, "abandon-pristine-invalidated", AuthorityStatusInvalidated) {
		t.Fatalf("invalidated lineage inventory = %#v", report)
	}

	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "abandon-pristine-invalidated", Reason: "retire invalidated lineage", Actor: "maintainer@example.com",
	}); err == nil || !strings.Contains(err.Error(), "holds authoritative artifact") {
		t.Fatalf("reclaim refusal = %v", err)
	}
}

// approvedCompactFixture closes a clean reviewing authority through the
// current last-event lifecycle before persisting terminal compact state.
func approvedCompactFixture(t *testing.T, repo, lineage string) (CompactRecord, CompactStore) {
	t.Helper()
	state, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, lineage))
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	state, started := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(started.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record, store
}

// authorizeAbandon fills the request's expected revision and maintainer
// authorization from the read-only inspection, unless the request already
// carries them or the lineage is not present. Tests that pin the binding's
// exact text build it by hand instead.
func authorizeAbandon(t *testing.T, ctx context.Context, repo string, request CompactAbandonRequest) CompactAbandonRequest {
	t.Helper()
	if request.MaintainerAuthorization != "" || strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return request
	}
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, request.LineageID)
	if err != nil || !eligibility.Eligible {
		return request
	}
	request.ExpectedRevision = eligibility.Revision
	request.MaintainerAuthorization = eligibility.Authorization(request.LineageID, request.Actor, request.Reason)
	return request
}

func abandonAuthorized(t *testing.T, ctx context.Context, repo string, request CompactAbandonRequest) (CompactAbandonResult, error) {
	t.Helper()
	return AbandonCompactStore(ctx, repo, authorizeAbandon(t, ctx, repo, request))
}

func abandonFixtureRequest(record CompactRecord, _ ...CompactDiscardedWorkSummary) CompactAbandonRequest {
	return CompactAbandonRequest{
		LineageID: record.State.LineageID,
		Reason:    CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
	}
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
	pristinePayload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	request := authorizeAbandon(t, context.Background(), repo, abandonFixtureRequest(record))
	committed, err := abandonAuthorized(t, context.Background(), repo, request)
	if err != nil {
		t.Fatalf("abandon stale pristine reviewing lineage: %v", err)
	}
	if committed.Schema != CompactReclaimRecordSchema || committed.Status != CompactReclaimCommitted ||
		committed.LineageID != record.State.LineageID || committed.SourcePath != store.Dir ||
		committed.Reason != request.Reason || committed.Actor != request.Actor || committed.ReclaimedAt.IsZero() {
		t.Fatalf("abandonment record = %#v", committed)
	}
	proof := committed.Abandonment
	if proof == nil || proof.Schema != CompactAbandonAuthorizationSchema || proof.LineageID != record.State.LineageID ||
		proof.Revision != record.Revision || proof.SnapshotIdentity != record.State.InitialSnapshot.Identity ||
		len(proof.DiscardedWork.CapturedLensResults) != 0 || proof.DiscardedWork.FindingsPresent {
		t.Fatalf("abandonment proof = %#v", proof)
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
	if persisted.Status != CompactReclaimCommitted || persisted.Abandonment == nil ||
		persisted.Abandonment.Schema != CompactAbandonAuthorizationSchema || persisted.Abandonment.Revision != record.Revision {
		t.Fatalf("persisted abandonment record = %#v", persisted)
	}
	approvedStateAfter, _ := os.ReadFile(approvedStore.StatePath())
	if !bytes.Equal(approvedStateBefore, approvedStateAfter) {
		t.Fatal("abandonment changed unrelated approved state bytes")
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
	replayed, err := abandonAuthorized(t, context.Background(), repo, request)
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
	if _, err := abandonAuthorized(t, context.Background(), repo, differing); err == nil ||
		!strings.Contains(err.Error(), "no committed abandonment record matches") {
		t.Fatalf("differing abandonment replay = %v", err)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh work\n")
	if _, err := createAtomicCompactAuthority(t, context.Background(), repo, newCompactTestState(t, repo, "abandon-fresh")); err != nil {
		t.Fatalf("post-abandon lineage-less start: %v", err)
	}
}

func TestAbandonQuarantinesTerminalInvalidatedLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	unrelated, unrelatedStore := approvedCompactFixture(t, repo, "abandon-unrelated-approved")
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
	unrelatedBefore, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	committed, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record))
	if err != nil {
		t.Fatalf("terminal invalidated abandonment: %v", err)
	}
	if committed.Status != CompactReclaimCommitted || len(committed.Cascaded) != 0 {
		t.Fatalf("terminal invalidated record = %#v", committed)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, payload) {
		t.Fatalf("quarantined invalidated bytes = %q, %v", moved, err)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("abandoned entry still present: %v", statErr)
	}
	unrelatedAfter, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil || !bytes.Equal(unrelatedBefore, unrelatedAfter) {
		t.Fatalf("abandonment touched another lineage (%s): %v", unrelated.State.LineageID, err)
	}
}

func TestAbandonAcceptsEveryStateAndRefusesOnlyInvalidRequests(t *testing.T) {
	repo := initSnapshotRepo(t)

	t.Run("terminal approved lineage", func(t *testing.T) {
		record, store := approvedCompactFixture(t, repo, "abandon-terminal-approved")
		committed, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record))
		if err != nil || committed.Status != CompactReclaimCommitted {
			t.Fatalf("terminal approved abandonment = %#v, %v", committed, err)
		}
		if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
			t.Fatalf("abandoned approved entry still present: %v", err)
		}
		if committed.Abandonment == nil || committed.Abandonment.Revision != record.Revision {
			t.Fatalf("approved abandonment proof = %#v", committed.Abandonment)
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
		if committed, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record)); err != nil || committed.Status != CompactReclaimCommitted {
			t.Fatalf("terminal escalated abandonment = %#v, %v", committed, err)
		}
	})

	t.Run("lineage with captured review data", func(t *testing.T) {
		state, _ := pendingCompactCorrection(t, repo, "abandon-corrected")
		store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record := writeCompactFixtureRecord(t, store, state)
		summary, err := compactDiscardedWorkSummary(t.Context(), store, record)
		if err != nil {
			t.Fatal(err)
		}
		request := abandonFixtureRequest(record, summary)
		if _, err := abandonAuthorized(t, context.Background(), repo, request); err != nil {
			t.Fatalf("captured review data abandonment = %v", err)
		}
	})

	t.Run("finalize journal artifact", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\nfinalize journal change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-finalize-journal")
		if err := os.WriteFile(filepath.Join(store.Dir, "finalize-attempt-journal.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record)); err != nil {
			t.Fatalf("finalize journal abandonment = %v", err)
		}
	})

	t.Run("interrupted atomic residue", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\natomic residue change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-atomic-residue")
		if err := os.WriteFile(filepath.Join(store.Dir, ".atomic-12345"), []byte("partial\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record)); err != nil {
			t.Fatalf("atomic residue abandonment = %v", err)
		}
	})

	t.Run("missing reason or actor", func(t *testing.T) {
		writeSnapshotFile(t, repo, "tracked.txt", "base\ncas change\n")
		record, store := pristineReviewingFixture(t, repo, "abandon-missing-reason")
		payload, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		missing := abandonFixtureRequest(record)
		missing.Reason = "  "
		if _, err := abandonAuthorized(t, context.Background(), repo, missing); err == nil ||
			!strings.Contains(err.Error(), "non-empty reason and actor") {
			t.Fatalf("missing reason = %v", err)
		}
		current, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(current, payload) {
			t.Fatalf("refused abandonment mutated the entry: %v", err)
		}
	})

	t.Run("absent lineage without committed record", func(t *testing.T) {
		request := CompactAbandonRequest{
			LineageID: "abandon-never-existed", ExpectedRevision: "r", Reason: "retire ghost", Actor: "maintainer@example.com",
		}
		if _, err := abandonAuthorized(t, context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "no committed abandonment record matches") {
			t.Fatalf("absent lineage refusal = %v", err)
		}
	})
}

func TestAbandonBindsActualAdmittedReviewerResult(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "abandon-admitted-result")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	summary, err := compactDiscardedWorkSummary(context.Background(), fixture.store, record)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := abandonAuthorized(t, context.Background(), fixture.store.repo, abandonFixtureRequest(record, summary))
	if err != nil {
		t.Fatalf("abandon admitted reviewer result: %v", err)
	}
	if got := committed.Abandonment.DiscardedWork.CapturedLensResults; len(got) != 1 || got[0] != "00-"+LensReliability {
		t.Fatalf("discarded admitted reviewer result = %v", got)
	}
}

// TestAbandonSummaryUsesAdmittedValuesInsteadOfDuplicateProjections proves the
// discarded-work summary preserves the captured inventory and findings semantics
// when compatibility projections are forged.
func TestAbandonSummaryUsesAdmittedValuesInsteadOfDuplicateProjections(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "abandon-tampered-projections")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := compactDiscardedWorkSummary(t.Context(), fixture.store, record)
	if err != nil {
		t.Fatal(err)
	}
	// Retired projections cannot be assembled into CompactState; semantic reads
	// must stay anchored to the admitted role value in the stored record.

	summary, err := compactDiscardedWorkSummary(t.Context(), fixture.store, record)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summary, baseline) {
		t.Fatalf("discarded work from tampered projections = %#v, want admitted summary %#v", summary, baseline)
	}
}

// TestAbandonSummaryDegradesToUnverifiableOnMalformedAdmittedEvidence: an
// admitted value that cannot be re-derived no longer refuses the abandonment;
// the discarded-work summary says it is unverifiable instead.
func TestAbandonSummaryDegradesToUnverifiableOnMalformedAdmittedEvidence(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "abandon-malformed-admitted")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	tampered := record
	tampered.State.AdmittedRoleResults = cloneCompactAdmittedRoleResults(record.State.AdmittedRoleResults)
	tampered.State.AdmittedRoleResults[0].Value = json.RawMessage(`{"malformed":`)
	if _, err := compactDiscardedWorkSummary(t.Context(), fixture.store, tampered); err == nil {
		t.Fatal("malformed admitted evidence produced a verified summary")
	}
	summary, err := compactAbandonDiscardedWork(t.Context(), fixture.store, tampered)
	if err != nil || !summary.Unverifiable {
		t.Fatalf("malformed admitted evidence summary = %#v, %v", summary, err)
	}
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
	prepared, err := abandonAuthorized(t, context.Background(), repo, request)
	if err == nil {
		t.Fatal("interrupted abandonment reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" || prepared.Abandonment == nil {
		t.Fatalf("rename-failure record = %#v", prepared)
	}
	if _, statErr := os.Stat(store.StatePath()); statErr != nil {
		t.Fatalf("interrupted abandonment mutated the source entry: %v", statErr)
	}

	reclaimQuarantineResidue = original
	committed, err := abandonAuthorized(t, context.Background(), repo, request)
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
	request := authorizeAbandon(t, context.Background(), repo, abandonFixtureRequest(record))

	original := persistReclaimRecord
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted {
			return errors.New("simulated crash before committed rewrite")
		}
		return original(record)
	}
	t.Cleanup(func() { persistReclaimRecord = original })
	prepared, err := abandonAuthorized(t, context.Background(), repo, request)
	if err == nil {
		t.Fatal("failed committed rewrite reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" || prepared.Abandonment == nil {
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
	if orphan.Status != CompactReclaimPrepared || orphan.Abandonment == nil {
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
	if _, replayErr := abandonAuthorized(t, context.Background(), repo, request); replayErr == nil ||
		!strings.Contains(replayErr.Error(), "no committed abandonment record matches") ||
		!strings.Contains(replayErr.Error(), filepath.Join(root, "quarantine")) {
		t.Fatalf("replay against prepared orphan = %v", replayErr)
	}
}

// TestAbandonQuarantinesCorruptedNonPristineInvalidatedRecord feeds a
// hand-crafted invalidated record that retains captured review data — a shape
// Invalidate can never produce — directly into the store. The record cannot be
// loaded, so abandonment quarantines the entry whole without interpreting it,
// preserving the exact corrupted bytes.
func TestAbandonQuarantinesCorruptedNonPristineInvalidatedRecord(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "abandon-corrupted-invalidated")
	// Hand-craft the corruption: flip to invalidated while retaining captured
	// review data, a shape Invalidate can never produce because it requires
	// pristineness before the transition. FollowUps survive every structural
	// validator that runs before the invalidated pristineness re-derivation,
	// so this exercises exactly that class.
	state, store := startReviewingCompactAuthority(t, repo, state)
	captureCompactLens(t, store, state, 0)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	record.State.State, record.State.InvalidationReason = StateInvalidated, "hand-crafted corruption"
	statePayload, err := json.Marshal(record.State)
	if err != nil {
		t.Fatal(err)
	}
	record.Revision = compactStateRevision(statePayload)
	corrupted, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), append(corrupted, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	payload = append(corrupted, '\n')

	committed, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record))
	if err != nil || committed.Status != CompactReclaimCommitted {
		t.Fatalf("corrupted invalidated abandonment = %#v, %v", committed, err)
	}
	if proof := committed.Abandonment; proof == nil || !proof.DiscardedWork.Unverifiable || proof.Revision != compactAbandonUnavailable || proof.SnapshotIdentity != compactAbandonUnavailable {
		t.Fatalf("uninterpreted abandonment proof = %#v", committed.Abandonment)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, payload) {
		t.Fatalf("quarantined corrupted bytes = %q, %v", moved, err)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("abandoned corrupted entry still present: %v", statErr)
	}
}

// recoveryChainFixture persists a recovery chain of n lineages (index 0 is the
// root predecessor) through the same fixture path the recovery tests use, and
// returns the records in chain order.
func recoveryChainFixture(t *testing.T, repo, prefix string, n int) []CompactRecord {
	t.Helper()
	chain := make([]CompactRecord, 0, n)
	for index := 0; index < n; index++ {
		writeSnapshotFile(t, repo, "tracked.txt", "base\n"+prefix+" chain change "+strings.Repeat("x", index+1)+"\n")
		state := newCompactTestState(t, repo, prefix+"-g"+string(rune('1'+index)))
		if index > 0 {
			predecessor := chain[index-1]
			state.Generation = predecessor.State.Generation + 1
			state.Recovery = &CompactRecoveryProvenance{
				PredecessorLineageID: predecessor.State.LineageID, PredecessorRevision: predecessor.Revision,
				Disposition: RecoveryInvalidated, Reason: "superseded by explicit recovery", Actor: "maintainer@example.com",
				RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
				MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, state.InitialSnapshot.Identity, "maintainer@example.com", "superseded by explicit recovery"),
			}
		}
		if index < n-1 {
			if err := state.Invalidate("superseded by explicit recovery"); err != nil {
				t.Fatal(err)
			}
		}
		store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		chain = append(chain, writeCompactFixtureRecord(t, store, state))
	}
	return chain
}

func abandonEntryAbsent(t *testing.T, repo, lineage string) bool {
	t.Helper()
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(filepath.Join(base, "v2", lineage))
	return os.IsNotExist(statErr)
}

// TestAbandonCascadesToRecoverySuccessorsWithoutOrphanEdges: abandoning a
// predecessor abandons every transitive successor with it, deepest first, and
// never touches ancestors or unrelated lineages. The rule is the smallest that
// leaves no dangling recovery edge: a successor cannot outlive the
// predecessor its edge names, and an ancestor does not depend on its
// descendants.
func TestAbandonCascadesToRecoverySuccessorsWithoutOrphanEdges(t *testing.T) {
	repo := initSnapshotRepo(t)
	unrelated, unrelatedStore := approvedCompactFixture(t, repo, "abandon-cascade-unrelated")
	chain := recoveryChainFixture(t, repo, "abandon-cascade", 3)
	unrelatedBefore, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	// Abandon the MIDDLE lineage: it and its successor go, the root stays.
	cascadeRequest := authorizeAbandon(t, context.Background(), repo, abandonFixtureRequest(chain[1]))
	committed, err := AbandonCompactStore(context.Background(), repo, cascadeRequest)
	if err != nil {
		t.Fatalf("cascade abandonment: %v", err)
	}
	if committed.Status != CompactReclaimCommitted || committed.LineageID != chain[1].State.LineageID {
		t.Fatalf("primary record = %#v", committed.CompactReclaimRecord)
	}
	if len(committed.Cascaded) != 1 || committed.Cascaded[0].LineageID != chain[2].State.LineageID ||
		committed.Cascaded[0].Status != CompactReclaimCommitted || committed.Cascaded[0].Abandonment == nil ||
		committed.Cascaded[0].Abandonment.Revision != chain[2].Revision {
		t.Fatalf("cascaded records = %#v", committed.Cascaded)
	}
	if !abandonEntryAbsent(t, repo, chain[1].State.LineageID) || !abandonEntryAbsent(t, repo, chain[2].State.LineageID) {
		t.Fatal("cascade left an abandoned entry in the v2 store")
	}
	if abandonEntryAbsent(t, repo, chain[0].State.LineageID) {
		t.Fatal("cascade quarantined the ancestor")
	}
	unrelatedAfter, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil || !bytes.Equal(unrelatedBefore, unrelatedAfter) {
		t.Fatalf("cascade touched an unrelated lineage (%s): %v", unrelated.State.LineageID, err)
	}
	scan, err := scanCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if violations, _ := compactAuthorityGraphViolations(scan.records); len(violations) != 0 {
		t.Fatalf("cascade left graph defects (orphan edge): %v", violations)
	}

	// Exact replay converges on the committed primary record.
	replayed, err := AbandonCompactStore(context.Background(), repo, cascadeRequest)
	if err != nil || replayed.QuarantinePath != committed.QuarantinePath {
		t.Fatalf("cascade replay = %#v, %v", replayed, err)
	}

	// A leaf abandons alone; abandoning the root afterwards has nothing left to cascade to.
	alone, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(chain[0]))
	if err != nil || len(alone.Cascaded) != 0 {
		t.Fatalf("root abandonment = %#v, %v", alone, err)
	}
}

// TestAbandonQuarantinesUnreadableRecordWithoutInterpretingIt: a record that
// cannot be loaded is moved whole and byte-for-byte; no field of it is read.
func TestAbandonQuarantinesUnreadableRecordWithoutInterpretingIt(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, unrelatedStore := approvedCompactFixture(t, repo, "abandon-unreadable-unrelated")
	unrelatedBefore, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\nunreadable change\n")
	_, store := pristineReviewingFixture(t, repo, "abandon-unreadable")
	truncated := truncateCompactStateFile(t, store)

	committed, err := abandonAuthorized(t, context.Background(), repo, CompactAbandonRequest{
		LineageID: "abandon-unreadable", Reason: CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatalf("abandon unreadable record: %v", err)
	}
	proof := committed.Abandonment
	if committed.Status != CompactReclaimCommitted || proof == nil || !proof.DiscardedWork.Unverifiable ||
		proof.Revision != compactAbandonUnavailable || proof.SnapshotIdentity != compactAbandonUnavailable {
		t.Fatalf("unreadable abandonment record = %#v", committed.CompactReclaimRecord)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", compactStateFileName))
	if err != nil || !bytes.Equal(moved, truncated) {
		t.Fatalf("quarantined unreadable bytes = %q, %v", moved, err)
	}
	if !abandonEntryAbsent(t, repo, "abandon-unreadable") {
		t.Fatal("unreadable entry still present")
	}
	unrelatedAfter, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil || !bytes.Equal(unrelatedBefore, unrelatedAfter) {
		t.Fatalf("unreadable abandonment touched another lineage: %v", err)
	}
}

// TestAbandonAbsorbsInterruptedStartWithoutReviewState: an entry with no
// review-state.json (a START that died before publishing), including one that
// already holds authoritative-looking artifacts reclaim refuses, is
// quarantined whole by abandon.
func TestAbandonAbsorbsInterruptedStartWithoutReviewState(t *testing.T) {
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for name, files := range map[string][]string{
		"abandon-start-empty":    nil,
		"abandon-start-residue":  {".atomic-9999", "review-receipt.json"},
		"abandon-start-other":    {"notes.txt"},
		"abandon-start-no-state": {"finalize-attempt-journal.json"},
	} {
		dir := filepath.Join(base, "v2", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("partial\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		committed, err := abandonAuthorized(t, context.Background(), repo, CompactAbandonRequest{
			LineageID: name, Reason: CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
		})
		if err != nil || committed.Status != CompactReclaimCommitted || len(committed.Residue) != len(files) {
			t.Fatalf("%s: abandon = %#v, %v", name, committed, err)
		}
		for _, file := range files {
			if _, statErr := os.Stat(filepath.Join(committed.QuarantinePath, "residue", file)); statErr != nil {
				t.Fatalf("%s: %s not preserved: %v", name, file, statErr)
			}
		}
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			t.Fatalf("%s: entry still present: %v", name, statErr)
		}
	}
}

// TestAbandonQuarantinesLegacyV1CollisionAlongsideV2: a lineage id present in
// both stores has both entries quarantined, v2 reported as the primary.
func TestAbandonQuarantinesLegacyV1CollisionAlongsideV2(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\ncollision change\n")
	_, store := pristineReviewingFixture(t, repo, "abandon-collision")
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	v1Dir := filepath.Join(base, "v1", "abandon-collision")
	if err := os.MkdirAll(v1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1Dir, "HEAD"), []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	committed, err := abandonAuthorized(t, context.Background(), repo, CompactAbandonRequest{
		LineageID: "abandon-collision", Reason: CompactAbandonReasonOperatorDisposition, Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatalf("abandon v1/v2 collision: %v", err)
	}
	if committed.SourcePath != store.Dir || len(committed.Cascaded) != 1 || committed.Cascaded[0].SourcePath != v1Dir ||
		committed.Cascaded[0].Status != CompactReclaimCommitted {
		t.Fatalf("collision records = %#v", committed)
	}
	moved, err := os.ReadFile(filepath.Join(committed.Cascaded[0].QuarantinePath, "residue", "HEAD"))
	if err != nil || string(moved) != "legacy\n" {
		t.Fatalf("v1 residue = %q, %v", moved, err)
	}
	if _, statErr := os.Stat(v1Dir); !os.IsNotExist(statErr) || !abandonEntryAbsent(t, repo, "abandon-collision") {
		t.Fatal("collision entries still present")
	}
}

// TestAbandonFailsFastWithLockBusyAndMutatesNothingWhileStoreIsHeld: a live
// holder of the v2 store lock makes abandon fail with the lock-busy error
// before any entry moves.
func TestAbandonFailsFastWithLockBusyAndMutatesNothingWhileStoreIsHeld(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nlocked change\n")
	record, store := pristineReviewingFixture(t, repo, "abandon-locked")
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	holder, err := acquireStoreLock(filepath.Join(base, "v2", "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	_, abandonErr := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record))
	holder.release()
	if !errors.Is(abandonErr, ErrStoreLockContended) || !errors.Is(abandonErr, ErrConcurrentUpdate) {
		t.Fatalf("abandon under a held lock = %v", abandonErr)
	}
	current, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("lock-busy abandon mutated the entry: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "quarantine")); !os.IsNotExist(statErr) {
		t.Fatalf("lock-busy abandon created a quarantine: %v", statErr)
	}
}

// TestAbandonResolvesSeveralLineagesStuckInReviewing is the #1793 shape: four
// lineages stuck in `reviewing` are each abandoned with no authority edits,
// the others stay byte-identical until their own turn, and the store ends
// authoritative and empty.
func TestAbandonResolvesSeveralLineagesStuckInReviewing(t *testing.T) {
	repo := initSnapshotRepo(t)
	stuck := make([]CompactRecord, 0, 4)
	stores := map[string]CompactStore{}
	for index := 0; index < 4; index++ {
		writeSnapshotFile(t, repo, "tracked.txt", "base\nstuck change "+strings.Repeat("y", index+1)+"\n")
		record, store := pristineReviewingFixture(t, repo, "stuck-reviewing-"+string(rune('a'+index)))
		stuck = append(stuck, record)
		stores[record.State.LineageID] = store
		gitSnapshot(t, repo, "add", "-A")
		gitSnapshot(t, repo, "commit", "-m", "advance past stuck lineage")
	}
	for index, record := range stuck {
		before := map[string][]byte{}
		for _, other := range stuck[index+1:] {
			payload, err := os.ReadFile(stores[other.State.LineageID].StatePath())
			if err != nil {
				t.Fatal(err)
			}
			before[other.State.LineageID] = payload
		}
		committed, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record))
		if err != nil || committed.Status != CompactReclaimCommitted || len(committed.Cascaded) != 0 {
			t.Fatalf("abandon %s = %#v, %v", record.State.LineageID, committed, err)
		}
		for lineage, payload := range before {
			current, readErr := os.ReadFile(stores[lineage].StatePath())
			if readErr != nil || !bytes.Equal(current, payload) {
				t.Fatalf("abandoning %s changed %s: %v", record.State.LineageID, lineage, readErr)
			}
		}
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil || !report.Complete || !report.Authoritative || len(report.Entries) != 0 {
		t.Fatalf("post-abandon inventory = %#v, %v", report, err)
	}
	if _, err := createAtomicCompactAuthority(t, context.Background(), repo, newCompactTestState(t, repo, "stuck-fresh-start")); err != nil {
		t.Fatalf("post-abandon start: %v", err)
	}
}

// TestInspectCompactAbandonmentIsEligibleForEveryPresentEntry pins the
// read-only prediction to the operation: any v2 or v1 entry is eligible, only
// an absent lineage is not.
func TestInspectCompactAbandonmentIsEligibleForEveryPresentEntry(t *testing.T) {
	repo := initSnapshotRepo(t)
	approved, _ := approvedCompactFixture(t, repo, "abandon-eligible-approved")
	got, err := InspectCompactPristineAbandonment(context.Background(), repo, approved.State.LineageID)
	if err != nil || !got.Eligible || got.Revision != approved.Revision {
		t.Fatalf("approved eligibility = %#v, %v", got, err)
	}
	got, err = InspectCompactPristineAbandonment(context.Background(), repo, "abandon-eligible-absent")
	if err != nil || got.Eligible {
		t.Fatalf("absent eligibility = %#v, %v", got, err)
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "v2", "abandon-eligible-empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = InspectCompactPristineAbandonment(context.Background(), repo, "abandon-eligible-empty")
	if err != nil || !got.Eligible {
		t.Fatalf("empty entry eligibility = %#v, %v", got, err)
	}
}
