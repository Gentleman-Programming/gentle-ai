package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests pin issues #2743/#2741: a compact record frozen by an earlier
// release under the retired snapshot-identity formula fails semantic
// validation and becomes unreadable to every consumer. The scoped-authority
// rule (#2495, `scanCompactAuthority`) says an entry nobody can read is ABSENT
// from the graph, not a verdict on it. Recovery, status, and abandonment must
// therefore keep serving a valid reviewing or escalated lineage while one
// unrelated historical record remains unreadable.

// retiredSnapshotIdentity recomputes a snapshot's identity with the EXACT
// formula every pre-rc.2 release used (verbatim from fbb55080^, the commit
// PR #2667 landed for #2659): the v1/v2 domain tags instead of v3/v4, the
// untracked-replay proof folded into the hashed values, and the
// intended-untracked paths hashed after them. A test that merely zeroed the
// identity would fail validation for the wrong reason; this reproduces the
// bytes a 2.2.x binary actually froze.
func retiredSnapshotIdentity(snapshot Snapshot) string {
	hash := sha256.New()
	if snapshot.Kind == TargetBaseWorkspaceOverlay {
		hash.Write([]byte("gentle-ai.review-snapshot/base-workspace-overlay/v1\x00"))
	} else if snapshot.Projection == ProjectionStaged {
		hash.Write([]byte("gentle-ai.review-snapshot/v2\x00"))
	} else {
		hash.Write([]byte("gentle-ai.review-snapshot/v1\x00"))
	}
	values := []string{string(snapshot.Kind), snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof}
	if snapshot.Projection == ProjectionStaged {
		values = []string{string(snapshot.Kind), string(snapshot.Projection), snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof}
	}
	for _, value := range values {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range snapshot.IntendedUntracked {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range snapshot.LedgerIDs {
		writeLengthPrefixed(hash, []byte(value))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func mustLoadCompactRecord(t *testing.T, store CompactStore) CompactRecord {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// outdateCompactSnapshotIdentity rewrites an already-persisted, checksum-valid
// compact state on disk so that its frozen snapshot identities carry the
// RETIRED pre-rc.2 formula's value — exactly what a record written by a 2.2.x
// release looks like to the current reader (#2743). Because Validate() runs
// before the checksum comparison in parseCompactRecord, the load failure is
// the same typed *CompactSemanticStateError the released binaries produce for
// historical authority.
func outdateCompactSnapshotIdentity(t *testing.T, store CompactStore) {
	t.Helper()
	record := mustLoadCompactRecord(t, store)
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	outdated := payload
	for _, snapshot := range []*Snapshot{&record.State.InitialSnapshot, &record.State.CurrentSnapshot} {
		retired := retiredSnapshotIdentity(*snapshot)
		if retired == snapshot.Identity {
			t.Fatalf("retired identity formula reproduced the current identity %q; the fixture would prove nothing", snapshot.Identity)
		}
		outdated = bytes.ReplaceAll(outdated, []byte(`"identity": "`+snapshot.Identity+`"`), []byte(`"identity": "`+retired+`"`))
		snapshot.Identity = retired
	}
	if bytes.Equal(outdated, payload) {
		t.Fatal("fixture did not contain the expected snapshot identity markers")
	}
	_, payload, err = makeCompactRecord(record.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	_, loadErr := store.Load()
	var semanticErr *CompactSemanticStateError
	if loadErr == nil || !errors.As(loadErr, &semanticErr) || !semanticErr.OutdatedIdentity ||
		!strings.Contains(semanticErr.Problem, "identity does not match its metadata") {
		t.Fatalf("retired-identity fixture load error = %v, want an outdated snapshot identity mismatch", loadErr)
	}
}

// outdatedForeignLineage plants one unrelated historical-style reviewing
// lineage whose record no longer loads, removing its worktree residue so the
// fixture never changes what another lineage's live snapshot sees.
func outdatedForeignLineage(t *testing.T, repo, lineage string) {
	t.Helper()
	foreign := startReviewingFixtureLineage(t, repo, lineage, "foreign historical candidate\n")
	if err := os.Remove(filepath.Join(repo, foreign+".txt")); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, foreign)
	if err != nil {
		t.Fatal(err)
	}
	outdateCompactSnapshotIdentity(t, store)
}

// TestRecoverCompactAuthorityIgnoresUnreadableForeignLineage is #2741: a
// valid escalated lineage must recover even though an unrelated stored lineage
// no longer loads. The unscoped recovery-graph loop used to abort on the
// foreign record before attempting anything for the lineage being recovered.
func TestRecoverCompactAuthorityIgnoresUnreadableForeignLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor := newCompactTestState(t, repo, "recover-scoped-predecessor")
	predecessor.State = StateEscalated
	if err := predecessor.Validate(); err != nil {
		t.Fatalf("escalated predecessor fixture is invalid: %v", err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord := writeCompactFixtureRecord(t, store, predecessor)
	outdatedForeignLineage(t, repo, "recover-scoped-foreign")

	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "recover-scoped-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: "recover the escalated candidate with a process helper",
		Actor: "maintainer", RecoveredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("recovery of a healthy lineage with an unreadable foreign record = %v", err)
	}
	if recovered.State.Recovery == nil || recovered.State.Recovery.PredecessorLineageID != predecessor.LineageID {
		t.Fatalf("recovered successor = %#v", recovered.State)
	}
}

// TestRecoverCompactAuthorityStillRefusesTheOutdatedLineageItself is the
// fail-closed control for the scoping above: absence from the graph is only
// ever the answer for a FOREIGN record. A recovery whose own predecessor is
// the outdated record must still refuse, because scoping never widens
// authority — an outdated record stays unavailable, it just stops taking
// unrelated lineages down with it.
func TestRecoverCompactAuthorityStillRefusesTheOutdatedLineageItself(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor := newCompactTestState(t, repo, "recover-outdated-predecessor")
	predecessor.State = StateEscalated
	if err := predecessor.Validate(); err != nil {
		t.Fatalf("escalated predecessor fixture is invalid: %v", err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord := writeCompactFixtureRecord(t, store, predecessor)
	outdateCompactSnapshotIdentity(t, store)

	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "recover-outdated-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: "recover the escalated candidate with a process helper",
		Actor: "maintainer", RecoveredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil {
		t.Fatal("recovery whose own predecessor is an outdated record succeeded")
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("refused recovery mutated its outdated predecessor authority")
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(successorStore.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("refused recovery created successor authority: %v", err)
	}
}

// TestTargetStatusStaysHealthyWithOutdatedHistoricalAuthority proves status
// still classifies a valid reviewing target normally while one unrelated
// pre-rc.2 compact record remains unreadable.
func TestTargetStatusStaysHealthyWithOutdatedHistoricalAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nstatus target\n")
	record, _ := pristineReviewingFixture(t, repo, "status-scoped-reviewing")
	if record.State.State != StateReviewing {
		t.Fatalf("fixture state = %s, want reviewing", record.State.State)
	}
	outdatedForeignLineage(t, repo, "status-scoped-historical")

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
	})
	if err != nil {
		t.Fatalf("negotiated status with outdated historical authority = %v", err)
	}
	if status.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("outdated historical authority made the whole repository report corrupted: %#v", status)
	}
	if status.Action == TargetStatusActionRepairAuthority {
		t.Fatalf("status demanded authority repair for an outdated historical record: %#v", status)
	}
}

// TestPristineAbandonmentIgnoresUnreadableForeignLineage: both the read-only
// eligibility probe and the abandon operation itself walked every store and
// refused repository-wide when one unrelated record did not load — locking the
// escape valve exactly when it is needed.
func TestPristineAbandonmentIgnoresUnreadableForeignLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	outdatedForeignLineage(t, repo, "abandon-scoped-foreign")

	writeSnapshotFile(t, repo, "tracked.txt", "base\naccidental change\n")
	record, _ := pristineReviewingFixture(t, repo, "abandon-scoped-pristine")

	eligibility, err := InspectCompactPristineAbandonment(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatalf("abandon eligibility with an unreadable foreign record = %v", err)
	}
	if !eligibility.Eligible || eligibility.Revision != record.Revision {
		t.Fatalf("pristine lineage lost abandon eligibility because of an unreadable foreign record: %#v", eligibility)
	}

	committed, err := AbandonPristineCompactStore(context.Background(), repo, abandonFixtureRequest(record))
	if err != nil {
		t.Fatalf("abandon with an unreadable foreign record = %v", err)
	}
	if committed.Status != CompactReclaimCommitted || committed.LineageID != record.State.LineageID {
		t.Fatalf("abandonment record = %#v", committed)
	}
}

// TestInspectAuthorityKeepsGenuineSemanticDamageMalformed proves the outdated
// class is exactly as narrow as the retired-identity signature: any other
// semantic-validation failure remains malformed while a valid reviewing
// lineage remains available to the inspection.
func TestInspectAuthorityKeepsGenuineSemanticDamageMalformed(t *testing.T) {
	repo := initSnapshotRepo(t)
	startReviewingFixtureLineage(t, repo, "inspect-semantic-target", "target candidate\n")
	foreign := startReviewingFixtureLineage(t, repo, "inspect-semantic-damage", "damaged candidate\n")
	store, err := CompactAuthoritativeStore(context.Background(), repo, foreign)
	if err != nil {
		t.Fatal(err)
	}
	persistSemanticallyInvalidCompactState(t, store)

	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.LoadedEntries != 1 {
		t.Fatalf("valid reviewing target was lost after foreign semantic damage: %#v", report.Totals)
	}
	if len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].LineageID != foreign ||
		report.EntryDiagnostics[0].Problem != "malformed_compact_state" {
		t.Fatalf("entry diagnostics = %#v", report.EntryDiagnostics)
	}
}
