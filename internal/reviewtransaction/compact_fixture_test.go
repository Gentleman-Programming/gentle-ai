package reviewtransaction

// This file holds fixture helpers that used to live in
// compact_reconcile_test.go, retired in Wave 7 S3b along with
// ReconcileInvalidRecoveryEdge (the CLI verb it served, `review
// reconcile-authority`, retired one slice earlier in S3a). Each helper here
// is reused by at least one OTHER test file (confirmed by grep before the
// retiring commit), so it moved rather than dying with its original home:
//
//   - writeCompactFixtureRecord: authority_disposition_closure_execute_test.go,
//     compact_inspect_test.go, compact_forged_authorization_test.go,
//     compact_batch_reconcile_lock_test.go, compact_batch_reconcile_plan_test.go,
//     authority_disposition_resume_test.go, authority_disposition_execute_test.go,
//     compact_abandon_test.go
//   - poisonedRecoveryFixture: compact_batch_reconcile_journal_test.go
//   - preContractRecoveryFixture, preContractFixtureAuthorization:
//     compact_damaged_store_exit_test.go (and preContractFixtureAuthorization
//     also by compact_batch_reconcile_plan_test.go,
//     compact_batch_reconcile_lock_test.go)
//
// TestClassifyCompactRecoveryEdgeAnomalies also moved here: it directly
// tests classifyCompactRecoveryEdgeAnomalies (compact_reconcile.go), which
// is LIVE, RETAINED production code with three other callers
// (authority_disposition_plan.go, compact_inspect.go,
// compact_batch_reconcile_plan.go) -- unlike ReconcileInvalidRecoveryEdge
// itself, the classifier was never reconcile-specific dead weight.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// startReviewingCompactAuthority starts a fresh compact review through the
// current atomic START boundary. Fixtures that need a later lifecycle state
// must build on this reviewing authority rather than minting a store record.
func startReviewingCompactAuthority(t *testing.T, repo string, state CompactState) (CompactState, CompactStore) {
	t.Helper()
	started, err := createAtomicCompactAuthority(t, context.Background(), repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if started.Replayed {
		t.Fatalf("fresh fixture start for lineage %q replayed existing authority", state.LineageID)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return started.Record.State, store
}

// newCompactFixtureStateForTarget builds a compact START state from the exact
// repository-derived target used by a fixture.
func newCompactFixtureStateForTarget(t *testing.T, repo, lineage string, target Target) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	switch risk {
	case RiskMedium:
		lenses = []string{LensReliability}
	case RiskHigh:
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// startReviewingFixtureLineage stores a currently reviewing authority over one
// explicitly intended fixture path.
func startReviewingFixtureLineage(t *testing.T, repo, lineage, content string) string {
	t.Helper()
	path := lineage + ".txt"
	writeSnapshotFile(t, repo, path, content)
	state := newCompactFixtureStateForTarget(t, repo, lineage, Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{path},
	})
	_, _ = startReviewingCompactAuthority(t, repo, state)
	return lineage
}

// persistSemanticallyInvalidCompactState changes only the reviewing state
// marker, so the loader reaches semantic validation before its checksum check.
// The invalidated state omits required invalidation evidence.
func persistSemanticallyInvalidCompactState(t *testing.T, store CompactStore) {
	t.Helper()
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	invalid := bytes.Replace(payload, []byte(`"state": "reviewing",`), []byte(`"state": "invalidated",`), 1)
	if bytes.Equal(invalid, payload) {
		t.Fatal("fixture did not contain the expected reviewing state marker")
	}
	if err := os.WriteFile(store.StatePath(), invalid, 0o644); err != nil {
		t.Fatal(err)
	}
}

// correctionRequiredCompactAuthority opens authority atomically, completes an
// admitted review with one causal finding, and persists its correction-needed
// state for store and status fixtures.
func correctionRequiredCompactAuthority(t *testing.T, repo, lineage string) (CompactState, CompactStore, CompactRecord) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) == 0 {
		t.Fatal("correction fixture unexpectedly selected no lenses")
	}
	state, store := startReviewingCompactAuthority(t, repo, state)
	started, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	finding := Finding{
		ID: "R3-001", Lens: state.SelectedLenses[0], Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
	}
	results[0].Findings = []Finding{finding}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results,
		Classifications: []FindingEvidence{{
			FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk",
		}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if state.State != StateCorrectionRequired {
		t.Fatalf("fixture state = %q, want %q", state.State, StateCorrectionRequired)
	}
	if _, err := store.Replace(started.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return state, store, record
}

// persistHistoricalFailedValidatorAuthority starts the predecessor through the
// current atomic boundary, then encodes the retired failed-validator shape
// needed only to prove that read-only historical-status routing remains intact.
func persistHistoricalFailedValidatorAuthority(t *testing.T, lineage string) (string, CompactState, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	state, store, _ := correctionRequiredCompactAuthority(t, repo, lineage)
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	state.CorrectionAttempts = []CompactCorrectionAttempt{{
		Snapshot: fix, ProposedLines: 1, ActualLines: 1, FixDeltaHash: fixHash,
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("7"), FixDeltaHash: fixHash},
	}}
	state.State, state.CurrentSnapshot, state.CumulativeCorrectionLines = StateCorrectionRequired, fix, 1
	state.ProposedCorrectionLines, state.ActualCorrectionLines = nil, nil
	state.FixDeltaHash, state.OriginalCriteria, state.CorrectionRegression = EmptyFixDeltaHash, nil, nil
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, state, record
}

// escalatedCompactAuthorityFixture creates reviewing authority first, then
// records a review-completion escalation through the normal compact transition.
func escalatedCompactAuthorityFixture(t *testing.T, repo, lineage string) (CompactState, CompactStore, CompactRecord) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) == 0 {
		t.Fatal("escalated fixture unexpectedly selected no lenses")
	}
	state, store := startReviewingCompactAuthority(t, repo, state)
	started, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	finding := Finding{
		ID: "R3-001", Lens: state.SelectedLenses[0], Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "reviewer evidence remains inconclusive", ProofRefs: []string{"candidate inspection was inconclusive"},
	}
	results[0].Findings = []Finding{finding}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results,
		Classifications: []FindingEvidence{{
			FindingID: finding.ID, Class: EvidenceInsufficient, Causality: CausalUnknown, Proof: "insufficient evidence",
		}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if state.State != StateEscalated {
		t.Fatalf("fixture state = %q, want %q", state.State, StateEscalated)
	}
	if _, err := store.Replace(started.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return state, store, record
}

// poisonedRecoveryFixture persists an escalated predecessor and a recovery
// successor whose sole structural anomaly is an unchanged target. mutate, when
// non-nil, adjusts the successor before it is persisted.
func poisonedRecoveryFixture(t *testing.T, repo string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state, predecessorStore, predecessor := escalatedCompactAuthorityFixture(t, repo, "reconcile-predecessor")
	successorState := newCompactTestState(t, repo, "reconcile-successor")
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry terminal validator", Actor: "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(state.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity, "maintainer@example.com", "retry terminal validator"),
	}
	if mutate != nil {
		mutate(&successorState)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, predecessorStore, successor, successorStore
}

func writeCompactFixtureRecord(t *testing.T, store CompactStore, state CompactState) CompactRecord {
	t.Helper()
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return record
}

func preContractRecoveryFixture(t *testing.T, repo, authorization string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state, predecessorStore, predecessor := escalatedCompactAuthorityFixture(t, repo, "reconcile-predecessor")
	writeSnapshotFile(t, repo, "tracked.txt", "pre-contract recovery target\n")
	successorState := newCompactTestState(t, repo, "reconcile-successor")
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry terminal validator", Actor: "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: authorization,
	}
	if mutate != nil {
		mutate(&successorState)
	}
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: successorState, Disposition: RecoveryEscalated, Reason: successorState.Recovery.Reason,
		Actor: successorState.Recovery.Actor, RecoveredAt: successorState.Recovery.RecoveredAt,
		MaintainerAuthorization: successorState.Recovery.MaintainerAuthorization,
	})
	if err != nil {
		t.Fatal(err)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, recovered.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return predecessor, predecessorStore, recovered, successorStore
}

