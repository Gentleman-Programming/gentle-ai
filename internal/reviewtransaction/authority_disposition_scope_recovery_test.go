package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const historicalInvalidCorrectionScopeClass = "invalid_correction_required_scope_recovery"

type historicalInvalidCorrectionScopeFixture struct {
	repo                  string
	predecessor           CompactRecord
	successor             CompactRecord
	predecessorStore      CompactStore
	successorStore        CompactStore
	predecessorBytes      []byte
	successorBytes        []byte
	receiptBytes          []byte
	candidateTreeEntry    string
	candidateDeletedBytes string
}

// historicalInvalidCorrectionScopeRecoveryFixture pins the #1589 shape: an
// exact-binding compact-v2 scope_changed edge from correction_required whose
// disjoint tracked-path transition is neither expansion nor contraction.
func historicalInvalidCorrectionScopeRecoveryFixture(t *testing.T, suffix string) historicalInvalidCorrectionScopeFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	predecessorState, _ := pendingCompactCorrection(t, repo, "scope-predecessor-"+suffix)
	if predecessorState.State != StateCorrectionRequired || !reflect.DeepEqual(predecessorState.GenesisPaths, []string{"tracked.txt"}) {
		t.Fatalf("predecessor does not pin correction_required tracked.txt scope: %#v", predecessorState)
	}
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := writeCompactFixtureRecord(t, predecessorStore, predecessorState)

	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	writeSnapshotFile(t, repo, "deleted.txt", "historical successor candidate\n")
	successorState := newCompactTestState(t, repo, "scope-successor-"+suffix)
	if !reflect.DeepEqual(successorState.InitialSnapshot.Paths, []string{"deleted.txt"}) {
		t.Fatalf("successor does not pin the disjoint deleted.txt scope: %v", successorState.InitialSnapshot.Paths)
	}
	successorState.Generation = predecessorState.Generation + 1
	const actor, reason = "maintainer@example.com", "historical correction scope recovery"
	successorState.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessorState.LineageID,
		PredecessorRevision:  predecessor.Revision,
		Disposition:          RecoveryScopeChanged,
		Reason:               reason,
		Actor:                actor,
		RecoveredAt:          time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(
			predecessorState.LineageID, predecessor.Revision, successorState.InitialSnapshot.Identity, actor, reason),
	}
	results := make([]LensResult, len(successorState.SelectedLenses))
	for index, lens := range successorState.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"historical review completed"}}
	}
	if err := successorState.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if err := successorState.CompleteVerification([]byte("historical verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	successorStore, err := CompactAuthoritativeStore(context.Background(), repo, successorState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	successor := writeCompactFixtureRecord(t, successorStore, successorState)
	receipt, err := successorState.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(successorStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}

	read := func(path string) []byte {
		t.Helper()
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	return historicalInvalidCorrectionScopeFixture{
		repo: repo, predecessor: predecessor, successor: successor,
		predecessorStore: predecessorStore, successorStore: successorStore,
		predecessorBytes: read(predecessorStore.StatePath()), successorBytes: read(successorStore.StatePath()),
		receiptBytes:          read(successorStore.ReceiptPath()),
		candidateTreeEntry:    strings.TrimSpace(gitSnapshot(t, repo, "ls-tree", successorState.InitialSnapshot.CandidateTree, "--", "deleted.txt")),
		candidateDeletedBytes: gitSnapshot(t, repo, "show", successorState.InitialSnapshot.CandidateTree+":deleted.txt"),
	}
}

func assertHistoricalScopeFixtureUnchanged(t *testing.T, fixture historicalInvalidCorrectionScopeFixture) {
	t.Helper()
	for path, want := range map[string][]byte{
		fixture.predecessorStore.StatePath(): fixture.predecessorBytes,
		fixture.successorStore.StatePath():   fixture.successorBytes,
		fixture.successorStore.ReceiptPath(): fixture.receiptBytes,
	} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s changed on refused repair: %v", path, err)
		}
	}
	quarantine := filepath.Join(filepath.Dir(filepath.Dir(fixture.successorStore.Dir)), "quarantine")
	if entries, err := os.ReadDir(quarantine); err != nil && !os.IsNotExist(err) || err == nil && len(entries) != 0 {
		t.Fatalf("refused repair created quarantine residue at %s: %v", quarantine, err)
	}
}

