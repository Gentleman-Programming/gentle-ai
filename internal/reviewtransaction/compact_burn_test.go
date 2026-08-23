package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func compactBurnFixture(t *testing.T, lineage string, state State) (string, string, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if state == StateApproved {
		record, _ := approvedCompactFixture(t, repo, lineage)
		return repo, base, record
	}
	record, _ := pristineReviewingFixture(t, repo, lineage)
	return repo, base, record
}

func compactBurnOwnedPaths(base, lineage string) []string {
	return []string{
		filepath.Join(base, "v2", lineage),
		filepath.Join(base, "effect-markers", "v1", lineage),
		filepath.Join(base, "incidents", lineage),
	}
}

func seedCompactBurnCompanions(t *testing.T, base, lineage string) {
	t.Helper()
	writeStoreResetFile(t, filepath.Join(base, "effect-markers", "v1", lineage, "marker.json"), "marker\n")
	writeStoreResetFile(t, filepath.Join(base, "incidents", lineage, "capture.json"), "capture\n")
}

func assertCompactBurnAbsent(t *testing.T, paths []string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("burn left %s: %v", path, err)
		}
	}
}

func compactBurnTreeDigest(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", rel, info.Mode())
		if info.Mode().IsRegular() {
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hash.Write(payload)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func assertNoCompactBurnStaging(t *testing.T, base, lineage string) {
	if _, err := os.Lstat(filepath.Join(base, ".compact-burn-"+lineage)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("burn left compact staging for %q: %v", lineage, err)
	}
	matches, err := filepath.Glob(filepath.Join(base, ".review-burn-v2-"+lineage+"-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("burn left compact staging for %q: %v", lineage, err)
	}
}

func seedBurnCandidateView(t *testing.T, base string) string {
	t.Helper()
	candidate := filepath.Join(filepath.Dir(base), "candidate-views", "frozen", "README.md")
	writeStoreResetFile(t, candidate, "keep\n")
	return candidate
}

func seedOtherCompactBurnLineage(t *testing.T, repo string) string {
	t.Helper()
	other := newCompactTestState(t, repo, "burn-other-lineage")
	store, err := CompactAuthoritativeStore(context.Background(), repo, other.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, other)
	return store.Dir
}

func assertBurnRefusalPreservesBytes(t *testing.T, base string, paths []string, burn func() error) error {
	t.Helper()
	before := make([]string, 0, len(paths))
	for _, path := range paths {
		before = append(before, compactBurnTreeDigest(t, path))
	}
	err := burn()
	if err == nil {
		t.Fatal("ineligible authority burned")
	}
	for index, path := range paths {
		if got := compactBurnTreeDigest(t, path); got != before[index] {
			t.Fatalf("burn changed %s: got %s, want %s", path, got, before[index])
		}
	}
	return err
}

func TestBurnApprovedCompactAuthorityRemovesOnlyExactOwnedTargets(t *testing.T) {
	const lineage = "burn-approved-compact"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	seedCompactBurnCompanions(t, base, lineage)

	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err != nil {
		t.Fatalf("burn approved compact authority: %v", err)
	}
	assertCompactBurnAbsent(t, compactBurnOwnedPaths(base, lineage))
	assertNoCompactBurnStaging(t, base, lineage)
}

func TestBurnApprovedCompactAuthorityRefusesWithoutChangingBytes(t *testing.T) {
	for _, testCase := range []struct {
		name, revision string
		state          State
	}{
		{name: "wrong revision", revision: "sha256:" + strings.Repeat("0", 64), state: StateApproved},
		{name: "reviewing state", state: StateReviewing},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			const lineage = "burn-preserve"
			repo, base, record := compactBurnFixture(t, lineage, testCase.state)
			seedCompactBurnCompanions(t, base, lineage)
			expected := testCase.revision
			if expected == "" {
				expected = record.Revision
			}
			paths := append(compactBurnOwnedPaths(base, lineage), seedOtherCompactBurnLineage(t, repo), seedBurnCandidateView(t, base))
			assertBurnRefusalPreservesBytes(t, base, paths, func() error {
				return BurnApprovedCompactAuthority(context.Background(), repo, lineage, expected)
			})
		})
	}
}

func TestBurnRenameFaultRollsBackEveryPreDeleteMove(t *testing.T) {
	for _, failed := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("target-%d", failed), func(t *testing.T) {
			const lineage = "burn-rename-rollback"
			repo, base, record := compactBurnFixture(t, lineage, StateApproved)
			seedCompactBurnCompanions(t, base, lineage)
			paths := compactBurnOwnedPaths(base, lineage)
			before := []string{compactBurnTreeDigest(t, paths[0]), compactBurnTreeDigest(t, paths[1]), compactBurnTreeDigest(t, paths[2])}
			stubStoreResetRename(t, func(oldpath, newpath string) error {
				if oldpath == paths[failed] {
					return fmt.Errorf("simulated rename fault at %d", failed)
				}
				return os.Rename(oldpath, newpath)
			})

			if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err == nil {
				t.Fatal("rename fault reported success")
			}
			for index, path := range paths {
				if got := compactBurnTreeDigest(t, path); got != before[index] {
					t.Fatalf("rollback did not restore bytes and modes at %s", path)
				}
			}
			assertNoCompactBurnStaging(t, base, lineage)
		})
	}
}

