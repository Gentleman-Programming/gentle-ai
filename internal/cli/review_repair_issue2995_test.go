package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// Issue #2995 work-unit 2 CLI contract: a store with more than one
// historical (outdated) authority entry gets its per-entry repair exits at
// the `review repair` boundary. Selectorless --preflight surfaces one exact
// successor-only selector per historical entry (and still mints no plan);
// a selector naming one historical entry mints exactly that entry's plan,
// even with sibling diagnostics present.

// stageIssue2995HistoricalCLIStore builds one repository whose compact-v2
// authority store carries exactly two historical (outdated) entries, through
// the product's own START path retired in place — the same shape
// TestReviewRepairPreflightBlocksHistoricalPlanForAdditionalAuthorityDiagnostic
// builds with one.
func stageIssue2995HistoricalCLIStore(t *testing.T) (string, []string) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "alpha.txt", "alpha\n", 0o644)
	first := startFacadeReview(t, repo)
	writeReviewStartCandidate(t, repo, "beta.txt", "beta\n", 0o644)
	second := startFacadeReviewResult(t, repo, "issue2995-beta-entry")
	lineages := []string{}
	for _, started := range []ReviewFacadeStartResult{first, second} {
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		retireCompactAuthorityForReviewRepairTest(t, store, record)
		lineages = append(lineages, started.LineageID)
	}
	sort.Strings(lineages)
	return repo, lineages
}

func TestIssue2995ReviewRepairPreflightSurfacesHistoricalSelectorsAndScopedPlans(t *testing.T) {
	repo, lineages := stageIssue2995HistoricalCLIStore(t)

	var output bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, output.Bytes(), &preflight)
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	if preflight.DispositionProviderInputs != nil || len(preflight.DispositionSelectors) != 2 ||
		preflight.Assessment.Status != reviewtransaction.AuthorityRepairUnsupported {
		t.Fatalf("selectorless two-historical preflight = %#v\n%s", preflight, output.String())
	}
	revisions := map[string]string{}
	for index, selector := range preflight.DispositionSelectors {
		if selector.PredecessorLineageID != "" || selector.PredecessorExpectedRevision != "" {
			t.Fatalf("historical selector carries predecessor identity: %#v", selector)
		}
		if selector.SuccessorLineageID != lineages[index] {
			t.Fatalf("selector %d names %q, want sorted entry %q", index, selector.SuccessorLineageID, lineages[index])
		}
		if !validReviewCapabilitySHA256(selector.SuccessorExpectedRevision) {
			t.Fatalf("selector for %q carries no raw-byte revision: %#v", selector.SuccessorLineageID, selector)
		}
		revisions[selector.SuccessorLineageID] = selector.SuccessorExpectedRevision
	}

	// Each surfaced selector re-runs preflight into exactly that entry's plan,
	// with no lineage identity leaked and no selector re-surfaced.
	digests := map[string]string{}
	for _, lineage := range lineages {
		var scopedOutput bytes.Buffer
		args := []string{"repair", "--preflight", "--cwd", repo, "--successor-lineage", lineage, "--successor-revision", revisions[lineage]}
		if err := RunReview(args, &scopedOutput); err != nil {
			t.Fatalf("selector-scoped preflight refused %q: %v\n%s", lineage, err, scopedOutput.String())
		}
		var scoped ReviewRepairResult
		decodeStrictReviewJSON(t, scopedOutput.Bytes(), &scoped)
		if err := scoped.Validate(); err != nil {
			t.Fatal(err)
		}
		if scoped.DispositionProviderInputs == nil || len(scoped.DispositionSelectors) != 0 ||
			!validReviewCapabilitySHA256(scoped.DispositionProviderInputs.PlanDigest) {
			t.Fatalf("scoped preflight for %q = %#v\n%s", lineage, scoped, scopedOutput.String())
		}
		if strings.Contains(scopedOutput.String(), repo) || strings.Contains(scopedOutput.String(), lineage) {
			t.Fatalf("scoped preflight leaked path or lineage identity: %s", scopedOutput.String())
		}
		digests[lineage] = scoped.DispositionProviderInputs.PlanDigest
	}
	if digests[lineages[0]] == digests[lineages[1]] {
		t.Fatal("two different historical entries derived the same plan digest")
	}
}

