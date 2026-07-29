package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestInventoryAuthorityReportsActiveMalformedAndMixedCollisionWithoutMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "active-lineage")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "v1", "active-lineage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v1", "active-lineage", "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "v2", "broken-lineage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v2", "broken-lineage", "review-state.json"), []byte("{broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, root)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != ReviewAuthorityStatusSchema || report.Complete {
		t.Fatalf("report header = %#v", report)
	}
	if !hasAuthorityInventoryStatus(report.Entries, "active-lineage", AuthorityStatusCollision) ||
		!hasAuthorityInventoryStatus(report.Entries, "broken-lineage", AuthorityStatusInvalid) {
		t.Fatalf("inventory entries = %#v", report.Entries)
	}
	if after := authorityBytes(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("read-only authority inventory changed authority bytes")
	}
}

func TestInventoryAuthorityReportsResetResidueAndOwnedLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	lineage := filepath.Join(root, "v2", "reset-lineage")
	if err := os.MkdirAll(lineage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineage, ".atomic-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireStoreLock(filepath.Join(root, "v2", "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if err := os.MkdirAll(filepath.Join(root, "v1", "ambiguous-lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v1", "ambiguous-lock", "LOCK"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, "reset-lineage", AuthorityStatusReset) {
		t.Fatalf("reset report = %#v", report)
	}
	if len(report.Locks) != 2 || report.Locks[1].Status != AuthorityLockOwned || report.Locks[1].Owner != nil ||
		report.Locks[0].Status != AuthorityLockAmbiguous || report.Locks[0].Problem == "" {
		t.Fatalf("lock evidence = %#v", report.Locks)
	}
}

func TestInventoryAuthorityDistinguishesReleasedBusyAndMalformedLockTruth(t *testing.T) {
	for _, tt := range []struct {
		name        string
		content     string
		hold        bool
		want        AuthorityLockStatus
		complete    bool
		wantProblem bool
	}{
		{
			name: "released metadata", complete: true, want: AuthorityLockReleased,
			content: `{"schema":"gentle-ai.review-store-lock/v1","owner_id":"released","pid":42,"host":"old-host","acquired_at":"2026-07-14T00:00:00Z"}` + "\n",
		},
		{name: "busy advisory lock", hold: true, complete: true, want: AuthorityLockOwned},
		{name: "malformed metadata", content: "not-json\n", want: AuthorityLockAmbiguous, wantProblem: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(root, "v2", "LOCK")
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.content != "" {
				if err := os.WriteFile(lockPath, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if tt.hold {
				if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			var release func() error
			if tt.hold {
				if runtime.GOOS == "windows" {
					file, openErr := os.OpenFile(lockPath, os.O_RDWR, 0o600)
					if openErr != nil {
						t.Fatal(openErr)
					}
					locked, lockErr := tryLockFile(file)
					if lockErr != nil || !locked {
						t.Fatalf("hold advisory lock = %t, %v", locked, lockErr)
					}
					release = func() error { return errors.Join(unlockFile(file), file.Close()) }
				} else {
					held, lockErr := acquireStoreLock(lockPath)
					if lockErr != nil {
						t.Fatal(lockErr)
					}
					before, err = os.ReadFile(lockPath)
					if err != nil {
						t.Fatal(err)
					}
					release = held.release
				}
				defer func() {
					if release != nil {
						_ = release()
					}
				}()
			}
			report, err := InventoryAuthority(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Locks) != 1 || report.Locks[0].Status != tt.want || report.Locks[0].Owner != nil ||
				report.Complete != tt.complete || (report.Locks[0].Problem != "") != tt.wantProblem {
				t.Fatalf("lock report = %#v", report)
			}
			if release != nil {
				_ = release()
				release = nil
			}
			after, err := os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("lock inventory mutated the existing LOCK inode contents")
			}
		})
	}
}

func TestInventoryLockProbeFailureIsAmbiguousAndNonMutating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "LOCK")
	payload := []byte(`{"schema":"gentle-ai.review-store-lock/v1","owner_id":"old","pid":42,"host":"old-host","acquired_at":"2026-07-14T00:00:00Z"}` + "\n")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	previous := probeExistingStoreLock
	probeExistingStoreLock = func(*os.File) (bool, error) { return false, errors.New("probe unavailable") }
	t.Cleanup(func() { probeExistingStoreLock = previous })
	evidence, exists := inventoryLock(AuthorityVersionCompact, "", path)
	if !exists || evidence.Status != AuthorityLockAmbiguous || !strings.Contains(evidence.Problem, "probe existing lock inode") || evidence.Owner != nil {
		t.Fatalf("probe failure evidence = %#v, exists=%t", evidence, exists)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payload, after) {
		t.Fatal("failed lock probe mutated LOCK")
	}
}

