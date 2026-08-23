package cli

import (
	"bytes"
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

// TestReviewFinalizeNoDiscoverableLineageNamesStartCommand pins that the
// dead-end reached by running finalize before any review start names the
// exact continuation command instead of only stating the concept, and that
// the wording stays honest for both a lineage that was never started and one
// started under a different --cwd (it never claims nothing was attempted).
func TestReviewFinalizeNoDiscoverableLineageNamesStartCommand(t *testing.T) {
	repo := initReviewCLIRepo(t)
	err := RunReviewFacadeFinalize([]string{"--cwd", repo}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "gentle-ai review start") || !strings.Contains(err.Error(), "--cwd") {
		t.Fatalf("finalize with no discoverable lineage error = %v, want it to name gentle-ai review start and --cwd", err)
	}
}

// TestReviewValidateDoesNotConsultAnActiveLineage pins that delivery remains
// non-deciding even while an active review lifecycle exists.
func TestReviewValidateDoesNotConsultAnActiveLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := "receipt-not-available-needs-finalize"
	writeReviewStartCandidate(t, repo, "docs/pending.md", "# pending\n\nplain prose, no executable content.\n", 0o644)
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", "pre-commit"}, &output); err != nil {
		t.Fatalf("delivery with active lineage: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GatePreCommit)
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
		"--repository-context", started.RepositoryContext.Handle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lens", "not-the-selected-lens", "--order", "0", "--preflight",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), reviewNextTransitionRefreshCommandV21) {
		t.Fatalf("opaque capture binding mismatch error = %v, want it to contain %q", err, reviewNextTransitionRefreshCommandV21)
	}
}
