//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// #2564 status truthfulness, reconciled with #2565's chain-derived binding:
// the bounded correction prescription renders whenever the immutable attempt
// chain holds unremediated failed evidence (naming the chain's evidence, and
// surviving an audited reset), the decision-required state renders the
// audited reset route, and only a chain with nothing left to remediate
// renders the fresh-verification route.

// seedUnmanagedFailedVerification drives the shared fixture: the disabled
// failed-verification ledger plus the admitted failed verify report that
// makes RemediationState.Required true.
func seedUnmanagedFailedVerification(t *testing.T, repo, change string, maxAttempts int) (RuntimeStore, string, RuntimeStatus) {
	t.Helper()
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
	store, failedEvidence, failed := seedFailedVerificationLedger(t, repo, change, maxAttempts)
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(failedEvidence, "fail"))
	return store, failedEvidence, failed
}

func resolveDisabledRemediationInstructions(t *testing.T, repo, change string) (Status, string) {
	t.Helper()
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: change, ReviewDisabled: true, IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.RemediationState.Required || status.RemediationState.LineageID != "" {
		t.Fatalf("fixture did not require unmanaged remediation: %#v", status.RemediationState)
	}
	if status.PhaseInstructions == nil {
		t.Fatal("PhaseInstructions is nil")
	}
	return status, strings.Join(status.PhaseInstructions.Remediate, "\n")
}

// TestStatusKeepsPrescribingChainBoundRemediationAfterReset pins the slice 1 /
// slice 2 reconciliation on the status side: after an audited reset and a
// fresh correction objective, the chain still binds the failed evidence, so
// the bounded correction prescription keeps rendering with the chain's own
// revision instead of degrading to a decision route.
func TestStatusKeepsPrescribingChainBoundRemediationAfterReset(t *testing.T) {
	const change = "post-reset-advice"
	repo := initRuntimeLedgerRepo(t)
	store, failedEvidence, failed := seedUnmanagedFailedVerification(t, repo, change, 1)
	if !failed.DecisionRequired {
		t.Fatalf("exhausted failed verification did not require a decision: %#v", failed)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: change + "-reset",
		Reason: "maintainer authorized bounded remediation", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: change + "-begin-correction", WorkUnit: "remediation",
		EvidenceGoal: "bounded correction", MaxAttempts: 1, MaxChangedLines: 200,
	}); err != nil {
		t.Fatal(err)
	}

	_, joined := resolveDisabledRemediationInstructions(t, repo, change)
	if !strings.Contains(joined, "--remediates-evidence-revision "+failedEvidence) {
		t.Fatalf("post-reset status lost the chain-bound correction prescription:\n%s", joined)
	}
	if !strings.Contains(joined, "Disabled/unmanaged remediation has one bounded correction attempt") {
		t.Fatalf("post-reset status lost the bounded correction framing:\n%s", joined)
	}
	if strings.Contains(joined, "cannot settle") {
		t.Fatalf("post-reset status degraded a satisfiable correction to the decision route:\n%s", joined)
	}
}

// TestStatusRendersResetRouteWhileRemediationNeedsADecision covers the wedge
// every #1974 occurrence reported: failed evidence with an exhausted budget is
// decision-required, so an immediate correction acquire would return
// blocked/maintainer_decision. Status renders the audited reset route with
// the exact current revision, and names the chain binding the post-reset
// acquire must declare.
func TestStatusRendersResetRouteWhileRemediationNeedsADecision(t *testing.T) {
	const change = "decision-required-advice"
	repo := initRuntimeLedgerRepo(t)
	_, failedEvidence, failed := seedUnmanagedFailedVerification(t, repo, change, 1)
	if !failed.DecisionRequired {
		t.Fatalf("exhausted failed verification did not require a decision: %#v", failed)
	}

	_, joined := resolveDisabledRemediationInstructions(t, repo, change)
	if strings.Contains(joined, "bounded correction attempt: run") {
		t.Fatalf("decision-required status still prescribes the blocked correction acquire:\n%s", joined)
	}
	if !strings.Contains(joined, "sdd-attempt reset") || !strings.Contains(joined, failed.Revision) {
		t.Fatalf("decision-required status does not render the audited reset route:\n%s", joined)
	}
	if !strings.Contains(joined, "--remediates-evidence-revision "+failedEvidence) {
		t.Fatalf("decision-required status does not name the chain binding for the post-reset acquire:\n%s", joined)
	}
}

// TestStatusStillPrescribesSatisfiableUnmanagedRemediation is the regression
// guard for the direct case: budget remains and the chain binds the failure,
// so the bounded correction prescription renders exactly as before, now
// declaring the remediation intent at acquire too.
func TestStatusStillPrescribesSatisfiableUnmanagedRemediation(t *testing.T) {
	const change = "satisfiable-advice"
	repo := initRuntimeLedgerRepo(t)
	_, failedEvidence, failed := seedUnmanagedFailedVerification(t, repo, change, 2)
	if failed.DecisionRequired || failed.EvidenceRevision != failedEvidence {
		t.Fatalf("fixture lost the live failed-evidence binding: %#v", failed)
	}

	_, joined := resolveDisabledRemediationInstructions(t, repo, change)
	if !strings.Contains(joined, "Disabled/unmanaged remediation has one bounded correction attempt") {
		t.Fatalf("satisfiable status lost the bounded correction prescription:\n%s", joined)
	}
	if !strings.Contains(joined, "--remediates-evidence-revision "+failedEvidence) {
		t.Fatalf("satisfiable status does not bind the settle to the failed evidence:\n%s", joined)
	}
	if !strings.Contains(joined, "--max-changed-lines 20 --remediates-evidence-revision "+failedEvidence) {
		t.Fatalf("satisfiable acquire prescription does not declare the remediation intent:\n%s", joined)
	}
}

// TestStatusRendersFreshVerificationRouteWhenNothingRemediable is the one
// state that truly cannot settle: the chain holds no failed evidence (here an
// interrupted-only chain), so prescribing `--remediates-evidence-revision`
// would demand a settlement Finish can never admit. Status renders the
// fresh-verification route instead.
func TestStatusRendersFreshVerificationRouteWhenNothingRemediable(t *testing.T) {
	const change = "nothing-remediable-advice"
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, change)
	store.ReviewDisabled = true
	begun, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: change + "-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: begun.Revision, RequestID: change + "-finish-interrupted", Outcome: AttemptInterrupted,
		Diagnosis:          "transport ended before verification settled",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "stalled process group cleanup completed",
		ProcessEvidence: "stalled process scan found no surviving descendants",
	}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(failedEvidence, "fail"))

	_, joined := resolveDisabledRemediationInstructions(t, repo, change)
	if strings.Contains(joined, "--remediates-evidence-revision "+failedEvidence) {
		t.Fatalf("nothing-remediable status still prescribes an unsatisfiable binding:\n%s", joined)
	}
	if !strings.Contains(joined, "cannot settle") || !strings.Contains(joined, "fresh verification objective") {
		t.Fatalf("nothing-remediable status does not render the fresh-verification route:\n%s", joined)
	}
}
