package reviewtransaction

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestInspectCompactRecoveryEdges(t *testing.T) {
	t.Run("empty authority", func(t *testing.T) {
		report, err := InspectCompactRecoveryEdges(context.Background(), initSnapshotRepo(t))
		if err != nil || !report.Complete || !report.Valid || report.Edges == nil || report.EntryDiagnostics == nil || report.Totals != (CompactRecoveryInspectionTotals{}) {
			t.Fatalf("empty inspection = %#v, err=%v", report, err)
		}
	})
	t.Run("all decodable edges and entry failures", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, _, _, validStore := inspectRecoveryPair(t, repo, "a-valid", false, "")
		inspectRecoveryPair(t, repo, "b-unchanged", true, "")
		inspectRecoveryPair(t, repo, "c-malformed", false, "legacy approval")
		_, danglingStore, _, _ := inspectRecoveryPair(t, repo, "d-dangling", false, "")
		inspectNoError(t, os.RemoveAll(danglingStore.Dir))
		forkRoot, _, _, _ := inspectRecoveryPair(t, repo, "e-fork", false, "")
		writeSnapshotFile(t, repo, "tracked.txt", "fork sibling\n")
		inspectRecoverySuccessor(t, repo, forkRoot, "e-fork-sibling", "")
		inspectRecoveryCycle(t, repo)
		base, _, err := reviewAuthorityRoot(context.Background(), repo)
		inspectNoError(t, err)
		const secretPath = "/private/secret/logical/path"
		broken := filepath.Join(base, "v2", "broken-entry")
		inspectNoError(t, os.MkdirAll(broken, 0o755))
		brokenState := newCompactTestState(t, repo, "broken-entry")
		brokenState.GenesisPaths = []string{secretPath}
		_, brokenPayload, err := makeCompactRecord(brokenState)
		inspectNoError(t, err)
		brokenPath := filepath.Join(broken, compactStateFileName)
		inspectNoError(t, os.WriteFile(brokenPath, brokenPayload, 0o644))
		before := inspectReadState(t, validStore.StatePath()) + inspectReadState(t, brokenPath)
		first, err := InspectCompactRecoveryEdges(context.Background(), repo)
		inspectNoError(t, err)
		second, err := InspectCompactRecoveryEdges(context.Background(), repo)
		inspectNoError(t, err)
		firstJSON, _ := json.Marshal(first)
		secondJSON, _ := json.Marshal(second)
		if !reflect.DeepEqual(first, second) || string(firstJSON) != string(secondJSON) {
			t.Fatalf("repeat inspection is not deterministic:\n%s\n%s", firstJSON, secondJSON)
		}
		if before != inspectReadState(t, validStore.StatePath())+inspectReadState(t, brokenPath) {
			t.Fatal("inspection mutated compact authority bytes")
		}
		wantTotals := CompactRecoveryInspectionTotals{CompactEntries: 13, LoadedEntries: 12, Edges: 8, ValidEdges: 1, InvalidEdges: 7, EntryDiagnostics: 1}
		if first.Totals != wantTotals || first.Complete || first.Valid || len(first.EntryDiagnostics) != 1 || first.EntryDiagnostics[0].LineageID != "broken-entry" {
			t.Fatalf("inspection summary = %#v, want totals %#v and one broken entry", first, wantTotals)
		}
		if strings.Contains(string(firstJSON), "legacy approval") || strings.Contains(string(firstJSON), secretPath) {
			t.Fatal("inspection exposed raw authorization or a persisted logical path")
		}
		if first.EntryDiagnostics[0].Problem != compactInspectionEntryMalformed {
			t.Fatalf("entry diagnostic = %#v", first.EntryDiagnostics[0])
		}
		checks := []struct {
			successor string
			valid     bool
			anomaly   string
			problems  []string
		}{
			{"a-valid-successor", true, "", nil},
			{"b-unchanged-successor", false, compactRecoveryEdgeUnchangedTarget, nil},
			{"c-malformed-successor", false, compactRecoveryEdgeMalformedAuthorization, nil},
			{"d-dangling-successor", false, "", []string{"missing predecessor"}},
			{"e-fork-successor", false, "", []string{"recovery fork"}},
			{"e-fork-sibling", false, "", []string{"recovery fork"}},
			{"cycle-a", false, "", []string{"recovery cycle", "recovery predecessor revision mismatch"}},
			{"cycle-b", false, "", []string{"recovery cycle", "recovery predecessor revision mismatch"}},
		}
		for _, check := range checks {
			edge := inspectedEdge(t, first, check.successor)
			if edge.Valid != check.valid || check.anomaly != "" && !slices.Contains(edge.AnomalyClasses, check.anomaly) {
				t.Fatalf("edge %q = %#v", check.successor, edge)
			}
			for _, problem := range check.problems {
				if !slices.Contains(edge.Problems, problem) {
					t.Fatalf("edge %q problems = %#v, want %q", check.successor, edge.Problems, problem)
				}
			}
		}
	})
}