func TestInventoryAuthorityRejectsStructurallyValidReceiptsThatMismatchTerminalAuthority(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		lineage string
		write   func(t *testing.T, repo string)
	}{
		{
			name:    "compact",
			lineage: "compact-stale-receipt",
			write: func(t *testing.T, repo string) {
				t.Helper()
				_, store, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-stale-receipt", []string{})
				receipt.PolicyHash = "sha256:" + strings.Repeat("a", 64)
				payload, err := json.MarshalIndent(receipt, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:    "legacy",
			lineage: "legacy-stale-receipt",
			write: func(t *testing.T, repo string) {
				t.Helper()
				transaction, receipt, _ := nativeGateFixture(t, repo, "legacy-stale-receipt")
				store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
				if err != nil {
					t.Fatal(err)
				}
				appendApprovedStoreChain(t, store, transaction)
				receipt.PolicyHash = "sha256:" + strings.Repeat("a", 64)
				if err := WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			fixture.write(t, repo)

			report, err := InventoryAuthority(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if report.Complete || report.Authoritative || report.Status != AuthorityStatusInvalid || !hasAuthorityInventoryStatus(report.Entries, fixture.lineage, AuthorityStatusInvalid) {
				t.Fatalf("mismatched receipt report = %#v", report)
			}
		})
	}
}

func TestInventoryAuthorityRejectsAmbiguousLockEvidence(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "v2", "LOCK")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative || report.Status != AuthorityStatusInvalid || len(report.Locks) != 1 || report.Locks[0].Status != AuthorityLockAmbiguous {
		t.Fatalf("ambiguous lock report = %#v", report)
	}
}

func TestInventoryAuthorityReportsRecoveredSuccessorAndSupersededPredecessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	predecessor, store, _ := approvedCompactCurrentChangesFixture(t, repo, "recovery-predecessor", []string{})
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "successor candidate\n")
	successor := newCompactTestState(t, repo, "recovery-successor")
	successor.Generation = predecessor.Generation + 1
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) ||
		!hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusRecovered) {
		t.Fatalf("recovery report = %#v", report)
	}
	recovered.State.Generation++
	_, payload, err := makeCompactRecord(recovered.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(store.Dir), successor.LineageID, "review-state.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil {
		t.Fatal("compact leaves accepted a recovery generation gap")
	}
	report, err = InventoryAuthority(context.Background(), repo)
	if err != nil || report.Complete || report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusInvalid) ||
		hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) {
		t.Fatalf("invalid recovery edge report = %#v, %v", report, err)
	}
}

func TestInventoryAuthorityReportsRecoveredInvalidatedSuccessorAndSupersededPredecessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor := newCompactTestState(t, repo, "invalidated-recovery-predecessor")
	store, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.Invalidate("candidate no longer applies"); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/invalidate", predecessor)
	if err != nil {
		t.Fatal(err)
	}

	successor := newCompactTestState(t, repo, "invalidated-recovery-successor")
	successor.Generation = predecessor.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: revision,
		Successor: successor, Disposition: RecoveryInvalidated, Reason: "verification invalidated authority", Actor: "maintainer",
	}); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) ||
		!hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusRecovered) {
		t.Fatalf("invalidated recovery report = %#v", report)
	}
}

func TestInventoryAuthorityKeepsReceiptlessTerminalLegacyChainReadableAsHistorical(t *testing.T) {
	repo := initSnapshotRepo(t)
	transaction, _, _ := nativeGateFixture(t, repo, "legacy-pre-receipt")
	store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, store, transaction)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, root)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || report.Status != AuthorityStatusHistorical ||
		!hasAuthorityInventoryStatus(report.Entries, "legacy-pre-receipt", AuthorityStatusHistorical) {
		t.Fatalf("receiptless terminal legacy report = %#v", report)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == "legacy-pre-receipt" && len(entry.Problems) != 0 {
			t.Fatalf("historical entry reported problems = %#v", entry.Problems)
		}
	}
	if after := authorityBytes(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("read-only authority inventory changed authority bytes")
	}
}

func TestInventoryAuthorityKeepsPresentButMalformedLegacyReceiptInvalid(t *testing.T) {
	repo := initSnapshotRepo(t)
	transaction, _, _ := nativeGateFixture(t, repo, "legacy-corrupt-receipt")
	store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, store, transaction)
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, []byte("{broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative || report.Status != AuthorityStatusInvalid ||
		!hasAuthorityInventoryStatus(report.Entries, "legacy-corrupt-receipt", AuthorityStatusInvalid) {
		t.Fatalf("malformed present receipt report = %#v", report)
	}
}

func TestInventoryAuthorityKeepsNonTerminalReceiptlessLegacyChainActive(t *testing.T) {
	repo := initSnapshotRepo(t)
	transaction, _, _ := nativeGateFixture(t, repo, "legacy-open-review")
	store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	reviewing := transaction
	reviewing.LensResults = nil
	for _, lens := range supportedLenses {
		setLensCounter(&reviewing.Counters, lens, 0)
	}
	reviewing.State = StateReviewing
	reviewing.LedgerHash = ""
	reviewing.EvidenceHash = ""
	reviewing.Release = nil
	reviewing.Counters.FinalVerifications = 0
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: reviewing}); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative ||
		!hasAuthorityInventoryStatus(report.Entries, "legacy-open-review", AuthorityStatusActive) {
		t.Fatalf("non-terminal receiptless legacy report = %#v", report)
	}
}

func authorityBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = payload
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func hasAuthorityInventoryStatus(entries []AuthorityInventoryEntry, lineage string, status AuthorityStatus) bool {
	for _, entry := range entries {
		if entry.LineageID == lineage && entry.Status == status {
			return true
		}
	}
	return false
}
