package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestCompactSemanticStateErrorTyped proves issue-1813's construction step:
// Load() wraps CompactState.Validate()'s anonymous errors.New into a typed
// *CompactSemanticStateError carrying the lineage ID, state, and problem, so
// downstream callers can distinguish a semantic-validation failure from a
// structural one (JSON decode, schema mismatch, or checksum error) via
// errors.As instead of string matching.
func TestCompactSemanticStateErrorTyped(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := "compact-semantic-state-error"
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	persistApprovedCompactState(t, repo, state)

	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	corruptCompactStateSemantics(t, store)

	_, loadErr := store.Load()
	if loadErr == nil {
		t.Fatal("semantically invalid compact state was loaded without error")
	}
	var semanticErr *CompactSemanticStateError
	if !errors.As(loadErr, &semanticErr) {
		t.Fatalf("Load() error is not a typed *CompactSemanticStateError: %T: %v", loadErr, loadErr)
	}
	if semanticErr.LineageID != lineage || semanticErr.State != StateInvalidated || strings.TrimSpace(semanticErr.Problem) == "" {
		t.Fatalf("typed semantic error = %#v", semanticErr)
	}
}

// TestCompactLineageQuarantinable proves issue-1813's quarantine predicate:
// compactLineageQuarantinable is true only when the error is a
// *CompactSemanticStateError (never a checksum/IO/parse failure) AND the
// failed state is one of the TERMINAL-for-lineage states {Approved,
// Escalated, Invalidated}.
func TestCompactLineageQuarantinable(t *testing.T) {
	semantic := func(state State) error {
		return &CompactSemanticStateError{LineageID: "lineage", State: state, Problem: "boom"}
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"approved semantic failure quarantinable", semantic(StateApproved), true},
		{"escalated semantic failure quarantinable", semantic(StateEscalated), true},
		{"invalidated semantic failure quarantinable", semantic(StateInvalidated), true},
		{"reviewing semantic failure not quarantinable", semantic(StateReviewing), false},
		{"correction-required semantic failure not quarantinable", semantic(StateCorrectionRequired), false},
		{"validating semantic failure not quarantinable", semantic(StateValidating), false},
		{"nil error not quarantinable", nil, false},
		{"generic error not quarantinable", errors.New("checksum mismatch"), false},
		{"wrapped non-semantic error not quarantinable", fmt.Errorf("read state: %w", os.ErrNotExist), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := compactLineageQuarantinable(tt.err)
			if ok != tt.want {
				t.Fatalf("compactLineageQuarantinable(%v) ok = %t, want %t", tt.err, ok, tt.want)
			}
			if ok && (got == nil || got.LineageID != "lineage") {
				t.Fatalf("compactLineageQuarantinable(%v) typed error = %#v", tt.err, got)
			}
			if !ok && got != nil {
				t.Fatalf("compactLineageQuarantinable(%v) returned a non-nil error for ok=false: %#v", tt.err, got)
			}
		})
	}
}