func TestIssue2995ReviewRepairHistoricalSelectorRefusals(t *testing.T) {
	repo, lineages := stageIssue2995HistoricalCLIStore(t)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	malformedDir := filepath.Join(authorityRoot, "v2", "zz-malformed-authority")
	if err := os.MkdirAll(malformedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := "sha256:" + strings.Repeat("0", 64)
	for _, test := range []struct {
		name    string
		lineage string
		wantIn  string
	}{
		{name: "malformed entry", lineage: "zz-malformed-authority", wantIn: "zz-malformed-authority"},
		{name: "missing lineage", lineage: "review-missingentry000", wantIn: "review-missingentry000"},
		{name: "stale expected revision", lineage: lineages[0], wantIn: "no longer matches"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			args := []string{"repair", "--preflight", "--cwd", repo, "--successor-lineage", test.lineage, "--successor-revision", stale}
			if err := RunReview(args, &output); err == nil || !strings.Contains(err.Error(), test.wantIn) {
				t.Fatalf("selector for %q error = %v, want a named refusal carrying %q\n%s", test.lineage, err, test.wantIn, output.String())
			}
		})
	}
	t.Run("detached successor revision refuses at the flag boundary", func(t *testing.T) {
		var output bytes.Buffer
		args := []string{"repair", "--preflight", "--cwd", repo, "--successor-lineage", lineages[0]}
		err := RunReview(args, &output)
		if err == nil || !strings.Contains(err.Error(), "--successor-revision") {
			t.Fatalf("successor-only selector without a revision error = %v, want a named flag refusal", err)
		}
	})
	t.Run("partial predecessor selector still demands all four flags", func(t *testing.T) {
		var output bytes.Buffer
		args := []string{"repair", "--preflight", "--cwd", repo, "--predecessor-lineage", "some-predecessor"}
		err := RunReview(args, &output)
		if err == nil || !strings.Contains(err.Error(), "--predecessor-lineage") {
			t.Fatalf("partial predecessor selector error = %v, want the exact-selector flag refusal", err)
		}
	})
}

func TestIssue2995ReviewRepairQuarantinesExactlySelectedHistoricalEntry(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineages := stageIssue2995HistoricalCLIStore(t)
	selected, sibling := lineages[0], lineages[1]
	authorityRoot := reviewCLIAuthorityRoot(t, repo)

	selectedPath := filepath.Join(authorityRoot, "v2", selected, "review-state.json")
	selectedBefore, err := os.ReadFile(selectedPath)
	if err != nil {
		t.Fatal(err)
	}
	siblingPath := filepath.Join(authorityRoot, "v2", sibling, "review-state.json")
	siblingBefore, err := os.ReadFile(siblingPath)
	if err != nil {
		t.Fatal(err)
	}

	// The surfaced selector names the selected entry's raw-byte revision.
	var output bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, output.Bytes(), &preflight)
	revision := ""
	for _, selector := range preflight.DispositionSelectors {
		if selector.SuccessorLineageID == selected {
			revision = selector.SuccessorExpectedRevision
		}
	}
	if revision == "" {
		t.Fatalf("preflight surfaced no selector for %q: %#v", selected, preflight.DispositionSelectors)
	}

	actor, reason := "maintainer@example.com", "quarantine released historical entry"
	selector := reviewtransaction.AuthorityDispositionSelector{SuccessorLineageID: selected, SuccessorExpectedRevision: revision}
	plan, err := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(context.Background(), repo, actor, reason, selector)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SeedSet[0] != selected {
		t.Fatalf("plan scoped %v, want the selected entry", plan.SeedSet)
	}
	authorization := authorityDispositionAuthorization(plan)
	args := []string{
		"repair", "--cwd", repo,
		"--plan-digest", plan.PlanDigest, "--inventory-revision", plan.AuthorityInventoryRevision,
		"--actor", actor, "--reason", reason, "--authorization", authorization,
		"--successor-lineage", selected, "--successor-revision", revision,
	}
	for attempt := 0; attempt < 2; attempt++ {
		var executed bytes.Buffer
		if err := RunReview(args, &executed); err != nil {
			t.Fatalf("historical disposition execution %d refused: %v\n%s", attempt, err, executed.String())
		}
		var result ReviewRepairResult
		decodeStrictReviewJSON(t, executed.Bytes(), &result)
		if err := result.Validate(); err != nil {
			t.Fatal(err)
		}
		if result.DispositionExecution == nil || result.DispositionExecution.Status != string(reviewtransaction.CompactReclaimCommitted) ||
			result.DispositionExecution.LineageID != selected {
			t.Fatalf("historical disposition execution %d = %#v\n%s", attempt, result, executed.String())
		}
	}

	if _, err := os.Stat(selectedPath); !os.IsNotExist(err) {
		t.Fatalf("selected entry still active after execution: %v", err)
	}
	siblingAfter, err := os.ReadFile(siblingPath)
	if err != nil || !bytes.Equal(siblingAfter, siblingBefore) {
		t.Fatalf("sibling historical entry changed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(authorityRoot, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("quarantine inventory = %v (%v), want exactly one entry after replay", entries, err)
	}
	residue, err := os.ReadFile(filepath.Join(authorityRoot, "quarantine", entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(residue, selectedBefore) {
		t.Fatalf("quarantine did not preserve the selected entry's bytes: %v", err)
	}
}
