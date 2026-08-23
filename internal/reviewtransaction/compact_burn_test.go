package reviewtransaction

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approvedCompactBurnFixture(t *testing.T, lineage string) (string, string, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	record, store := approvedCompactFixture(t, repo, lineage)
	return repo, base, store, record
}

func seedCompactBurnCompanions(t *testing.T, base, lineage string) {
	t.Helper()
	writeStoreResetFile(t, filepath.Join(base, "effect-markers", "v1", lineage, "marker.json"), "marker\n")
	writeStoreResetFile(t, filepath.Join(base, "incidents", lineage, "capture.json"), "capture\n")
}

func assertCompactBurnAbsent(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("burn left %s: %v", path, err)
		}
	}
}

func TestBurnApprovedCompactAuthorityRemovesOnlyExactOwnedTargets(t *testing.T) {
	const lineage = "burn-approved-compact"
	repo, base, store, record := approvedCompactBurnFixture(t, lineage)
	seedCompactBurnCompanions(t, base, lineage)
	_, otherStore := approvedCompactFixture(t, repo, "burn-other-lineage")

	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err != nil {
		t.Fatalf("burn approved compact authority: %v", err)
	}
	assertCompactBurnAbsent(t,
		store.Dir,
		filepath.Join(base, "effect-markers", "v1", lineage),
		filepath.Join(base, "incidents", lineage),
	)
	if _, err := os.Lstat(otherStore.Dir); err != nil {
		t.Fatalf("burn removed another lineage: %v", err)
	}
}

func TestBurnApprovedCompactAuthorityUsesNoStagingOrReplay(t *testing.T) {
	const lineage = "burn-direct-delete"
	repo, base, store, record := approvedCompactBurnFixture(t, lineage)
	seedCompactBurnCompanions(t, base, lineage)
	renames := 0
	stubStoreResetRename(t, func(oldpath, newpath string) error {
		renames++
		return os.Rename(oldpath, newpath)
	})

	if err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision); err != nil {
		t.Fatal(err)
	}
	if renames != 0 {
		t.Fatalf("direct authority burn used %d staging renames", renames)
	}
	assertCompactBurnAbsent(t, store.Dir)
	matches, err := filepath.Glob(filepath.Join(base, ".review-burn-v2-"+lineage+"-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("direct authority burn left staging: %v", matches)
	}
}

func TestBurnApprovedCompactAuthorityRefusesInexactRevisionWithoutDeletion(t *testing.T) {
	const lineage = "burn-inexact-revision"
	repo, _, store, record := approvedCompactBurnFixture(t, lineage)
	want := "sha256:" + strings.Repeat("0", 64)

	err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, want)
	var conflict *CompactRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("inexact burn error = %v, want revision conflict", err)
	}
	if after, loadErr := store.Load(); loadErr != nil || after.Revision != record.Revision {
		t.Fatalf("inexact burn changed authority: %#v, %v", after, loadErr)
	}
}

func TestBurnApprovedCompactAuthorityKeepsAuthorityWhenCompanionCleanupFails(t *testing.T) {
	const lineage = "burn-companion-cleanup-failure"
	repo, base, store, record := approvedCompactBurnFixture(t, lineage)
	seedCompactBurnCompanions(t, base, lineage)
	companion := filepath.Join(base, "effect-markers", "v1", lineage)
	stubStoreResetRemoveTree(t, func(path string) error {
		if path == companion {
			return errors.New("injected companion cleanup failure")
		}
		return os.RemoveAll(path)
	})

	err := BurnApprovedCompactAuthority(context.Background(), repo, lineage, record.Revision)
	var incomplete *ReviewAuthorityBurnIncompleteError
	if !errors.As(err, &incomplete) || len(incomplete.Residue) != 1 || incomplete.Residue[0] != companion {
		t.Fatalf("companion cleanup error = %v", err)
	}
	if _, loadErr := store.Load(); loadErr != nil {
		t.Fatalf("companion cleanup failure removed authority: %v", loadErr)
	}
}
