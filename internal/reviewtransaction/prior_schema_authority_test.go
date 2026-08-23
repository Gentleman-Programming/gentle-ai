package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests pin the prior-schema classification contract: a compact record
// whose frozen snapshot identities were minted by the retired pre-fbb55080
// formula is PRIOR-SCHEMA — provably written by an older release and still
// self-consistent under that formula — never generic corruption. Discovery
// blocked exclusively by prior-schema history must reclassify the live target
// through the ordinary fresh-start route instead of the terminal
// corrupted_or_unverifiable_authority stop, while any record matching neither
// formula keeps today's fail-closed behavior.

// TestRetiredCompactSnapshotIdentityMatchesVerbatimRetiredFormula proves the
// production forensic formula (retiredCompactSnapshotIdentity) reproduces the
// exact pre-fbb55080 snapshotIdentityForProjection body — domain tags v1/v2/
// base-workspace-overlay/v1, the untracked-replay proof folded into the
// values, and the length-prefixed intended entries — for every domain-tag
// shape, and that it always differs from the current formula.
func TestRetiredCompactSnapshotIdentityMatchesVerbatimRetiredFormula(t *testing.T) {
	tree := func(seed string) string {
		sum := sha256.Sum256([]byte(seed))
		return hex.EncodeToString(sum[:])[:40]
	}
	digest := func(seed string) string {
		sum := sha256.Sum256([]byte(seed))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	snapshots := []Snapshot{
		{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, BaseTree: tree("base"), CandidateTree: tree("candidate"),
			PathsDigest: digest("paths"), IntendedUntrackedProof: digest("proof"),
			IntendedUntracked: []string{"a.txt", "b.txt"}, LedgerIDs: []string{}},
		{Kind: TargetCurrentChanges, Projection: ProjectionStaged, BaseTree: tree("base"), CandidateTree: tree("candidate"),
			PathsDigest: digest("paths"), IntendedUntrackedProof: digest("proof"),
			IntendedUntracked: []string{}, LedgerIDs: []string{"R1-001"}},
		{Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionWorkspace, BaseTree: tree("base"), CandidateTree: tree("candidate"),
			PathsDigest: digest("paths"), IntendedUntrackedProof: digest("proof"),
			IntendedUntracked: []string{"overlay.txt"}, LedgerIDs: []string{}},
		{Kind: TargetFixDiff, Projection: ProjectionWorkspace, BaseTree: tree("base"), CandidateTree: tree("fix"),
			PathsDigest: digest("paths"), IntendedUntrackedProof: digest("proof"),
			IntendedUntracked: []string{}, LedgerIDs: []string{"R3-001", "R3-002"}},
	}
	for index, snapshot := range snapshots {
		retired := retiredCompactSnapshotIdentity(snapshot)
		if verbatim := retiredSnapshotIdentity(snapshot); retired != verbatim {
			t.Fatalf("snapshot %d: production retired formula %q != verbatim pre-fbb55080 formula %q", index, retired, verbatim)
		}
		current := snapshotIdentityForProjection(snapshot.Kind, snapshot.Projection, snapshot.BaseTree, snapshot.CandidateTree,
			snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
		if retired == current {
			t.Fatalf("snapshot %d: retired formula reproduced the current identity %q; the domains would be indistinguishable", index, retired)
		}
	}
}

// retireCompactStateIdentities rewrites every snapshot identity in the state
// to the retired pre-fbb55080 formula and coherently retires the evidence and
// correction-target bindings that pin those identities, exactly as a 2.2.x
// release froze them. It returns nothing: callers persist the state
// themselves and assert on the load classification.
func retireCompactStateIdentities(state *CompactState) {
	retired := map[string]string{}
	mint := func(snapshot *Snapshot) {
		value := retiredSnapshotIdentity(*snapshot)
		retired[snapshot.Identity] = value
		snapshot.Identity = value
	}
	mint(&state.InitialSnapshot)
	mint(&state.CurrentSnapshot)
	for index := range state.CorrectionAttempts {
		mint(&state.CorrectionAttempts[index].Snapshot)
	}
	if state.CorrectionVerificationTarget != nil {
		mint(state.CorrectionVerificationTarget)
	}
	if value, seen := retired[state.EvidenceTargetIdentity]; seen {
		state.EvidenceTargetIdentity = value
	}
	for index := range state.CorrectionAttempts {
		if value, seen := retired[state.CorrectionAttempts[index].CorrectionTargetIdentity]; seen {
			state.CorrectionAttempts[index].CorrectionTargetIdentity = value
		}
	}
}

// persistCompactStateBytes writes a record for the state exactly as the store
// serializes one, without any write-path validation, so fixtures can persist
// historical shapes the current release refuses to mint.
func persistCompactStateBytes(t *testing.T, repo string, state CompactState) CompactStore {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

// correctedApprovedCompactState builds the exact record shape 32 of the 36
// previously unclassifiable real-store records carry: approved through one
// bounded correction, so the evidence record binding and the correction
// attempt both pin snapshot identities.
func correctedApprovedCompactState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	state, fix := pendingCompactCorrection(t, repo, lineage)
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: append([]string(nil), state.FixFindingIDs...), FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	payload := []byte("repository verification passed\n")
	passed, err := NewVerificationEvidenceRecord(state.LineageID, hash("a"), fix, payload, VerificationOutcomePassed)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteCorrectionVerification(fix, 1, validation, passed, payload); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved || len(state.CorrectionAttempts) != 1 || state.EvidenceTargetIdentity == "" {
		t.Fatalf("corrected approved fixture state = %#v", state)
	}
	return state
}

// TestCorrectedApprovedPriorSchemaRecordClassifiesOutdated pins the forensic
// classification for the evidence-bound, correction-carrying shape: the
// retired identities thread through the correction attempt, the correction
// target binding, and the verification evidence binding, and the record must
// still classify as prior-schema (OutdatedIdentity), never generic damage.
func TestCorrectedApprovedPriorSchemaRecordClassifiesOutdated(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedApprovedCompactState(t, repo, "prior-schema-corrected")
	retireCompactStateIdentities(&state)
	store := persistCompactStateBytes(t, repo, state)

	_, loadErr := store.Load()
	var semantic *CompactSemanticStateError
	if loadErr == nil || !errors.As(loadErr, &semantic) {
		t.Fatalf("prior-schema corrected record load error = %v, want a semantic state error", loadErr)
	}
	if !semantic.OutdatedIdentity {
		t.Fatalf("prior-schema corrected record classified as generic damage: %v", loadErr)
	}
}

// priorSchemaLineage plants one lineage whose record is byte-faithful to a
// pre-fbb55080 release: a plain approved review with every identity minted by
// the retired formula. The worktree residue is removed so the fixture never
// changes what any live snapshot sees.
func priorSchemaLineage(t *testing.T, repo, lineage string) CompactStore {
	t.Helper()
	planted := quarantineFixtureHealthyLineage(t, repo, lineage, "prior-schema candidate content\n")
	if err := os.Remove(filepath.Join(repo, planted+".txt")); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, planted)
	if err != nil {
		t.Fatal(err)
	}
	outdateCompactSnapshotIdentity(t, store)
	return store
}

// corruptSnapshotIdentityBeyondRecognition rewrites a persisted record so its
// snapshot identities match NEITHER the current nor the retired formula —
// genuine corruption, the control shape that must keep failing closed.
func corruptSnapshotIdentityBeyondRecognition(t *testing.T, store CompactStore) {
	t.Helper()
	record := mustLoadCompactRecord(t, store)
	for _, snapshot := range []*Snapshot{&record.State.InitialSnapshot, &record.State.CurrentSnapshot} {
		sum := sha256.Sum256([]byte("unrecognized identity domain " + snapshot.Identity))
		snapshot.Identity = "sha256:" + hex.EncodeToString(sum[:])
	}
	_, payload, err := makeCompactRecord(record.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	_, loadErr := store.Load()
	var semantic *CompactSemanticStateError
	if loadErr == nil || !errors.As(loadErr, &semantic) || semantic.OutdatedIdentity {
		t.Fatalf("unrecognized-identity fixture load error = %v, want a non-outdated semantic failure", loadErr)
	}
}

// TestExplicitLineageStatusOffersFreshStartForPriorSchemaAuthority is the
// classification fix itself: STATUS naming a lineage whose record is provably
// prior-schema must answer with the ordinary fresh-start shape for the live
// candidate — the prior-schema authority stays untouched on disk as inert
// history — instead of the terminal corrupted classification.
func TestExplicitLineageStatusOffersFreshStartForPriorSchemaAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	store := priorSchemaLineage(t, repo, "prior-schema-only")
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh live work\n")

	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: store.lineageID,
	})
	if err != nil {
		t.Fatalf("status naming a prior-schema lineage = %v", err)
	}
	if status.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("prior-schema authority still classifies corrupted: %#v", status)
	}
	if status.Applicability != TargetApplicabilityUnrelated || status.Action != TargetStatusActionStart {
		t.Fatalf("prior-schema authority did not reclassify to the fresh-start shape: %#v", status)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("read-only status mutated the prior-schema record")
	}
}