// TestLoadCompactRecoveryRecordsIsTheSingleSeam satisfies tasks.md 1.1
// (mandatory obligation (a)): InspectCompactRecoveryEdges and
// deriveAuthorityDispositionPlanAtRepo (authority_disposition_plan.go) MUST
// both call loadCompactRecoveryRecords for their report/records inputs, and
// no independent second record-loading path may feed either one. It proves
// this the same way review_inspect_authority_test.go proves a single-call
// domain: swap the package-level seam var for an instrumented wrapper and
// count calls plus compare the exact report/record content each caller saw.
func TestLoadCompactRecoveryRecordsIsTheSingleSeam(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "seam", "seam target\n")

	original := loadCompactRecoveryRecords
	t.Cleanup(func() { loadCompactRecoveryRecords = original })
	calls := 0
	var seenReports []CompactRecoveryInspectionReport
	var seenRecordCounts []int
	loadCompactRecoveryRecords = func(ctx context.Context, gotRepo string) (CompactRecoveryInspectionReport, map[string]CompactRecord, error) {
		calls++
		report, records, err := original(ctx, gotRepo)
		seenReports = append(seenReports, report)
		seenRecordCounts = append(seenRecordCounts, len(records))
		return report, records, err
	}

	ctx := context.Background()
	if _, err := InspectCompactRecoveryEdges(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "maintainer@example.com", "quarantine forged recovery authorization"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("loadCompactRecoveryRecords calls = %d, want 2 (one per caller, no independent second load path)", calls)
	}
	firstJSON, err := json.Marshal(seenReports[0])
	inspectNoError(t, err)
	secondJSON, err := json.Marshal(seenReports[1])
	inspectNoError(t, err)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("InspectCompactRecoveryEdges and deriveAuthorityDispositionPlanAtRepo saw different report content through the seam:\n%s\n%s", firstJSON, secondJSON)
	}
	if seenRecordCounts[0] != seenRecordCounts[1] {
		t.Fatalf("InspectCompactRecoveryEdges and deriveAuthorityDispositionPlanAtRepo saw different record counts through the seam: %d vs %d", seenRecordCounts[0], seenRecordCounts[1])
	}
}

