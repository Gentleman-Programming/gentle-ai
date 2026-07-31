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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// driftedRecoveryBindingPair persists one escalated predecessor with its
// receipt and a changed-target escalated recovery successor whose recorded
// gentle-ai.review-recovery-authorization/v1 binding names a target identity
// the successor never persisted. Every other line of the binding re-derives
// exactly from the persisted predecessor record and the persisted provenance,
// so the drift is the sole deviation.
//
// The shape is written straight to store bytes because it is unreachable
// through the CLI: RecoverCompactAuthority and ImportCompactTransport both run
// validateCompactRecoveryEdge at write time and refuse it.
func driftedRecoveryBindingPair(t *testing.T, repo, suffix, target, authorizedTarget string, mutate ...func(*CompactState)) (CompactRecord, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, "drifted-predecessor-"+suffix)
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
	writeSnapshotFile(t, repo, "tracked.txt", target)
	successorState := newCompactTestState(t, repo, "drifted-successor-"+suffix)
	successorState.Generation = state.Generation + 1
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Actor: "maintainer@example.com",
		Reason:      "refresh current verify-report evidence after approved review",
		RecoveredAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	}
	successorState.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		state.LineageID, predecessor.Revision, authorizedTarget,
		successorState.Recovery.Actor, successorState.Recovery.Reason)
	for _, adjust := range mutate {
		adjust(&successorState)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	return predecessor, successor, successorStore
}