// TestExplicitLineageStatusKeepsCorruptedForUnrecognizedIdentity is the
// fail-closed control: a record whose identities match neither formula is
// genuine corruption and keeps today's terminal classification.
func TestExplicitLineageStatusKeepsCorruptedForUnrecognizedIdentity(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := quarantineFixtureHealthyLineage(t, repo, "unrecognized-identity", "corrupted candidate content\n")
	if err := os.Remove(filepath.Join(repo, lineage+".txt")); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	corruptSnapshotIdentityBeyondRecognition(t, store)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh live work\n")

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage,
	})
	if err != nil {
		t.Fatalf("status naming a corrupted lineage = %v", err)
	}
	if status.Applicability != TargetApplicabilityCorrupted || status.Action != TargetStatusActionRepairAuthority {
		t.Fatalf("unrecognized-identity record lost its fail-closed classification: %#v", status)
	}
}

// priorSchemaRecoveryChain builds a real recovery chain (predecessor plus a
// recovered successor) so ancestry-walk classification can be pinned, then
// hands both stores back for per-test mutation.
func priorSchemaRecoveryChain(t *testing.T, prefix string) (string, CompactStore, CompactStore, string) {
	t.Helper()
	repo, predecessor, predecessorStore, predecessorRecord := correctionScopeRecoveryFixture(t, prefix+"-predecessor")
	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, prefix+"-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "correction requires a process helper",
		Actor: "maintainer", RecoveredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, recovered.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, predecessorStore, successorStore, recovered.State.LineageID
}

