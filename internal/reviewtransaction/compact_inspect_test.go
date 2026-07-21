package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validInspectRecoveryFixture(t *testing.T, repo string) (CompactRecord, CompactStore, CompactRecord, CompactStore) {
	t.Helper()
	return poisonedRecoveryFixture(t, repo, func(state *CompactState) {
		writeSnapshotFile(t, repo, "tracked.txt", "valid recovery target\n")
		changed := newCompactTestState(t, repo, state.LineageID)
		state.InitialSnapshot, state.CurrentSnapshot = changed.InitialSnapshot, changed.CurrentSnapshot
		state.GenesisPaths = changed.GenesisPaths
		state.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
			state.Recovery.PredecessorLineageID, state.Recovery.PredecessorRevision,
			state.InitialSnapshot.Identity, state.Recovery.Actor, state.Recovery.Reason)
	})
}

func TestInspectCompactAuthorityEmpty(t *testing.T) {
	report, err := InspectCompactAuthority(context.Background(), initSnapshotRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary != (CompactAuthorityInspectionSummary{}) || len(report.Edges) != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("empty inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityAllValid(t *testing.T) {
	repo := initSnapshotRepo(t)
	validInspectRecoveryFixture(t, repo)
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TotalEdges != 1 || report.Summary.ValidEdges != 1 || report.Summary.InvalidEdges != 0 || len(report.Edges) != 0 {
		t.Fatalf("valid inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityUnchangedTarget(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successor, _ := poisonedRecoveryFixture(t, repo, nil)
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Edges) != 1 || report.Edges[0].SuccessorLineageID != successor.State.LineageID || report.Edges[0].AnomalyClass != "unchanged_target" || !strings.HasPrefix(report.Edges[0].ValidationError, "escalated recovery successor target has not changed") {
		t.Fatalf("unchanged-target inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityMalformedAuthorization(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successor, _ := preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Edges) != 1 || report.Edges[0].SuccessorLineageID != successor.State.LineageID || report.Edges[0].AnomalyClass != "malformed_recovery_authorization" {
		t.Fatalf("malformed-authorization inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityCombinedAnomalies(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successor, _ := combinedRecoveryFixture(t, repo, nil)
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Edges) != 1 || report.Edges[0].SuccessorLineageID != successor.State.LineageID || report.Edges[0].AnomalyClass != "unchanged_target,malformed_recovery_authorization" {
		t.Fatalf("combined inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityMultipleInvalid(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successor, _ := poisonedRecoveryFixture(t, repo, nil)
	second := successor.State
	second.LineageID = "inspect-successor-a"
	secondStore, err := CompactAuthoritativeStore(context.Background(), repo, second.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, secondStore, second)
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.InvalidEdges != 2 || len(report.Edges) != 2 || report.Edges[0].SuccessorLineageID > report.Edges[1].SuccessorLineageID {
		t.Fatalf("multiple invalid inspection = %#v", report)
	}
}

func TestInspectCompactAuthorityDeterminism(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, _, successor, _ := poisonedRecoveryFixture(t, repo, nil)
	second := successor.State
	second.LineageID = "inspect-successor-a"
	store, err := CompactAuthoritativeStore(context.Background(), repo, second.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, second)
	first, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	secondReport, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(secondReport)
	golden, err := os.ReadFile(filepath.Join("testdata", "inspect_authority_determinism.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) || !bytes.Equal(firstJSON, golden) {
		t.Fatalf("inspection JSON is not deterministic\nfirst=%s\ngolden=%s", firstJSON, golden)
	}
}

func TestInspectCompactAuthorityReadOnlyInvariant(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, predecessorStore, _, successorStore := poisonedRecoveryFixture(t, repo, nil)
	paths := []string{predecessorStore.StatePath(), successorStore.StatePath()}
	before := make([][]byte, len(paths))
	mtimes := make([]int64, len(paths))
	for i, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[i], mtimes[i] = payload, info.ModTime().UnixNano()
	}
	if _, err := InspectCompactAuthority(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		payload, _ := os.ReadFile(path)
		info, _ := os.Stat(path)
		if !bytes.Equal(payload, before[i]) || info.ModTime().UnixNano() != mtimes[i] {
			t.Fatalf("inspection mutated %s", path)
		}
	}
}

func TestInspectCompactAuthorityLoadError(t *testing.T) {
	repo := initSnapshotRepo(t)
	validInspectRecoveryFixture(t, repo)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "v2", "inspect-corrupt")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "review-state.json"), []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := InspectCompactAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TotalEdges != 1 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Code != "load_failure" || report.Diagnostics[0].Message == "" {
		t.Fatalf("load-error inspection = %#v", report)
	}
}
