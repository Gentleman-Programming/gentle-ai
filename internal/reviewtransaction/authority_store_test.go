package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// fixtureNewLineageAuthority builds a valid NewLineageAuthority, reusing
// shadow_relation_test.go's fixtureCandidateIdentity (same package, same
// real-Git-repo characterization suite) rather than a second candidate
// identity fixture.
func fixtureNewLineageAuthority(lineageID string, state NewLineageState) NewLineageAuthority {
	return NewLineageAuthority{
		LineageID:             lineageID,
		State:                 state,
		CandidateIdentity:     fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "c", "d"),
		Tier:                  RiskLow,
		CorrectionBudgetLines: 0,
	}
}

func readDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// TestAuthorityStoreNewLineageWritesExactlyTwoArtifacts is task 3.1's RED
// test (spec rdd-authority-store, "New lineage writes exactly two
// artifacts"): from start through receipt issuance, the v3 lineage
// directory holds review-state.json and, once issued, review-receipt.json —
// nothing else.
func TestAuthorityStoreNewLineageWritesExactlyTwoArtifacts(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "two-artifact-lineage")
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureNewLineageAuthority("two-artifact-lineage", NewLineageStateReviewing)
	revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = authority
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readDirNames(t, store.Dir); !equalStringSlices(got, []string{"review-state.json"}) {
		t.Fatalf("artifacts after start = %v, want [review-state.json]", got)
	}

	revision, err = store.Mutate(context.Background(), revision, func(next *NewLineageAuthority) error {
		next.State = NewLineageStateApproved
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteReceipt(context.Background(), NewLineageReceipt{
		Schema: NewLineageReceiptSchema, LineageID: "two-artifact-lineage", TerminalState: NewLineageStateApproved,
		AuthorityRevision: revision, CandidateIdentity: authority.CandidateIdentity,
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"review-receipt.json", "review-state.json"}
	if got := readDirNames(t, store.Dir); !equalStringSlices(got, want) {
		t.Fatalf("artifacts after receipt = %v, want %v", got, want)
	}
}

// TestAuthorityStoreMutateRefusesStaleExpectedRevision is task 3.2's first
// RED assertion (spec rdd-authority-store, "Stale revision refuses the
// write"): a CAS mismatch refuses and leaves the artifact unchanged.
func TestAuthorityStoreMutateRefusesStaleExpectedRevision(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "stale-revision-lineage")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = fixtureNewLineageAuthority("stale-revision-lineage", NewLineageStateReviewing)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	staleExpected := "sha256:" + strings.Repeat("0", 64)
	_, err = store.Mutate(context.Background(), staleExpected, func(next *NewLineageAuthority) error {
		next.State = NewLineageStateApproved
		return nil
	})
	var conflict *NewLineageRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Mutate with stale revision error = %v, want *NewLineageRevisionConflictError", err)
	}
	if conflict.CurrentRevision != revision {
		t.Fatalf("conflict.CurrentRevision = %q, want %q", conflict.CurrentRevision, revision)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("stale-revision Mutate mutated the state artifact")
	}
}

// TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation is
// task 3.2's second RED assertion: an exact-replay request (identical
// digest) resolves from review-state.json alone, without a store write.
func TestAuthorityStoreResolveReplayReturnsStoredTransitionWithoutMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "replay-lineage")
	if err != nil {
		t.Fatal(err)
	}
	transition := json.RawMessage(`{"kind":"continue"}`)
	revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = fixtureNewLineageAuthority("replay-lineage", NewLineageStateReviewing)
		return next.RecordTransition("request-digest-1", transition)
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != revision {
		t.Fatalf("record.Revision = %q, want %q", record.Revision, revision)
	}
	got, isReplay, err := record.ResolveReplay("request-digest-1")
	if err != nil || !isReplay {
		t.Fatalf("ResolveReplay(exact) = (%s, %v, %v), want (_, true, nil)", got, isReplay, err)
	}
	// The stored transition round-trips through MarshalIndent as part of the
	// enclosing record (encoding/json re-indents a nested json.RawMessage
	// along with the rest of the document), so compare parsed JSON rather
	// than exact bytes — the persisted transition is semantically, not
	// byte-for-byte, identical to what RecordTransition was given.
	var gotDecoded, wantDecoded any
	if err := json.Unmarshal(got, &gotDecoded); err != nil {
		t.Fatalf("decode replayed transition: %v", err)
	}
	if err := json.Unmarshal(transition, &wantDecoded); err != nil {
		t.Fatalf("decode fixture transition: %v", err)
	}
	if !reflect.DeepEqual(gotDecoded, wantDecoded) {
		t.Fatalf("ResolveReplay(exact) transition = %s, want %s", got, transition)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ResolveReplay consumed budget: state artifact changed")
	}

	// A genuinely different request while NOT correcting is not a replay,
	// and is not itself refused — only an in-flight correction is exclusive.
	_, isReplay, err = record.ResolveReplay("request-digest-2")
	if err != nil || isReplay {
		t.Fatalf("ResolveReplay(different, reviewing) = (_, %v, %v), want (_, false, nil)", isReplay, err)
	}
}

