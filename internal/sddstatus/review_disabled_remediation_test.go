package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDisabledReviewAdmitsUnmanagedRemediationForAdmittedFailure reproduces
// #2182. With receipt-driven review globally disabled, an admitted failed
// verification could not enter remediation: status refused on "bounded review
// transaction is missing", a transaction that policy itself prevented from
// existing, and left remediationState.required false with no correction route
// at all.
//
// A kill switch that a downstream check overrides is not a kill switch. The
// switch already gates the review authority lookup; it did not gate the
// classification that consumes the absence of what the lookup would have found.
func TestDisabledReviewAdmitsUnmanagedRemediationForAdmittedFailure(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("fail", 1, 0, "1/1", "1/1", 0, 0))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}

	reasons := strings.Join(status.BlockedReasons, "\n")
	if strings.Contains(reasons, "bounded review transaction") {
		t.Fatalf("a disabled switch still demanded review authority: %v", status.BlockedReasons)
	}
	if !status.RemediationState.Required {
		t.Fatalf("admitted failure exposed no correction route under a disabled switch: %#v", status.RemediationState)
	}
	if status.RemediationState.FailedEvidenceRevision == "" {
		t.Fatalf("unmanaged remediation lost the failed evidence it corrects: %#v", status.RemediationState)
	}
	if status.NextRecommended != "remediate" {
		t.Fatalf("nextRecommended = %q, want remediate", status.NextRecommended)
	}
	// The switch removes the review obligation; it must never fabricate the
	// approval that obligation would have produced.
	if status.ReviewGate != nil {
		t.Fatalf("disabled review published a gate result: %#v", status.ReviewGate)
	}
	if state := status.RemediationState; state.LineageID != "" || state.Generation != 0 || state.FixBatch != 0 ||
		state.CorrectionBudgetTotal != 0 || state.CorrectionBudgetRemaining != 0 {
		t.Fatalf("unmanaged remediation invented review-authority provenance: %#v", state)
	}
}

// TestEnabledReviewStillRequiresItsBoundedTransaction is the converse: the gate
// is scoped to the switch being off, and turning it on keeps the original
// refusal exactly.
func TestEnabledReviewStillRequiresItsBoundedTransaction(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("fail", 1, 0, "1/1", "1/1", 0, 0))

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if reasons := strings.Join(status.BlockedReasons, "\n"); !strings.Contains(reasons, "bounded review transaction is missing") {
		t.Fatalf("enabled review stopped requiring its bounded transaction: %v", status.BlockedReasons)
	}
	if status.RemediationState.Required {
		t.Fatalf("enabled review admitted remediation with no transaction: %#v", status.RemediationState)
	}
}

// TestDisabledStatusWithoutAnUnmanagedCorrectionDoesNotBlockAfterReenable is
// the scope control: merely having used disabled status does not make a fresh
// passing verification wait for review authority after the mode changes back.
func TestDisabledStatusWithoutAnUnmanagedCorrectionDoesNotBlockAfterReenable(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, "thin", "- [x] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))

	if _, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin", ReviewDisabled: true}); err != nil {
		t.Fatal(err)
	}
	reenabled, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if reenabled.Dependencies.Archive != DependencyReady || reenabled.NextRecommended != "archive" || reenabled.ReviewGate != nil {
		t.Fatalf("re-enabled status without an unmanaged correction = archive %q next %q gate=%#v", reenabled.Dependencies.Archive, reenabled.NextRecommended, reenabled.ReviewGate)
	}
}