// captureCompactReviewResults makes one fixture successor non-pristine, so the
// abandonment gate refuses to discard it and the drift class is the only exit.
func captureCompactReviewResults(t *testing.T) func(*CompactState) {
	t.Helper()
	return func(state *CompactState) {
		results := make([]LensResult, 0, len(state.SelectedLenses))
		for _, lens := range state.SelectedLenses {
			results = append(results, LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed once"}})
		}
		if err := state.CompleteReview(CompactReviewInput{
			LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// driftedRecoveryBindingDescendant persists one valid escalated recovery
// successor of an existing successor, so the existing successor stops being a
// leaf and quarantining it whole would orphan a dependent edge.
func driftedRecoveryBindingDescendant(t *testing.T, repo, lineage string, successor CompactRecord) CompactStore {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", lineage+" target\n")
	state := newCompactTestState(t, repo, lineage)
	state.Generation = successor.State.Generation + 1
	state.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: successor.State.LineageID, PredecessorRevision: successor.Revision,
		Disposition: RecoveryEscalated, Actor: "maintainer@example.com", Reason: "chained retry",
		RecoveredAt: time.Date(2026, 7, 31, 14, 0, 0, 0, time.UTC),
	}
	state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		successor.State.LineageID, successor.Revision, state.InitialSnapshot.Identity,
		state.Recovery.Actor, state.Recovery.Reason)
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, state)
	return store
}

// TestSanctionedCompactRecoveryExitsNamesRecoveryBindingRepairForTheDriftClass
// pins the operator-facing half of the fix: the drifted edge stops reading as a
// dead end on the two surfaces every reproduction quotes — the sanctioned exit
// in `review inspect-authority`, and the START refusal an operator actually
// hits — and both name an operation whose own read-only prediction admits it.
func TestSanctionedCompactRecoveryExitsNamesRecoveryBindingRepairForTheDriftClass(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	// The successor holds captured review results, so `review abandon` refuses
	// it and this is the shape every reproduction observes as a dead end.
	_, successor, _ := driftedRecoveryBindingPair(t, repo, "exit", "drifted exit target\n", hash("9"), captureCompactReviewResults(t))

	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || report.Totals.InvalidEdges != 1 || len(report.Edges) != 1 ||
		!strings.Contains(report.Edges[0].NonReconcilableReason, "bound to different content") {
		t.Fatalf("drifted inspection = %#v", report)
	}
	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 1 || exits[0].SuccessorLineageID != successor.State.LineageID ||
		exits[0].Operation != "review repair-recovery-binding" || exits[0].Blocked != "" {
		t.Fatalf("drifted sanctioned exit = %#v", exits)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "post-drift candidate\n")
	_, startErr := StartCompactAuthority(ctx, repo, CompactStartRequest{State: newCompactTestState(t, repo, "drifted-fresh")})
	if startErr == nil {
		t.Fatal("start accepted a repository whose compact authority graph is invalid")
	}
	for _, want := range []string{
		"invalid compact authority graph",
		"gentle-ai review repair-recovery-binding",
		"--expected-predecessor-revision",
		"gentle-ai review inspect-authority",
	} {
		if !strings.Contains(startErr.Error(), want) {
			t.Fatalf("start refusal does not name %q:\n%s", want, startErr)
		}
	}
}

func setRecoveryAuthorization(build func(state *CompactState) string) func(*CompactState) {
	return func(state *CompactState) { state.Recovery.MaintainerAuthorization = build(state) }
}

func exactRecoveryAuthorization(state *CompactState) string {
	recovery := state.Recovery
	return compactRecoveryAuthorizationBinding(recovery.PredecessorLineageID, recovery.PredecessorRevision,
		state.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
}

// TestClassifyCompactRecoveryAuthorizationTargetDriftAdmitsOnlyTheDriftShape
// accepts exactly one shape and refuses every legitimate neighbour. The rows
// that matter most are the retry edge and the two bench damage shapes: a naive
// "initial_snapshot.identity != target_identity" predicate would quarantine
// every valid retry edge on a corrected lineage, and a predicate widened to
// "schema-prefixed and does not re-derive" would swallow accounting drift that
// is deliberately outside this class.
func TestClassifyCompactRecoveryAuthorizationTargetDriftAdmitsOnlyTheDriftShape(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, successor, _ := driftedRecoveryBindingPair(t, repo, "admit", "drifted admit target\n", hash("9"))
	classification := classifyCompactRecoveryAuthorizationTargetDrift(predecessor, successor)
	if !classification.Admitted || classification.RefusalReason != nil ||
		classification.AuthorizedTargetIdentity != hash("9") ||
		classification.PersistedTargetIdentity != successor.State.InitialSnapshot.Identity ||
		!validSHA256(classification.RecordedAuthorizationSHA256) ||
		classification.ValidationError == nil {
		t.Fatalf("drift classification = %#v", classification)
	}
	recorded := sha256.Sum256([]byte(successor.State.Recovery.MaintainerAuthorization))
	if classification.RecordedAuthorizationSHA256 != "sha256:"+hex.EncodeToString(recorded[:]) {
		t.Fatalf("recorded authorization digest = %q", classification.RecordedAuthorizationSHA256)
	}

	tests := []struct {
		name  string
		build func(t *testing.T, repo string) (CompactRecord, CompactRecord)
		want  string
	}{
		{"valid edge", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "valid", "valid target\n", hash("9"), setRecoveryAuthorization(exactRecoveryAuthorization))
			return p, s
		}, "validates"},
		{"unchanged target", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, _, s, _ := poisonedRecoveryFixture(t, repo, nil)
			return p, s
		}, "outside the inexact-authorization sentinel"},
		{"bench recorded reason drift", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "reason", "reason drift target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				recovery := state.Recovery
				return compactRecoveryAuthorizationBinding(recovery.PredecessorLineageID, recovery.PredecessorRevision,
					state.InitialSnapshot.Identity, recovery.Actor, "a reason that drifted after the fact")
			}))
			return p, s
		}, "sole deviation"},
		{"bench authorization predecessor revision drift", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "revision", "revision drift target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				recovery := state.Recovery
				return compactRecoveryAuthorizationBinding(recovery.PredecessorLineageID, hash("a"),
					state.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
			}))
			return p, s
		}, "sole deviation"},
		{"actor drift", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "actor", "actor drift target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				recovery := state.Recovery
				return compactRecoveryAuthorizationBinding(recovery.PredecessorLineageID, recovery.PredecessorRevision,
					hash("9"), "other@example.com", recovery.Reason)
			}))
			return p, s
		}, "sole deviation"},
		{"pre-contract free-form authorization", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "precontract", "pre-contract target\n", hash("9"), setRecoveryAuthorization(func(*CompactState) string {
				return "maintainer approved incident retry per the 2.1.6 runbook"
			}))
			return p, s
		}, "LF-only"},
		{"carriage returns", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "crlf", "crlf target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				return strings.ReplaceAll(state.Recovery.MaintainerAuthorization, "\n", "\r\n")
			}))
			return p, s
		}, "LF-only"},
		{"five line authorization", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "fiveline", "five line target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				lines := strings.Split(state.Recovery.MaintainerAuthorization, "\n")
				return strings.Join(lines[:len(lines)-1], "\n")
			}))
			return p, s
		}, "six-line"},
		{"reordered keys", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "reordered", "reordered target\n", hash("9"), setRecoveryAuthorization(func(state *CompactState) string {
				lines := strings.Split(state.Recovery.MaintainerAuthorization, "\n")
				lines[4], lines[5] = lines[5], lines[4]
				return strings.Join(lines, "\n")
			}))
			return p, s
		}, "out of order"},
		{"target identity is not a digest", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "digest", "digest target\n", "not-a-digest")
			return p, s
		}, "not a sha256 identity"},
		{"accounting-only escalation", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "accounting", "accounting target\n", hash("9"), func(state *CompactState) {
				state.Recovery.Evidence = &CompactRecoveredEvidence{SuccessorTargetIdentity: hash("9")}
			})
			return p, s
		}, "accounting-only escalation"},
		{"final verification retry proof", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "retryproof", "retry proof target\n", hash("9"), func(state *CompactState) {
				state.Recovery.FinalVerificationRetry = &CompactFinalVerificationRetryProof{TerminalRevision: hash("b")}
			})
			return p, s
		}, "final-verification retry"},
		{"final verification retry disposition", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "retryedge", "retry edge target\n", hash("9"), func(state *CompactState) {
				state.Recovery.Disposition = RecoveryFinalVerificationRetry
			})
			return p, s
		}, "outside the escalated recovery-authorization class"},
		{"scope-changed disposition", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "scope", "scope target\n", hash("9"), func(state *CompactState) {
				state.Recovery.Disposition = RecoveryScopeChanged
			})
			return p, s
		}, "outside the escalated recovery-authorization class"},
		{"release scope sentinel", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "release", "release target\n", hash("9"), setRecoveryAuthorization(func(*CompactState) string {
				return ReleaseScopeRecoveryAuthorization
			}))
			return p, s
		}, "LF-only"},
		{"not a recovery successor", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "norecovery", "no recovery target\n", hash("9"), func(state *CompactState) {
				state.Recovery = nil
			})
			return p, s
		}, "not a recovery successor"},
		{"second graph defect", func(t *testing.T, repo string) (CompactRecord, CompactRecord) {
			p, s, _ := driftedRecoveryBindingPair(t, repo, "generation", "generation target\n", hash("9"), func(state *CompactState) {
				state.Generation += 3
			})
			return p, s
		}, "outside the inexact-authorization sentinel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolated := initSnapshotRepo(t)
			refusedPredecessor, refusedSuccessor := tt.build(t, isolated)
			refused := classifyCompactRecoveryAuthorizationTargetDrift(refusedPredecessor, refusedSuccessor)
			if refused.Admitted || refused.RefusalReason == nil || !strings.Contains(refused.RefusalReason.Error(), tt.want) {
				t.Fatalf("classification = %#v, want a refusal naming %q", refused, tt.want)
			}
		})
	}
}

