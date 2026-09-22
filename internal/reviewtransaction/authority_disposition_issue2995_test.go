package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Issue #2995 work-unit 2: selector-scoped historical disposition exits.
// A store with MORE than one historical (outdated) authority entry has no
// sanctioned exit today: the selectorless historical plan hard-gates to a
// single diagnostic and the content-mismatch edge path never sees these
// records. These tests pin the unit-2 contract: an EXACT successor-only
// selector mints the historical disposition plan for exactly the entry it
// names — even with sibling diagnostics present — while selectorless
// derivation (the j92 posture) keeps refusing every multi-diagnostic store.

const (
	// Raw-byte pins copied from internal/reviewtransaction/testdata/issue2995/
	// PROVENANCE.md. Each test re-hashes the fixture against its pin, so a
	// drifted fixture fails here instead of proving nothing.
	issue2995ApprovedFixtureName  = "released-v2.2.0-approved-review-state.json"
	issue2995EscalatedFixtureName = "released-v2.2.0-escalated-review-state.json"
	issue2995ApprovedDigest       = "sha256:bdc0db81f30867ba11eb6179f68d8a76651d77c4b0af668dabd855c23961ccbb"
	issue2995EscalatedDigest      = "sha256:3d9a686b5a9f43abcb8b474e44986f8b72e57631cf851619096eefaeba854b12"
)

// issue2995StageFixtureStore copies the named released v2.2.0 fixtures
// byte-identical into a temp repository's compact-v2 authority store, exactly
// the way a v2.2.x binary left them on disk, and returns the repository plus
// the bytes per lineage for byte-preservation assertions.
func issue2995StageFixtureStore(t *testing.T, names ...string) (string, map[string][]byte) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string][]byte{}
	for _, name := range names {
		lineage := issue2995ApprovedLineage
		if name == issue2995EscalatedFixtureName {
			lineage = issue2995EscalatedLineage
		} else if name != issue2995ApprovedFixtureName {
			t.Fatalf("unknown issue2995 fixture %q", name)
		}
		payload := issue2995FixtureBytes(t, name)
		sum := sha256.Sum256(payload)
		want := issue2995ApprovedDigest
		if name == issue2995EscalatedFixtureName {
			want = issue2995EscalatedDigest
		}
		if digest := "sha256:" + hex.EncodeToString(sum[:]); digest != want {
			t.Fatalf("fixture %s digest drifted: %s, want pin %s", name, digest, want)
		}
		dir := filepath.Join(base, "v2", lineage)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "review-state.json"), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		fixtures[lineage] = payload
	}
	return repo, fixtures
}

