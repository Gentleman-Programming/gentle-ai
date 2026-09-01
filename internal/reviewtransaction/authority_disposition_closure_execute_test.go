package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// forgedRecoveryDescendant extends an existing recovery lineage (a forged
// seed or another descendant) with one more legitimate, validly-authorized
// recovery hop, producing a real multi-node closure through the exact same
// derivation path production code uses (deriveAuthorityDispositionPlanAtRepo
// -> authorityDispositionClosure's report-only BFS/DFS). It mirrors
// compact_reconcile_test.go's "successor has its own successor" fixture
// shape, plus forgedRecoveryPair's own "changed target" step: parent
// identifies the node this descendant recovers from; its Generation must be
// set so classifyCompactRecoveryEdgeAnomalies sees a legitimate, VALID edge
// (a descendant.Generation <= parent.State.Generation classifies as an
// anomaly), and the tracked file content must actually change before the
// descendant's snapshot is captured (an unchanged target classifies as
// unchanged_target — only relevant when parent.State is Escalated, but
// applied unconditionally here so every call site gets a genuinely valid
// edge on request).
func forgedRecoveryDescendant(t *testing.T, repo string, parent CompactRecord, suffix string) CompactRecord {
	t.Helper()
	parentLineage, parentRevision := parent.State.LineageID, parent.Revision
	writeSnapshotFile(t, repo, "tracked.txt", "descendant target "+suffix+"\n")
	descendant := newCompactTestState(t, repo, "descendant-"+suffix)
	descendant.Generation = parent.State.Generation + 1
	descendant.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: parentLineage, PredecessorRevision: parentRevision,
		Disposition: RecoveryEscalated, Reason: "legitimate descendant of " + parentLineage, Actor: "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(parentLineage, parentRevision, descendant.InitialSnapshot.Identity, "maintainer@example.com", "legitimate descendant of "+parentLineage),
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, descendant.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return writeCompactFixtureRecord(t, store, descendant)
}

// twoNodeClosureFixture builds a real, closed-classified 2-node closure
// through forgedRecoveryPair (predecessor -(forged, invalid)-> seed) plus one
// legitimate descendant hop (seed -(valid)-> child), so
// authorityDispositionClosure's BFS/DFS walks the valid edge onto child too
// (S1's TestAuthorityDispositionClosureMultiChainAssumption already proved
// this walk is correct). Returns the derived, authorized plan and the
// child's own CompactRecord for direct store-path assertions.
func twoNodeClosureFixture(t *testing.T, repo, suffix string) (AuthorityDispositionPlan, CompactRecord) {
	t.Helper()
	_, seed, _ := forgedRecoveryPair(t, repo, suffix, "two-node closure target "+suffix+"\n")
	child := forgedRecoveryDescendant(t, repo, seed, suffix+"-child")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization closure")
	if len(plan.Closure) != 2 {
		t.Fatalf("fixture did not derive a 2-node closure: %#v", plan.Closure)
	}
	if plan.Closure[len(plan.Closure)-1] != seed.State.LineageID {
		t.Fatalf("fixture closure does not end with the seed: %v, seed=%q", plan.Closure, seed.State.LineageID)
	}
	if plan.Closure[0] != child.State.LineageID {
		t.Fatalf("fixture closure does not start with the descendant child: %v, child=%q", plan.Closure, child.State.LineageID)
	}
	return plan, child
}

// threeNodeClosureFixture builds a real, closed-classified 3-node closure:
// predecessor -(forged, invalid)-> seed -(valid)-> child -(valid)-> grandchild.
// Returns the derived, authorized plan and the child/grandchild records for
// direct store-path assertions. Reused by Slice S3's resume and
// crash-position-matrix tests, which need >=3 nodes to distinguish "resumed
// mid-closure" from "resumed at the last non-seed position".
func threeNodeClosureFixture(t *testing.T, repo, suffix string) (plan AuthorityDispositionPlan, child, grandchild CompactRecord) {
	t.Helper()
	_, seed, _ := forgedRecoveryPair(t, repo, suffix, "three-node closure target "+suffix+"\n")
	child = forgedRecoveryDescendant(t, repo, seed, suffix+"-child")
	grandchild = forgedRecoveryDescendant(t, repo, child, suffix+"-grandchild")
	plan = authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization closure")
	want := []string{grandchild.State.LineageID, child.State.LineageID, seed.State.LineageID}
	if !reflectStringSliceEqual(plan.Closure, want) {
		t.Fatalf("fixture did not derive the expected 3-node descendant-first closure: got %v, want %v", plan.Closure, want)
	}
	return plan, child, grandchild
}