func driftedRepairCandidate(t *testing.T, repo, successorLineage string) CompactRecoveryBindingRepairCandidate {
	t.Helper()
	candidate, admitted := newCompactRecoveryBindingRepairExitProbe(context.Background(), repo).candidate(successorLineage)
	if !admitted {
		t.Fatalf("recovery binding repair eligibility for %q = %#v", successorLineage, candidate)
	}
	return candidate
}

func driftedRepairRequest(t *testing.T, candidate CompactRecoveryBindingRepairCandidate) CompactRecoveryBindingRepairRequest {
	t.Helper()
	request := CompactRecoveryBindingRepairRequest{
		PredecessorLineageID: candidate.PredecessorLineageID, ExpectedPredecessorRevision: candidate.ExpectedPredecessorRevision,
		SuccessorLineageID: candidate.SuccessorLineageID, ExpectedSuccessorRevision: candidate.ExpectedSuccessorRevision,
		Actor: "maintainer@example.com", Reason: "quarantine the drifted recovery binding",
		RepairedAt: time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC),
	}
	request.MaintainerAuthorization = compactRecoveryBindingRepairAuthorizationBinding(
		candidate.RepositoryBinding, candidate, request.Actor, request.Reason)
	return request
}

func sameCompactReclaimRecord(left, right CompactReclaimRecord) bool {
	if left.Schema != right.Schema || left.Status != right.Status || left.LineageID != right.LineageID ||
		left.Reason != right.Reason || left.Actor != right.Actor || left.SourcePath != right.SourcePath ||
		left.QuarantinePath != right.QuarantinePath || !left.ReclaimedAt.Equal(right.ReclaimedAt) ||
		!reflect.DeepEqual(left.Residue, right.Residue) {
		return false
	}
	return reflect.DeepEqual(left.RecoveryBindingRepair, right.RecoveryBindingRepair)
}

// assertRepairUntouched proves a refusal moved nothing: the successor bytes are
// byte-identical and no quarantine entry was created.
func assertRepairUntouched(t *testing.T, repo string, store CompactStore, payload []byte) {
	t.Helper()
	current, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("a refused recovery binding repair rewrote the successor state: %v", err)
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused recovery binding repair created %d quarantine entries", len(entries))
	}
}