func TestIssue2995HistoricalSelectorMintsPlanForExactlyOneOfTwoHistoricalEntries(t *testing.T) {
	repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
	ctx := context.Background()

	report, records, err := loadCompactRecoveryRecords(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.historical) != 2 || len(report.EntryDiagnostics) != 2 {
		t.Fatalf("fixture store inspection = %d historical / %d diagnostics, want 2/2", len(report.historical), len(report.EntryDiagnostics))
	}
	for _, diagnostic := range report.EntryDiagnostics {
		if diagnostic.Problem != compactInspectionEntryOutdated {
			t.Fatalf("fixture entry %s classified %q, want %q", diagnostic.LineageID, diagnostic.Problem, compactInspectionEntryOutdated)
		}
	}
	if len(report.Edges) != 0 {
		t.Fatalf("historical fixture store carries %d edges, want 0 (historical records are forensic bytes, not graph nodes)", len(report.Edges))
	}

	// The selectorless gate is unchanged: a two-historical store refuses.
	if _, err := deriveAuthorityDispositionPlan(report, records, "binding", "", ""); !errors.Is(err, errAuthorityDispositionPlanNotDerivable) {
		t.Fatalf("selectorless derivation on a two-historical store = %v, want errAuthorityDispositionPlanNotDerivable", err)
	}

	selectors, err := ListAuthorityDispositionSelectorsAtRepo(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 2 {
		t.Fatalf("enumerated %d selectors, want one per historical entry", len(selectors))
	}
	if selectors[0].SuccessorLineageID != issue2995EscalatedLineage || selectors[1].SuccessorLineageID != issue2995ApprovedLineage {
		t.Fatalf("selectors not sorted by lineage: %q then %q", selectors[0].SuccessorLineageID, selectors[1].SuccessorLineageID)
	}
	for _, selector := range selectors {
		if selector.PredecessorLineageID != "" || selector.PredecessorExpectedRevision != "" {
			t.Fatalf("historical selector for %q carries predecessor identity %+v; historical entries name no predecessor revision", selector.SuccessorLineageID, selector)
		}
	}
	if selectors[0].SuccessorExpectedRevision != issue2995EscalatedDigest || selectors[1].SuccessorExpectedRevision != issue2995ApprovedDigest {
		t.Fatalf("selector expected revisions drifted from the raw-byte pins: %+v", selectors)
	}

	seenDigests := map[string]bool{}
	for _, selector := range selectors {
		plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "maintainer@example.com", "quarantine released historical entry", selector)
		if err != nil {
			t.Fatalf("selector-scoped derivation refused %q: %v", selector.SuccessorLineageID, err)
		}
		if plan.AnomalyClass != compactHistoricalSnapshotIdentityClass {
			t.Fatalf("plan class = %q, want %q", plan.AnomalyClass, compactHistoricalSnapshotIdentityClass)
		}
		if !slices.Equal(plan.SeedSet, []string{selector.SuccessorLineageID}) || !slices.Equal(plan.Closure, []string{selector.SuccessorLineageID}) {
			t.Fatalf("plan scopes %v / %v, want exactly the selected entry", plan.SeedSet, plan.Closure)
		}
		if plan.ExpectedRevisions[selector.SuccessorLineageID] != selector.SuccessorExpectedRevision {
			t.Fatalf("plan expected revision %q does not bind the entry's raw-byte digest", plan.ExpectedRevisions[selector.SuccessorLineageID])
		}
		if plan.Selector == nil || *plan.Selector != selector {
			t.Fatalf("plan does not bind the requested selector: %+v vs %+v", plan.Selector, selector)
		}
		if err := admitClosureDisposition(plan); err != nil {
			t.Fatalf("historical plan refused by its own executor admission: %v", err)
		}
		// The inventory revision binds the WHOLE store's historical content:
		// both entries contribute, so either entry's plan carries the same
		// whole-store revision.
		inventory, err := authorityInventoryRevision(records, report.historical)
		if err != nil {
			t.Fatal(err)
		}
		if plan.AuthorityInventoryRevision != inventory {
			t.Fatalf("plan inventory revision %q does not bind all loadable + historical content (%q)", plan.AuthorityInventoryRevision, inventory)
		}
		if seenDigests[plan.PlanDigest] {
			t.Fatalf("two different entries derived the same plan digest %q", plan.PlanDigest)
		}
		seenDigests[plan.PlanDigest] = true
		again, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "", selector)
		if err != nil {
			t.Fatal(err)
		}
		if again.PlanDigest != plan.PlanDigest {
			t.Fatalf("actor/reason leaked into the historical plan digest: %q vs %q", again.PlanDigest, plan.PlanDigest)
		}
	}
}