func TestHistoricalInvalidCorrectionRequiredScopeRecoveryUsesAuthorizedDisposition(t *testing.T) {
	ctx := context.Background()
	fixture := historicalInvalidCorrectionScopeRecoveryFixture(t, "single")
	report, records, err := loadCompactRecoveryRecords(ctx, fixture.repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || len(report.Edges) != 1 || report.Edges[0].Valid ||
		!strings.Contains(report.Edges[0].Problems[0], "correction-required scope recovery requires repository-derived path expansion or pure genesis-scope contraction") {
		t.Fatalf("historical edge did not reproduce #1589: %#v", report)
	}
	status, err := AssessTargetStatus(ctx, fixture.repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.successor.State.LineageID})
	if err != nil || status.Applicability != TargetApplicabilityCorrupted || status.Action != TargetStatusActionRepairAuthority {
		t.Fatalf("pre-repair status = %#v, %v", status, err)
	}
	classification := classifyCompactRecoveryEdgeAnomalies(
		records[fixture.predecessor.State.LineageID], records[fixture.successor.State.LineageID])
	if classification.DispositionClass != historicalInvalidCorrectionScopeClass {
		t.Fatalf("DispositionClass = %q, want %q (validation error: %v)", classification.DispositionClass, historicalInvalidCorrectionScopeClass, classification.ValidationError)
	}
	inexact := records[fixture.successor.State.LineageID]
	inexactRecovery := *inexact.State.Recovery
	inexactRecovery.MaintainerAuthorization = "historical free-form authorization"
	inexact.State.Recovery = &inexactRecovery
	if got := classifyCompactRecoveryEdgeAnomalies(records[fixture.predecessor.State.LineageID], inexact); got.DispositionClass != "" {
		t.Fatalf("inexact historical authorization broadened into disposition class %q", got.DispositionClass)
	}
	plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, fixture.repo, "maintainer@example.com", "quarantine invalid historical correction scope")
	if err != nil {
		t.Fatalf("current review repair route did not derive an authorized plan: %v", err)
	}
	if plan.AnomalyClass != historicalInvalidCorrectionScopeClass || !reflect.DeepEqual(plan.Closure, []string{fixture.successor.State.LineageID}) {
		t.Fatalf("derived plan = %#v", plan)
	}

	wrongSelector := AuthorityDispositionSelector{
		PredecessorLineageID: fixture.predecessor.State.LineageID, PredecessorExpectedRevision: fixture.predecessor.Revision,
		SuccessorLineageID: fixture.successor.State.LineageID, SuccessorExpectedRevision: hash("wrong-revision"),
	}
	if _, err := deriveAuthorityDispositionPlanAtRepo(ctx, fixture.repo, plan.Actor, plan.Reason, wrongSelector); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("wrong exact selector error = %v, want ErrConcurrentUpdate", err)
	}
	assertHistoricalScopeFixtureUnchanged(t, fixture)
	if _, err := RepairAuthorityDisposition(ctx, fixture.repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, "forged authorization"); err == nil {
		t.Fatal("forged disposition authorization was accepted")
	}
	assertHistoricalScopeFixtureUnchanged(t, fixture)

	authorization := authorityDispositionAuthorizationBinding(plan)
	repaired, err := RepairAuthorityDisposition(ctx, fixture.repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.AuthorityDisposition == nil || repaired.AuthorityDisposition.AnomalyClass != historicalInvalidCorrectionScopeClass {
		t.Fatalf("repair audit proof = %#v", repaired.AuthorityDisposition)
	}
	predecessorAfter, err := os.ReadFile(fixture.predecessorStore.StatePath())
	if err != nil || !bytes.Equal(predecessorAfter, fixture.predecessorBytes) {
		t.Fatalf("predecessor bytes changed: %v", err)
	}
	for name, want := range map[string][]byte{compactStateFileName: fixture.successorBytes, compactReceiptFileName: fixture.receiptBytes} {
		got, err := os.ReadFile(filepath.Join(repaired.QuarantinePath, "residue", name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("quarantined %s did not preserve exact bytes: %v", name, err)
		}
	}
	candidateTree := fixture.successor.State.InitialSnapshot.CandidateTree
	if got := strings.TrimSpace(gitSnapshot(t, fixture.repo, "ls-tree", candidateTree, "--", "deleted.txt")); got != fixture.candidateTreeEntry || !strings.HasPrefix(got, "100644 blob ") {
		t.Fatalf("candidate path/mode changed: %q, want %q", got, fixture.candidateTreeEntry)
	}
	if got := gitSnapshot(t, fixture.repo, "show", candidateTree+":deleted.txt"); got != fixture.candidateDeletedBytes {
		t.Fatalf("candidate bytes changed: %q, want %q", got, fixture.candidateDeletedBytes)
	}
	after, err := InspectCompactRecoveryEdges(ctx, fixture.repo)
	if err != nil || !after.Valid || after.Totals.InvalidEdges != 0 {
		t.Fatalf("post-repair graph is not healthy: %#v, %v", after, err)
	}
	postStatus, err := AssessTargetStatus(ctx, fixture.repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.successor.State.LineageID})
	if err != nil || postStatus.Applicability != TargetApplicabilityUnrelated || postStatus.Action != TargetStatusActionStart {
		t.Fatalf("post-repair status = %#v, %v", postStatus, err)
	}
	if replayed, err := RepairAuthorityDisposition(ctx, fixture.repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, authorization); err != nil || replayed.QuarantinePath != repaired.QuarantinePath {
		t.Fatalf("idempotent replay = %#v, %v", replayed, err)
	}
}