// TestRepairCompactRecoveryBindingCommitsReplaysAndPreservesUnrelatedAuthority
// is the executor half of the fix: an authorized, compare-and-swap guarded
// disposition that moves the successor whole, preserves the drifted
// authorization bytes verbatim in the quarantined residue, leaves unrelated
// authority byte-identical, and converges on an exact replay.
func TestRepairCompactRecoveryBindingCommitsReplaysAndPreservesUnrelatedAuthority(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	predecessor, successor, successorStore := driftedRecoveryBindingPair(t, repo, "commit", "drifted commit target\n", hash("9"), captureCompactReviewResults(t))
	unrelatedStore, err := CompactAuthoritativeStore(ctx, repo, "drifted-unrelated")
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, unrelatedStore, newCompactTestState(t, repo, "drifted-unrelated"))
	unrelatedBefore, err := os.ReadFile(unrelatedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	successorBefore, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	predecessorStore, err := CompactAuthoritativeStore(ctx, repo, predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	candidate := driftedRepairCandidate(t, repo, successor.State.LineageID)
	if candidate.Class != AuthorityRepairClassCompactV2RecoveryTargetDrift ||
		candidate.Cause != AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted ||
		candidate.Disposition != AuthorityRepairDispositionQuarantineDriftedRecoveryBinding ||
		candidate.PredecessorLineageID != predecessor.State.LineageID ||
		candidate.ExpectedPredecessorRevision != predecessor.Revision ||
		candidate.ExpectedSuccessorRevision != successor.Revision || candidate.Blocked != "" {
		t.Fatalf("published candidate = %#v", candidate)
	}
	request := driftedRepairRequest(t, candidate)
	committed, err := RepairCompactRecoveryBinding(ctx, repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Status != CompactReclaimCommitted || committed.LineageID != successor.State.LineageID ||
		committed.RecoveryBindingRepair == nil || committed.InvalidRecoveryEdge != nil ||
		committed.MalformedRecoveryAuthorization != nil || committed.ClassifiedAuthorityRepair != nil ||
		committed.LegacyAliasRepair != nil || committed.PristineAbandonment != nil ||
		committed.IncompleteAbandonment != nil || committed.MalformedLegacyFreeze != nil ||
		committed.LegacyFixScopeQuarantine != nil {
		t.Fatalf("committed recovery binding repair = %#v", committed)
	}
	proof := committed.RecoveryBindingRepair
	if proof.AuthorizedTargetIdentity != hash("9") || proof.PersistedTargetIdentity != successor.State.InitialSnapshot.Identity ||
		proof.PredecessorRevision != predecessor.Revision || proof.SuccessorRevision != successor.Revision ||
		proof.AuthorizationSchema != AuthorityRepairAuthorizationSchema || !validSHA256(proof.RequestDigest) ||
		!strings.Contains(proof.ValidationError, "exact maintainer authorization binding") {
		t.Fatalf("committed proof = %#v", proof)
	}
	if _, statErr := os.Stat(successorStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("committed recovery binding repair left the source entry: %v", statErr)
	}
	moved, err := os.ReadFile(filepath.Join(committed.QuarantinePath, "residue", compactStateFileName))
	if err != nil || !bytes.Equal(moved, successorBefore) {
		t.Fatalf("quarantined residue is not the persisted successor byte-for-byte: %v", err)
	}
	for _, preserved := range []struct {
		store CompactStore
		want  []byte
	}{{unrelatedStore, unrelatedBefore}, {predecessorStore, predecessorBefore}} {
		after, readErr := os.ReadFile(preserved.store.StatePath())
		if readErr != nil || !bytes.Equal(after, preserved.want) {
			t.Fatalf("recovery binding repair changed unrelated authority %q: %v", preserved.store.Dir, readErr)
		}
	}

	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.Totals.InvalidEdges != 0 {
		t.Fatalf("post-repair inspection = %#v", report)
	}
	inventory, err := InventoryAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.Complete || !inventory.Authoritative {
		t.Fatalf("post-repair inventory = %#v", inventory)
	}

	replayed, err := RepairCompactRecoveryBinding(ctx, repo, request)
	if err != nil || !sameCompactReclaimRecord(replayed, committed) {
		t.Fatalf("exact replay = %#v, %v", replayed, err)
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("exact replay created a second quarantine path: %d, %v", len(entries), err)
	}
}

// TestRepairCompactRecoveryBindingRefusesEveryMismatchWithoutMutation walks the
// full refusal matrix. Every row must refuse before any durable mutation, so
// the successor bytes stay identical and no quarantine entry is created.
func TestRepairCompactRecoveryBindingRefusesEveryMismatchWithoutMutation(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "refuse", "drifted refuse target\n", hash("9"), captureCompactReviewResults(t))
	unrelatedStore, err := CompactAuthoritativeStore(ctx, repo, "drifted-bystander")
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, unrelatedStore, newCompactTestState(t, repo, "drifted-bystander"))
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	candidate := driftedRepairCandidate(t, repo, successor.State.LineageID)

	tests := []struct {
		name    string
		prepare func(request *CompactRecoveryBindingRepairRequest)
		want    string
	}{
		{"wrong actor", func(request *CompactRecoveryBindingRepairRequest) { request.Actor = "other@example.com" }, "exact maintainer authorization binding"},
		{"wrong reason", func(request *CompactRecoveryBindingRepairRequest) { request.Reason = "a different reason" }, "exact maintainer authorization binding"},
		{"trailing newline", func(request *CompactRecoveryBindingRepairRequest) { request.MaintainerAuthorization += "\n" }, "exact maintainer authorization binding"},
		{"carriage returns", func(request *CompactRecoveryBindingRepairRequest) {
			request.MaintainerAuthorization = strings.ReplaceAll(request.MaintainerAuthorization, "\n", "\r\n")
		}, "request is invalid"},
		{"empty actor", func(request *CompactRecoveryBindingRepairRequest) { request.Actor = "" }, "request is invalid"},
		{"identical lineages", func(request *CompactRecoveryBindingRepairRequest) {
			request.PredecessorLineageID = request.SuccessorLineageID
		}, "request is invalid"},
		{"stale predecessor revision", func(request *CompactRecoveryBindingRepairRequest) {
			request.ExpectedPredecessorRevision = hash("f")
		}, ErrConcurrentUpdate.Error()},
		{"stale successor revision", func(request *CompactRecoveryBindingRepairRequest) {
			request.ExpectedSuccessorRevision = hash("f")
		}, ErrConcurrentUpdate.Error()},
		{"mismatched predecessor lineage", func(request *CompactRecoveryBindingRepairRequest) {
			request.PredecessorLineageID = "drifted-bystander"
		}, "names predecessor"},
		{"authorization bound to another target", func(request *CompactRecoveryBindingRepairRequest) {
			forged := candidate
			forged.AuthorizedTargetIdentity = hash("e")
			request.MaintainerAuthorization = compactRecoveryBindingRepairAuthorizationBinding(
				candidate.RepositoryBinding, forged, request.Actor, request.Reason)
		}, "exact maintainer authorization binding"},
		{"authorization bound to another repository", func(request *CompactRecoveryBindingRepairRequest) {
			request.MaintainerAuthorization = compactRecoveryBindingRepairAuthorizationBinding(
				hash("d"), candidate, request.Actor, request.Reason)
		}, "exact maintainer authorization binding"},
		{"unknown successor", func(request *CompactRecoveryBindingRepairRequest) {
			request.SuccessorLineageID = "drifted-absent"
		}, "holds no compact authority state"},
		{"successor that is not a recovery successor", func(request *CompactRecoveryBindingRepairRequest) {
			request.SuccessorLineageID = "drifted-bystander"
		}, "not a recovery successor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := driftedRepairRequest(t, candidate)
			tt.prepare(&request)
			record, err := RepairCompactRecoveryBinding(ctx, repo, request)
			if err == nil {
				t.Fatalf("recovery binding repair accepted %s: %#v", tt.name, record)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("refusal for %s does not name %q:\n%v", tt.name, tt.want, err)
			}
			assertRepairUntouched(t, repo, successorStore, persisted)
		})
	}
}

