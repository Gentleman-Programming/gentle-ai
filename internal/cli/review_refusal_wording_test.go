package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewStatusTargetSelectorsRequireContractNamesValue pins that the
// refusal for using --contract-gated target selectors without --contract
// names the exact contract value the caller must pass, not only the concept.
func TestReviewStatusTargetSelectorsRequireContractNamesValue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	err := RunReviewStatus([]string{"--cwd", repo, "--lineage", "target-selector-needs-contract"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--contract "+ReviewIntegrationContractV1) {
		t.Fatalf("review status target selector error = %v, want it to name --contract %s", err, ReviewIntegrationContractV1)
	}
}

// TestReviewStartTargetRequiresContractNamesValue pins that the refusal for
// --target without --contract names the exact contract value.
func TestReviewStartTargetRequiresContractNamesValue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--target", "sha256:" + strings.Repeat("a", 64)}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--contract "+ReviewIntegrationContractV1) {
		t.Fatalf("review start --target error = %v, want it to name --contract %s", err, ReviewIntegrationContractV1)
	}
}

// TestReviewRepairRequiresContractNamesValue pins that the refusal for an
// unsupported --contract on review repair names both exact supported values.
func TestReviewRepairRequiresContractNamesValue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	err := RunReviewRepair([]string{"--cwd", repo, "--contract", "gentle-ai.review-integration/v3"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), ReviewIntegrationContractV1) || !strings.Contains(err.Error(), ReviewIntegrationContractV2) {
		t.Fatalf("review repair contract error = %v, want it to name %s and %s", err, ReviewIntegrationContractV1, ReviewIntegrationContractV2)
	}
}

// TestReviewValidateRequiresGateNamesValidGates pins that the refusal for a
// missing --gate on review validate enumerates every valid gate value,
// derived from the same source the validator uses so they cannot diverge.
func TestReviewValidateRequiresGateNamesValidGates(t *testing.T) {
	repo := initReviewCLIRepo(t)
	err := RunReviewFacadeValidate([]string{"--cwd", repo}, io.Discard)
	if err == nil {
		t.Fatalf("review validate without --gate succeeded")
	}
	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		if !strings.Contains(err.Error(), string(gate)) {
			t.Fatalf("review validate --gate error = %v, missing gate %q", err, gate)
		}
	}
}

// TestReviewValidateRefusesPendingAcknowledgement pins that an explicit compact
// lineage is managed but cannot govern delivery before approval acknowledgement.
func TestReviewValidateRefusesPendingAcknowledgement(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := "receipt-not-available-before-closure"
	writeReviewStartCandidate(t, repo, "docs/pending.md", "# pending\n\nplain prose, no executable content.\n", 0o644)
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", "pre-commit"}, &output); err != nil {
		t.Fatalf("delivery with pending acknowledgement: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode pending acknowledgement gate: %v\n%s", err, output.String())
	}
	if result.Allowed || result.Result != reviewtransaction.GateInvalidated || result.Context.LineageID != lineage || result.Delivery != "" {
		t.Fatalf("pending acknowledgement gate = %#v, want managed denial", result)
	}
}

// TestReviewCaptureResultOpaqueBindingMismatchNamesRefreshCommand pins that
// an opaque repository-context capture-binding mismatch names the exact
// runnable command that refreshes the native next transition, derived from
// the same source review_capabilities.go's bootstrap advertisement uses, and
// says who must run it (the parent orchestrator holds --cwd; this opaque
// caller does not).
func TestReviewCaptureResultOpaqueBindingMismatchNamesRefreshCommand(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "opaque-capture-binding-mismatch")
	if started.RepositoryContext == nil || len(started.SelectedLenses) == 0 {
		t.Fatalf("START result = %#v", started)
	}
	err := RunReviewCaptureResult([]string{
		"--cwd", repo,
		"--repository-context", started.RepositoryContext.Handle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lens", "not-the-selected-lens", "--order", "0", "--preflight",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), reviewNextTransitionRefreshCommandV21) {
		t.Fatalf("opaque capture binding mismatch error = %v, want it to contain %q", err, reviewNextTransitionRefreshCommandV21)
	}
}
