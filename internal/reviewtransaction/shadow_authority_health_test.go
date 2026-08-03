package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestShadowAuthorityHealthThreeValueClassification satisfies tasks.md 4.1
// and rdd-authority-graph-classification's "Three-Value Health
// Classification" requirement: a consistent graph classifies healthy, and a
// classified leaf anomaly classifies repairable.
func TestShadowAuthorityHealthThreeValueClassification(t *testing.T) {
	tests := []struct {
		name   string
		report CompactRecoveryInspectionReport
		want   shadowAuthorityHealth
	}{
		{
			name:   "no lineages at all is healthy",
			report: CompactRecoveryInspectionReport{Complete: true, Valid: true},
			want:   shadowAuthorityHealthy,
		},
		{
			name: "no detected anomaly is healthy",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: true,
				Edges: []CompactRecoveryEdgeInspection{
					{PredecessorLineageID: "root", SuccessorLineageID: "child", Valid: true},
				},
			},
			want: shadowAuthorityHealthy,
		},
		{
			name: "classified leaf anomaly is repairable",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{
						PredecessorLineageID: "root", SuccessorLineageID: "leaf",
						Valid: false, AnomalyClasses: []string{compactRecoveryEdgeUnchangedTarget},
					},
				},
			},
			want: shadowAuthorityRepairable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shadowClassifyAuthorityHealth(tt.report); got != tt.want {
				t.Fatalf("shadowClassifyAuthorityHealth() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestShadowAuthorityHealthFailClosedOnUnknownShape satisfies tasks.md 4.3
// and the "Fail-Closed on Unknown Shape" requirement: an anomaly shape that
// is not a closed, evidence-backed, leaf-only class must never classify
// healthy or repairable — never a generic repairable fallback.
func TestShadowAuthorityHealthFailClosedOnUnknownShape(t *testing.T) {
	tests := []struct {
		name   string
		report CompactRecoveryInspectionReport
	}{
		{
			name: "dangling predecessor has no anomaly class",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{
						PredecessorLineageID: "missing", SuccessorLineageID: "dangling",
						Valid: false, Problems: []string{"missing predecessor"},
					},
				},
			},
		},
		{
			name: "recovery fork has no anomaly class",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{PredecessorLineageID: "root", SuccessorLineageID: "fork-a", Valid: false, Problems: []string{"recovery fork"}},
					{PredecessorLineageID: "root", SuccessorLineageID: "fork-b", Valid: false, Problems: []string{"recovery fork"}},
				},
			},
		},
		{
			name: "recovery cycle has no anomaly class",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{PredecessorLineageID: "cycle-b", SuccessorLineageID: "cycle-a", Valid: false, Problems: []string{"recovery cycle"}},
					{PredecessorLineageID: "cycle-a", SuccessorLineageID: "cycle-b", Valid: false, Problems: []string{"recovery cycle"}},
				},
			},
		},
		{
			name: "incomplete inspection carries an entry diagnostic",
			report: CompactRecoveryInspectionReport{
				Complete: false, Valid: false,
				EntryDiagnostics: []CompactRecoveryEntryDiagnostic{
					{LineageID: "broken-entry", Problem: compactInspectionEntryMalformed},
				},
			},
		},
		{
			name: "classified anomaly is not leaf-only — successor carries its own descendant",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{
						PredecessorLineageID: "root", SuccessorLineageID: "mid",
						Valid: false, AnomalyClasses: []string{compactRecoveryEdgeUnchangedTarget},
					},
					{PredecessorLineageID: "mid", SuccessorLineageID: "descendant", Valid: true},
				},
			},
		},
		{
			name: "one leaf-classified edge alongside one unclassifiable edge blocks the whole graph",
			report: CompactRecoveryInspectionReport{
				Complete: true, Valid: false,
				Edges: []CompactRecoveryEdgeInspection{
					{
						PredecessorLineageID: "root", SuccessorLineageID: "leaf",
						Valid: false, AnomalyClasses: []string{compactRecoveryEdgeMalformedAuthorization},
					},
					{PredecessorLineageID: "missing", SuccessorLineageID: "dangling", Valid: false, Problems: []string{"missing predecessor"}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shadowClassifyAuthorityHealth(tt.report)
			if got != shadowAuthorityBlocked {
				t.Fatalf("shadowClassifyAuthorityHealth() = %q, want %q (never a generic repairable fallback)", got, shadowAuthorityBlocked)
			}
		})
	}
}

