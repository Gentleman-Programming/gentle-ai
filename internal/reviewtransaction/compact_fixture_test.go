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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// poisonedRecoveryFixture persists an escalated predecessor with its receipt
// and a recovery successor whose sole structural anomaly is an unchanged
// target. mutate, when non-nil, adjusts the successor before it is persisted.
func poisonedRecoveryFixture(t *testing.T, repo string, mutate func(*CompactState)) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, "reconcile-predecessor")
	state.State = StateEscalated
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := writeCompactFixtureRecord(t, predecessorStore, state)
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(predecessorStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
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
	state := correctedCompactTestState(t, repo, "reconcile-predecessor")
	state.State = StateEscalated
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := writeCompactFixtureRecord(t, predecessorStore, state)
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(predecessorStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
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
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, predecessorStore, successor, successorStore
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
