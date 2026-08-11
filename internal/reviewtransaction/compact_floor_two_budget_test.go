package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// TestCompactFloorTwoBudgetGrantsAtomicReplacementForOneLineCandidate
// verifies that a newly persisted compact state over a small candidate
// receives a correction budget of at least two, so an atomic one-line
// replacement (one Git addition plus one Git deletion) fits within the
// frozen budget. This is the behavior-first reproduction for issue #2247.
func TestCompactFloorTwoBudgetGrantsAtomicReplacementForOneLineCandidate(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	legacyBudget, _ := CorrectionBudget(lines)
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{
		LineageID: "floor-two-one-line", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: hash("a"), RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatalf("NewCompactState: %v", err)
	}
	if state.CorrectionBudget < 2 {
		t.Errorf("small candidate correction budget = %d, want >= 2 for an atomic replacement", state.CorrectionBudget)
	}
	if state.CorrectionBudget < legacyBudget {
		t.Errorf("floor-two budget %d is below legacy budget %d", state.CorrectionBudget, legacyBudget)
	}
	if state.CorrectionBudgetPolicy != CorrectionBudgetPolicyFloorTwo {
		t.Errorf("policy = %q, want %q", state.CorrectionBudgetPolicy, CorrectionBudgetPolicyFloorTwo)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("floor-two state validation: %v", err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatalf("persist floor-two state: %v", err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != revision || loaded.State.CorrectionBudgetPolicy != CorrectionBudgetPolicyFloorTwo ||
		!bytes.Contains(payload, []byte(`"correction_budget_policy": "floor_two"`)) {
		t.Fatalf("persisted floor-two state = %#v", loaded)
	}
}

// TestCompactFloorTwoBudgetPreservesLegacyHistoricalStateValidation
// verifies that a compact state persisted without the floor-two policy
// field (a historical state from an older build) still validates
// byte-for-byte with the legacy CorrectionBudget formula. Historical
// states must not be migrated or rejected.
func TestCompactFloorTwoBudgetPreservesLegacyHistoricalStateValidation(t *testing.T) {
	for _, tt := range []struct {
		name           string
		originalLines  int
		legacyBudget   int
		wantValidation bool
	}{
		{name: "one-line historical budget one", originalLines: 1, legacyBudget: 1, wantValidation: true},
		{name: "two-line historical budget one", originalLines: 2, legacyBudget: 1, wantValidation: true},
		{name: "five-line historical budget three", originalLines: 5, legacyBudget: 3, wantValidation: true},
		{name: "one-line historical budget two is not legacy", originalLines: 1, legacyBudget: 2, wantValidation: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := CompactState{
				Schema: CompactStateSchema, LineageID: "legacy-budget-lineage", Generation: 1,
				State: StateReviewing, InitialSnapshot: validCompactSnapshot(t, tt.originalLines),
				CurrentSnapshot: validCompactSnapshot(t, tt.originalLines),
				GenesisPaths:    []string{"tracked.txt"}, PolicyHash: hash("b"),
				RiskLevel: RiskHigh, SelectedLenses: supportedLenses,
				OriginalChangedLines: tt.originalLines, CorrectionBudget: tt.legacyBudget,
				LensResults: []LensResult{}, Findings: []Finding{},
				Classifications: map[string]FindingEvidence{}, Outcomes: map[string]EvidenceOutcome{},
				FixFindingIDs: []string{}, FollowUps: []FollowUp{}, FixDeltaHash: EmptyFixDeltaHash,
			}
			err := state.Validate()
			if tt.wantValidation && err != nil {
				t.Fatalf("historical legacy budget %d for %d original lines should validate: %v", tt.legacyBudget, tt.originalLines, err)
			}
			if !tt.wantValidation && err == nil {
				t.Fatalf("budget %d for %d original lines should NOT validate as legacy but did", tt.legacyBudget, tt.originalLines)
			}
		})
	}
}

// TestCompactFloorTwoBudgetRejectsUnknownPolicy verifies that a compact
// state carrying an unrecognized budget policy value is rejected.
func TestCompactFloorTwoBudgetRejectsUnknownPolicy(t *testing.T) {
	state := CompactState{
		Schema: CompactStateSchema, LineageID: "unknown-policy-lineage", Generation: 1,
		State: StateReviewing, InitialSnapshot: validCompactSnapshot(t, 4),
		CurrentSnapshot: validCompactSnapshot(t, 4),
		GenesisPaths:    []string{"tracked.txt"}, PolicyHash: hash("c"),
		RiskLevel: RiskHigh, SelectedLenses: supportedLenses,
		OriginalChangedLines: 4, CorrectionBudget: 2,
		CorrectionBudgetPolicy: "floor_three",
		LensResults:            []LensResult{}, Findings: []Finding{},
		Classifications: map[string]FindingEvidence{}, Outcomes: map[string]EvidenceOutcome{},
		FixFindingIDs: []string{}, FollowUps: []FollowUp{}, FixDeltaHash: EmptyFixDeltaHash,
	}
	if err := state.Validate(); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("unknown policy validation error = %v, want a policy error", err)
	}
}

func TestCompactSuccessorRejectsBudgetPolicyDriftWhenNumericBudgetCoincides(t *testing.T) {
	snapshot := validCompactSnapshot(t, 4)
	legacyBudget, err := CorrectionBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	compactBudget, err := CompactCorrectionBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	if legacyBudget != compactBudget {
		t.Fatalf("test setup budgets differ: legacy=%d compact=%d", legacyBudget, compactBudget)
	}
	previous := CompactState{
		Schema: CompactStateSchema, LineageID: "policy-immutable", Generation: 1, State: StateReviewing,
		InitialSnapshot: snapshot, CurrentSnapshot: snapshot, GenesisPaths: append([]string{}, snapshot.Paths...),
		PolicyHash: hash("policy-immutable"), RiskLevel: RiskHigh, SelectedLenses: supportedLenses,
		OriginalChangedLines: 4, CorrectionBudget: compactBudget, CorrectionBudgetPolicy: CorrectionBudgetPolicyFloorTwo,
		LensResults: []LensResult{}, Findings: []Finding{}, Classifications: map[string]FindingEvidence{},
		Outcomes: map[string]EvidenceOutcome{}, FixFindingIDs: []string{}, FollowUps: []FollowUp{}, FixDeltaHash: EmptyFixDeltaHash,
	}
	for _, policy := range []string{"", "floor_three"} {
		next := previous
		next.State = StateCorrectionRequired
		next.CorrectionBudgetPolicy = policy
		if err := validateCompactSuccessor("sha256:"+strings.Repeat("a", 64), previous, next, "review/complete-review"); !errors.Is(err, ErrInvalidSuccessor) {
			t.Fatalf("successor policy %q error = %v, want ErrInvalidSuccessor", policy, err)
		}
	}
}

func TestCompactHistoricalPolicylessPersistencePreservesBytesAndRevision(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "historical-policyless-round-trip")
	state.CorrectionBudget, _ = CorrectionBudget(state.OriginalChangedLines)
	state.CorrectionBudgetPolicy = ""
	if err := state.Validate(); err != nil {
		t.Fatalf("historical policy-less state validation: %v", err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatalf("persist historical policy-less state: %v", err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != revision || loaded.State.CorrectionBudgetPolicy != "" || !bytes.Equal(before, after) {
		t.Fatalf("historical policy-less persistence changed: revision=%s/%s policy=%q bytes_equal=%v", loaded.Revision, revision, loaded.State.CorrectionBudgetPolicy, bytes.Equal(before, after))
	}
}

// TestCompactFloorTwoBudgetOneLineReplacementAccounting verifies the
// core issue #2247 scenario: a one-line candidate's floor-two budget
// admits an atomic replacement correction that counts as one Git addition
// plus one Git deletion (two changed lines), which the legacy budget of
// one could never fit.
func TestCompactFloorTwoBudgetOneLineReplacementAccounting(t *testing.T) {
	budget, err := CompactCorrectionBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	if budget < 2 {
		t.Fatalf("CompactCorrectionBudget(1) = %d, want >= 2", budget)
	}
	replacementChangedLines := 2 // one addition + one deletion
	if replacementChangedLines > budget {
		t.Fatalf("atomic one-line replacement (%d changed lines) exceeds floor-two budget %d", replacementChangedLines, budget)
	}
}

// TestCompactCorrectionBudgetBoundaries verifies the floor-two policy
// formula across representative original line counts.
func TestCompactCorrectionBudgetBoundaries(t *testing.T) {
	tests := []struct {
		original int
		want     int
	}{
		{original: 0, want: 0}, {original: 1, want: 2}, {original: 2, want: 2},
		{original: 3, want: 2}, {original: 4, want: 2}, {original: 5, want: 3},
		{original: 6, want: 3}, {original: 7, want: 4}, {original: 8, want: 4},
		{original: 196, want: 98}, {original: 399, want: 200}, {original: 400, want: 200},
		{original: 401, want: 200}, {original: 867, want: 200},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := CompactCorrectionBudget(tt.original)
			if err != nil || got != tt.want {
				t.Fatalf("CompactCorrectionBudget(%d) = %d, %v; want %d", tt.original, got, err, tt.want)
			}
		})
	}
	if _, err := CompactCorrectionBudget(-1); err == nil {
		t.Fatal("CompactCorrectionBudget() accepted negative original lines")
	}
}

func validCompactSnapshot(t *testing.T, lines int) Snapshot {
	t.Helper()
	repo := initSnapshotRepo(t)
	content := "base\n"
	for i := 0; i < lines; i++ {
		content += "line\n"
	}
	writeSnapshotFile(t, repo, "tracked.txt", content)
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