// TestAuthorityDispositionExecuteOrderedTransactionFollowsClosureOrder is the
// order-inversion mutation proof tasks.md 2.2 requires: a 3-node closure
// (grandchild -> child -> seed) is executed, and the exact sequence of
// compactReclaimPhasePrepared hook invocations — one per node, in the order
// each node's two-phase quarantine begins — is asserted to equal
// plan.Closure exactly. A wrong emit order (e.g. seed processed before its
// descendants) fails this test immediately; it does not merely produce a
// coincidentally-valid end state.
func TestAuthorityDispositionExecuteOrderedTransactionFollowsClosureOrder(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, seed, _ := forgedRecoveryPair(t, repo, "order", "order target\n")
	child := forgedRecoveryDescendant(t, repo, seed, "order-child")
	grandchild := forgedRecoveryDescendant(t, repo, child, "order-grandchild")
	plan := authorizedPlanFixture(t, repo, "maintainer@example.com", "quarantine forged recovery authorization order")
	wantOrder := []string{grandchild.State.LineageID, child.State.LineageID, seed.State.LineageID}
	if !reflectStringSliceEqual(plan.Closure, wantOrder) {
		t.Fatalf("fixture closure = %v, want descendant-first seed-last %v", plan.Closure, wantOrder)
	}

	var observedOrder []string
	originalHook := compactReclaimPhaseHook
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhasePrepared {
			observedOrder = append(observedOrder, record.LineageID)
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	if !reflectStringSliceEqual(observedOrder, wantOrder) {
		t.Fatalf("observed per-node prepare order = %v, want closure order (descendant-first, seed-last) %v — order inversion", observedOrder, wantOrder)
	}
}

// TestAuthorityDispositionExecuteCASAllNRefusesNamingDriftedMember is the
// CAS-drift mutation proof tasks.md 2.3/2.4 require: every closure member's
// ExpectedRevision is checked before the first move. The submitted plan's
// ExpectedRevisions[child] is corrupted to a stale value after authorization
// was computed on the correct plan — the authorization binding
// (schema/repository/class/plan_digest/inventory_revision/actor/reason) does
// not cover ExpectedRevisions, and plan.PlanDigest is a cached field the
// corruption never touches, so this exercises CAS-all-N specifically without
// tripping the earlier, unrelated "plan digest drifted under lock
// re-derivation" refusal (which fires on schema-level drift, not a single
// member's stale expectation). The refusal must name exactly the corrupted
// member — not a generic cardinality or "the seed" message.
func TestAuthorityDispositionExecuteCASAllNRefusesNamingDriftedMember(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child := twoNodeClosureFixture(t, repo, "cas")

	childStore, err := CompactAuthoritativeStore(context.Background(), repo, child.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	beforeChildBytes, err := os.ReadFile(childStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	plan.ExpectedRevisions[child.State.LineageID] = "sha256:stale-drifted-revision-for-the-descendant"

	_, err = executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("CAS-all-N execution error = %v, want ErrConcurrentUpdate", err)
	}
	if !strings.Contains(err.Error(), child.State.LineageID) {
		t.Fatalf("CAS-all-N refusal does not name the drifted non-seed member %q: %v", child.State.LineageID, err)
	}
	afterChildBytes, err := os.ReadFile(childStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeChildBytes) != string(afterChildBytes) {
		t.Fatal("CAS-refused execution mutated the descendant's bytes before the first move")
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(filepath.Join(base, "quarantine")); !os.IsNotExist(statErr) {
		t.Fatalf("CAS-refused execution created a quarantine directory before validating every member: %v", statErr)
	}
}

// TestAuthorityDispositionExecuteCrashAfterNMinus1LeavesOnlySeedUnmoved
// satisfies tasks.md 2.1: a crash injected right after the descendant (the
// first of a 2-node closure) commits, before the seed's own quarantine
// begins, leaves a retained graph that still classifies cleanly (Complete,
// no dangling reference to the now-gone descendant), with only the seed
// unmoved. The seed's own still-invalid forged predecessor edge is
// deliberately NOT required to be Valid here — that anomaly is exactly what
// the closure exists to fix, and fixing it is the LAST step (the seed is
// seed-last by design); Valid only becomes true once the seed itself also
// commits.
func TestAuthorityDispositionExecuteCrashAfterNMinus1LeavesOnlySeedUnmoved(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child := twoNodeClosureFixture(t, repo, "crash")
	seed := plan.Closure[len(plan.Closure)-1]

	childStore, err := CompactAuthoritativeStore(context.Background(), repo, child.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	seedStore, err := CompactAuthoritativeStore(context.Background(), repo, seed)
	if err != nil {
		t.Fatal(err)
	}

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected crash after descendant commits, before seed starts")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == child.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	_, err = executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, injected) {
		t.Fatalf("crash-after-N-1 execution error = %v, want the injected interruption", err)
	}

	if _, statErr := os.Stat(childStore.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("descendant directory survived the crash-after-N-1 point: %v", statErr)
	}
	if _, statErr := os.Stat(seedStore.Dir); statErr != nil {
		t.Fatalf("seed directory did not survive the crash-after-N-1 point (should be unmoved): %v", statErr)
	}

	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatalf("post-crash inspection: %v", err)
	}
	// "Classifies cleanly" means Complete (no entry diagnostics, no
	// corruption, no ambiguous/dangling reference to the now-gone
	// descendant) — NOT Valid. The seed itself has not yet been disposed at
	// this exact interruption point, so its original, still-unfixed forged
	// predecessor edge is legitimately still invalid; that is the
	// pre-existing anomaly the whole closure exists to fix, not evidence of
	// a dangling reference introduced by this crash.
	if !report.Complete {
		t.Fatalf("post-crash retained graph did not classify cleanly (entry diagnostics present): %#v", report.Totals)
	}
	if report.Totals.EntryDiagnostics != 0 {
		t.Fatalf("post-crash retained graph has entry diagnostics — a dangling/corrupted reference: %#v", report.Totals)
	}
	for _, edge := range report.Edges {
		if edge.PredecessorLineageID == child.State.LineageID || edge.SuccessorLineageID == child.State.LineageID {
			t.Fatalf("retained graph still references the already-quarantined descendant %q", child.State.LineageID)
		}
	}
}

// TestAuthorityDispositionExecutePartialClosureReportsNoSuccess satisfies
// tasks.md 2.5/2.6: an interruption before the last (seed) node commits never
// reports success, and no closure-manifest.json is written anywhere — the
// manifest is written once, only after every closure member commits
// (design.md's Data Flow: the manifest step follows the ordered loop, before
// readback). The descendant's own reclaim-record.json still shows committed
// — nothing already committed is undone.
func TestAuthorityDispositionExecutePartialClosureReportsNoSuccess(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, child := twoNodeClosureFixture(t, repo, "partial")

	originalHook := compactReclaimPhaseHook
	injected := errors.New("injected partial-closure interruption")
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if current == compactReclaimPhaseCommitted && record.LineageID == child.State.LineageID {
			return injected
		}
		return originalHook(ctx, current, record)
	}
	t.Cleanup(func() { compactReclaimPhaseHook = originalHook })

	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if !errors.Is(err, injected) {
		t.Fatalf("partial-closure execution error = %v, want the injected interruption", err)
	}
	if record.Status == CompactReclaimCommitted && record.LineageID == plan.Closure[len(plan.Closure)-1] {
		t.Fatal("a partial closure reported the seed as committed — success was reported before the last node committed")
	}

	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil {
		t.Fatalf("read quarantine root: %v", err)
	}
	sawChildCommitted := false
	for _, entry := range entries {
		manifestPath := filepath.Join(quarantineRoot, entry.Name(), "closure-manifest.json")
		if _, statErr := os.Stat(manifestPath); statErr == nil {
			t.Fatalf("a closure-manifest.json exists after an interrupted (not fully committed) closure: %s", manifestPath)
		}
		if !strings.HasPrefix(entry.Name(), child.State.LineageID+"-") {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(payload), `"status": "committed"`) {
			sawChildCommitted = true
		}
	}
	if !sawChildCommitted {
		t.Fatal("the descendant's own record does not show committed — a prior successful step was undone, not just left incomplete")
	}
}

// TestAuthorityDispositionClosureManifestWrittenInSeedQuarantineDir satisfies
// tasks.md 2.7/2.8: after a full, successful N-node closure disposition, a
// forensic-only closure-manifest.json exists inside the SEED node's own
// quarantine directory (design decision D5), naming the plan_digest,
// anomaly_class, and the full ordered closure.
func TestAuthorityDispositionClosureManifestWrittenInSeedQuarantineDir(t *testing.T) {
	repo := initSnapshotRepo(t)
	plan, _ := twoNodeClosureFixture(t, repo, "manifest")

	record, err := executeAuthorityDisposition(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}
	if record.LineageID != plan.Closure[len(plan.Closure)-1] {
		t.Fatalf("returned record is not the seed's: LineageID=%q, want %q", record.LineageID, plan.Closure[len(plan.Closure)-1])
	}
	manifestPath := filepath.Join(record.QuarantinePath, "closure-manifest.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read closure-manifest.json in the seed's quarantine dir: %v", err)
	}
	var manifest AuthorityDispositionClosureManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != AuthorityDispositionClosureManifestSchema {
		t.Fatalf("manifest schema = %q, want %q", manifest.Schema, AuthorityDispositionClosureManifestSchema)
	}
	if manifest.PlanDigest != plan.PlanDigest {
		t.Fatalf("manifest plan_digest = %q, want %q", manifest.PlanDigest, plan.PlanDigest)
	}
	if manifest.AnomalyClass != plan.AnomalyClass {
		t.Fatalf("manifest anomaly_class = %q, want %q", manifest.AnomalyClass, plan.AnomalyClass)
	}
	if !reflectStringSliceEqual(manifest.OrderedClosure, plan.Closure) {
		t.Fatalf("manifest ordered_closure = %v, want %v", manifest.OrderedClosure, plan.Closure)
	}
	if !reflectStringSliceEqual(manifest.Disposed, plan.Closure) {
		t.Fatalf("manifest disposed = %v, want the full closure %v on a successful run", manifest.Disposed, plan.Closure)
	}
}