// TestRepairCompactRecoveryBindingRechecksCASUnderMaintenanceOwnership proves
// the preflight prediction is advisory and the in-lock compare-and-swap is what
// authorizes anything, and that the refusal distinguishes "the store changed
// under you" from "you were always wrong".
func TestRepairCompactRecoveryBindingRechecksCASUnderMaintenanceOwnership(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "cas", "drifted cas target\n", hash("9"), captureCompactReviewResults(t))
	candidate := driftedRepairCandidate(t, repo, successor.State.LineageID)
	request := driftedRepairRequest(t, candidate)

	advanced := successor.State
	provenance := *successor.State.Recovery
	provenance.RecoveredAt = provenance.RecoveredAt.Add(time.Hour)
	advanced.Recovery = &provenance
	compactRecoveryBindingRepairAfterAssessmentHook = func() {
		writeCompactFixtureRecord(t, successorStore, advanced)
		compactRecoveryBindingRepairAfterAssessmentHook = func() {}
	}
	t.Cleanup(func() { compactRecoveryBindingRepairAfterAssessmentHook = func() {} })
	if _, err := RepairCompactRecoveryBinding(ctx, repo, request); !errors.Is(err, ErrConcurrentUpdate) ||
		!strings.Contains(err.Error(), "changed before exclusive maintenance ownership") {
		t.Fatalf("stale recovery binding repair = %v", err)
	}
	if _, statErr := os.Stat(successorStore.Dir); statErr != nil {
		t.Fatalf("stale recovery binding repair moved the source: %v", statErr)
	}

	// A request that never matched is refused plainly; only a request that did
	// match earns the concurrency wording.
	neverMatched := driftedRepairRequest(t, candidate)
	neverMatched.ExpectedSuccessorRevision = hash("c")
	neverMatched.MaintainerAuthorization = compactRecoveryBindingRepairAuthorizationBinding(
		candidate.RepositoryBinding, candidate, neverMatched.Actor, neverMatched.Reason)
	if _, err := RepairCompactRecoveryBinding(ctx, repo, neverMatched); !errors.Is(err, ErrConcurrentUpdate) ||
		strings.Contains(err.Error(), "changed before exclusive maintenance ownership") {
		t.Fatalf("never-matching recovery binding repair = %v", err)
	}
	if _, statErr := os.Stat(successorStore.Dir); statErr != nil {
		t.Fatalf("never-matching recovery binding repair moved the source: %v", statErr)
	}
}

// TestRepairCompactRecoveryBindingSequentiallyRepairsTwoDriftedEdges is the
// boundary proof: the repository never has to contain only one unsupported
// edge. Two independent drifted edges repair one transaction at a time, and the
// mid-state stays honestly fail-closed while still advertising the second.
func TestRepairCompactRecoveryBindingSequentiallyRepairsTwoDriftedEdges(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, alpha, _ := driftedRecoveryBindingPair(t, repo, "alpha", "drifted alpha target\n", hash("9"), captureCompactReviewResults(t))
	_, bravo, _ := driftedRecoveryBindingPair(t, repo, "bravo", "drifted bravo target\n", hash("8"), captureCompactReviewResults(t))

	if _, err := CompactAuthorityLeaves(ctx, repo); err == nil ||
		!strings.Contains(err.Error(), "invalid compact authority graph: escalated recovery requires an exact maintainer authorization binding") {
		t.Fatalf("two-edge leaves error = %v", err)
	}
	before, err := InventoryAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Complete || before.Authoritative ||
		!hasAuthorityInventoryStatus(before.Entries, alpha.State.LineageID, AuthorityStatusInvalid) ||
		!hasAuthorityInventoryStatus(before.Entries, bravo.State.LineageID, AuthorityStatusInvalid) {
		t.Fatalf("two-edge inventory = %#v", before)
	}
	candidates, counts, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{})
	if err != nil || len(candidates) != 2 || counts.AdmissibleCandidates != 2 || counts.BlockedCandidates != 0 ||
		counts.RecoveryEdges != 2 || counts.InvalidEdges != 2 {
		t.Fatalf("two-edge candidates = %#v, %#v, %v", candidates, counts, err)
	}

	first := driftedRepairRequest(t, driftedRepairCandidate(t, repo, alpha.State.LineageID))
	committedFirst, err := RepairCompactRecoveryBinding(ctx, repo, first)
	if err != nil {
		t.Fatal(err)
	}

	// The mid-state is the direct proof: still fail-closed, still advertising
	// the second edge, and the first repair still converges on replay.
	middle, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if middle.Valid || middle.Totals.InvalidEdges != 1 {
		t.Fatalf("mid-state inspection = %#v", middle)
	}
	remaining, remainingCounts, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{})
	if err != nil || len(remaining) != 1 || remaining[0].SuccessorLineageID != bravo.State.LineageID ||
		remainingCounts.AdmissibleCandidates != 1 {
		t.Fatalf("mid-state candidates = %#v, %#v, %v", remaining, remainingCounts, err)
	}
	replayedFirst, err := RepairCompactRecoveryBinding(ctx, repo, first)
	if err != nil || !sameCompactReclaimRecord(replayedFirst, committedFirst) {
		t.Fatalf("mid-state replay = %#v, %v", replayedFirst, err)
	}

	if _, err := RepairCompactRecoveryBinding(ctx, repo, driftedRepairRequest(t, remaining[0])); err != nil {
		t.Fatal(err)
	}
	after, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Valid || after.Totals.InvalidEdges != 0 {
		t.Fatalf("post-repair inspection = %#v", after)
	}
	inventory, err := InventoryAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.Complete || !inventory.Authoritative {
		t.Fatalf("post-repair inventory = %#v", inventory)
	}
	leaves, err := CompactAuthorityLeaves(ctx, repo)
	if err != nil || len(leaves) != 2 {
		t.Fatalf("post-repair leaves = %#v, %v", leaves, err)
	}
}

