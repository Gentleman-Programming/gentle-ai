package cli

import (
	"context"
	"io"
	"path/filepath"
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

// TestReviewValidateCompactReceiptRequiresNativeAuthorityFlagsNamesThem pins
// that combining a compact receipt with --request names the concrete native
// authority flags (--lineage and --gate) the caller must use instead.
func TestReviewValidateCompactReceiptRequiresNativeAuthorityFlagsNamesThem(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "compact-receipt-needs-native-flags"
	approveTrackedGoChangeWithWarningFinding(t, repo, lineage)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	err = RunReviewValidate([]string{
		"--cwd", repo, "--receipt", store.ReceiptPath(), "--request", filepath.Join(t.TempDir(), "request.json"),
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--lineage") || !strings.Contains(err.Error(), "--gate") {
		t.Fatalf("compact receipt + --request error = %v, want it to name --lineage and --gate", err)
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

// TestReviewValidateReceiptNotAvailableNamesFinalizeCommandWithLineage pins
// that reaching a gate before the discovered lineage was ever finalized
// names the exact continuation command and the concrete lineage ID, not only
// the concept of finalizing.
func TestReviewValidateReceiptNotAvailableNamesFinalizeCommandWithLineage(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "receipt-not-available-needs-finalize"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", "pre-commit"}, io.Discard)
	want := "gentle-ai review finalize --lineage " + lineage
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("validate before finalize error = %v, want it to contain %q", err, want)
	}
}

// TestReviewCaptureResultOpaqueBindingMismatchNamesRefreshCommand pins that
// an opaque repository-context capture-binding mismatch names the exact
// runnable command that refreshes the native next transition, derived from
// the same source review_capabilities.go's bootstrap advertisement uses, and
// says who must run it (the parent orchestrator holds --cwd; this opaque
// caller does not).
func TestReviewCaptureResultOpaqueBindingMismatchNamesRefreshCommand(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), reviewNextTransitionRefreshCommandV2) {
		t.Fatalf("opaque capture binding mismatch error = %v, want it to contain %q", err, reviewNextTransitionRefreshCommandV2)
	}
}
