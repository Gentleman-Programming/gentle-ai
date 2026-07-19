package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// malformedLegacyFreezeFixture persists one shipped legacy-v1 lineage whose
// review/freeze-findings successor captured state the semantic replay
// considers unrelated to the findings freeze — the #1311 corruption class.
// The chain is structurally intact and content-addressed; only the semantic
// historical replay fails.
func malformedLegacyFreezeFixture(t *testing.T, repo, lineage string) (Store, string, string) {
	t.Helper()
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{
		LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1,
		Snapshot: Snapshot{
			Kind: TargetCurrentChanges, BaseTree: tree("a"), CandidateTree: tree("b"),
			PathsDigest: hash("a"), IntendedUntracked: []string{},
			IntendedUntrackedProof: hash("b"), Paths: []string{"internal/example.go"}, Identity: hash("c"),
		},
		PolicyHash: hash("d"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	freeze := historicalFreezeTransition(t, *tx)
	// The persisted event also captured an evidence hash the freeze transition
	// never writes, so the semantic historical replay reports that the freeze
	// changed unrelated transaction state while the event stays structurally
	// valid, content-addressed, and decodable.
	freeze.EvidenceHash = hash("0")
	malformed := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: freeze})
	if _, err := freeze.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	head := writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: malformed, Transaction: freeze})
	return store, head, malformed
}

// TestMalformedLegacyFreezeEventBricksInventoryWithNoFamilyExit reproduces
// #1311: one semantically invalid legacy freeze event marks the lineage
// invalid, forces the whole inventory complete:false/authoritative:false, and
// every existing quarantine-family member refuses the entry.
func TestMalformedLegacyFreezeEventBricksInventoryWithNoFamilyExit(t *testing.T) {
	repo := initSnapshotRepo(t)
	approvedCompactFixture(t, repo, "legacy-quarantine-unrelated-approved")
	store, _, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-broken")

	_, loadErr := store.LoadChain()
	if loadErr == nil || !errors.Is(loadErr, ErrInvalidSuccessor) ||
		!strings.Contains(loadErr.Error(), "historical findings freeze changed unrelated transaction state") {
		t.Fatalf("malformed freeze load error = %v", loadErr)
	}
	t.Logf("verbatim load failure: %v", loadErr)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative || report.Status != AuthorityStatusInvalid ||
		!hasAuthorityInventoryStatus(report.Entries, "legacy-freeze-broken", AuthorityStatusInvalid) {
		t.Fatalf("bricked inventory = %#v", report)
	}
	for _, entry := range report.Entries {
		if entry.LineageID != "legacy-freeze-broken" {
			continue
		}
		if len(entry.Problems) != 1 || !strings.Contains(entry.Problems[0], "historical findings freeze changed unrelated transaction state") {
			t.Fatalf("bricked entry problems = %#v", entry.Problems)
		}
		t.Logf("verbatim inventory classification: status=%s problems=%q", entry.Status, entry.Problems)
	}

	// The audited quarantine family routes the anomaly nowhere: reclaim,
	// reconcile-authority, and abandon all operate on compact-v2 entries only.
	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "legacy-freeze-broken", Reason: "retire malformed legacy history", Actor: "maintainer@example.com",
	}); err == nil || !strings.Contains(err.Error(), "inspect reclaim target") {
		t.Fatalf("reclaim refusal = %v", err)
	} else {
		t.Logf("verbatim reclaim refusal: %v", err)
	}
	if _, err := ReconcileInvalidRecoveryEdge(context.Background(), repo, CompactReconcileRequest{
		PredecessorLineageID: "legacy-quarantine-unrelated-approved", ExpectedPredecessorRevision: hash("1"),
		SuccessorLineageID: "legacy-freeze-broken", ExpectedSuccessorRevision: hash("2"),
		Reason: "retire malformed legacy history", Actor: "maintainer@example.com",
		MaintainerAuthorization: "irrelevant",
	}); err == nil || !strings.Contains(err.Error(), "holds no compact authority state") {
		t.Fatalf("reconcile-authority refusal = %v", err)
	} else {
		t.Logf("verbatim reconcile-authority refusal: %v", err)
	}
	if _, err := AbandonPristineCompactStore(context.Background(), repo, CompactAbandonRequest{
		LineageID: "legacy-freeze-broken", ExpectedRevision: hash("3"),
		Reason: "retire malformed legacy history", Actor: "maintainer@example.com",
		MaintainerAuthorization: "irrelevant",
	}); err == nil || !strings.Contains(err.Error(), "no committed abandonment record matches") {
		t.Fatalf("abandon refusal = %v", err)
	} else {
		t.Logf("verbatim abandon refusal: %v", err)
	}
}