// TestRepairCompactRecoveryBindingRefusesNonLeafSuccessorAndNamesLeafFirstOrder
// pins the honest boundary of a single-edge repair: quarantining a successor
// that carries its own successor would orphan the dependent edge, so the repair
// refuses, names the ordering rule, and the preflight still lists the candidate
// with the lineage to disposition first.
func TestRepairCompactRecoveryBindingRefusesNonLeafSuccessorAndNamesLeafFirstOrder(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "chain", "drifted chain target\n", hash("9"), captureCompactReviewResults(t))
	candidate := driftedRepairCandidate(t, repo, successor.State.LineageID)
	request := driftedRepairRequest(t, candidate)
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	driftedRecoveryBindingDescendant(t, repo, "drifted-descendant", successor)

	blocked, admitted := newCompactRecoveryBindingRepairExitProbe(ctx, repo).candidate(successor.State.LineageID)
	if !admitted || !strings.Contains(blocked.Blocked, "drifted-descendant") ||
		!strings.Contains(blocked.Blocked, "one leaf edge at a time") {
		t.Fatalf("non-leaf eligibility = %#v, %v", blocked, admitted)
	}
	candidates, counts, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{})
	if err != nil || len(candidates) != 1 || candidates[0].Blocked == "" ||
		counts.AdmissibleCandidates != 0 || counts.BlockedCandidates != 1 {
		t.Fatalf("non-leaf candidates = %#v, %#v, %v", candidates, counts, err)
	}
	if _, err := RepairCompactRecoveryBinding(ctx, repo, request); err == nil ||
		!strings.Contains(err.Error(), "one leaf edge at a time") ||
		!strings.Contains(err.Error(), "gentle-ai review inspect-authority") {
		t.Fatalf("non-leaf recovery binding repair = %v", err)
	}
	assertRepairUntouched(t, repo, successorStore, persisted)

	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil {
		t.Fatal(err)
	}
	named := false
	for _, exit := range exits {
		if exit.SuccessorLineageID != successor.State.LineageID {
			continue
		}
		named = exit.Operation == "" && strings.Contains(exit.Blocked, CompactRecoveryEdgeExitRepairRecoveryBinding)
	}
	if !named {
		t.Fatalf("non-leaf sanctioned exits = %#v", exits)
	}
}

// TestRepairCompactRecoveryBindingResumesEachDurablePhase interrupts the repair
// at each durable seam and proves the returned record locates the orphan and a
// retry converges on committed from either crash point.
func TestRepairCompactRecoveryBindingResumesEachDurablePhase(t *testing.T) {
	ctx := context.Background()
	t.Run("before the residue rename", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := driftedRecoveryBindingPair(t, repo, "prepared", "prepared target\n", hash("9"), captureCompactReviewResults(t))
		request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
		original := reclaimQuarantineResidue
		t.Cleanup(func() { reclaimQuarantineResidue = original })
		reclaimQuarantineResidue = func(string, string) error { return errors.New("interrupted before the rename") }
		prepared, err := RepairCompactRecoveryBinding(ctx, repo, request)
		if err == nil || prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" {
			t.Fatalf("interrupted repair = %#v, %v", prepared, err)
		}
		if _, statErr := os.Stat(filepath.Join(prepared.QuarantinePath, "reclaim-record.json")); statErr != nil {
			t.Fatalf("interrupted repair did not persist its prepared record: %v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(prepared.QuarantinePath, "residue")); !os.IsNotExist(statErr) {
			t.Fatalf("interrupted repair moved residue it reported as unmoved: %v", statErr)
		}
		reclaimQuarantineResidue = original
		committed, err := RepairCompactRecoveryBinding(ctx, repo, request)
		if err != nil || committed.Status != CompactReclaimCommitted || committed.QuarantinePath != prepared.QuarantinePath {
			t.Fatalf("resumed repair = %#v, %v", committed, err)
		}
	})
	t.Run("before the audit commit", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := driftedRecoveryBindingPair(t, repo, "renamed", "renamed target\n", hash("9"), captureCompactReviewResults(t))
		request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
		original := persistReclaimRecord
		t.Cleanup(func() { persistReclaimRecord = original })
		writes := 0
		persistReclaimRecord = func(record CompactReclaimRecord) error {
			writes++
			if writes == 2 {
				return errors.New("interrupted before the audit commit")
			}
			return original(record)
		}
		prepared, err := RepairCompactRecoveryBinding(ctx, repo, request)
		if err == nil || prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" {
			t.Fatalf("interrupted repair = %#v, %v", prepared, err)
		}
		if _, statErr := os.Stat(filepath.Join(prepared.QuarantinePath, "residue")); statErr != nil {
			t.Fatalf("interrupted repair reported a moved residue it did not move: %v", statErr)
		}
		persistReclaimRecord = original
		committed, err := RepairCompactRecoveryBinding(ctx, repo, request)
		if err != nil || committed.Status != CompactReclaimCommitted || committed.QuarantinePath != prepared.QuarantinePath {
			t.Fatalf("resumed repair = %#v, %v", committed, err)
		}
	})
}