// TestLoadCompactRecoveryRecordsUnderMaintenanceHoldAgreesWithOrdinarySeam
// pins the agreement loadCompactRecoveryRecordsUnderMaintenanceHold's own doc
// comment (authority_disposition_execute.go) asserts but nothing tested
// directly before this: it mirrors loadCompactRecoveryRecords's exact
// read-and-classify body for a caller that already holds base's exclusive
// maintenance lock, differing only in the per-entry file read
// (loadCompactRecordLocked vs Load), never the classification algorithm. The
// under-lock CAS comparison executeAuthorityDisposition runs
// (lockedAuthorityDispositionMutation comparing currentPlan.PlanDigest
// against plan.PlanDigest) rests entirely on this agreement holding: if the
// two seams ever classified the same authority state differently, a
// re-derivation under lock could silently diverge from what the read-only
// plan preview promised.
func TestLoadCompactRecoveryRecordsUnderMaintenanceHoldAgreesWithOrdinarySeam(t *testing.T) {
	repo := initSnapshotRepo(t)
	forgedRecoveryPair(t, repo, "agreement", "agreement target\n")
	inspectRecoveryPair(t, repo, "agreement-pristine", false, "")

	ctx := context.Background()
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	inspectNoError(t, err)

	ordinaryReport, ordinaryRecords, err := loadCompactRecoveryRecords(ctx, root)
	inspectNoError(t, err)
	lockedReport, lockedRecords, err := loadCompactRecoveryRecordsUnderMaintenanceHold(ctx, root)
	inspectNoError(t, err)

	ordinaryJSON, err := json.Marshal(ordinaryReport)
	inspectNoError(t, err)
	lockedJSON, err := json.Marshal(lockedReport)
	inspectNoError(t, err)
	if string(ordinaryJSON) != string(lockedJSON) {
		t.Fatalf("loadCompactRecoveryRecordsUnderMaintenanceHold disagrees with loadCompactRecoveryRecords on inspection:\n%s\n%s", ordinaryJSON, lockedJSON)
	}
	if len(ordinaryReport.Edges) == 0 {
		t.Fatal("fixture produced no recovery edges; the agreement this test proves is vacuous")
	}
	if len(ordinaryRecords) != len(lockedRecords) {
		t.Fatalf("record counts disagree: ordinary=%d locked=%d", len(ordinaryRecords), len(lockedRecords))
	}
	for lineage, ordinaryRecord := range ordinaryRecords {
		lockedRecord, found := lockedRecords[lineage]
		if !found {
			t.Fatalf("locked-hold read is missing lineage %q the ordinary seam loaded", lineage)
		}
		ordinaryRecordJSON, err := json.Marshal(ordinaryRecord)
		inspectNoError(t, err)
		lockedRecordJSON, err := json.Marshal(lockedRecord)
		inspectNoError(t, err)
		if string(ordinaryRecordJSON) != string(lockedRecordJSON) {
			t.Fatalf("record %q disagrees between seams:\n%s\n%s", lineage, ordinaryRecordJSON, lockedRecordJSON)
		}
	}
}

func TestCompactRecoveryInspectionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectCompactRecoveryEdges(ctx, initSnapshotRepo(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("entry traversal cancellation error = %v", err)
	}
	tests := []struct {
		name   string
		checks int
		edges  []CompactRecoveryEdgeInspection
		run    func(context.Context, []CompactRecoveryEdgeInspection) error
	}{
		{"fork sibling marking", 3, []CompactRecoveryEdgeInspection{{PredecessorLineageID: "root", SuccessorLineageID: "a", Valid: true}, {PredecessorLineageID: "root", SuccessorLineageID: "b", Valid: true}}, markCompactRecoveryForks},
		{"cycle participant marking", 6, []CompactRecoveryEdgeInspection{{PredecessorLineageID: "b", SuccessorLineageID: "a", Valid: true}, {PredecessorLineageID: "a", SuccessorLineageID: "b", Valid: true}}, markCompactRecoveryCycles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(&cancelAfterChecksContext{Context: context.Background(), remaining: tt.checks}, tt.edges); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error = %v", err)
			}
			if slices.ContainsFunc(tt.edges, func(edge CompactRecoveryEdgeInspection) bool { return !edge.Valid || len(edge.Problems) != 0 }) {
				t.Fatalf("edges mutated after mid-pass cancellation: %#v", tt.edges)
			}
		})
	}
	values := []string{"c", "b", "a"}
	if err := sortCompactInspection(&cancelAfterChecksContext{Context: context.Background(), remaining: 1}, values, cmp.Compare[string]); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-sort cancellation error = %v", err)
	}
}