func TestHistoricalInvalidCorrectionScopeRepairRefusesCandidateEvidenceDrift(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift func(t *testing.T, repo string)
	}{
		{name: "candidate tree bytes", drift: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "deleted.txt", "drifted successor candidate\n")
		}},
		{name: "candidate path set and tree", drift: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
			writeSnapshotFile(t, repo, "tracked.txt", "drifted tracked candidate\n")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := historicalInvalidCorrectionScopeRecoveryFixture(t, strings.ReplaceAll(test.name, " ", "-"))
			plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, fixture.repo, "maintainer@example.com", "quarantine invalid historical correction scope")
			if err != nil {
				t.Fatal(err)
			}
			oldAuthorization := authorityDispositionAuthorizationBinding(plan)
			test.drift(t, fixture.repo)
			driftedSnapshot, err := (SnapshotBuilder{Repo: fixture.repo}).Build(ctx, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			if err != nil {
				t.Fatal(err)
			}
			drifted := fixture.successor.State
			drifted.InitialSnapshot, drifted.CurrentSnapshot = driftedSnapshot, driftedSnapshot
			drifted.EvidenceTargetIdentity = driftedSnapshot.Identity
			provenance := *drifted.Recovery
			provenance.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
				fixture.predecessor.State.LineageID, fixture.predecessor.Revision, driftedSnapshot.Identity, provenance.Actor, provenance.Reason)
			drifted.Recovery = &provenance
			writeCompactFixtureRecord(t, fixture.successorStore, drifted)
			driftedBytes, err := os.ReadFile(fixture.successorStore.StatePath())
			if err != nil {
				t.Fatal(err)
			}

			if _, err := RepairAuthorityDisposition(ctx, fixture.repo, plan.PlanDigest, plan.AuthorityInventoryRevision, plan.Actor, plan.Reason, oldAuthorization); err == nil {
				t.Fatal("candidate evidence drift was accepted")
			}
			got, err := os.ReadFile(fixture.successorStore.StatePath())
			if err != nil || !bytes.Equal(got, driftedBytes) {
				t.Fatalf("candidate evidence drift refusal mutated authority: %v", err)
			}
			receipt, err := os.ReadFile(fixture.successorStore.ReceiptPath())
			if err != nil || !bytes.Equal(receipt, fixture.receiptBytes) {
				t.Fatalf("candidate evidence drift refusal mutated receipt: %v", err)
			}
		})
	}
}

func TestHistoricalInvalidCorrectionScopeSelectorsEnumerateAllEdges(t *testing.T) {
	fixture := historicalInvalidCorrectionScopeRecoveryFixture(t, "multi")
	forgedRecoveryPair(t, fixture.repo, "multi-content-mismatch", "independent forged target\n")
	selectors, err := ListAuthorityDispositionSelectorsAtRepo(context.Background(), fixture.repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 2 {
		t.Fatalf("selectors = %#v, want both independent invalid edges", selectors)
	}
	if _, err := deriveAuthorityDispositionPlanAtRepo(context.Background(), fixture.repo, "maintainer@example.com", "ambiguous without selector"); !errors.Is(err, errAuthorityDispositionPlanNotDerivable) {
		t.Fatalf("unselected multi-edge derivation error = %v", err)
	}
	var selected AuthorityDispositionSelector
	for _, selector := range selectors {
		if selector.SuccessorLineageID == fixture.successor.State.LineageID {
			selected = selector
		}
	}
	plan, err := deriveAuthorityDispositionPlanAtRepo(context.Background(), fixture.repo, "maintainer@example.com", "select exact historical scope edge", selected)
	if err != nil || plan.AnomalyClass != historicalInvalidCorrectionScopeClass || plan.Selector == nil || *plan.Selector != selected {
		t.Fatalf("selected plan = %#v, %v", plan, err)
	}
}
