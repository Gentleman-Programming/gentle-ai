package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func injectRetiredCompactStateField(t *testing.T, statePath, field string) string {
	t.Helper()
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	state, ok := record["state"].(map[string]any)
	if !ok {
		t.Fatal("compact record has no state object")
	}
	state[field] = true
	statePayload, err := json.Marshal(record["state"])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(CompactStateSchema+"\x00"), statePayload...))
	record["revision"] = "sha256:" + hex.EncodeToString(sum[:])
	updated, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(updated, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return record["revision"].(string)
}

func injectRetiredCompactNestedStateField(t *testing.T, statePath, parent, field string, value any) string {
	t.Helper()
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	state, ok := record["state"].(map[string]any)
	if !ok {
		t.Fatal("compact record has no state object")
	}
	nested, ok := state[parent].(map[string]any)
	if !ok {
		t.Fatalf("compact state has no %q object", parent)
	}
	nested[field] = value
	statePayload, err := json.Marshal(record["state"])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(CompactStateSchema+"\x00"), statePayload...))
	record["revision"] = "sha256:" + hex.EncodeToString(sum[:])
	updated, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(updated, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return record["revision"].(string)
}

// historicalRecoveredSuccessorFixture builds an invalidated predecessor and a
// real recovered successor so tests can retrofit the retired nested
// recovery.review_start field older builds persisted.
func historicalRecoveredSuccessorFixture(t *testing.T, repo string) (CompactState, CompactState, CompactStore) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	predecessor := newCompactTestState(t, repo, "recovered-start-predecessor")
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := predecessorStore.Replace("", "review/start", predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.Invalidate("recovered-start fixture predecessor"); err != nil {
		t.Fatal(err)
	}
	revision, err = predecessorStore.Replace(revision, "review/invalidate", predecessor)
	if err != nil {
		t.Fatal(err)
	}
	successor := newCompactTestState(t, repo, "recovered-start-successor")
	successor.Generation = predecessor.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: revision,
		Successor: successor, Disposition: RecoveryInvalidated, Reason: "verification invalidated authority", Actor: "maintainer",
	}); err != nil {
		t.Fatal(err)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return predecessor, successor, successorStore
}

// retiredRecoveryReviewStartValue mimics the recovered-start provenance object
// a pre-release local build nested under state.recovery.
func retiredRecoveryReviewStartValue() map[string]any {
	return map[string]any{"operation": "review/start", "resumed": true}
}

func TestCompactStoreLoadsHistoricalRecoveryReviewStartReadOnly(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, successor, store := historicalRecoveredSuccessorFixture(t, repo)
	revision := injectRetiredCompactNestedStateField(t, store.StatePath(), "recovery", "review_start", retiredRecoveryReviewStartValue())
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical recovery.review_start record: %v", err)
	}
	if record.Revision != revision || record.State.LineageID != successor.LineageID || !record.HistoricalCompat {
		t.Fatalf("historical record = %#v, want read-only revision %s", record, revision)
	}
	if record.State.Recovery == nil || record.State.Recovery.PredecessorLineageID != predecessor.LineageID {
		t.Fatalf("historical record dropped recovery provenance: %#v", record.State.Recovery)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err != nil {
		t.Fatalf("historical record poisoned the compact authority graph: %v", err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !hasAuthorityInventoryStatus(report.Entries, successor.LineageID, AuthorityStatusRecovered) ||
		!hasAuthorityInventoryStatus(report.Entries, predecessor.LineageID, AuthorityStatusSuperseded) {
		t.Fatalf("historical recovered inventory = %#v", report)
	}
	next := record.State
	if err := next.Invalidate("historical maintenance"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", next); err == nil || !errors.Is(err, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical mutation error = %v", err)
	}
	if _, err := store.ExportTransport(); err == nil || !errors.Is(err, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical transport export error = %v", err)
	}
	if after, err := os.ReadFile(store.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("historical authority bytes changed: %v", err)
	}
}

func TestCompactStoreStillRejectsUnknownNestedRecoveryFields(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, store := historicalRecoveredSuccessorFixture(t, repo)
	injectRetiredCompactNestedStateField(t, store.StatePath(), "recovery", "bogus", true)
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown nested non-retired field load error = %v", err)
	}
}

func TestCompactStoreStillRejectsTopLevelReviewStartField(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "top-level-review-start-lineage")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	// review_start is retired only nested under recovery; the same name at the
	// top level was never persisted and must stay strictly rejected.
	injectRetiredCompactStateField(t, store.StatePath(), "review_start")
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), `unknown field "review_start"`) {
		t.Fatalf("top-level review_start load error = %v", err)
	}
}

func TestNewCompactAuthorityNeverPersistsRetiredRecoveryReviewStart(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, successor, store := historicalRecoveredSuccessorFixture(t, repo)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := makeCompactRecord(record.State)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("review_start")) {
		t.Fatal("new compact authority serialized the retired recovery.review_start field")
	}
	if record.HistoricalCompat {
		t.Fatalf("clean recovered successor %q is marked as historical compatibility authority", successor.LineageID)
	}
}

func TestCompactStoreLoadsHistoricalZeroEditEscalationReadOnly(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "historical-lineage")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	revision := injectRetiredCompactStateField(t, store.StatePath(), "zero_edit_escalation")
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical zero_edit_escalation record: %v", err)
	}
	if record.Revision != revision || record.State.LineageID != state.LineageID || !record.HistoricalCompat {
		t.Fatalf("historical record = %#v, want read-only revision %s", record, revision)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err != nil {
		t.Fatalf("historical record poisoned the compact authority graph: %v", err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !hasAuthorityInventoryStatus(report.Entries, state.LineageID, AuthorityStatusActive) {
		t.Fatalf("historical inventory = %#v", report)
	}
	next := record.State
	if err := next.Invalidate("historical maintenance"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/invalidate", next); err == nil || !errors.Is(err, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical mutation error = %v", err)
	}
	if _, err := store.ExportTransport(); err == nil || !errors.Is(err, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical transport export error = %v", err)
	}
	if after, err := os.ReadFile(store.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("historical authority bytes changed: %v", err)
	}
}

func TestCompactStoreStillRejectsUnknownNonRetiredFields(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "unknown-field-lineage")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	injectRetiredCompactStateField(t, store.StatePath(), "future_unknown_field")
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown non-retired field load error = %v", err)
	}
}

func TestNewCompactAuthorityNeverPersistsRetiredZeroEditEscalation(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "fresh-lineage")
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("zero_edit_escalation")) {
		t.Fatal("new compact authority serialized a retired compatibility field")
	}
	if record.HistoricalCompat {
		t.Fatal("new compact record is marked as historical compatibility authority")
	}
	statePayload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(statePayload, []byte("zero_edit_escalation")) {
		t.Fatal("new compact state serialization emitted a retired compatibility field")
	}
}