type cancelAfterChecksContext struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterChecksContext) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}
func inspectRecoveryPair(t *testing.T, repo, prefix string, unchanged bool, authorization string) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	state := correctedCompactTestState(t, repo, prefix+"-predecessor")
	state.State = StateEscalated
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	inspectNoError(t, err)
	predecessor := writeCompactFixtureRecord(t, store, state)
	if !unchanged {
		writeSnapshotFile(t, repo, "tracked.txt", prefix+" successor\n")
	}
	successor, successorStore := inspectRecoverySuccessor(t, repo, predecessor, prefix+"-successor", authorization)
	return predecessor, store, successor, successorStore
}
func inspectRecoverySuccessor(t *testing.T, repo string, predecessor CompactRecord, lineage, authorization string) (CompactRecord, CompactStore) {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	state.Generation = predecessor.State.Generation + 1
	state.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.State.LineageID, PredecessorRevision: predecessor.Revision,
		Disposition: RecoveryEscalated, Reason: "retry", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), MaintainerAuthorization: authorization,
	}
	if authorization == "" {
		state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, state.InitialSnapshot.Identity, state.Recovery.Actor, state.Recovery.Reason)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	inspectNoError(t, err)
	return writeCompactFixtureRecord(t, store, state), store
}
func inspectRecoveryCycle(t *testing.T, repo string) {
	t.Helper()
	for _, edge := range []struct{ lineage, predecessor, revision string }{{"cycle-a", "cycle-b", "a"}, {"cycle-b", "cycle-a", "b"}} {
		state := newCompactTestState(t, repo, edge.lineage)
		state.Recovery = &CompactRecoveryProvenance{
			PredecessorLineageID: edge.predecessor, PredecessorRevision: hash(edge.revision),
			Disposition: RecoveryInvalidated, Reason: "cycle fixture", Actor: "maintainer@example.com",
			RecoveredAt: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		}
		store, err := CompactAuthoritativeStore(context.Background(), repo, edge.lineage)
		inspectNoError(t, err)
		writeCompactFixtureRecord(t, store, state)
	}
}
func inspectReadState(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	inspectNoError(t, err)
	return string(payload)
}
func inspectedEdge(t *testing.T, report CompactRecoveryInspectionReport, successor string) CompactRecoveryEdgeInspection {
	t.Helper()
	for _, edge := range report.Edges {
		if edge.SuccessorLineageID == successor {
			return edge
		}
	}
	t.Fatalf("edge %q was omitted: %#v", successor, report.Edges)
	return CompactRecoveryEdgeInspection{}
}
func inspectNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestSanctionedExitsDecoupleHistoricalExitsFromDispositionSeed pins the
// CodeRabbit follow-up hardening on SanctionedCompactRecoveryExits: the
// historical per-entry exits and the selectorless disposition-seed
// derivation are independent computations. The derivation now runs
// unconditionally — it must never append a second repair exit for a
// historical-class plan historicalDispositionExitLineages already covered,
// and it must still feed dispositionSeed when the admitted plan is
// non-historical. Derivation errors stay unpropagated in every case.
func TestSanctionedExitsDecoupleHistoricalExitsFromDispositionSeed(t *testing.T) {
	ctx := context.Background()

	t.Run("all-historical N=2 store keeps exactly one repair exit per entry", func(t *testing.T) {
		repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		inspectNoError(t, err)
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		inspectNoError(t, err)
		// The unconditional selectorless derivation refuses this store (two
		// diagnostics), so dispositionSeed stays empty and the exits are
		// exactly the two historical per-entry repair exits — the independence
		// pin: no derivation-driven extra exit may appear next to them.
		if len(exits) != 2 || exits[0].SuccessorLineageID != issue2995EscalatedLineage || exits[1].SuccessorLineageID != issue2995ApprovedLineage {
			t.Fatalf("all-historical N=2 exits = %+v, want exactly the two per-entry repair exits in lineage order", exits)
		}
		for _, exit := range exits {
			if exit.Operation != CompactRecoveryEdgeExitRepair || exit.Blocked != "" {
				t.Fatalf("historical exit for %q = %#v, want an unblocked repair", exit.SuccessorLineageID, exit)
			}
		}
	})

	t.Run("all-historical N=1 store keeps exactly one repair exit", func(t *testing.T) {
		repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName)
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		inspectNoError(t, err)
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		inspectNoError(t, err)
		// Here the unconditional derivation DOES close (the N=1 selectorless
		// historical plan) and must not append a duplicate repair exit for the
		// lineage historicalDispositionExitLineages already covered.
		if len(exits) != 1 || exits[0].SuccessorLineageID != issue2995ApprovedLineage || exits[0].Operation != CompactRecoveryEdgeExitRepair {
			t.Fatalf("all-historical N=1 exits = %+v, want exactly one unduplicated repair exit", exits)
		}
	})

	t.Run("non-historical admitted plan still seeds a blocked content-mismatch edge without historical exits", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, successor, _ := forgedRecoveryPair(t, repo, "seedpin", "forged seed pin target\n")
		// A valid recovery from the forged successor makes it INTERIOR, so
		// abandon's own prediction refuses it and the seed path is the only
		// reachable repair advertisement for its content-mismatch edge.
		interiorSuccessor, _ := inspectRecoverySuccessor(t, repo, successor, "seedpin-interior", "")
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		inspectNoError(t, err)
		if len(report.historical) != 0 {
			t.Fatalf("fixture store unexpectedly carries historical entries: %d", len(report.historical))
		}
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		inspectNoError(t, err)
		var successorExit *CompactRecoverySanctionedExit
		for index := range exits {
			if exits[index].SuccessorLineageID == successor.State.LineageID {
				successorExit = &exits[index]
			}
		}
		if successorExit == nil || successorExit.Operation != CompactRecoveryEdgeExitRepair || successorExit.Blocked != "" {
			t.Fatalf("interior forged successor exit = %#v, want the disposition-seed repair for %q", successorExit, interiorSuccessor.State.LineageID)
		}
	})

	t.Run("historical exits do not suppress the disposition seed for a non-historical admitted plan", func(t *testing.T) {
		// The reachability probe for the decoupled branch: one forensic
		// historical entry (the only diagnostic, so historicalExits is
		// non-empty) alongside a loaded content-mismatch pair. The selectorless
		// derivation closes on the EDGE plan (non-historical), and its seed
		// must still drive the interior successor's repair exit.
		repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName)
		_, successor, _ := forgedRecoveryPair(t, repo, "mixedseed", "forged mixed seed target\n")
		inspectRecoverySuccessor(t, repo, successor, "mixedseed-interior", "")
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		inspectNoError(t, err)
		if len(report.historical) != 1 || len(report.EntryDiagnostics) != 1 {
			t.Fatalf("mixed store = %d historical / %d diagnostics, want 1/1", len(report.historical), len(report.EntryDiagnostics))
		}
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		inspectNoError(t, err)
		seen := map[string]int{}
		sawHistoricalExit, sawSeedRepair := false, false
		for _, exit := range exits {
			seen[exit.SuccessorLineageID]++
			switch exit.SuccessorLineageID {
			case issue2995ApprovedLineage:
				sawHistoricalExit = exit.Operation == CompactRecoveryEdgeExitRepair && exit.Blocked == ""
			case successor.State.LineageID:
				sawSeedRepair = exit.Operation == CompactRecoveryEdgeExitRepair && exit.Blocked == ""
			}
		}
		// The interior's own (leaf, unchanged-target) edge keeps its
		// pre-existing abandon exit; the pin is that the seeded repair appears
		// alongside the historical per-entry exit, once per lineage.
		if !sawHistoricalExit || !sawSeedRepair || seen[issue2995ApprovedLineage] != 1 || seen[successor.State.LineageID] != 1 {
			t.Fatalf("mixed-store exits = %+v, want the historical repair exit and the seeded interior repair exit, once per lineage", exits)
		}
	})
}
