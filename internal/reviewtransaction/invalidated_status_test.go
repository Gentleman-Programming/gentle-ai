package reviewtransaction

import (
	"context"
	"testing"
)

func TestInventoryAuthorityKeepsValidInvalidatedAuthoritiesCompleteAndAuthoritative(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")

	compact := newCompactTestState(t, repo, "invalidated-compact-standalone")
	store, err := CompactAuthoritativeStore(context.Background(), repo, compact.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", compact)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.Invalidate("candidate no longer applies"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/invalidate", compact); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || report.Status != AuthorityStatusInvalidated {
		t.Fatalf("standalone invalidated compact report = %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].Status != AuthorityStatusInvalidated ||
		report.Entries[0].State != StateInvalidated || len(report.Entries[0].Problems) != 0 {
		t.Fatalf("standalone invalidated compact entry = %#v", report.Entries)
	}
}