func TestBurnRenameFaultPreservesStagingWhenRollbackRestoreFails(t *testing.T) {
	const lineage = "burn-rollback-residue"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	seedCompactBurnCompanions(t, base, lineage)
	paths := compactBurnOwnedPaths(base, lineage)
	before := []string{compactBurnTreeDigest(t, paths[0]), compactBurnTreeDigest(t, paths[1]), compactBurnTreeDigest(t, paths[2])}
	forwardFailure := errors.New("simulated later forward rename fault")
	reverseFailure := errors.New("simulated reverse rename fault")
	var staging, stagedAuthority string
	stubStoreResetRename(t, func(oldpath, newpath string) error {
		switch {
		case oldpath == paths[2]:
			return forwardFailure
		case newpath == paths[1] && stagedAuthority != "":
			return reverseFailure
		case oldpath == paths[1] && filepath.Base(newpath) == "effect-marker":
			staging, stagedAuthority = filepath.Dir(newpath), newpath
		}
		return os.Rename(oldpath, newpath)
	})

	err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision)
	if staging == "" || stagedAuthority == "" {
		t.Fatal("test did not observe the staged authority")
	}
	if _, err := os.Stat(stagedAuthority); err != nil {
		t.Fatalf("rollback removed failed-to-restore staged authority %s: %v", stagedAuthority, err)
	}
	if got := compactBurnTreeDigest(t, stagedAuthority); got != before[1] {
		t.Fatalf("rollback changed failed-to-restore authority bytes: got %s, want %s", got, before[1])
	}
	for _, assertion := range []struct {
		path string
		want string
	}{
		{path: paths[0], want: before[0]},
		{path: paths[2], want: before[2]},
	} {
		if got := compactBurnTreeDigest(t, assertion.path); got != assertion.want {
			t.Fatalf("rollback changed %s: got %s, want %s", assertion.path, got, assertion.want)
		}
	}
	var incomplete *ReviewAuthorityBurnIncompleteError
	if !errors.As(err, &incomplete) || len(incomplete.Residue) != 1 || incomplete.Residue[0] != staging || !strings.Contains(err.Error(), staging) {
		t.Fatalf("rollback error = %v, want incomplete residue blocker naming %s", err, staging)
	}
	if !errors.Is(err, forwardFailure) || !errors.Is(err, reverseFailure) {
		t.Fatalf("rollback error = %v, want both forward and reverse failures", err)
	}
}

