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
)

func TestReconcileInvalidRecoveryEdgesCommitsAtomicallyAndReplaysExactly(t *testing.T) {
	repo, request := compactBatchPreparedRequest(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	inspectNoError(t, err)
	firstMove := request.ExpectedPlan.QuarantineEntries[0]
	statePath := filepath.Join(base, "v2", firstMove.LineageID, compactStateFileName)
	wantState, err := os.ReadFile(statePath)
	inspectNoError(t, err)

	journal, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("reconcile invalid recovery edges: %v", err)
	}
	if journal.Schema != CompactBatchReconcileJournalSchema || journal.Status != CompactBatchReconcileCommitted ||
		journal.RequestSHA256 == "" || len(journal.Moves) != len(request.ExpectedPlan.QuarantineEntries) ||
		!reflect.DeepEqual(journal.Plan, request.ExpectedPlan) {
		t.Fatalf("committed batch journal = %#v", journal)
	}
	if _, err := os.Stat(compactBatchReconcileMarkerPath(base)); !os.IsNotExist(err) {
		t.Fatalf("committed batch left global marker: %v", err)
	}
	auditPath := filepath.Join(base, filepath.FromSlash(journal.AuditRelativePath))
	persisted, err := readCompactBatchReconcileJournal(auditPath)
	if err != nil || !reflect.DeepEqual(persisted, journal) {
		t.Fatalf("persisted committed journal = %#v, %v", persisted, err)
	}
	gotState, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(journal.Moves[0].QuarantineRelativePath), compactStateFileName))
	if err != nil || !bytes.Equal(gotState, wantState) {
		t.Fatalf("quarantined source bytes changed: %v", err)
	}
	for _, move := range journal.Moves {
		if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(move.SourceRelativePath))); !os.IsNotExist(err) {
			t.Fatalf("source %q remains after commit: %v", move.SourceRelativePath, err)
		}
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) == 0 {
		t.Fatalf("retained authority leaves = %#v, %v", leaves, err)
	}

	replayed, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err != nil || !reflect.DeepEqual(replayed, journal) {
		t.Fatalf("exact committed replay = %#v, %v", replayed, err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("exact replay minted another quarantine: %#v, %v", entries, err)
	}
}

func TestReconcileInvalidRecoveryEdgesRejectsUnexpectedV2ResidueBeforeMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, predecessorStore, _, successorStore := poisonedRecoveryFixture(t, repo, nil)
	if _, err := os.Stat(predecessorStore.ReceiptPath()); err != nil {
		t.Fatalf("valid predecessor receipt: %v", err)
	}
	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	inspectNoError(t, err)
	preparation, err := PrepareCompactBatchReconciliation(
		context.Background(), repo, compactBatchInvalidEdges(report), compactBatchTestActor, compactBatchTestReason,
	)
	inspectNoError(t, err)
	request := compactBatchRequestFromPreparation(preparation)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	inspectNoError(t, err)
	residue := filepath.Join(base, "v2", "unexpected-residue")
	inspectNoError(t, os.WriteFile(residue, []byte("not authority\n"), 0o644))
	incomplete, err := InspectCompactRecoveryEdges(context.Background(), repo)
	inspectNoError(t, err)
	if incomplete.Complete || !reflect.DeepEqual(incomplete.EntryDiagnostics, []CompactRecoveryEntryDiagnostic{{
		LineageID: "unexpected-residue", Problem: compactInspectionEntryUnexpected,
	}}) {
		t.Fatalf("unexpected v2 residue inspection = %#v", incomplete)
	}

	journal, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err == nil || !strings.Contains(err.Error(), "inspection is incomplete") || journal.Status != "" {
		t.Fatalf("unexpected v2 residue reconcile = %#v, %v", journal, err)
	}
	if _, err := os.Stat(successorStore.StatePath()); err != nil {
		t.Fatalf("refused batch moved invalid successor: %v", err)
	}
	if _, err := os.Stat(compactBatchReconcileMarkerPath(base)); !os.IsNotExist(err) {
		t.Fatalf("refused batch persisted a marker: %v", err)
	}
	if predecessor.State.State != StateEscalated {
		t.Fatalf("fixture predecessor state = %q", predecessor.State.State)
	}
}