// TestCompactAuthorityLeavesQuarantine proves issue-1813: one invalid
// TERMINAL lineage among healthy ones is excluded from selector-free
// enumeration alone (CompactAuthorityLeaves, target_status_projection.go's
// selector-free branch) without affecting any other healthy lineage, while
// the explicit-selector path and the quarantined lineage's own Load() still
// fail closed.
func TestCompactAuthorityLeavesQuarantine(t *testing.T) {
	repo := initSnapshotRepo(t)
	healthyA := quarantineFixtureHealthyLineage(t, repo, "quarantine-healthy-a", "healthy candidate a\n")
	quarantined := quarantineFixtureHealthyLineage(t, repo, "quarantine-invalid", "quarantined candidate\n")
	healthyB := quarantineFixtureHealthyLineage(t, repo, "quarantine-healthy-b", "healthy candidate b\n")

	quarantinedStore, err := CompactAuthoritativeStore(context.Background(), repo, quarantined)
	if err != nil {
		t.Fatal(err)
	}
	corruptCompactStateSemantics(t, quarantinedStore)

	t.Run("Load itself still fails closed", func(t *testing.T) {
		_, err := quarantinedStore.Load()
		var semanticErr *CompactSemanticStateError
		if err == nil || !errors.As(err, &semanticErr) {
			t.Fatalf("quarantined lineage direct Load() = %v, want a *CompactSemanticStateError", err)
		}
	})

	t.Run("CompactAuthorityLeaves excludes only the quarantined lineage", func(t *testing.T) {
		leaves, err := CompactAuthorityLeaves(context.Background(), repo)
		if err != nil {
			t.Fatalf("CompactAuthorityLeaves poisoned by one quarantined lineage: %v", err)
		}
		names := map[string]bool{}
		for _, leaf := range leaves {
			names[leaf.lineageID] = true
		}
		if !names[healthyA] || !names[healthyB] {
			t.Fatalf("healthy lineages excluded: %#v", names)
		}
		if names[quarantined] {
			t.Fatalf("quarantined lineage was not excluded: %#v", names)
		}
	})

	t.Run("selector-free status projection excludes only the quarantined lineage", func(t *testing.T) {
		candidates, err := loadCompactTargetStatusCandidates(context.Background(), repo, "")
		if err != nil {
			t.Fatalf("selector-free target status poisoned by one quarantined lineage: %v", err)
		}
		if _, ok := candidates[healthyA]; !ok {
			t.Fatalf("healthy lineage %q excluded from selector-free candidates: %#v", healthyA, candidates)
		}
		if _, ok := candidates[healthyB]; !ok {
			t.Fatalf("healthy lineage %q excluded from selector-free candidates: %#v", healthyB, candidates)
		}
		if _, ok := candidates[quarantined]; ok {
			t.Fatalf("quarantined lineage was not excluded from selector-free candidates: %#v", candidates)
		}
	})

	t.Run("explicit selector on the quarantined lineage still fails closed", func(t *testing.T) {
		_, err := loadCompactTargetStatusCandidates(context.Background(), repo, quarantined)
		var semanticErr *CompactSemanticStateError
		if err == nil || !errors.As(err, &semanticErr) {
			t.Fatalf("explicit selector on quarantined lineage = %v, want a *CompactSemanticStateError", err)
		}
	})

	t.Run("explicit selector on a healthy lineage is unaffected", func(t *testing.T) {
		candidates, err := loadCompactTargetStatusCandidates(context.Background(), repo, healthyA)
		if err != nil {
			t.Fatalf("explicit selector on healthy lineage failed: %v", err)
		}
		if _, ok := candidates[healthyA]; !ok {
			t.Fatalf("explicit selector on healthy lineage missing its own candidate: %#v", candidates)
		}
	})
}

// TestAuthorityInventoryDiagnosticQuarantine proves issue-1813's diagnostic
// surface: a TERMINAL lineage excluded for failing semantic validation
// surfaces a structured AuthorityInventoryDiagnostic naming it, on every
// status query, and — critically — that diagnostic MUST NOT flip
// report.Complete/report.Authoritative for the healthy lineages. Flipping
// them would reproduce 1813 through reviewAuthorityCorruptionConfinedToLegacyEntries
// (review_facade.go), a different door to the same store-wide poisoning bug.
func TestAuthorityInventoryDiagnosticQuarantine(t *testing.T) {
	repo := initSnapshotRepo(t)
	healthy := quarantineFixtureHealthyLineage(t, repo, "inventory-quarantine-healthy", "healthy candidate\n")
	quarantined := quarantineFixtureHealthyLineage(t, repo, "inventory-quarantine-invalid", "quarantined candidate\n")

	store, err := CompactAuthoritativeStore(context.Background(), repo, quarantined)
	if err != nil {
		t.Fatal(err)
	}
	corruptCompactStateSemantics(t, store)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("quarantined terminal lineage flipped report.Complete/Authoritative = %#v", report)
	}
	if !hasAuthorityInventoryStatus(report.Entries, healthy, AuthorityStatusApproved) {
		t.Fatalf("healthy lineage missing from inventory entries: %#v", report.Entries)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == quarantined {
			t.Fatalf("quarantined lineage still enumerated as a healthy entry: %#v", entry)
		}
	}
	found := false
	for _, diagnostic := range report.Diagnostics {
		if strings.Contains(diagnostic.Path, quarantined) && strings.HasPrefix(diagnostic.Problem, "quarantined-terminal-lineage:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no structured quarantine diagnostic named the excluded lineage: %#v", report.Diagnostics)
	}

	// Querying status again must not accumulate a repeated diagnostic (never
	// a repeated stderr warning on every unrelated operation): a fresh
	// inventory call surfaces exactly the same one diagnostic, not two.
	second, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	quarantineDiagnostics := 0
	for _, diagnostic := range second.Diagnostics {
		if strings.Contains(diagnostic.Path, quarantined) {
			quarantineDiagnostics++
		}
	}
	if quarantineDiagnostics != 1 {
		t.Fatalf("repeated inventory calls accumulated diagnostics: %#v", second.Diagnostics)
	}
}