func TestBurnRetryRepairsInterruptedCompactStaging(t *testing.T) {
	const lineage = "burn-interrupted-staging"
	for _, fault := range []string{"restores", "conflict", "malformed", "mismatch", "missing", "unrestorable"} {
		t.Run(fault, func(t *testing.T) {
			_, base, record := compactBurnFixture(t, lineage, StateApproved)
			expected := record.Revision
			if fault == "mismatch" {
				expected = "sha256:" + strings.Repeat("0", 64)
			}
			staging := compactBurnStagingPath(base, lineage, expected)
			staged := filepath.Join(staging, "authority")
			if fault != "missing" {
				if err := os.MkdirAll(staging, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(base, "v2", lineage), staged); err != nil {
					t.Fatal(err)
				}
			}
			switch fault {
			case "conflict":
				writeStoreResetFile(t, filepath.Join(base, "v2", lineage, "live"), "live")
			case "malformed":
				writeStoreResetFile(t, filepath.Join(staging, "unexpected"), "bad")
			case "missing":
				writeStoreResetFile(t, filepath.Join(staging, "effect-marker", "state"), "state")
			case "unrestorable":
				stubStoreResetRename(t, func(oldpath, newpath string) error {
					if oldpath == staged {
						return errors.New("simulated recovery restore failure")
					}
					return os.Rename(oldpath, newpath)
				})
			}
			err := recoverCompactBurnStaging(base, lineage, expected, new(bool))
			if (err == nil) != (fault == "restores") {
				t.Fatalf("recovery error = %v", err)
			}
			if fault == "restores" {
				if _, err := os.Lstat(filepath.Join(base, "v2", lineage)); err != nil {
					t.Fatal(err)
				}
				assertNoCompactBurnStaging(t, base, lineage)
			} else if _, err := os.Lstat(staging); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBurnLeavesCandidateViewsAndOtherLineagesUntouched(t *testing.T) {
	const lineage = "burn-exact-target"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	other := newCompactTestState(t, repo, "burn-other-lineage")
	otherStore, err := CompactAuthoritativeStore(context.Background(), repo, other.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, otherStore, other)
	candidate := filepath.Join(filepath.Dir(base), "candidate-views", "frozen", "README.md")
	writeStoreResetFile(t, candidate, "keep\n")

	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err != nil {
		t.Fatal(err)
	}
	assertCompactBurnAbsent(t, []string{filepath.Join(base, "v2", lineage)})
	if _, err := os.Stat(otherStore.StatePath()); err != nil {
		t.Fatalf("burn removed another lineage: %v", err)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("burn removed candidate views: %v", err)
	}
}

func TestBurnApprovedCompactAuthorityMaintenanceLeaseFailurePreservesPublishedAuthority(t *testing.T) {
	const lineage = "burn-maintenance-failure"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := record.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(lineage, record.Revision, "maintenance-lease-failure")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	before := compactBurnTreeDigest(t, store.Dir)

	holder, err := acquireMaintenanceLock(context.Background(), filepath.Join(filepath.Dir(base), "REVIEW-MAINTENANCE.lock"), maintenanceShared)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := BurnApprovedCompactAuthority(ctx, repo, lineage, record.Revision); err == nil {
		t.Fatal("burn succeeded while maintenance lease blocked it")
	}
	if got := compactBurnTreeDigest(t, store.Dir); got != before {
		t.Fatalf("maintenance lease failure changed approved authority, receipt, or journal: got %s, want %s", got, before)
	}
	assertNoCompactBurnStaging(t, base, lineage)
}

func TestBurnWaitsForTheMaintenanceLeaseBeforeClassifying(t *testing.T) {
	const lineage = "burn-maintenance-contention"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	holder, err := acquireMaintenanceLock(context.Background(), filepath.Join(filepath.Dir(base), "REVIEW-MAINTENANCE.lock"), maintenanceShared)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	asked := make(chan struct{})
	acquire := storeResetAcquireLease
	stubStoreResetAcquireLease(t, func(ctx context.Context, repo string) (*MaintenanceLock, error) {
		close(asked)
		return acquire(ctx, repo)
	})
	done := make(chan error, 1)
	go func() { done <- BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision) }()
	<-asked
	if _, err := os.Stat(filepath.Join(base, "v2", lineage, compactStateFileName)); err != nil {
		t.Fatalf("burn classified before holding maintenance lease: %v", err)
	}
	if err := holder.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("burn did not resume after maintenance lease release")
	}
}

func TestBurnPartialRemoveReportsNonAuthoritativeResidue(t *testing.T) {
	const lineage = "burn-partial-remove"
	repo, base, record := compactBurnFixture(t, lineage, StateApproved)
	seedCompactBurnCompanions(t, base, lineage)
	retry, removals := false, 0
	stubStoreResetRename(t, func(oldpath, newpath string) error {
		if retry {
			t.Errorf("deleting retry restored or restaged %s", oldpath)
		}
		return os.Rename(oldpath, newpath)
	})
	stubStoreResetRemoveTree(t, func(path string) error {
		removals++
		if removals > 1 {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			return errors.New("simulated retry result loss")
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			if err := os.RemoveAll(filepath.Join(path, entries[0].Name())); err != nil {
				return err
			}
		}
		return errors.New("simulated partial remove failure")
	})

	err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision)
	var incomplete *ReviewAuthorityBurnIncompleteError
	if !errors.As(err, &incomplete) || len(incomplete.Residue) == 0 || !strings.Contains(err.Error(), "non-authoritative residue") {
		t.Fatalf("partial remove = %v, want explicit residue blocker", err)
	}
	assertCompactBurnAbsent(t, compactBurnOwnedPaths(base, lineage))
	retry = true
	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err != nil {
		t.Fatalf("deleting retry: %v", err)
	}
	assertCompactBurnAbsent(t, compactBurnOwnedPaths(base, lineage))
	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err == nil {
		t.Fatal("fresh total absence reported success")
	}
}