func malformedLegacyFreezeQuarantineRequest(t *testing.T, repo, lineage, head string) LegacyQuarantineRequest {
	t.Helper()
	request := LegacyQuarantineRequest{
		LineageID: lineage, ExpectedRevision: head,
		ExpectedDiagnostic: "historical findings freeze changed unrelated transaction state",
		Disposition:        LegacyMalformedFreezeQuarantineDisposition,
		Reason:             "retire malformed shipped legacy history", Actor: "maintainer@example.com",
	}
	// The command derives the authorization binding over the canonical
	// repository root (filepath.Abs -> EvalSymlinks -> Clean). On Windows CI
	// t.TempDir() yields 8.3 short-name components (e.g. RUNNER~1) that
	// EvalSymlinks expands, so a binding built from the raw repo path would
	// never match. Bind over the same canonical root the command uses.
	_, repository, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request.MaintainerAuthorization = legacyQuarantineAuthorizationBinding(
		repository, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic,
		request.Disposition, request.Actor, request.Reason,
	)
	return request
}

func TestQuarantineMalformedLegacyFreezePreservesHistoryAndRestoresInventory(t *testing.T) {
	repo := initSnapshotRepo(t)
	approved, approvedStore := approvedCompactFixture(t, repo, "legacy-quarantine-unrelated-approved")
	store, head, malformed := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-broken")
	request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-broken", head)

	headBytes, err := os.ReadFile(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(store.Dir, "events", strings.TrimPrefix(malformed, "sha256:")+".json")
	eventBytes, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	approvedStateBefore, err := os.ReadFile(approvedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	approvedReceiptBefore, err := os.ReadFile(approvedStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	committed, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("quarantine malformed legacy freeze: %v", err)
	}
	proof := committed.MalformedLegacyFreeze
	if committed.Status != CompactReclaimCommitted || proof == nil ||
		proof.LineageID != request.LineageID || proof.HeadRevision != head ||
		proof.EventRevision != malformed || proof.Operation != "review/freeze-findings" ||
		proof.Diagnostic != request.ExpectedDiagnostic || proof.Disposition != request.Disposition ||
		!reflect.DeepEqual(proof.ChangedFields, []string{"evidence_hash"}) {
		t.Fatalf("legacy quarantine record = %#v", committed)
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatalf("legacy source still present: %v", err)
	}
	movedHead, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "HEAD"))
	if err != nil || !bytes.Equal(movedHead, headBytes) {
		t.Fatalf("quarantined HEAD = %q, %v", movedHead, err)
	}
	movedEvent, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", "events", filepath.Base(eventPath)))
	if err != nil || !bytes.Equal(movedEvent, eventBytes) {
		t.Fatalf("quarantined freeze event = %q, %v", movedEvent, err)
	}
	approvedStateAfter, _ := os.ReadFile(approvedStore.StatePath())
	approvedReceiptAfter, _ := os.ReadFile(approvedStore.ReceiptPath())
	if !bytes.Equal(approvedStateBefore, approvedStateAfter) || !bytes.Equal(approvedReceiptBefore, approvedReceiptAfter) {
		t.Fatal("legacy quarantine changed unrelated compact authority")
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative ||
		!hasAuthorityInventoryStatus(report.Entries, approved.State.LineageID, AuthorityStatusApproved) {
		t.Fatalf("post-quarantine inventory = %#v", report)
	}

	replayed, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("idempotent legacy quarantine replay: %v", err)
	}
	if replayed.QuarantinePath != committed.QuarantinePath || replayed.Status != CompactReclaimCommitted {
		t.Fatalf("replayed legacy quarantine = %#v", replayed)
	}
}

