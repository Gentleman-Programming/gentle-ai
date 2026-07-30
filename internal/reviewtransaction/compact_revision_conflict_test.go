package reviewtransaction

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// TestCompactRevisionConflictIsTypedAndProvesNonMutation is the store-side half
// of the classification fix. A lost compare-and-set on the compact revision is
// not an unknown outcome: the CAS is the gate in front of the only write in
// replaceContextGuarded, so a refusal there proves the authoritative state file
// was not touched. The refusal must say so in a form a caller can branch on,
// while staying inside the ErrConcurrentUpdate chain every bounded retry helper
// in this package already keys off.
func TestCompactRevisionConflictIsTypedAndProvesNonMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, store, revision := persistedCompactCorrectionRequired(t, repo, "compact-revision-conflict")

	winner := state
	if err := winner.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	committed, err := store.Replace(revision, "review/begin-fix", winner)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	loser := state
	if err := loser.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace(revision, "review/begin-fix", loser)
	var conflict *CompactRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale-revision commit error = %v, want a typed *CompactRevisionConflictError", err)
	}
	if conflict.Expected != revision || conflict.Current != committed {
		t.Fatalf("revision conflict = %#v, want expected %q and current %q", conflict, revision, committed)
	}
	// Compatibility: every bounded retry helper and every existing caller
	// branches on ErrConcurrentUpdate, so typing the refusal must not remove it
	// from the chain.
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("typed revision conflict left the ErrConcurrentUpdate chain: %v", err)
	}
	// And it must not impersonate the narrower "never acquired the lock"
	// sentinel: this caller did acquire the lock and then lost the CAS.
	if errors.Is(err, ErrStoreLockContended) {
		t.Fatalf("revision conflict claims advisory contention it never hit: %v", err)
	}

	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused compare-and-set mutated the authoritative compact state bytes")
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != committed {
		t.Fatalf("authority after the refused compare-and-set = %#v, %v; want revision %q", loaded, err, committed)
	}
}