// TestAuthorityStoreResolveReplayRefusesSecondCorrectionAttempt is task
// 3.2's third RED assertion (design decision 3, "different request in
// correcting ⇒ refuse (budget spent)"): once a lineage is correcting, only
// the exact replay of the correction already recorded is accepted again.
func TestAuthorityStoreResolveReplayRefusesSecondCorrectionAttempt(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "one-correction-lineage")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = fixtureNewLineageAuthority("one-correction-lineage", NewLineageStateCorrecting)
		return next.RecordTransition("correction-request-1", json.RawMessage(`{"kind":"collect"}`))
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if _, isReplay, err := record.ResolveReplay("correction-request-1"); err != nil || !isReplay {
		t.Fatalf("ResolveReplay(exact correction replay) = (_, %v, %v), want (_, true, nil)", isReplay, err)
	}
	_, isReplay, err := record.ResolveReplay("correction-request-2")
	if !errors.Is(err, ErrOneCorrectionOnly) {
		t.Fatalf("ResolveReplay(second correction) error = %v, want ErrOneCorrectionOnly", err)
	}
	if isReplay {
		t.Fatal("ResolveReplay(second correction) reported isReplay = true")
	}
}

// TestAuthorityStoreCrashResidueDoesNotCorruptPublishedState is task 3.4's
// crash/replay integration test: a stray, incompletely-published temp file
// left behind by an interrupted writeAtomic call (store.go:947) never
// becomes a third readable artifact and never displaces the last
// successfully published record.
func TestAuthorityStoreCrashResidueDoesNotCorruptPublishedState(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "crash-residue-lineage")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = fixtureNewLineageAuthority("crash-residue-lineage", NewLineageStateReviewing)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash between writeAtomic's temp-file write and its rename:
	// a stray .atomic-* file exists in the lineage directory, but the
	// publish that would have renamed it over review-state.json never
	// completed.
	stray := filepath.Join(store.Dir, ".atomic-crash-residue")
	if err := os.WriteFile(stray, []byte("truncated payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := store.Load()
	if err != nil {
		t.Fatalf("Load after crash residue: %v", err)
	}
	if record.Revision != revision {
		t.Fatalf("Load after crash residue revision = %q, want %q", record.Revision, revision)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("crash residue altered the published state artifact")
	}

	// A correctly-CAS'd Mutate still succeeds: the stray file does not wedge
	// the store lock or the CAS comparison.
	next, err := store.Mutate(context.Background(), revision, func(authority *NewLineageAuthority) error {
		authority.State = NewLineageStateCorrecting
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate after crash residue: %v", err)
	}
	if next == revision {
		t.Fatal("Mutate after crash residue did not advance the revision")
	}
}

// TestAuthorityStoreReceiptImmutableAfterIssuance is task 3.5 (spec
// rdd-authority-store, "Post-issuance write attempt is rejected"): once
// review-receipt.json is written, a differing WriteReceipt is refused and
// the original bytes are unchanged; a byte-identical re-issue is idempotent.
func TestAuthorityStoreReceiptImmutableAfterIssuance(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "receipt-immutable-lineage")
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureNewLineageAuthority("receipt-immutable-lineage", NewLineageStateReviewing)
	revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = authority
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err = store.Mutate(context.Background(), revision, func(next *NewLineageAuthority) error {
		next.State = NewLineageStateApproved
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := NewLineageReceipt{
		Schema: NewLineageReceiptSchema, LineageID: "receipt-immutable-lineage", TerminalState: NewLineageStateApproved,
		AuthorityRevision: revision, CandidateIdentity: authority.CandidateIdentity,
	}
	if err := store.WriteReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	// Differs only in a field WriteReceipt's authority-binding checks do not
	// compare (the opaque issued-transition payload), so it reaches
	// publishImmutable and is refused there — the immutability boundary
	// under test, not the earlier binding-mismatch guard.
	differing := receipt
	differing.IssuedTransition = json.RawMessage(`{"kind":"different"}`)
	err = store.WriteReceipt(context.Background(), differing)
	var conflict *ImmutablePublicationConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("WriteReceipt(differing) error = %v, want *ImmutablePublicationConflictError", err)
	}
	after, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected receipt write changed the receipt artifact")
	}

	if err := store.WriteReceipt(context.Background(), receipt); err != nil {
		t.Fatalf("idempotent re-issue of the identical receipt: %v", err)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
