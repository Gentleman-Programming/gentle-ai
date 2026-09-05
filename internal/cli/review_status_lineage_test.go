package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewStatusLineageInventoryContinuationAndSelectorValidation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var startOutput bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &startOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startOutput.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	// 1. Uncontracted status with existing valid lineage exits 0 and returns scoped inventory
	var out bytes.Buffer
	err := RunReviewStatus([]string{"--cwd", repo, "--lineage", started.LineageID}, &out)
	if err != nil {
		t.Fatalf("RunReviewStatus with valid lineage failed: %v", err)
	}
	var report reviewtransaction.AuthorityStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode status report: %v, raw: %s", err, out.String())
	}
	if len(report.Entries) != 1 || report.Entries[0].LineageID != started.LineageID {
		t.Fatalf("unexpected inventory entries: %#v", report.Entries)
	}
	if report.Status != reviewtransaction.AuthorityStatusActive {
		t.Fatalf("unexpected report status: %v", report.Status)
	}

	// 2. Uncontracted status with nonexistent lineage exits with error naming the lineage
	out.Reset()
	err = RunReviewStatus([]string{"--cwd", repo, "--lineage", "review-0000000000000000"}, &out)
	if err == nil || !strings.Contains(err.Error(), "review-0000000000000000") {
		t.Fatalf("expected error naming nonexistent lineage, got: %v", err)
	}

	// 3. Uncontracted status with other selectors (e.g. --base-ref) requires contract
	out.Reset()
	err = RunReviewStatus([]string{"--cwd", repo, "--base-ref", "HEAD"}, &out)
	if err == nil || !strings.Contains(err.Error(), reviewStatusTargetSelectorsRequireContractReason) {
		t.Fatalf("expected contract required error, got: %v", err)
	}
	if !strings.Contains(reviewStatusTargetSelectorsRequireContractReason, ReviewIntegrationContractV1) ||
		!strings.Contains(reviewStatusTargetSelectorsRequireContractReason, ReviewIntegrationContractV2) {
		t.Fatalf("selector contract reason does not name both v1 and v2: %q", reviewStatusTargetSelectorsRequireContractReason)
	}
}

// TestReviewStatusContractedLineageScopesInventoryInsteadOfLiveTarget is the
// RED-first proof for the residual half of #1997: --lineage was fixed only
// for the uncontracted path above, but the issue's own reproduction used
// --contract explicitly. Without validating occupancy in the contracted
// branch, `review status --contract <v1|v2> --lineage <id>` silently
// resolved the live current target regardless of the named lineage: a
// nonexistent lineage returned that unrelated target's status at exit 0,
// byte-identical to no selector at all. The contracted branch keeps its own
// richer native envelope (unlike the uncontracted path's plain inventory
// report) because plain contracted status must still honor authority
// locking and repair/negotiation semantics real callers depend on — proven
// by naming a real lineage and by a bare query with no selector at all —
// but it must fail closed instead of silently substituting the live target
// when the named lineage does not exist.
func TestReviewStatusContractedLineageScopesInventoryInsteadOfLiveTarget(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var startOutput bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &startOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startOutput.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	// 1. --contract v2 --lineage <real> names that exact lineage in the
	// native negotiated envelope.
	var out bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", started.LineageID}, &out); err != nil {
		t.Fatalf("RunReviewStatus with contract and valid lineage failed: %v", err)
	}
	var native ReviewTargetStatusResult
	if err := json.Unmarshal(out.Bytes(), &native); err != nil {
		t.Fatalf("failed to decode contracted status with valid lineage: %v, raw: %s", err, out.String())
	}
	if native.Authority == nil || native.Authority.LineageID != started.LineageID {
		t.Fatalf("contracted status with valid lineage did not name it: %#v", native.Authority)
	}

	// 2. --contract v2 --lineage <nonexistent> fails closed with a typed
	// refusal naming the lineage; it must not fall back to reporting the
	// live current target at exit 0. RunReview (not RunReviewStatus) is
	// used here because only the CLI dispatcher wraps a returned error into
	// the JSON failure envelope; RunReviewStatus surfaces the bare Go error.
	out.Reset()
	err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", "review-0000000000000000"}, &out)
	if err == nil {
		t.Fatalf("expected nonexistent lineage under --contract to fail closed, got success: %s", out.String())
	}
	failure := decodeReviewIntegrationFailure(t, out.Bytes())
	if !strings.Contains(failure.Cause, "review-0000000000000000") {
		t.Fatalf("contracted failure does not name the nonexistent lineage: %#v", failure)
	}
	if failure.AuthorityApplicability == string(reviewtransaction.TargetApplicabilityCurrent) {
		t.Fatalf("nonexistent lineage under --contract resolved the live current target instead of failing closed: %#v", failure)
	}

	// 3. --contract v2 with no selector is unaffected: it still reports the
	// live current target.
	out.Reset()
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2}, &out); err != nil {
		t.Fatalf("RunReviewStatus with contract and no selector failed: %v", err)
	}
	native = ReviewTargetStatusResult{}
	if err := json.Unmarshal(out.Bytes(), &native); err != nil {
		t.Fatalf("failed to decode unselected contracted status: %v, raw: %s", err, out.String())
	}
	if native.Applicability == "" {
		t.Fatalf("unselected contracted status did not report the live current target: %s", out.String())
	}
}