// TestExplicitLineageStatusOffersFreshStartForWholePriorSchemaAncestry: when
// EVERY record blocking the named lineage is prior-schema, the whole chain is
// inert history and the live target reclassifies to fresh start.
func TestExplicitLineageStatusOffersFreshStartForWholePriorSchemaAncestry(t *testing.T) {
	repo, predecessorStore, successorStore, successorLineage := priorSchemaRecoveryChain(t, "prior-chain")
	outdateCompactSnapshotIdentity(t, successorStore)
	outdateCompactSnapshotIdentity(t, predecessorStore)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh live work\n")

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: successorLineage,
	})
	if err != nil {
		t.Fatalf("status naming a whole prior-schema chain = %v", err)
	}
	if status.Applicability != TargetApplicabilityUnrelated || status.Action != TargetStatusActionStart {
		t.Fatalf("whole prior-schema ancestry did not reclassify to fresh start: %#v", status)
	}
}

// TestExplicitLineageStatusKeepsLiveAuthorityOverPriorSchemaPredecessor pins
// the fresh-start exit to the NAMED lineage: a current-schema lineage whose
// recovery predecessor is prior-schema still owns live authority, so
// candidate loading must keep it instead of answering with the empty map
// that routes STATUS to fresh start.
func TestExplicitLineageStatusKeepsLiveAuthorityOverPriorSchemaPredecessor(t *testing.T) {
	repo, predecessorStore, _, successorLineage := priorSchemaRecoveryChain(t, "live-over-prior")
	outdateCompactSnapshotIdentity(t, predecessorStore)

	candidates, err := loadCompactTargetStatusCandidates(context.Background(), repo, successorLineage)
	if err != nil {
		t.Fatalf("candidates for a live lineage over a prior-schema predecessor = %v", err)
	}
	if _, held := candidates[successorLineage]; !held {
		t.Fatalf("live named lineage lost its authority to its prior-schema predecessor: %#v", candidates)
	}
}

// TestPriorSchemaToleranceIsViolationScoped pins the suppression to the one
// dangling-predecessor violation the prior-schema gap explains. A second
// co-present violation on the same lineage is unconstructible through
// compactAuthorityGraphViolations (it records at most one violation per
// lineage and the dangling check short-circuits first), so the scoping is
// proven at the unit level of the filtering predicate.
func TestPriorSchemaToleranceIsViolationScoped(t *testing.T) {
	record := CompactRecord{State: CompactState{LineageID: "successor",
		Recovery: &CompactRecoveryProvenance{PredecessorLineageID: "gone"}}}
	priorSchema := map[string]bool{"gone": true}
	dangling := fmt.Errorf("dangling predecessor for %q", "successor")
	if !compactPriorSchemaToleratedViolation("successor", dangling, record, priorSchema) {
		t.Fatal("the dangling-predecessor violation a prior-schema gap explains must be tolerated")
	}
	for _, violation := range []error{fmt.Errorf("fork at %q", "gone"), errors.New("recovery cycle"),
		fmt.Errorf("predecessor revision mismatch for %q", "successor")} {
		if compactPriorSchemaToleratedViolation("successor", violation, record, priorSchema) {
			t.Fatalf("unrelated violation %v must keep blocking", violation)
		}
	}
	if compactPriorSchemaToleratedViolation("successor", dangling, record, map[string]bool{}) {
		t.Fatal("a missing predecessor without prior-schema proof must keep blocking")
	}
}

// TestExplicitLineageStatusKeepsCorruptedForMixedAncestry: a prior-schema
// record recovering from genuinely corrupted authority is a mixed blocking
// set, and mixed keeps today's fail-closed behavior.
func TestExplicitLineageStatusKeepsCorruptedForMixedAncestry(t *testing.T) {
	repo, predecessorStore, successorStore, successorLineage := priorSchemaRecoveryChain(t, "mixed-chain")
	outdateCompactSnapshotIdentity(t, successorStore)
	corruptSnapshotIdentityBeyondRecognition(t, predecessorStore)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfresh live work\n")

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: successorLineage,
	})
	if err != nil {
		t.Fatalf("status naming a mixed prior-schema/corrupt chain = %v", err)
	}
	if status.Applicability != TargetApplicabilityCorrupted {
		t.Fatalf("mixed ancestry lost its fail-closed classification: %#v", status)
	}
}