func TestReconcileInvalidRecoveryEdgesFinalVerificationRejectsLateUnexpectedV2Residue(t *testing.T) {
	repo, request := compactBatchPreparedRequest(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	inspectNoError(t, err)

	originalRename := renameCompactBatchReconcileEntry
	injected := false
	renameCompactBatchReconcileEntry = func(source, destination string) error {
		if err := os.Rename(source, destination); err != nil {
			return err
		}
		if !injected {
			injected = true
			return os.WriteFile(filepath.Join(base, "v2", "unexpected-residue"), []byte("late residue\n"), 0o644)
		}
		return nil
	}
	t.Cleanup(func() { renameCompactBatchReconcileEntry = originalRename })

	journal, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err == nil || !strings.Contains(err.Error(), "retained authority graph is incomplete or invalid") ||
		journal.Status != CompactBatchReconcilePrepared || !injected {
		t.Fatalf("late unexpected v2 residue reconcile = %#v, injected=%v, err=%v", journal, injected, err)
	}
	if _, err := os.Stat(compactBatchReconcileMarkerPath(base)); err != nil {
		t.Fatalf("late residue failure lost prepared marker: %v", err)
	}
}

func TestCompactStoreStaleReadHandlesFailClosedDuringPreparedBatch(t *testing.T) {
	repo, request := compactBatchPreparedRequest(t)
	retained := request.ExpectedPlan.RetainedEntries[0]
	stale, err := CompactAuthoritativeStore(context.Background(), repo, retained.LineageID)
	inspectNoError(t, err)

	originalRename := renameCompactBatchReconcileEntry
	readErrors := map[string]error{}
	renameCompactBatchReconcileEntry = func(source, destination string) error {
		if err := os.Rename(source, destination); err != nil {
			return err
		}
		if len(readErrors) == 0 {
			_, readErrors["load"] = stale.Load()
			_, readErrors["pending-finalize-read-only"] = stale.PendingFinalizeAttemptReadOnly()
			_, readErrors["export-transport"] = stale.ExportTransport()
		}
		return nil
	}
	t.Cleanup(func() { renameCompactBatchReconcileEntry = originalRename })

	journal, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err != nil || journal.Status != CompactBatchReconcileCommitted {
		t.Fatalf("batch with stale read probes = %#v, %v", journal, err)
	}
	for operation, readErr := range readErrors {
		if !errors.Is(readErr, ErrCompactBatchReconcilePrepared) {
			t.Errorf("stale %s error = %v", operation, readErr)
		}
	}
}

func TestReconcileInvalidRecoveryEdgesPartialMoveStaysFailClosedUntilExactReplay(t *testing.T) {
	repo, request := compactBatchPreparedRequest(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	inspectNoError(t, err)

	originalRename := renameCompactBatchReconcileEntry
	moves := 0
	renameCompactBatchReconcileEntry = func(source, destination string) error {
		if moves == 1 {
			return errors.New("injected second move failure")
		}
		moves++
		return os.Rename(source, destination)
	}
	t.Cleanup(func() { renameCompactBatchReconcileEntry = originalRename })

	prepared, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err == nil || prepared.Status != CompactBatchReconcilePrepared || moves != 1 || !strings.Contains(err.Error(), "injected second move failure") {
		t.Fatalf("partial move result = %#v, moves=%d, err=%v", prepared, moves, err)
	}
	if _, err := os.Stat(compactBatchReconcileMarkerPath(base)); err != nil {
		t.Fatalf("partial move lost prepared marker: %v", err)
	}
	if _, err := DiscoverCompactStores(context.Background(), repo); !errors.Is(err, ErrCompactBatchReconcilePrepared) {
		t.Fatalf("partial authority discovery error = %v", err)
	}
	status, err := InventoryAuthority(context.Background(), repo)
	if err != nil || status.Complete || status.Authoritative {
		t.Fatalf("partial authority status = %#v, %v", status, err)
	}

	different := request
	different.Reason = "different operation"
	if _, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, different); err == nil || !strings.Contains(err.Error(), "different prepared batch") {
		t.Fatalf("different replay error = %v", err)
	}

	renameCompactBatchReconcileEntry = originalRename
	committed, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err != nil || committed.Status != CompactBatchReconcileCommitted {
		t.Fatalf("exact partial replay = %#v, %v", committed, err)
	}
	if _, err := os.Stat(compactBatchReconcileMarkerPath(base)); !os.IsNotExist(err) {
		t.Fatalf("exact replay left marker: %v", err)
	}
}

func TestReconcileInvalidRecoveryEdgesRetriesEveryDurabilityBoundary(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*testing.T, string)
	}{
		{
			name: "prepared marker write",
			inject: func(t *testing.T, base string) {
				original := writeCompactBatchReconcileJournalAtomic
				writeCompactBatchReconcileJournalAtomic = func(path string, payload []byte, mode os.FileMode) error {
					if path == compactBatchReconcileMarkerPath(base) {
						return errors.New("injected marker write failure")
					}
					return original(path, payload, mode)
				}
				t.Cleanup(func() { writeCompactBatchReconcileJournalAtomic = original })
			},
		},
		{
			name: "prepared audit write",
			inject: func(t *testing.T, base string) {
				original := writeCompactBatchReconcileJournalAtomic
				writeCompactBatchReconcileJournalAtomic = func(path string, payload []byte, mode os.FileMode) error {
					var journal CompactBatchReconcileJournal
					_ = json.Unmarshal(payload, &journal)
					if path != compactBatchReconcileMarkerPath(base) && journal.Status == CompactBatchReconcilePrepared {
						return errors.New("injected prepared audit write failure")
					}
					return original(path, payload, mode)
				}
				t.Cleanup(func() { writeCompactBatchReconcileJournalAtomic = original })
			},
		},
		{
			name: "post-move directory sync",
			inject: func(t *testing.T, base string) {
				original := syncCompactBatchReconcileDirectory
				failed := false
				syncCompactBatchReconcileDirectory = func(path string) error {
					if !failed && path == filepath.Join(base, "v2") {
						failed = true
						return errors.New("injected v2 sync failure")
					}
					return original(path)
				}
				t.Cleanup(func() { syncCompactBatchReconcileDirectory = original })
			},
		},
		{
			name: "committed audit write",
			inject: func(t *testing.T, base string) {
				original := writeCompactBatchReconcileJournalAtomic
				writeCompactBatchReconcileJournalAtomic = func(path string, payload []byte, mode os.FileMode) error {
					var journal CompactBatchReconcileJournal
					_ = json.Unmarshal(payload, &journal)
					if path != compactBatchReconcileMarkerPath(base) && journal.Status == CompactBatchReconcileCommitted {
						return errors.New("injected committed audit write failure")
					}
					return original(path, payload, mode)
				}
				t.Cleanup(func() { writeCompactBatchReconcileJournalAtomic = original })
			},
		},
		{
			name: "prepared marker removal",
			inject: func(t *testing.T, _ string) {
				original := removeCompactBatchReconcileMarker
				removeCompactBatchReconcileMarker = func(string) error { return errors.New("injected marker removal failure") }
				t.Cleanup(func() { removeCompactBatchReconcileMarker = original })
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, request := compactBatchPreparedRequest(t)
			base, _, err := reviewAuthorityRoot(context.Background(), repo)
			inspectNoError(t, err)
			tt.inject(t, base)
			prepared, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
			if err == nil {
				t.Fatal("injected durability failure was accepted")
			}
			if prepared.Status != "" && prepared.Status != CompactBatchReconcilePrepared {
				t.Fatalf("failure returned non-prepared journal: %#v", prepared)
			}

			writeCompactBatchReconcileJournalAtomic = writeAtomic
			syncCompactBatchReconcileDirectory = SyncReviewDirectory
			removeCompactBatchReconcileMarker = os.Remove
			committed, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
			if err != nil || committed.Status != CompactBatchReconcileCommitted {
				t.Fatalf("retry after durability failure = %#v, %v", committed, err)
			}
		})
	}
}

