package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func legacyAliasRepairFixture(t *testing.T, repo, lineage string, operation ...string) (Store, string, string) {
	t.Helper()
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	tx := newTestTransaction(t, ModeOrdinary4R)
	tx.LineageID = lineage
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	if err := freezeTestFindings(tx, []Finding{{ID: "R1-ALIAS", Severity: "CRITICAL"}}); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: head, Transaction: *tx})
	if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-ALIAS", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "historical proof"}}); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: head, Transaction: *tx})
	if err := tx.BeginFix(hash("a")); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/begin-fix", PreviousRevision: head, Transaction: *tx})
	fix := tx.Snapshot
	fix.Kind, fix.BaseTree, fix.CandidateTree, fix.LedgerIDs, fix.Identity = TargetFixDiff, tx.FinalCandidateTree, tree("c"), []string{"R1-ALIAS"}, hash("b")
	if err := tx.CompleteFix(fix, hash("c"), fix.LedgerIDs); err != nil {
		t.Fatal(err)
	}
	// review/complete-fix is the historically valid predecessor used by the
	// affected gentle-pi chains; the following alias is the sole invalid event.
	head = writeStoreEvent(t, store, Record{Operation: "review/complete-fix", PreviousRevision: head, Transaction: *tx})
	legacy := historicalValidationTransition(*tx)
	alias := "review/validate-fix"
	if len(operation) == 1 {
		alias = operation[0]
	}
	bad := writeStoreEvent(t, store, Record{Operation: alias, PreviousRevision: head, Transaction: legacy})
	return store, bad, bad
}

func legacyAliasRepairRequest(t *testing.T, repo, lineage, revision string) LegacyAliasRepairRequest {
	t.Helper()
	_, repository, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := LegacyAliasRepairRequest{
		LineageID: lineage, ExpectedRevision: revision,
		ExpectedDiagnostic: "unsupported historical v1 operation alias",
		Disposition:        LegacyAliasRepairDisposition,
		Reason:             "quarantine exact approved historical alias chain", Actor: "maintainer@example.com",
	}
	request.MaintainerAuthorization = legacyAliasRepairAuthorizationBinding(
		repository, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic,
		request.Disposition, request.Actor, request.Reason,
	)
	return request
}

func TestRepairHistoricalLegacyAliasPreservesEvidenceAndRestoresOnlySafeInventory(t *testing.T) {
	repo := initSnapshotRepo(t)
	approved, approvedStore := approvedCompactFixture(t, repo, "alias-repair-approved")
	store, head, event := legacyAliasRepairFixture(t, repo, "alias-repair-broken")
	request := legacyAliasRepairRequest(t, repo, "alias-repair-broken", head)

	eventPath := filepath.Join(store.Dir, "events", strings.TrimPrefix(event, "sha256:")+".json")
	eventBytes, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	approvedState, err := os.ReadFile(approvedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	committed, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("repair historical alias: %v", err)
	}
	proof := committed.LegacyAliasRepair
	if committed.Status != CompactReclaimCommitted || proof == nil ||
		proof.LineageID != request.LineageID || proof.HeadRevision != head ||
		!strings.Contains(strings.Join(proof.Operations, ","), "review/validate-fix") ||
		proof.Diagnostic != request.ExpectedDiagnostic || proof.Disposition != request.Disposition {
		t.Fatalf("repair record = %#v", committed)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "events", filepath.Base(eventPath)))
	if err != nil || !bytes.Equal(moved, eventBytes) {
		t.Fatalf("quarantined alias event = %q, %v", moved, err)
	}
	if after, err := os.ReadFile(approvedStore.StatePath()); err != nil || !bytes.Equal(after, approvedState) {
		t.Fatalf("unrelated approved authority changed: %v", err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, approved.State.LineageID, AuthorityStatusApproved) {
		t.Fatalf("post-repair inventory = %#v", report)
	}

	replayed, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
	if err != nil || replayed.QuarantinePath != committed.QuarantinePath {
		t.Fatalf("idempotent repair = %#v, %v", replayed, err)
	}
}

func TestRepairHistoricalLegacyCompleteFixAliasUsesValidationCanonicalOperation(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, head, event := legacyAliasRepairFixture(t, repo, "alias-repair-historical-complete", "review/complete-fix")
	request := legacyAliasRepairRequest(t, repo, "alias-repair-historical-complete", head)

	committed, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("repair historical complete-fix alias: %v", err)
	}
	if committed.LegacyAliasRepair == nil || !strings.Contains(strings.Join(committed.LegacyAliasRepair.Operations, ","), "review/complete-fix") {
		t.Fatalf("complete-fix repair record = %#v", committed)
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatalf("historical complete-fix source remains: %v", err)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "events", strings.TrimPrefix(event, "sha256:")+".json"))
	if err != nil || len(moved) == 0 {
		t.Fatalf("quarantined historical complete-fix event = %q, %v", moved, err)
	}
}