func TestQuarantineMalformedLegacyFreezeRefusesOutsideExactClass(t *testing.T) {
	t.Run("stale revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-stale")
		request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-stale", head)
		request.ExpectedRevision = hash("f")
		request.MaintainerAuthorization = legacyQuarantineAuthorizationBinding(
			repo, request.LineageID, request.ExpectedRevision, request.ExpectedDiagnostic,
			request.Disposition, request.Actor, request.Reason,
		)
		if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale revision error = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("stale request moved source: %v", err)
		}
	})

	t.Run("unsupported diagnostic and disposition", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-unsupported")
		for _, mutate := range []func(*LegacyQuarantineRequest){
			func(request *LegacyQuarantineRequest) { request.ExpectedDiagnostic = "different diagnostic" },
			func(request *LegacyQuarantineRequest) { request.Disposition = "delete-history" },
		} {
			request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-unsupported", head)
			mutate(&request)
			if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); err == nil ||
				!strings.Contains(err.Error(), "supports only") {
				t.Fatalf("unsupported request error = %v", err)
			}
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("unsupported request moved source: %v", err)
		}
	})

	t.Run("inexact authorization", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-auth")
		request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-auth", head)
		request.MaintainerAuthorization += "\n"
		if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "exact maintainer authorization binding") {
			t.Fatalf("inexact authorization error = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("inexact authorization moved source: %v", err)
		}
	})

	t.Run("mixed store identity", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-collision")
		compact, err := CompactAuthoritativeStore(context.Background(), repo, "legacy-freeze-collision")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(compact.Dir, 0o755); err != nil {
			t.Fatal(err)
		}
		request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-collision", head)
		if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "also exists in compact-v2") {
			t.Fatalf("mixed identity error = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("mixed identity request moved source: %v", err)
		}
	})

	t.Run("active ownership", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-owned")
		lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.release()
		request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-owned", head)
		if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) ||
			!strings.Contains(err.Error(), "active ownership") {
			t.Fatalf("active ownership error = %v", err)
		}
	})

	t.Run("valid legacy history", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, err := AuthoritativeStore(context.Background(), repo, "legacy-freeze-valid")
		if err != nil {
			t.Fatal(err)
		}
		tx, err := NewTransaction(Start{
			LineageID: "legacy-freeze-valid", Mode: ModeOrdinary4R, Generation: 1,
			Snapshot: Snapshot{
				Kind: TargetCurrentChanges, BaseTree: tree("a"), CandidateTree: tree("b"),
				PathsDigest: hash("a"), IntendedUntracked: []string{}, IntendedUntrackedProof: hash("b"),
				Paths: []string{"internal/example.go"}, Identity: hash("c"),
			},
			PolicyHash: hash("d"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.StartReview(); err != nil {
			t.Fatal(err)
		}
		genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
		head := writeStoreEvent(t, store, Record{
			Operation: "review/freeze-findings", PreviousRevision: genesis,
			Transaction: historicalFreezeTransition(t, *tx),
		})
		request := malformedLegacyFreezeQuarantineRequest(t, repo, "legacy-freeze-valid", head)
		if _, err := QuarantineMalformedLegacyFreeze(context.Background(), repo, request); err == nil ||
			!strings.Contains(err.Error(), "no supported malformed findings-freeze event") {
			t.Fatalf("valid legacy refusal = %v", err)
		}
		if _, err := os.Stat(store.Dir); err != nil {
			t.Fatalf("valid legacy request moved source: %v", err)
		}
	})
}
