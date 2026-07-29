package reviewtransaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPlanCompactBatchReconciliationBindsExactGraph(t *testing.T) {
	snapshot, declarations := compactBatchPlanningFixture(t)
	wantSnapshot := compactBatchJSON(t, snapshot)
	wantDeclarations := compactBatchJSON(t, declarations)

	plan, err := PlanCompactBatchReconciliation(snapshot, declarations)
	if err != nil {
		t.Fatalf("plan compact batch reconciliation: %v", err)
	}
	if got := compactBatchJSON(t, snapshot); got != wantSnapshot {
		t.Fatal("planning mutated the snapshot input")
	}
	if got := compactBatchJSON(t, declarations); got != wantDeclarations {
		t.Fatal("planning mutated the declaration input")
	}
	if got, want := compactBatchEntryLineages(plan.InvalidRoots), []string{"chain-invalid-a", "independent-successor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid roots = %#v, want %#v", got, want)
	}
	if got, want := compactBatchEntryLineages(plan.QuarantineEntries), []string{"chain-invalid-a", "chain-invalid-b", "chain-valid-descendant", "independent-successor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("quarantine entries = %#v, want %#v", got, want)
	}
	if len(plan.ValidDescendantSuffixes) != 1 || plan.ValidDescendantSuffixes[0].InvalidRoot.LineageID != "chain-invalid-a" {
		t.Fatalf("valid descendant suffixes = %#v", plan.ValidDescendantSuffixes)
	}
	if got, want := compactBatchEntryLineages(plan.ValidDescendantSuffixes[0].Entries), []string{"chain-valid-descendant"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("valid descendant suffix = %#v, want %#v", got, want)
	}
	if got, want := compactBatchEntryLineages(plan.RetainedEntries), []string{"chain-predecessor", "independent-predecessor", "retained-predecessor", "retained-successor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained entries = %#v, want %#v", got, want)
	}
	if len(plan.InvalidEdges) != 3 || len(plan.RetainedEdges) != 1 || plan.RetainedEdges[0].SuccessorLineageID != "retained-successor" {
		t.Fatalf("planned graph = invalid %#v, retained %#v", plan.InvalidEdges, plan.RetainedEdges)
	}
	for _, entry := range append(append([]CompactBatchReconcileEntryIdentity{}, plan.QuarantineEntries...), plan.RetainedEntries...) {
		if entry.LineageID == "" || entry.Revision == "" || entry.InitialTargetIdentity == "" || entry.CurrentTargetIdentity == "" {
			t.Fatalf("entry identity is not fully bound: %#v", entry)
		}
	}

	wantPlan := compactBatchJSON(t, plan)
	slices.Reverse(snapshot.Records)
	slices.Reverse(declarations)
	reordered, err := PlanCompactBatchReconciliation(snapshot, declarations)
	if err != nil {
		t.Fatalf("plan reordered compact batch reconciliation: %v", err)
	}
	if got := compactBatchJSON(t, reordered); got != wantPlan {
		t.Fatalf("reordered plan is not deterministic:\n%s\n%s", wantPlan, got)
	}

	snapshot.Records[0].State.LineageID = "mutated-input"
	declarations[0].AnomalyClasses[0] = "mutated-input"
	if got := compactBatchJSON(t, plan); got != wantPlan {
		t.Fatal("plan aliases mutable snapshot or declaration input")
	}
}