// TestReadBackAuthorityDispositionRefusesOnAnyClosureMemberReference proves
// the over-collection guard readback widens to (design decision D6):
// rdd-closure-disposition-execution's readback MUST refuse if ANY closure
// member is still referenced by a retained edge, not only the seed
// (record.LineageID). A hypothetical partial-removal defect leaving a
// non-seed closure member ("leftover-descendant") still live, with its own
// legitimate downstream edge, is caught and named — this exercises the
// widened check directly and deterministically, independent of whether
// production quarantine itself could ever actually leave this residue.
func TestReadBackAuthorityDispositionRefusesOnAnyClosureMemberReference(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, root, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	leftover := newCompactTestState(t, repo, "leftover-descendant")
	leftover.State = StateEscalated
	leftoverStore, err := CompactAuthoritativeStore(context.Background(), repo, leftover.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	leftoverRecord := writeCompactFixtureRecord(t, leftoverStore, leftover)
	downstream := forgedRecoveryDescendant(t, repo, leftoverRecord, "leftover-downstream")
	_ = downstream

	record := CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimCommitted, LineageID: "seed-not-actually-present",
		AuthorityDisposition: &AuthorityDispositionProof{
			Schema: AuthorityDispositionProofSchema, Closure: []string{"leftover-descendant", "seed-not-actually-present"},
		},
	}
	_, err = readBackAuthorityDisposition(context.Background(), root, record)
	if err == nil {
		t.Fatal("readback admitted a retained graph still referencing a non-seed closure member")
	}
	if !strings.Contains(err.Error(), "leftover-descendant") {
		t.Fatalf("readback refusal does not name the still-referenced non-seed closure member: %v", err)
	}
}

