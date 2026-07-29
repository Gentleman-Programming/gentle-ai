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

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := NewTransaction(Start{LineageID: "invalidated-legacy-standalone", Mode: ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: hash("a")})
	if err != nil || transaction.StartReview() != nil {
		t.Fatalf("create legacy review: %v", err)
	}
	legacy, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := legacy.Append("", Record{Operation: "review/start", Transaction: *transaction})
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Invalidate("candidate no longer applies"); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Append(genesis, Record{Operation: "review/invalidate", Transaction: *transaction}); err != nil {
		t.Fatal(err)
	}

	report, err = InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || report.Status != AuthorityStatusActive || len(report.Entries) != 2 {
		t.Fatalf("two-authority invalidated report = %#v", report)
	}
	for _, entry := range report.Entries {
		if entry.Status != AuthorityStatusInvalidated || entry.State != StateInvalidated || len(entry.Problems) != 0 {
			t.Fatalf("invalidated entry = %#v", entry)
		}
	}
}

func TestInventoryAuthorityKeepsMalformedInvalidatedAuthorityFailClosed(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "invalidated-with-receipt")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Invalidate("candidate no longer applies"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/invalidate", state); err != nil {
		t.Fatal(err)
	}
	terminal := newCompactTestState(t, repo, "invalidated-with-receipt-terminal")
	terminal.State = StateEscalated
	receipt, err := terminal.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative ||
		!hasAuthorityInventoryStatus(report.Entries, state.LineageID, AuthorityStatusInvalid) {
		t.Fatalf("malformed invalidated report = %#v", report)
	}
}