func TestPlanCompactBatchReconciliationRejectsInexactDeclarationsAndEvidence(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CompactBatchReconcileSnapshot, *[]CompactRecoveryEdgeInspection)
		wantErr string
	}{
		{
			name: "incomplete declaration",
			mutate: func(_ *CompactBatchReconcileSnapshot, declarations *[]CompactRecoveryEdgeInspection) {
				*declarations = (*declarations)[1:]
			},
			wantErr: "declaration set is incomplete",
		},
		{
			name: "extra declaration for valid edge",
			mutate: func(snapshot *CompactBatchReconcileSnapshot, declarations *[]CompactRecoveryEdgeInspection) {
				for _, edge := range snapshot.Inspection.Edges {
					if edge.Valid {
						*declarations = append(*declarations, edge)
						return
					}
				}
			},
			wantErr: "is not currently invalid",
		},
		{
			name: "duplicate declaration",
			mutate: func(_ *CompactBatchReconcileSnapshot, declarations *[]CompactRecoveryEdgeInspection) {
				*declarations = append(*declarations, (*declarations)[0])
			},
			wantErr: "duplicate declaration",
		},
		{
			name: "unknown declared anomaly class",
			mutate: func(_ *CompactBatchReconcileSnapshot, declarations *[]CompactRecoveryEdgeInspection) {
				(*declarations)[0].AnomalyClasses = []string{"future_anomaly"}
			},
			wantErr: "unsupported anomaly class",
		},
		{
			name: "stale inspection revision",
			mutate: func(snapshot *CompactBatchReconcileSnapshot, _ *[]CompactRecoveryEdgeInspection) {
				snapshot.Inspection.Edges[0].SuccessorRevision = hash("stale")
			},
			wantErr: "inspection does not match the exact records",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, declarations := compactBatchPlanningFixture(t)
			tt.mutate(&snapshot, &declarations)
			if _, err := PlanCompactBatchReconciliation(snapshot, declarations); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("planning error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPlanCompactBatchReconciliationRejectsUnsupportedGraphShapes(t *testing.T) {
	tests := []struct {
		name    string
		fixture func(*testing.T) CompactBatchReconcileSnapshot
		wantErr string
	}{
		{
			name: "malformed entry",
			fixture: func(t *testing.T) CompactBatchReconcileSnapshot {
				repo := initSnapshotRepo(t)
				inspectRecoveryPair(t, repo, "valid", false, "")
				base, _, err := reviewAuthorityRoot(context.Background(), repo)
				inspectNoError(t, err)
				broken := filepath.Join(base, "v2", "broken-entry")
				inspectNoError(t, os.MkdirAll(broken, 0o755))
				inspectNoError(t, os.WriteFile(filepath.Join(broken, compactStateFileName), []byte("not json\n"), 0o644))
				return loadCompactBatchPlanningSnapshot(t, repo)
			},
			wantErr: "inspection is incomplete",
		},
		{
			name: "dangling predecessor",
			fixture: func(t *testing.T) CompactBatchReconcileSnapshot {
				repo := initSnapshotRepo(t)
				_, predecessorStore, _, _ := inspectRecoveryPair(t, repo, "dangling", false, "")
				inspectNoError(t, os.RemoveAll(predecessorStore.Dir))
				return loadCompactBatchPlanningSnapshot(t, repo)
			},
			wantErr: "missing predecessor",
		},
		{
			name: "recovery fork",
			fixture: func(t *testing.T) CompactBatchReconcileSnapshot {
				repo := initSnapshotRepo(t)
				predecessor, _, _, _ := inspectRecoveryPair(t, repo, "fork", false, "")
				writeSnapshotFile(t, repo, "tracked.txt", "fork sibling\n")
				inspectRecoverySuccessor(t, repo, predecessor, "fork-sibling", "")
				return loadCompactBatchPlanningSnapshot(t, repo)
			},
			wantErr: "recovery fork",
		},
		{
			name: "recovery cycle",
			fixture: func(t *testing.T) CompactBatchReconcileSnapshot {
				repo := initSnapshotRepo(t)
				inspectRecoveryCycle(t, repo)
				return loadCompactBatchPlanningSnapshot(t, repo)
			},
			wantErr: "recovery cycle",
		},
		{
			name: "unsupported edge corruption",
			fixture: func(t *testing.T) CompactBatchReconcileSnapshot {
				repo := initSnapshotRepo(t)
				_, _, successor, store := inspectRecoveryPair(t, repo, "corrupt", false, "")
				successor.State.Generation += 2
				writeCompactFixtureRecord(t, store, successor.State)
				return loadCompactBatchPlanningSnapshot(t, repo)
			},
			wantErr: "outside supported reconciliation classes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := tt.fixture(t)
			declarations := compactBatchInvalidEdges(snapshot.Inspection)
			if _, err := PlanCompactBatchReconciliation(snapshot, declarations); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("planning error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPlanCompactBatchReconciliationRejectsNonContiguousDescendantClosure(t *testing.T) {
	repo := initSnapshotRepo(t)
	rootState := correctedCompactTestState(t, repo, "noncontiguous-root")
	rootState.State = StateEscalated
	rootStore, err := CompactAuthoritativeStore(context.Background(), repo, rootState.LineageID)
	inspectNoError(t, err)
	root := writeCompactFixtureRecord(t, rootStore, rootState)

	first, _ := compactBatchRecoverySuccessor(t, repo, root, "noncontiguous-invalid-a", []string{"noncontiguous-a.txt"}, preContractFixtureAuthorization, true)
	middle, _ := compactBatchRecoverySuccessor(t, repo, first, "noncontiguous-valid", []string{"noncontiguous-a.txt", "noncontiguous-b.txt"}, "", true)
	compactBatchRecoverySuccessor(t, repo, middle, "noncontiguous-invalid-b", []string{"noncontiguous-a.txt", "noncontiguous-b.txt", "noncontiguous-c.txt"}, preContractFixtureAuthorization, false)

	snapshot := loadCompactBatchPlanningSnapshot(t, repo)
	if _, err := PlanCompactBatchReconciliation(snapshot, compactBatchInvalidEdges(snapshot.Inspection)); err == nil || !strings.Contains(err.Error(), "non-contiguous invalid edge closure") {
		t.Fatalf("non-contiguous planning error = %v", err)
	}
}

func TestValidateCompactBatchRetainedGraphRejectsInvalidResult(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, predecessorStore, _, _ := inspectRecoveryPair(t, repo, "retained-dangling", false, "")
	inspectNoError(t, os.RemoveAll(predecessorStore.Dir))
	snapshot := loadCompactBatchPlanningSnapshot(t, repo)
	records := make(map[string]CompactRecord, len(snapshot.Records))
	for _, record := range snapshot.Records {
		records[record.State.LineageID] = record
	}
	if err := validateCompactBatchRetainedGraph(records); err == nil || !strings.Contains(err.Error(), "retained graph remains invalid") {
		t.Fatalf("retained graph validation error = %v", err)
	}
}

func compactBatchPlanningFixture(t *testing.T) (CompactBatchReconcileSnapshot, []CompactRecoveryEdgeInspection) {
	t.Helper()
	repo := initSnapshotRepo(t)
	return compactBatchPlanningFixtureInRepo(t, repo)
}

func compactBatchPlanningFixtureInRepo(t *testing.T, repo string) (CompactBatchReconcileSnapshot, []CompactRecoveryEdgeInspection) {
	t.Helper()
	inspectRecoveryPair(t, repo, "retained", false, "")
	inspectRecoveryPair(t, repo, "independent", false, preContractFixtureAuthorization)

	rootState := correctedCompactTestState(t, repo, "chain-predecessor")
	rootState.State = StateEscalated
	rootStore, err := CompactAuthoritativeStore(context.Background(), repo, rootState.LineageID)
	inspectNoError(t, err)
	root := writeCompactFixtureRecord(t, rootStore, rootState)
	invalidA, _ := compactBatchRecoverySuccessor(t, repo, root, "chain-invalid-a", []string{"chain-a.txt"}, preContractFixtureAuthorization, true)
	invalidB, _ := compactBatchRecoverySuccessor(t, repo, invalidA, "chain-invalid-b", []string{"chain-a.txt", "chain-b.txt"}, preContractFixtureAuthorization, true)
	compactBatchRecoverySuccessor(t, repo, invalidB, "chain-valid-descendant", []string{"chain-a.txt", "chain-b.txt", "chain-c.txt"}, "", false)

	snapshot := loadCompactBatchPlanningSnapshot(t, repo)
	declarations := compactBatchInvalidEdges(snapshot.Inspection)
	slices.Reverse(declarations)
	return snapshot, declarations
}

func loadCompactBatchPlanningSnapshot(t *testing.T, repo string) CompactBatchReconcileSnapshot {
	t.Helper()
	report, err := InspectCompactRecoveryEdges(context.Background(), repo)
	inspectNoError(t, err)
	stores, err := DiscoverCompactStores(context.Background(), repo)
	inspectNoError(t, err)
	records := make([]CompactRecord, 0, len(stores))
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr == nil {
			records = append(records, record)
		}
	}
	return CompactBatchReconcileSnapshot{Inspection: report, Records: records}
}

func compactBatchInvalidEdges(report CompactRecoveryInspectionReport) []CompactRecoveryEdgeInspection {
	edges := make([]CompactRecoveryEdgeInspection, 0, report.Totals.InvalidEdges)
	for _, edge := range report.Edges {
		if !edge.Valid {
			edges = append(edges, edge)
		}
	}
	return edges
}

func compactBatchRecoverySuccessor(t *testing.T, repo string, predecessor CompactRecord, lineage string, intended []string, authorization string, escalated bool) (CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestStateWithIntended(t, repo, lineage, intended)
	if escalated {
		state.State = StateEscalated
	}
	state.Generation = predecessor.State.Generation + 1
	state.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID:    predecessor.State.LineageID,
		PredecessorRevision:     predecessor.Revision,
		Disposition:             RecoveryEscalated,
		Reason:                  "batch recovery fixture",
		Actor:                   "maintainer@example.com",
		RecoveredAt:             time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: authorization,
	}
	if authorization == "" {
		state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
			predecessor.State.LineageID, predecessor.Revision, state.InitialSnapshot.Identity,
			state.Recovery.Actor, state.Recovery.Reason)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	inspectNoError(t, err)
	return writeCompactFixtureRecord(t, store, state), store
}

func compactBatchEntryLineages(entries []CompactBatchReconcileEntryIdentity) []string {
	lineages := make([]string, len(entries))
	for index, entry := range entries {
		lineages[index] = entry.LineageID
	}
	return lineages
}

func compactBatchJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