// TestCompactQuarantineNeverAppliesToNonTerminalOrStructuralCorruption proves
// issue-1813's negative boundary (task 7.11): quarantine applies ONLY to a
// TERMINAL-for-lineage state (Approved, Escalated, Invalidated) failing pure
// semantic validation. A genuinely corrupt ACTIVE (non-terminal: Reviewing)
// lineage still fails closed end-to-end through every enumeration path this
// change touched, exactly like before.
func TestCompactQuarantineNeverAppliesToNonTerminalOrStructuralCorruption(t *testing.T) {
	repo := initSnapshotRepo(t)
	healthy := quarantineFixtureHealthyLineage(t, repo, "quarantine-boundary-healthy", "healthy candidate\n")
	corrupted := "quarantine-boundary-reviewing"
	writeSnapshotFile(t, repo, corrupted+".txt", "reviewing candidate\n")
	reviewingState := newCompactStartStateForTarget(t, repo, corrupted, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{corrupted + ".txt"}})
	store, err := CompactAuthoritativeStore(context.Background(), repo, corrupted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", reviewingState); err != nil {
		t.Fatal(err)
	}
	// Corrupt the still-Reviewing state semantically: Reviewing is never a
	// quarantinable state, so this must still fail closed everywhere.
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	corruptedPayload := bytes.Replace(payload, []byte(`"generation": 1,`), []byte(`"generation": 0,`), 1)
	if bytes.Equal(corruptedPayload, payload) {
		t.Fatal("fixture did not contain the expected reviewing-state generation field")
	}
	if err := os.WriteFile(store.StatePath(), corruptedPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(); err == nil {
		t.Fatal("corrupted reviewing-state lineage loaded without error")
	}

	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil {
		t.Fatal("CompactAuthorityLeaves silently tolerated a corrupt non-terminal lineage")
	}

	if _, err := loadCompactTargetStatusCandidates(context.Background(), repo, ""); err == nil {
		t.Fatal("selector-free target status silently tolerated a corrupt non-terminal lineage")
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative {
		t.Fatalf("corrupt non-terminal lineage did not flip report.Complete/Authoritative: %#v", report)
	}
	if !hasAuthorityInventoryStatus(report.Entries, corrupted, AuthorityStatusInvalid) {
		t.Fatalf("corrupt non-terminal lineage missing its Invalid entry: %#v", report.Entries)
	}
	if !hasAuthorityInventoryStatus(report.Entries, healthy, AuthorityStatusApproved) {
		t.Fatalf("healthy lineage still reported inventory-readable alongside the failed-closed corruption: %#v", report.Entries)
	}
}

func quarantineFixtureHealthyLineage(t *testing.T, repo, lineage, content string) string {
	t.Helper()
	writeSnapshotFile(t, repo, lineage+".txt", content)
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{lineage + ".txt"}})
	persistApprovedCompactState(t, repo, state)
	return lineage
}

// corruptCompactStateSemantics rewrites an already-persisted, checksum-valid
// approved compact state on disk into one that fails ONLY
// CompactState.Validate()'s semantic checks: state=invalidated without the
// required invalidation_reason. Because Validate() runs before the checksum
// comparison in parseCompactRecord, this reproduces a pure semantic-class
// failure, never a checksum/IO/parse one.
func corruptCompactStateSemantics(t *testing.T, store CompactStore) {
	t.Helper()
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	corrupted := bytes.Replace(payload, []byte(`"state": "approved",`), []byte(`"state": "invalidated",`), 1)
	if bytes.Equal(corrupted, payload) {
		t.Fatal("fixture did not contain the expected approved state marker")
	}
	if err := os.WriteFile(store.StatePath(), corrupted, 0o644); err != nil {
		t.Fatal(err)
	}
}