// TestShadowAuthorityHealthNoMutationOrExecution satisfies tasks.md 4.2 and
// the "No Mutation or Execution" requirement: classification over a real
// authority graph must not modify any authority record, leave the
// maintenance lock held, or create a disposition/quarantine artifact.
//
// The read-only InspectCompactRecoveryEdges surface this classifier delegates
// to legitimately creates the maintenance LOCK coordination file itself (it
// takes a shared lock to read consistently, exactly as `review
// inspect-authority` already does) — that file's mere existence is not a
// mutation. What "no mutation or execution" actually forbids is: authority
// state bytes changing, the lock still being HELD afterward (proven by a
// following exclusive acquisition succeeding immediately), and a quarantine
// disposition being created.
func TestShadowAuthorityHealthNoMutationOrExecution(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, predecessorStore, _, successorStore := inspectRecoveryPair(t, repo, "leaf", true, "")
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	inspectNoError(t, err)

	quarantineRoot := filepath.Join(base, "quarantine")
	if _, statErr := os.Stat(quarantineRoot); !os.IsNotExist(statErr) {
		t.Fatalf("quarantine directory unexpectedly present before classification: %v", statErr)
	}
	before := inspectReadState(t, predecessorStore.StatePath()) + inspectReadState(t, successorStore.StatePath())

	health, err := shadowAuthorityHealthAtRepo(context.Background(), repo)
	if err != nil {
		t.Fatalf("shadowAuthorityHealthAtRepo: %v", err)
	}
	if health != shadowAuthorityRepairable {
		t.Fatalf("health = %q, want %q", health, shadowAuthorityRepairable)
	}

	after := inspectReadState(t, predecessorStore.StatePath()) + inspectReadState(t, successorStore.StatePath())
	if before != after {
		t.Fatal("classification mutated authority state bytes")
	}
	exclusive, lockErr := acquireMaintenanceLock(context.Background(), compactMaintenanceLockPath(base), maintenanceExclusive)
	if lockErr != nil {
		t.Fatalf("classification left the maintenance lock held: %v", lockErr)
	}
	exclusive.Release()
	if _, statErr := os.Stat(quarantineRoot); !os.IsNotExist(statErr) {
		t.Fatalf("classification created a quarantine disposition: %v", statErr)
	}
}

// TestShadowAuthorityHealthClassificationRegressionAfterDispositionPlanDerivation
// satisfies Wave 2 tasks.md 1.10 and the rdd-authority-graph-classification
// MODIFIED "No Mutation or Execution" scenario "Repairable result feeds a
// separate plan derivation, not the classifier itself": deriving an
// AuthorityDispositionPlan from a classified edge's evidence is a wholly
// separate consumer (authority_disposition_plan.go). Re-running the
// classifier alone, before or after that derivation, still returns exactly
// the same result and produces no plan and no mutation of its own.
func TestShadowAuthorityHealthClassificationRegressionAfterDispositionPlanDerivation(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successorStore := forgedRecoveryPair(t, repo, "regression", "regression target\n")
	before := inspectReadState(t, successorStore.StatePath())

	beforeHealth, err := shadowAuthorityHealthAtRepo(context.Background(), repo)
	inspectNoError(t, err)
	if beforeHealth != shadowAuthorityBlocked {
		t.Fatalf("classification before plan derivation = %q, want %q (classification vocabulary is unchanged by Wave 2)", beforeHealth, shadowAuthorityBlocked)
	}

	// A separate consumer derives a plan from the SAME evidence. The
	// classifier itself is never called by this derivation.
	if _, err := deriveAuthorityDispositionPlanAtRepo(context.Background(), repo, "maintainer@example.com", "quarantine forged recovery authorization"); err != nil {
		t.Fatalf("deriveAuthorityDispositionPlanAtRepo: %v", err)
	}

	afterHealth, err := shadowAuthorityHealthAtRepo(context.Background(), repo)
	inspectNoError(t, err)
	if afterHealth != beforeHealth {
		t.Fatalf("re-running the classifier alone after plan derivation changed the result: %q -> %q", beforeHealth, afterHealth)
	}
	after := inspectReadState(t, successorStore.StatePath())
	if before != after {
		t.Fatal("classifier or plan derivation mutated authority state bytes")
	}
}

// TestShadowAuthorityHealthDeterministicEvidenceBacked satisfies tasks.md 4.4
// and the "Deterministic, Evidence-Backed Classification" requirement: the
// same graph state, inspected twice with no state change between
// inspections, classifies identically both times.
func TestShadowAuthorityHealthDeterministicEvidenceBacked(t *testing.T) {
	report := CompactRecoveryInspectionReport{
		Complete: true, Valid: false,
		Edges: []CompactRecoveryEdgeInspection{
			{
				PredecessorLineageID: "root", SuccessorLineageID: "leaf",
				Valid: false, AnomalyClasses: []string{compactRecoveryEdgeUnchangedTarget},
			},
		},
	}
	first := shadowClassifyAuthorityHealth(report)
	second := shadowClassifyAuthorityHealth(report)
	if first != second || first != shadowAuthorityRepairable {
		t.Fatalf("repeat pure classification = (%q, %q), want both %q", first, second, shadowAuthorityRepairable)
	}

	repo := initSnapshotRepo(t)
	inspectRecoveryPair(t, repo, "idempotent", true, "")
	firstLive, err := shadowAuthorityHealthAtRepo(context.Background(), repo)
	inspectNoError(t, err)
	secondLive, err := shadowAuthorityHealthAtRepo(context.Background(), repo)
	inspectNoError(t, err)
	if firstLive != secondLive {
		t.Fatalf("repeat live classification = (%q, %q), want identical", firstLive, secondLive)
	}
}