func TestIssue2995HistoricalSelectorRefusesMalformedMissingAndStale(t *testing.T) {
	repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
	ctx := context.Background()
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	malformedDir := filepath.Join(base, "v2", "zz-malformed-authority")
	if err := os.MkdirAll(malformedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("malformed entry refuses by name", func(t *testing.T) {
		selector := AuthorityDispositionSelector{SuccessorLineageID: "zz-malformed-authority", SuccessorExpectedRevision: issue2995ApprovedDigest}
		_, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "", selector)
		if err == nil || !errors.Is(err, errAuthorityDispositionPlanNotDerivable) || !strings.Contains(err.Error(), "zz-malformed-authority") || !strings.Contains(err.Error(), compactInspectionEntryMalformed) {
			t.Fatalf("malformed-entry selector error = %v, want a named refusal carrying the lineage and the malformed diagnosis", err)
		}
	})
	t.Run("missing lineage refuses by name", func(t *testing.T) {
		selector := AuthorityDispositionSelector{SuccessorLineageID: "review-missingentry000", SuccessorExpectedRevision: issue2995ApprovedDigest}
		_, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "", selector)
		if err == nil || !errors.Is(err, errAuthorityDispositionPlanNotDerivable) || !strings.Contains(err.Error(), "review-missingentry000") {
			t.Fatalf("missing-lineage selector error = %v, want a named refusal carrying the lineage", err)
		}
	})
	t.Run("stale expected revision refuses as drift", func(t *testing.T) {
		selector := AuthorityDispositionSelector{SuccessorLineageID: issue2995ApprovedLineage, SuccessorExpectedRevision: issue2995EscalatedDigest}
		_, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "", selector)
		if !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale-revision selector error = %v, want ErrConcurrentUpdate", err)
		}
	})
	t.Run("loadable entry refuses", func(t *testing.T) {
		// The malformed store refuses for zz-malformed; prove a HEALTHY loaded
		// record refuses too by staging one live record next to the fixtures.
		liveState := newCompactTestState(t, repo, "review-liveauthority0000")
		liveStore, err := CompactAuthoritativeStore(ctx, repo, liveState.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		writeCompactFixtureRecord(t, liveStore, liveState)
		selector := AuthorityDispositionSelector{SuccessorLineageID: liveState.LineageID, SuccessorExpectedRevision: issue2995ApprovedDigest}
		_, err = deriveAuthorityDispositionPlanAtRepo(ctx, repo, "", "", selector)
		if err == nil || !errors.Is(err, errAuthorityDispositionPlanNotDerivable) || !strings.Contains(err.Error(), liveState.LineageID) {
			t.Fatalf("loadable-entry selector error = %v, want a named refusal carrying the lineage", err)
		}
	})
}

func TestIssue2995SingleHistoricalStoreSelectorlessUnchanged(t *testing.T) {
	repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName)
	ctx := context.Background()

	// j92's exact posture: one historical entry, selectorless derivation
	// still mints the same plan, with no selector bound.
	plan := derivePlanFixture(t, repo, "", "")
	if plan.AnomalyClass != compactHistoricalSnapshotIdentityClass {
		t.Fatalf("plan class = %q, want %q", plan.AnomalyClass, compactHistoricalSnapshotIdentityClass)
	}
	if !slices.Equal(plan.Closure, []string{issue2995ApprovedLineage}) || plan.Selector != nil {
		t.Fatalf("selectorless historical plan changed shape: closure=%v selector=%v", plan.Closure, plan.Selector)
	}

	selectors, err := ListAuthorityDispositionSelectorsAtRepo(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 1 || selectors[0].SuccessorLineageID != issue2995ApprovedLineage {
		t.Fatalf("single-historical enumeration = %+v, want exactly the one entry", selectors)
	}
}

