package reviewtransaction

import (
	"context"
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
	if report.Schema != ReviewAuthorityStatusSchema {
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
	repo, predecessor, store, record := correctionScopeRecoveryFixture(t, "recovery-predecessor")
	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "recovery-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer",
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
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
	// The generation gap invalidates the SUCCESSOR's edge. The successor stops
	// being authority and its predecessor stops being superseded by it; the
	// predecessor itself was never damaged and comes back as the leaf.
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil {
		t.Fatalf("a recovery generation gap poisoned enumeration: %v", err)
	}
	names := map[string]bool{}
	for _, leaf := range leaves {
		names[leaf.lineageID] = true
	}
	if names[successor.LineageID] {
		t.Fatalf("the generation-gapped successor was enumerated as authority: %#v", names)
	}
	if !names[predecessor.LineageID] {
		t.Fatalf("the undamaged predecessor was excluded: %#v", names)
	}
	report, err = InventoryAuthority(context.Background(), repo)
	if err != nil || !hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusInvalid) ||
		hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) {
		t.Fatalf("invalid recovery edge report = %#v, %v", report, err)
	}
}

func TestInventoryAuthorityReportsRecoveredEscalatedSuccessorAndSupersededPredecessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, _, record := escalatedCompactAuthorityFixture(t, repo, "escalated-recovery-predecessor")
	writeSnapshotFile(t, repo, "tracked.txt", "changed recovery target\n")

	successor := recoveredEvidenceSuccessor(t, repo, predecessor, "escalated-recovery-successor")
	const actor, reason = "maintainer@example.com", "recover escalated review"
	authorization := compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason)
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
		MaintainerAuthorization: authorization,
	}); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || !hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) ||
		!hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusRecovered) {
		t.Fatalf("escalated recovery report = %#v", report)
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