func TestReconcileInvalidRecoveryEdgesCancellationLeavesReplayableMarker(t *testing.T) {
	repo, request := compactBatchPreparedRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	originalRename := renameCompactBatchReconcileEntry
	renameCompactBatchReconcileEntry = func(source, destination string) error {
		err := os.Rename(source, destination)
		cancel()
		return err
	}
	t.Cleanup(func() { renameCompactBatchReconcileEntry = originalRename })

	prepared, err := ReconcileInvalidRecoveryEdges(ctx, repo, request)
	if !errors.Is(err, context.Canceled) || prepared.Status != CompactBatchReconcilePrepared {
		t.Fatalf("canceled batch = %#v, %v", prepared, err)
	}
	renameCompactBatchReconcileEntry = originalRename
	committed, err := ReconcileInvalidRecoveryEdges(context.Background(), repo, request)
	if err != nil || committed.Status != CompactBatchReconcileCommitted {
		t.Fatalf("replay canceled batch = %#v, %v", committed, err)
	}
}

func compactBatchPreparedRequest(t *testing.T) (string, CompactBatchReconcileRequest) {
	t.Helper()
	repo := initSnapshotRepo(t)
	_, declarations := compactBatchPlanningFixtureInRepo(t, repo)
	preparation, err := PrepareCompactBatchReconciliation(context.Background(), repo, declarations, compactBatchTestActor, compactBatchTestReason)
	inspectNoError(t, err)
	return repo, compactBatchRequestFromPreparation(preparation)
}