// receiptFreeLastEventClosedCompactState creates a terminal clean-review state
// without materializing any receipt artifact.
func receiptFreeLastEventClosedCompactState(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	state := newCompactTestStateWithIntended(t, repo, lineage, intended)
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	return state
}

const preContractFixtureAuthorization = "maintainer approved incident retry per the 2.1.6 runbook"

// TestClassifyCompactRecoveryEdgeAnomalies directly tests
// classifyCompactRecoveryEdgeAnomalies (compact_reconcile.go) -- live,
// retained production code (authority_disposition_plan.go, compact_inspect.go,
// compact_batch_reconcile_plan.go all still call it after Wave 7 S3a/S3b).
func TestClassifyCompactRecoveryEdgeAnomalies(t *testing.T) {
	setUnchangedTarget := func(predecessor CompactRecord, successor *CompactRecord) {
		successor.State.InitialSnapshot = predecessor.State.CurrentSnapshot
		successor.State.CurrentSnapshot = predecessor.State.CurrentSnapshot
		successor.State.GenesisPaths = append([]string(nil), predecessor.State.GenesisPaths...)
		recovery := successor.State.Recovery
		recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
			recovery.PredecessorLineageID, recovery.PredecessorRevision,
			successor.State.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
	}

	tests := []struct {
		name                    string
		mutate                  func(CompactRecord, *CompactRecord)
		wantValid               bool
		wantAnomalies           string
		wantRefusal             string
		wantAuthorizationDigest bool
		wantDispositionClass    string
		checkRepairability      bool
		wantRepairable          bool
	}{
		{name: "valid edge", wantValid: true},
		{
			name: "unchanged target",
			mutate: func(predecessor CompactRecord, successor *CompactRecord) {
				setUnchangedTarget(predecessor, successor)
			},
			wantAnomalies: compactRecoveryEdgeUnchangedTarget,
		},
		{
			name: "malformed recovery authorization",
			mutate: func(_ CompactRecord, successor *CompactRecord) {
				successor.State.Recovery.MaintainerAuthorization = preContractFixtureAuthorization
			},
			wantAnomalies: compactRecoveryEdgeMalformedAuthorization, wantAuthorizationDigest: true,
			checkRepairability: true,
		},
		{
			name: "dual anomaly in canonical order",
			mutate: func(predecessor CompactRecord, successor *CompactRecord) {
				setUnchangedTarget(predecessor, successor)
				successor.State.Recovery.MaintainerAuthorization = preContractFixtureAuthorization
			},
			wantAnomalies: compactCombinedRecoveryAnomalies, wantAuthorizationDigest: true,
		},
		{
			// This is one of Wave 2's two content-mismatch branches
			// (rdd-authority-disposition-plan, design decision 2):
			// errCompactRecoveryTargetUnchanged with a schema-prefixed but
			// wrong-content authorization.
			name: "unchanged target with schema-prefixed different-content authorization is non-reconcilable corruption",
			mutate: func(predecessor CompactRecord, successor *CompactRecord) {
				successor.State.InitialSnapshot = predecessor.State.CurrentSnapshot
				successor.State.CurrentSnapshot = predecessor.State.CurrentSnapshot
				successor.State.GenesisPaths = append([]string(nil), predecessor.State.GenesisPaths...)
				recovery := successor.State.Recovery
				recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
					recovery.PredecessorLineageID, recovery.PredecessorRevision,
					successor.State.InitialSnapshot.Identity, recovery.Actor, "different reason")
			},
			wantRefusal:          "unchanged target is not the sole anomaly",
			wantDispositionClass: compactContentMismatchedRecoveryAuthorizationClass,
		},
		{
			// The other content-mismatch branch:
			// ErrCompactRecoveryAuthorizationInexact with a schema-prefixed but
			// wrong-content authorization.
			name: "schema-prefixed different-content authorization is non-reconcilable corruption",
			mutate: func(_ CompactRecord, successor *CompactRecord) {
				recovery := successor.State.Recovery
				recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
					recovery.PredecessorLineageID, recovery.PredecessorRevision,
					successor.State.InitialSnapshot.Identity, recovery.Actor, "different reason")
			},
			wantRefusal:          "corruption, not a pre-contract authorization",
			wantDispositionClass: compactContentMismatchedRecoveryAuthorizationClass,
			checkRepairability:   true,
			wantRepairable:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			predecessor, _, successor, _ := preContractRecoveryFixture(t, repo, "placeholder", func(state *CompactState) {
				recovery := state.Recovery
				recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
					recovery.PredecessorLineageID, recovery.PredecessorRevision,
					state.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
			})
			if tt.mutate != nil {
				tt.mutate(predecessor, &successor)
			}
			var err error
			successor, _, err = makeCompactRecord(successor.State)
			if err != nil {
				t.Fatal(err)
			}

			got := classifyCompactRecoveryEdgeAnomalies(predecessor, successor)
			if got.Valid != tt.wantValid || strings.Join(got.Anomalies, ",") != tt.wantAnomalies {
				t.Fatalf("classification = %#v, want valid=%t anomalies=%q", got, tt.wantValid, tt.wantAnomalies)
			}
			if tt.wantRefusal == "" && got.NonReconcilableError != nil {
				t.Fatalf("unexpected non-reconcilable error: %v", got.NonReconcilableError)
			}
			if tt.wantRefusal != "" && (got.NonReconcilableError == nil || !strings.Contains(got.NonReconcilableError.Error(), tt.wantRefusal)) {
				t.Fatalf("non-reconcilable error = %v, want substring %q", got.NonReconcilableError, tt.wantRefusal)
			}
			if got.DispositionClass != tt.wantDispositionClass {
				t.Fatalf("disposition class = %q, want %q", got.DispositionClass, tt.wantDispositionClass)
			}
			if tt.checkRepairability {
				var authorizationErr *CompactRecoveryAuthorizationInexactError
				if !errors.As(got.ValidationError, &authorizationErr) {
					t.Fatalf("validation error = %T %v, want authorization error", got.ValidationError, got.ValidationError)
				}
				if authorizationErr.Repairable != tt.wantRepairable {
					t.Fatalf("authorization repairable = %t, want %t", authorizationErr.Repairable, tt.wantRepairable)
				}
			}
			wantDigest := ""
			if tt.wantAuthorizationDigest {
				digest := sha256.Sum256([]byte(preContractFixtureAuthorization))
				wantDigest = "sha256:" + hex.EncodeToString(digest[:])
			}
			if got.RecordedAuthorizationSHA256 != wantDigest {
				t.Fatalf("authorization proof = %q, want %q", got.RecordedAuthorizationSHA256, wantDigest)
			}
		})
	}
}
