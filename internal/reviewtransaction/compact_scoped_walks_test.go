package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These tests pin issues #2743/#2741: a compact record frozen by an earlier
// release under the retired snapshot-identity formula fails semantic
// validation and becomes unreadable to every consumer. The scoped-authority
// rule (#2495, `scanCompactAuthority`) says an entry nobody can read is ABSENT
// from the graph, not a verdict on it — but four walks still iterated every
// discovered store and failed closed on the first unreadable foreign record:
// RecoverCompactAuthority's recovery-graph loop (#2741),
// InspectCompactFinalVerificationRetrySource (the STATUS path that turned one
// escalated lineage plus one historical record into repository-wide
// `corrupted_or_unverifiable_authority`, #2743), the two `review abandon`
// walks, and the RAR receipt superseded probe.

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

// outdatedForeignLineage plants one unrelated historical-style lineage whose
// record no longer loads, removing its worktree residue so the fixture never
// changes what any other lineage's live snapshot sees.
func outdatedForeignLineage(t *testing.T, repo, lineage string) {
	t.Helper()
	foreign := quarantineFixtureHealthyLineage(t, repo, lineage, "foreign historical candidate\n")
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
// healthy correction-required lineage authorized for scope-changed recovery
// must recover even though an unrelated stored lineage no longer loads. The
// unscoped "validate recovery graph" loop used to abort on the foreign record
// before attempting anything for the lineage actually being recovered.
func TestRecoverCompactAuthorityIgnoresUnreadableForeignLineage(t *testing.T) {
	repo, predecessor, _, predecessorRecord := correctionScopeRecoveryFixture(t, "recover-scoped-predecessor")
	outdatedForeignLineage(t, repo, "recover-scoped-foreign")

	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "recover-scoped-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "correction requires a process helper",
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
// authority — an outdated record stays gate-invalid, it just stops taking
// unrelated lineages down with it.
func TestRecoverCompactAuthorityStillRefusesTheOutdatedLineageItself(t *testing.T) {
	repo, predecessor, store, predecessorRecord := correctionScopeRecoveryFixture(t, "recover-outdated-predecessor")
	outdateCompactSnapshotIdentity(t, store)

	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "recover-outdated-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "correction requires a process helper",
		Actor: "maintainer", RecoveredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

	before := compactAuthorityFileSnapshot(t, repo)
	if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil {
		t.Fatal("recovery whose own predecessor is an outdated record succeeded")
	}
	if after := compactAuthorityFileSnapshot(t, repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("refused recovery mutated authority: %#v != %#v", after, before)
	}
}

// TestInspectFinalVerificationRetrySourceIgnoresUnreadableForeignLineage is
// #2743's STATUS mechanism: any escalated lineage routes negotiated status
// through the retry-source inspection, whose unscoped store walk returned the
// first foreign load error and flipped the whole repository to
// corrupted_or_unverifiable_authority.
func TestInspectFinalVerificationRetrySourceIgnoresUnreadableForeignLineage(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "retry-scoped-source", "retry-scoped-successor")
	outdatedForeignLineage(t, fixture.repo, "retry-scoped-foreign")

	eligibility, ok, err := InspectCompactFinalVerificationRetrySource(
		context.Background(), fixture.repo, fixture.predecessor.State.LineageID, fixture.predecessor.Revision)
	if err != nil {
		t.Fatalf("retry-source inspection with an unreadable foreign record = %v", err)
	}
	if !ok {
		t.Fatal("escalated lineage lost retry-source eligibility because of an unreadable foreign record")
	}
	if eligibility.IncidentClass != FinalVerificationIncidentProceduralToolingFailure {
		t.Fatalf("eligibility = %#v", eligibility)
	}
}

// TestTargetStatusStaysHealthyWithOutdatedHistoricalAuthority reproduces
// #2743's reported shape end-to-end at the status layer: a repository holding
// pre-rc.2 compact-v2 history, one unrelated new lineage that escalates, and
// the immediately following STATUS. Every escalated lineage routes through
// InspectCompactFinalVerificationRetrySource, whose unscoped walk turned the
// first outdated historical record into `applicability: corrupted` with a
// terminal `corrupted_or_unverifiable_authority` stop and zero eligible
// repair candidates. Status must classify the live target normally instead.
func TestTargetStatusStaysHealthyWithOutdatedHistoricalAuthority(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "status-scoped-escalated", "status-scoped-successor")
	if fixture.predecessor.State.State != StateEscalated {
		t.Fatalf("fixture predecessor state = %s, want escalated", fixture.predecessor.State.State)
	}
	outdatedForeignLineage(t, fixture.repo, "status-scoped-historical")

	status, err := AssessTargetStatus(context.Background(), fixture.repo, TargetStatusRequest{
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

// TestPreCommitGateAllowsApprovedReceiptWithOutdatedHistoricalAuthority pins
// the delivery half: the pre-commit gate validates the approved lineage's own
// receipt and must keep allowing it while outdated pre-rc.2 records sit in
// the same store. Delivery never widens — the approved lineage still proves
// its own authority — it simply stops being hostage to unrelated history.
func TestPreCommitGateAllowsApprovedReceiptWithOutdatedHistoricalAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	outdatedForeignLineage(t, repo, "gate-scoped-historical")
	writeSnapshotFile(t, repo, "tracked.txt", "base\napproved delivery\n")
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "gate-scoped-approved", []string{})

	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePreCommit, LineageID: state.LineageID,
	})
	if got.Result != GateAllow {
		t.Fatalf("pre-commit gate with outdated historical authority = %#v, want allow", got)
	}
	if got.Context.CandidateTree != receipt.FinalCandidateTree {
		t.Fatalf("gate context candidate tree = %q, want the approved %q", got.Context.CandidateTree, receipt.FinalCandidateTree)
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

// TestCompactLineageSupersededUnderMaintenanceIgnoresUnreadableForeignLineage:
// the RAR receipt binding probe walked every store to prove the approved
// lineage has no recovery successor, and failed the whole receipt derivation
// on the first unreadable foreign record.
func TestCompactLineageSupersededUnderMaintenanceIgnoresUnreadableForeignLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\napproved change\n")
	approved, _ := approvedCompactFixture(t, repo, "rar-scoped-approved")
	outdatedForeignLineage(t, repo, "rar-scoped-foreign")

	superseded, err := compactLineageSupersededUnderMaintenance(context.Background(), repo, approved.State.LineageID)
	if err != nil {
		t.Fatalf("superseded probe with an unreadable foreign record = %v", err)
	}
	if superseded {
		t.Fatal("approved lineage reported superseded without any recovery successor")
	}
}