// TestRepairCompactRecoveryBindingConcurrentExecutionCommitsAndReplays proves
// two racing maintainers with the exact same authorized request both observe
// the same committed record and only one quarantine exists.
func TestRepairCompactRecoveryBindingConcurrentExecutionCommitsAndReplays(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, _ := driftedRecoveryBindingPair(t, repo, "concurrent", "concurrent target\n", hash("9"), captureCompactReviewResults(t))
	request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))

	var arrivals atomic.Int32
	compactRecoveryBindingRepairAfterAssessmentHook = func() {
		if arrivals.Add(1) == 1 {
			for arrivals.Load() < 2 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	t.Cleanup(func() { compactRecoveryBindingRepairAfterAssessmentHook = func() {} })
	records := make([]CompactReclaimRecord, 2)
	failures := make([]error, 2)
	var group sync.WaitGroup
	group.Add(len(records))
	for index := range records {
		go func(index int) {
			defer group.Done()
			records[index], failures[index] = RepairCompactRecoveryBinding(ctx, repo, request)
		}(index)
	}
	group.Wait()
	for index, err := range failures {
		if err != nil {
			t.Fatalf("concurrent recovery binding repair %d = %v", index, err)
		}
	}
	if !sameCompactReclaimRecord(records[0], records[1]) || records[0].Status != CompactReclaimCommitted {
		t.Fatalf("concurrent recovery binding repair records = %#v", records)
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("concurrent recovery binding repair quarantines = %d, %v", len(entries), err)
	}
}

// TestRepairCompactRecoveryBindingFailsClosedOnAnUnreadableRelatedEntry is the
// completeness proof for the one graph view every fence reads. The leaf-first
// blocker, the removal-regression invariant and the readback proof all derive
// from the same record set, so an entry silently dropped for failing to load is
// invisible to all three at once: the repair would move a successor a real,
// unread descendant still names, INTRODUCING a dangling predecessor — precisely
// the defect it exists to clear. The damage shape used here is the unreadable
// state an interrupted write leaves behind, which is deliberately outside the
// narrow quarantinable tolerance.
func TestRepairCompactRecoveryBindingFailsClosedOnAnUnreadableRelatedEntry(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "unreadable", "drifted unreadable target\n", hash("9"), captureCompactReviewResults(t))
	request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	descendantStore := driftedRecoveryBindingDescendant(t, repo, "unreadable-descendant", successor)
	descendantBytes, err := os.ReadFile(descendantStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descendantStore.StatePath(), descendantBytes[:len(descendantBytes)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, loadErr := descendantStore.Load(); loadErr == nil {
		t.Fatal("the truncated descendant still loads, so the fixture proves nothing")
	} else if _, quarantinable := compactLineageQuarantinable(loadErr); quarantinable {
		t.Fatalf("the truncated descendant is quarantinable, so it is the wrong damage shape: %v", loadErr)
	}

	if candidate, admitted := newCompactRecoveryBindingRepairExitProbe(ctx, repo).candidate(successor.State.LineageID); admitted {
		t.Fatalf("unreadable-neighbour eligibility = %#v, %v", candidate, admitted)
	}
	if _, err := RepairCompactRecoveryBinding(ctx, repo, request); err == nil ||
		!strings.Contains(err.Error(), "does not load") {
		t.Fatalf("unreadable-neighbour repair = %v", err)
	}
	assertRepairUntouched(t, repo, successorStore, persisted)
	// The repository stays exactly as damaged as it already was: the refusal
	// never trades one defect for a dangling predecessor.
	if _, leavesErr := CompactAuthorityLeaves(ctx, repo); leavesErr != nil &&
		strings.Contains(leavesErr.Error(), "dangling predecessor") {
		t.Fatalf("a refused repair introduced a graph defect: %v", leavesErr)
	}
}

// TestRepairCompactRecoveryBindingReAdmitsBeforeResumingAPreparedMove proves a
// prepared replay is a mutation, not a convergence. The source entry is still in
// place at that point, so resuming on the recorded proof alone would perform the
// durable move without any live gate. A descendant that appeared inside the
// crash window must block the resume exactly as it blocks a fresh repair.
func TestRepairCompactRecoveryBindingReAdmitsBeforeResumingAPreparedMove(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "readmit", "drifted readmit target\n", hash("9"), captureCompactReviewResults(t))
	request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	original := reclaimQuarantineResidue
	t.Cleanup(func() { reclaimQuarantineResidue = original })
	reclaimQuarantineResidue = func(string, string) error { return errors.New("interrupted before the rename") }
	prepared, err := RepairCompactRecoveryBinding(ctx, repo, request)
	if err == nil || prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" {
		t.Fatalf("interrupted repair = %#v, %v", prepared, err)
	}
	reclaimQuarantineResidue = original

	driftedRecoveryBindingDescendant(t, repo, "readmit-descendant", successor)
	resumed, err := RepairCompactRecoveryBinding(ctx, repo, request)
	if err == nil || !strings.Contains(err.Error(), "one leaf edge at a time") ||
		!strings.Contains(err.Error(), "gentle-ai review repair-recovery-binding --preflight") {
		t.Fatalf("resumed repair against a new descendant = %#v, %v", resumed, err)
	}
	current, readErr := os.ReadFile(successorStore.StatePath())
	if readErr != nil || !bytes.Equal(current, persisted) {
		t.Fatalf("a refused resume moved or rewrote the source: %v", readErr)
	}
	if _, statErr := os.Stat(filepath.Join(prepared.QuarantinePath, "residue")); !os.IsNotExist(statErr) {
		t.Fatalf("a refused resume moved the residue: %v", statErr)
	}
}

// TestRepairCompactRecoveryBindingRefusesWhileABatchReconciliationIsPrepared
// keeps the advertisement and the executor on the same side of the maintenance
// boundary: a prepared batch owns authority maintenance, so the verb is neither
// advertised nor runnable inside that window, and the refusal is typed rather
// than a hang.
func TestRepairCompactRecoveryBindingRefusesWhileABatchReconciliationIsPrepared(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, successorStore := driftedRecoveryBindingPair(t, repo, "batch", "drifted batch target\n", hash("9"), captureCompactReviewResults(t))
	request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
	persisted, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	marker := compactBatchReconcileMarkerPath(base)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := RepairCompactRecoveryBinding(ctx, repo, request); !errors.Is(err, ErrCompactBatchReconcilePrepared) {
		t.Fatalf("prepared-batch repair = %v", err)
	}
	if candidate, admitted := newCompactRecoveryBindingRepairExitProbe(ctx, repo).candidate(successor.State.LineageID); admitted {
		t.Fatalf("prepared-batch eligibility = %#v, %v", candidate, admitted)
	}
	if _, _, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{}); !errors.Is(err, ErrCompactBatchReconcilePrepared) {
		t.Fatalf("prepared-batch candidates = %v", err)
	}
	assertRepairUntouched(t, repo, successorStore, persisted)
}

