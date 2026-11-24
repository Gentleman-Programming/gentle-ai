package reviewtransaction

import (
	"context"
	"testing"
)

func TestInvalidatePristineKeepsCompactTerminal(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")

	compact := newCompactTestState(t, repo, "invalidate-compact")
	compactStore, _ := CompactAuthoritativeStore(context.Background(), repo, compact.LineageID)
	revision, err := compactStore.Replace("", "review/start", compact)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.Invalidate("operator abandoned"); err != nil {
		t.Fatal(err)
	}
	invalidated, err := compactStore.Replace(revision, "review/invalidate", compact)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := compactStore.Replace(revision, "review/invalidate", compact); err != nil || retry != invalidated {
		t.Fatalf("compact exact retry = %q, %v", retry, err)
	}
}

func TestInvalidatePristineRejectsLiveCandidateDriftWithoutMutation(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(t *testing.T, repo string)
	}{
		{"unstaged", func(t *testing.T, repo string) { writeSnapshotFile(t, repo, "tracked.txt", "changed after review\n") }},
		{"staged", func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "tracked.txt", "changed after review\n")
			gitSnapshot(t, repo, "add", "tracked.txt")
		}},
		{"commit-a", func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "tracked.txt", "changed after review\n")
			gitSnapshot(t, repo, "commit", "-am", "drift")
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			state := newCompactTestState(t, repo, "invalidate-drift-"+mutate.name)
			store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
			revision, err := store.Replace("", "review/start", state)
			if err != nil {
				t.Fatal(err)
			}
			mutate.apply(t, repo)
			if err := state.Invalidate("operator abandoned"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Replace(revision, "review/invalidate", state); err == nil {
				t.Fatal("live candidate drift was accepted")
			}
			loaded, err := store.Load()
			if err != nil || loaded.Revision != revision || loaded.State.State != StateReviewing {
				t.Fatalf("drift changed compact authority: %#v, %v", loaded, err)
			}
		})
	}
}