// TestInspectAuthorityClassifiesOutdatedIdentityAsOutdated pins the honest
// diagnostics half of #2743: a record whose only load failure is the retired
// snapshot-identity formula is OUTDATED — gate-invalid, preserved for
// forensics — not malformed, and the damage narration must stop describing it
// as the residue of an interrupted write.
func TestInspectAuthorityClassifiesOutdatedIdentityAsOutdated(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := quarantineFixtureHealthyLineage(t, repo, "inspect-outdated-identity", "outdated candidate\n")
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	outdateCompactSnapshotIdentity(t, store)

	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].LineageID != lineage ||
		report.EntryDiagnostics[0].Problem != "outdated_compact_state" {
		t.Fatalf("entry diagnostics = %#v", report.EntryDiagnostics)
	}

	kinds := CompactAuthorityDamageKinds(report)
	if len(kinds) != 1 {
		t.Fatalf("damage kinds = %#v", kinds)
	}
	if strings.Contains(kinds[0], "interrupted write") || strings.Contains(kinds[0], "does not parse") {
		t.Fatalf("outdated record still narrated as damage: %q", kinds[0])
	}
	if !strings.Contains(kinds[0], "outdated_compact_state") || !strings.Contains(kinds[0], "earlier release") {
		t.Fatalf("outdated record narration is not honest about its class: %q", kinds[0])
	}
}

// TestInspectAuthorityKeepsGenuineSemanticDamageMalformed proves the outdated
// class is exactly as narrow as the retired-identity signature: any other
// semantic-validation failure keeps the existing malformed classification.
func TestInspectAuthorityKeepsGenuineSemanticDamageMalformed(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := quarantineFixtureHealthyLineage(t, repo, "inspect-semantic-damage", "damaged candidate\n")
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	corruptCompactStateSemantics(t, store)

	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].Problem != "malformed_compact_state" {
		t.Fatalf("entry diagnostics = %#v", report.EntryDiagnostics)
	}
}