func TestIssue2995HistoricalSanctionedExitsArePerEntryAndConservative(t *testing.T) {
	ctx := context.Background()

	t.Run("all-historical N=2 exposes one exit per entry", func(t *testing.T) {
		repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		if err != nil {
			t.Fatal(err)
		}
		if len(exits) != 2 {
			t.Fatalf("sanctioned exits = %+v, want one per historical entry", exits)
		}
		if exits[0].SuccessorLineageID != issue2995EscalatedLineage || exits[1].SuccessorLineageID != issue2995ApprovedLineage {
			t.Fatalf("exits not one-per-entry in lineage order: %+v", exits)
		}
		for _, exit := range exits {
			if exit.Operation != CompactRecoveryEdgeExitRepair {
				t.Fatalf("historical exit operation = %q, want %q", exit.Operation, CompactRecoveryEdgeExitRepair)
			}
		}
	})
	t.Run("mixed store keeps historical entries exitless", func(t *testing.T) {
		repo, _ := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
		base, _, err := reviewAuthorityRoot(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		malformedDir := filepath.Join(base, "v2", "zz-malformed-authority")
		if err := os.MkdirAll(malformedDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(malformedDir, "review-state.json"), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		if err != nil {
			t.Fatal(err)
		}
		if len(exits) != 0 {
			t.Fatalf("mixed store sanctioned exits = %+v, want none (the j92 posture must not grow historical exits next to malformed damage)", exits)
		}
	})
	t.Run("malformed-only store still has no exit", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		base, _, err := reviewAuthorityRoot(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		malformedDir := filepath.Join(base, "v2", "zz-malformed-authority")
		if err := os.MkdirAll(malformedDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(malformedDir, "review-state.json"), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := InspectCompactRecoveryEdges(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		exits, err := SanctionedCompactRecoveryExits(ctx, repo, report)
		if err != nil {
			t.Fatal(err)
		}
		if len(exits) != 0 {
			t.Fatalf("malformed entry gained an exit: %+v", exits)
		}
	})
}

func TestIssue2995SelectorScopedHistoricalExecutionQuarantinesExactlySelectedEntry(t *testing.T) {
	repo, fixtures := issue2995StageFixtureStore(t, issue2995ApprovedFixtureName, issue2995EscalatedFixtureName)
	ctx := context.Background()
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	selectors, err := ListAuthorityDispositionSelectorsAtRepo(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 2 {
		t.Fatalf("selectors = %+v, want the two historical entries", selectors)
	}
	selected, sibling := selectors[0], selectors[1]

	plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, "maintainer@example.com", "quarantine released historical entry", selected)
	if err != nil {
		t.Fatal(err)
	}
	plan.Authorization = authorityDispositionAuthorizationBinding(plan)

	record, err := executeAuthorityDisposition(ctx, repo, plan)
	if err != nil {
		t.Fatalf("selector-scoped historical execution refused: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.LineageID != selected.SuccessorLineageID {
		t.Fatalf("execution record = %s/%s, want committed for the selected entry", record.Status, record.LineageID)
	}

	// The selected entry is gone from v2/; the sibling stays byte-identical.
	if _, err := os.Stat(filepath.Join(base, "v2", selected.SuccessorLineageID)); !os.IsNotExist(err) {
		t.Fatalf("selected entry still active after execution: %v", err)
	}
	siblingPayload, err := os.ReadFile(filepath.Join(base, "v2", sibling.SuccessorLineageID, "review-state.json"))
	if err != nil || !bytes.Equal(siblingPayload, fixtures[sibling.SuccessorLineageID]) {
		t.Fatalf("sibling historical entry changed: %v", err)
	}

	// The quarantine preserves the selected entry's exact bytes and nothing
	// else was quarantined.
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("quarantine entries = %v (%v), want exactly the selected entry", entries, err)
	}
	residue, err := os.ReadFile(filepath.Join(base, "quarantine", entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(residue, fixtures[selected.SuccessorLineageID]) {
		t.Fatalf("quarantine did not preserve the selected entry's bytes: %v", err)
	}

	// Replay under the same plan commits once: discovery returns the existing
	// record and no second quarantine entry appears.
	replay, err := executeAuthorityDisposition(ctx, repo, plan)
	if err != nil {
		t.Fatalf("replay refused: %v", err)
	}
	if replay.LineageID != record.LineageID {
		t.Fatalf("replay record = %s, want the original committed record", replay.LineageID)
	}
	entries, err = os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("replay changed the quarantine inventory: %v (%v)", entries, err)
	}
}
