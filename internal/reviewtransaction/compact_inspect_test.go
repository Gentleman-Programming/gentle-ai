package reviewtransaction

import (
	"bytes"
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

func TestFinalVerificationRetryTerminalAuthorityTraversesReadOnlyGraphSurfaces(t *testing.T) {
	for _, terminal := range []struct {
		name     string
		approved bool
	}{
		{name: "approved", approved: true},
		{name: "escalated", approved: false},
	} {
		t.Run(terminal.name, func(t *testing.T) {
			fixture, successor, store, receipt := terminalFinalVerificationRetryFixture(t, terminal.name, terminal.approved)
			beforeState := readRetryAuthorityBytes(t, store)
			beforeRevision := successor.Revision

			if err := validateCompactFinalVerificationRetryEdge(fixture.predecessor, successor.State); err != nil {
				t.Fatalf("direct retry edge validation: %v", err)
			}
			inventory, err := InventoryAuthority(context.Background(), fixture.repo)
			if err != nil || !inventory.Complete || !inventory.Authoritative {
				t.Fatalf("authority inventory = %#v, %v", inventory, err)
			}
			leaves, err := CompactAuthorityLeaves(context.Background(), fixture.repo)
			if err != nil || len(leaves) != 1 || leaves[0].lineageID != successor.State.LineageID {
				t.Fatalf("authority leaves = %#v, %v", leaves, err)
			}
			inspection, err := InspectCompactRecoveryEdges(context.Background(), fixture.repo)
			if err != nil {
				t.Fatal(err)
			}
			edge := inspectedEdge(t, inspection, successor.State.LineageID)
			if !edge.Valid || len(edge.AnomalyClasses) != 0 || len(edge.Problems) != 0 {
				t.Fatalf("retry inspection edge = %#v", edge)
			}
			payload, err := os.ReadFile(store.ReceiptPath())
			if err != nil {
				t.Fatal(err)
			}
			discovered, err := ParseCompactReceipt(payload)
			if err != nil || !compactReceiptEqual(discovered, receipt) {
				t.Fatalf("discovered retry receipt = %#v, %v", discovered, err)
			}
			after := mustLoadCompactRecord(t, store)
			if after.Revision != beforeRevision || !bytes.Equal(beforeState["state"], readRetryAuthorityBytes(t, store)["state"]) ||
				!bytes.Equal(beforeState["receipt"], readRetryAuthorityBytes(t, store)["receipt"]) {
				t.Fatal("read-only retry traversal rewrote authority state, receipt, or revision")
			}
		})
	}
}

func TestFinalVerificationRetryCorruptionFailsClosedAcrossAuthoritySurfaces(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*CompactState)
	}{
		{name: "lifecycle", mutate: func(state *CompactState) { state.State = StateValidating }},
		{name: "frozen policy", mutate: func(state *CompactState) { state.PolicyHash = payloadDigest([]byte("forged-policy")) }},
		{name: "source proof", mutate: func(state *CompactState) {
			state.Recovery.FinalVerificationRetry.FailedEvidenceHash = payloadDigest([]byte("forged-proof"))
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			fixture, successor, store, receipt := terminalFinalVerificationRetryFixture(t, "corrupted-"+strings.ReplaceAll(mutation.name, " ", "-"), true)
			mutation.mutate(&successor.State)
			_, payload, err := makeCompactRecord(successor.State)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
				t.Fatal(err)
			}

			inventory, err := InventoryAuthority(context.Background(), fixture.repo)
			quarantined := slices.ContainsFunc(inventory.Diagnostics, func(diagnostic AuthorityInventoryDiagnostic) bool {
				return strings.Contains(diagnostic.Path, successor.State.LineageID)
			})
			if err != nil || inventory.Authoritative && !quarantined {
				t.Fatalf("corrupted authority inventory = %#v, %v", inventory, err)
			}
			leaves, leafErr := CompactAuthorityLeaves(context.Background(), fixture.repo)
			if leafErr == nil && slices.ContainsFunc(leaves, func(leaf CompactStore) bool { return leaf.lineageID == successor.State.LineageID }) {
				t.Fatalf("corrupted retry authority was accepted as a leaf: %#v", leaves)
			}
			inspection, err := InspectCompactRecoveryEdges(context.Background(), fixture.repo)
			if err != nil {
				t.Fatal(err)
			}
			for _, edge := range inspection.Edges {
				if edge.SuccessorLineageID == successor.State.LineageID && edge.Valid {
					t.Fatalf("corrupted retry inspection = %#v", inspection)
				}
			}
			if len(inspection.Edges) == 0 && !slices.ContainsFunc(inspection.EntryDiagnostics, func(diagnostic CompactRecoveryEntryDiagnostic) bool {
				return diagnostic.LineageID == successor.State.LineageID
			}) {
				t.Fatalf("corrupted retry was neither rejected nor diagnosed: %#v", inspection)
			}
			if gate := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: receipt.LineageID}); gate.Result != GateInvalidated {
				t.Fatalf("corrupted retry gate = %#v", gate)
			}
		})
	}
}

func terminalFinalVerificationRetryFixture(t *testing.T, suffix string, approved bool) (finalVerificationRetryFixture, CompactRecord, CompactStore, CompactReceipt) {
	t.Helper()
	fixture := newFinalVerificationRetryFixture(t, "retry-graph-source-"+suffix, "retry-graph-successor-"+suffix)
	successor, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := successor.State.CompleteVerification([]byte("retry terminal verification\n"), approved); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, successor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(successor.Revision, "review/complete-verification", successor.State)
	if err != nil {
		t.Fatal(err)
	}
	successor.Revision = revision
	receipt, err := successor.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return fixture, successor, store, receipt
}

func readRetryAuthorityBytes(t *testing.T, store CompactStore) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for name, path := range map[string]string{"state": store.StatePath(), "receipt": store.ReceiptPath()} {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[name] = payload
	}
	return result
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