// TestAuthorityDispositionExecuteUnrelatedLineageByteIdenticalAcrossClosure
// satisfies tasks.md 2.9, mirroring bench/axis_damaged_store.go's
// requireDispositionWitnessBytesUnchanged pattern at the unit level: a
// completely unrelated third lineage (no predecessor/successor relationship
// to any closure member) keeps byte-identical store bytes across a 2-node
// closure disposition.
func TestAuthorityDispositionExecuteUnrelatedLineageByteIdenticalAcrossClosure(t *testing.T) {
	repo := initSnapshotRepo(t)

	// The witness must exist BEFORE the plan is derived: authority_inventory_
	// revision covers every loaded record, so adding one AFTER derivation
	// would (correctly) trip the unrelated "plan digest drifted under lock
	// re-derivation" staleness guard — a different concern from this test's
	// byte-identity assertion.
	witness := newCompactTestState(t, repo, "unrelated-witness-lineage")
	witnessStore, err := CompactAuthoritativeStore(context.Background(), repo, witness.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, witnessStore, witness)
	before, err := os.ReadFile(witnessStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	plan, _ := twoNodeClosureFixture(t, repo, "witness")

	if _, err := executeAuthorityDisposition(context.Background(), repo, plan); err != nil {
		t.Fatalf("executeAuthorityDisposition: %v", err)
	}

	after, err := os.ReadFile(witnessStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("an unrelated third lineage's store bytes changed across a closure disposition it has no relationship to")
	}
}