// TestSanctionedCompactRecoveryExitsPrefersRepairOverAbandonForAPristineDrift
// pins a deliberate precedence change on an already-working path. A PRISTINE
// drifted successor used to be offered `review abandon`, which discards it.
// The drift class is preferred because it moves the successor whole into the
// audited quarantine with the natively re-derived proof, so the drifted
// authorization bytes survive verbatim. Abandonment stays eligible; the order
// is the decision, not an accident of evaluation.
func TestSanctionedCompactRecoveryExitsPrefersRepairOverAbandonForAPristineDrift(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, _ := driftedRecoveryBindingPair(t, repo, "pristine", "drifted pristine target\n", hash("9"))
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, successor.State.LineageID)
	if err != nil || !eligibility.Eligible {
		t.Fatalf("pristine abandonment eligibility = %#v, %v", eligibility, err)
	}
	report, err := InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
	if err != nil || len(exits) != 1 || exits[0].SuccessorLineageID != successor.State.LineageID ||
		exits[0].Operation != CompactRecoveryEdgeExitRepairRecoveryBinding || exits[0].Blocked != "" {
		t.Fatalf("pristine drifted sanctioned exit = %#v, %v", exits, err)
	}
}

// TestRepairCompactRecoveryBindingRefusesForgedReplayProof proves the replay
// path re-derives the drift from the quarantined bytes instead of trusting the
// recorded proof, so a fabricated audit can never replay as a genuine repair.
func TestRepairCompactRecoveryBindingRefusesForgedReplayProof(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, successor, _ := driftedRecoveryBindingPair(t, repo, "forgedreplay", "forged replay target\n", hash("9"), captureCompactReviewResults(t))
	request := driftedRepairRequest(t, driftedRepairCandidate(t, repo, successor.State.LineageID))
	committed, err := RepairCompactRecoveryBinding(ctx, repo, request)
	if err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(committed.QuarantinePath, "residue", compactStateFileName)
	payload, err := os.ReadFile(residue)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(residue, bytes.Replace(payload, []byte(hash("9")), []byte(hash("7")), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RepairCompactRecoveryBinding(ctx, repo, request); err == nil ||
		!strings.Contains(err.Error(), "refused forged replay proof") {
		t.Fatalf("forged replay = %v", err)
	}
}

// driftedRecoveryBindingSibling persists a SECOND drifted successor of an
// existing predecessor, so one predecessor forks into two drifted edges. The
// pair helper always mints its own predecessor, so a fork cannot be built from
// two of those.
func driftedRecoveryBindingSibling(t *testing.T, repo, lineage, target, authorizedTarget string, predecessor CompactRecord, mutate ...func(*CompactState)) CompactRecord {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", target)
	state := newCompactTestState(t, repo, lineage)
	state.Generation = predecessor.State.Generation + 1
	state.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.State.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Actor: "maintainer@example.com",
		Reason:      "refresh current verify-report evidence after approved review",
		RecoveredAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	}
	state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		predecessor.State.LineageID, predecessor.Revision, authorizedTarget,
		state.Recovery.Actor, state.Recovery.Reason)
	for _, adjust := range mutate {
		adjust(&state)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return writeCompactFixtureRecord(t, store, state)
}

// TestRepairCompactRecoveryBindingBreaksForkedSiblingDeadlock pins the fork
// shape. One predecessor with two drifted successors must never have each
// sibling refuse citing the other: compactAuthorityRemovalRegression documents
// that exact mutual refusal as leaving a store unrecoverable forever, because
// neither edge can go first. Drifted edges fail validateCompactRecoveryEdge and
// so are never recorded as a fork by compactAuthorityGraphViolations, which is
// why two of them can coexist under one predecessor and why this ordering is
// the verb's own responsibility.
func TestRepairCompactRecoveryBindingBreaksForkedSiblingDeadlock(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	predecessor, first, _ := driftedRecoveryBindingPair(t, repo, "fork", "forked first target\n", hash("9"), captureCompactReviewResults(t))
	second := driftedRecoveryBindingSibling(t, repo, "forked-sibling", "forked second target\n", hash("8"), predecessor, captureCompactReviewResults(t))
	if first.State.LineageID >= second.State.LineageID {
		t.Fatalf("fixture ordering broken: %q must sort before %q", first.State.LineageID, second.State.LineageID)
	}

	candidates, _, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{})
	if err != nil || len(candidates) != 2 {
		t.Fatalf("forked candidates = %#v, %v", candidates, err)
	}
	if candidates[0].SuccessorLineageID != first.State.LineageID || candidates[0].Blocked != "" {
		t.Fatalf("published-first forked candidate must be runnable, got %#v", candidates[0])
	}
	if candidates[1].SuccessorLineageID != second.State.LineageID ||
		!strings.Contains(candidates[1].Blocked, first.State.LineageID) {
		t.Fatalf("second forked candidate must defer to the first, got %#v", candidates[1])
	}

	if _, err := RepairCompactRecoveryBinding(ctx, repo, driftedRepairRequest(t, candidates[0])); err != nil {
		t.Fatalf("repair of the published-first forked edge = %v", err)
	}
	remaining, _, err := InspectCompactRecoveryBindingRepairCandidates(ctx, repo, CompactRecoveryBindingRepairSelector{})
	if err != nil || len(remaining) != 1 || remaining[0].SuccessorLineageID != second.State.LineageID || remaining[0].Blocked != "" {
		t.Fatalf("after the first repair the sibling must be runnable, got %#v, %v", remaining, err)
	}
	if _, err := RepairCompactRecoveryBinding(ctx, repo, driftedRepairRequest(t, remaining[0])); err != nil {
		t.Fatalf("repair of the forked sibling = %v", err)
	}
	if _, err := CompactAuthorityLeaves(ctx, repo); err != nil {
		t.Fatalf("forked repository must be valid after both repairs: %v", err)
	}
}