func TestRepairHistoricalLegacyAliasLeavesInventoryFailClosedForOtherCorruption(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-first")
	legacyAliasRepairFixture(t, repo, "alias-repair-unhandled", "review/unknown-fix")
	request := legacyAliasRepairRequest(t, repo, "alias-repair-first", head)
	if _, err := RepairHistoricalLegacyAlias(context.Background(), repo, request); err != nil {
		t.Fatal(err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, "alias-repair-unhandled", AuthorityStatusInvalid) {
		t.Fatalf("repair incorrectly made mixed inventory authoritative: %#v", report)
	}
}

func TestRepairHistoricalLegacyAliasRefusesOutsideApprovedClass(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, repo string, store Store, request *LegacyAliasRepairRequest)
		want    string
	}{
		{name: "wrong revision", prepare: func(_ *testing.T, _ string, _ Store, request *LegacyAliasRepairRequest) {
			request.ExpectedRevision = hash("f")
		}, want: ErrConcurrentUpdate.Error()},
		{name: "malformed authorization", prepare: func(_ *testing.T, _ string, _ Store, request *LegacyAliasRepairRequest) {
			request.MaintainerAuthorization += "\n"
		}, want: "exact maintainer authorization binding"},
		{name: "unsupported disposition", prepare: func(_ *testing.T, _ string, _ Store, request *LegacyAliasRepairRequest) {
			request.Disposition = "delete-history"
		}, want: "supports only"},
		{name: "mixed compact authority", prepare: func(t *testing.T, repo string, _ Store, _ *LegacyAliasRepairRequest) {
			compact, err := CompactAuthoritativeStore(context.Background(), repo, "alias-repair-refusal")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(compact.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "also exists in compact-v2"},
		{name: "active ownership", prepare: func(t *testing.T, _ string, store Store, _ *LegacyAliasRepairRequest) {
			lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = lock.release() })
		}, want: "active ownership"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-refusal")
			request := legacyAliasRepairRequest(t, repo, "alias-repair-refusal", head)
			tt.prepare(t, repo, store, &request)
			_, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("repair error = %v, want %q", err, tt.want)
			}
			if _, statErr := os.Stat(store.Dir); statErr != nil {
				t.Fatalf("refused request moved source: %v", statErr)
			}
		})
	}

	t.Run("unknown alias", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-unknown", "review/unknown-fix")
		request := legacyAliasRepairRequest(t, repo, "alias-repair-unknown", head)
		if _, err := RepairHistoricalLegacyAlias(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "unsupported successor anomaly") {
			t.Fatalf("unknown alias repair error = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("unknown alias moved source: %v", err)
		}
	})

	t.Run("valid chain is a no-op refusal", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-valid", "review/validate-fix-delta")
		request := legacyAliasRepairRequest(t, repo, "alias-repair-valid", head)
		if _, err := RepairHistoricalLegacyAlias(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "no approved unsupported") {
			t.Fatalf("valid chain repair error = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("valid no-op moved source: %v", err)
		}
	})
}

func TestRepairHistoricalLegacyAliasLeavesPreparedAuditOnPartialFailure(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-partial")
	request := legacyAliasRepairRequest(t, repo, "alias-repair-partial", head)
	original := reclaimQuarantineResidue
	reclaimQuarantineResidue = func(_, _ string) error { return errors.New("rename failed") }
	t.Cleanup(func() { reclaimQuarantineResidue = original })

	record, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
	if err == nil || record.Status != CompactReclaimPrepared || record.QuarantinePath == "" {
		t.Fatalf("partial repair = %#v, %v", record, err)
	}
	if _, statErr := os.Stat(store.Dir); statErr != nil {
		t.Fatalf("partial failure moved source: %v", statErr)
	}
}

func TestRepairHistoricalLegacyAliasRespectsSharedMaintenanceLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-maintenance-lock")
	lockContext, cancelLock := context.WithTimeout(context.Background(), time.Second)
	defer cancelLock()
	lock, err := AcquireReviewMaintenanceExclusive(lockContext, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	request := legacyAliasRepairRequest(t, repo, "alias-repair-maintenance-lock", head)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := RepairHistoricalLegacyAlias(ctx, repo, request); !errors.Is(err, ErrAuthorityLockCancelled) {
		t.Fatalf("maintenance contention error = %v", err)
	}
}

func TestRepairHistoricalLegacyAliasUsesCommonAuthorityAcrossWorktrees(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-linked-worktree")
	linked := filepath.Join(t.TempDir(), "linked")
	gitSnapshot(t, repo, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { gitSnapshot(t, repo, "worktree", "remove", "--force", linked) })
	request := legacyAliasRepairRequest(t, linked, "alias-repair-linked-worktree", head)
	if _, err := RepairHistoricalLegacyAlias(context.Background(), linked, request); err != nil {
		t.Fatalf("repair through linked worktree: %v", err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil || !report.Complete || !report.Authoritative {
		t.Fatalf("primary worktree inventory after linked repair = %#v, %v", report, err)
	}
}

func TestRepairHistoricalLegacyAliasConcurrentRequestsDoNotCorruptAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "alias-repair-concurrent")
	request := legacyAliasRepairRequest(t, repo, "alias-repair-concurrent", head)
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			_, err := RepairHistoricalLegacyAlias(context.Background(), repo, request)
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("concurrent repair did not commit or replay")
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil || !report.Complete || !report.Authoritative {
		t.Fatalf("inventory after concurrent repair = %#v, %v", report, err)
	}
}
