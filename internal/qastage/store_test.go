package qastage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// PersistentQAStateStore is QA's own, self-contained CAS + append-only record
// store. It replaces the originally planned OpenRuntimeStoreAdapter: v3.7.0's
// sddstatus.RuntimeStore retired its public Begin/Finish surface (see
// internal/cli/sdd_attempt.go: "Runtime attempt admission, settlement and
// budgets are retired"), so there is nothing SDD-shaped left to adapt. See
// docs/migration/qa-orchestrator-v3-design.md, section 7 (3A.3), for the
// verified incompatibility and this minimal adaptation.

func TestPersistentQAStateStore_HeadIsEmptyInitially(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())

	head, err := store.Head(context.Background(), "some-change")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if head.Revision != "" {
		t.Fatalf("expected an empty head for a never-used change, got %+v", head)
	}
}

func TestPersistentQAStateStore_AppendCreatesFirstRecord(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())
	ctx := context.Background()

	record, err := store.Append(ctx, "change-a", "", []byte(`{"stage":"explore"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Revision == "" {
		t.Fatal("expected a non-empty revision for the first record")
	}
	if record.Parent != "" {
		t.Fatalf("expected an empty parent for the first record, got %q", record.Parent)
	}

	head, err := store.Head(ctx, "change-a")
	if err != nil {
		t.Fatalf("unexpected error reading head: %v", err)
	}
	if head.Revision != record.Revision {
		t.Fatalf("expected head to reflect the just-appended record, got %+v vs %+v", head, record)
	}
	if string(head.Data) != `{"stage":"explore"}` {
		t.Fatalf("expected head data to round-trip, got %q", head.Data)
	}
}

func TestPersistentQAStateStore_AppendChainsOnCorrectExpectedRevision(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())
	ctx := context.Background()

	first, err := store.Append(ctx, "change-b", "", []byte("first"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := store.Append(ctx, "change-b", first.Revision, []byte("second"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Parent != first.Revision {
		t.Fatalf("expected second.Parent=%q, got %q", first.Revision, second.Parent)
	}

	head, err := store.Head(ctx, "change-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if head.Revision != second.Revision {
		t.Fatal("expected head to advance to the second record")
	}
}

func TestPersistentQAStateStore_AppendRejectsStaleExpectedRevision(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())
	ctx := context.Background()

	first, err := store.Append(ctx, "change-c", "", []byte("first"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = store.Append(ctx, "change-c", "sha256:0000000000000000000000000000000000000000000000000000000000000000", []byte("conflicting"))
	if err == nil {
		t.Fatal("expected a conflict error for a stale expected-revision")
	}
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a *RevisionConflictError, got %T: %v", err, err)
	}

	head, err := store.Head(ctx, "change-c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if head.Revision != first.Revision {
		t.Fatal("expected head to remain unchanged after a rejected append")
	}
}

func TestPersistentQAStateStore_HistoryIsAppendOnly(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())
	ctx := context.Background()

	first, err := store.Append(ctx, "change-d", "", []byte("first"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := store.Append(ctx, "change-d", first.Revision, []byte("second"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	history, err := store.History(ctx, "change-d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 records in history, got %d", len(history))
	}
	if history[0].Revision != first.Revision || history[1].Revision != second.Revision {
		t.Fatalf("expected history in append order (oldest first), got %+v", history)
	}
}

func TestPersistentQAStateStore_ConcurrentAppendsOnlyOneWins(t *testing.T) {
	store := NewPersistentQAStateStore(t.TempDir())
	ctx := context.Background()

	first, err := store.Append(ctx, "change-e", "", []byte("first"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const attempts = 8
	var successes int64
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			if _, err := store.Append(ctx, "change-e", first.Revision, []byte("racer")); err == nil {
				atomic.AddInt64(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 of %d concurrent appends on the same expected-revision to win, got %d", attempts, successes)
	}

	history, err := store.History(ctx, "change-e")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected exactly 2 records (first + the single winner), got %d", len(history))
	}
}
