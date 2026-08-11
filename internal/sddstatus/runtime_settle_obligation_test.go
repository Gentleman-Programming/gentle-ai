package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// An attempt is a bounded, spendable resource. Learning what its settle will
// demand AFTER the work is done costs the operator that attempt, and every
// report in this class paid it: #2912's reporter did 118 lines of honest work,
// was refused at settle for a flag naming evidence they had never seen, and
// closed the attempt as interrupted rather than record a claim they could not
// verify.
//
// The demand itself is correct and stays. What was missing is that acquire —
// the operation that hands out the attempt — knew the whole time and said
// nothing.
//
// The obligation is derived from the same chain predicate the settle uses,
// never re-derived alongside it. That is the #2114 lesson: two derivations of
// one fact drift, and the surface that reports early is exactly the one that
// gets it wrong.

// TestAcquireNamesTheSettleObligationTheChainAlreadyHolds is #2912's scenario,
// stopped one step earlier. The chain holds an unremediated failure, an
// audited reset moved to a distinct objective, and #1974 slice 2 deliberately
// keeps the binding alive across that reset. So the obligation is real, and
// acquire must say so while the attempt is still unspent.
func TestAcquireNamesTheSettleObligationTheChainAlreadyHolds(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "settle-obligation")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "objective A work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "a-begin", WorkUnit: "media verification",
		EvidenceGoal: "verify media pipeline", MaxAttempts: 1, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "media pipeline verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "audited-reset",
		Reason: "media objective abandoned; scoring is a separate work unit", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = reset

	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "b-acquire", WorkUnit: "scoring", EvidenceGoal: "verify scoring",
		MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.State != CompactStateProceed {
		t.Fatalf("acquire = %#v, want state=proceed: the obligation is a notice, never a block", acquired)
	}
	if acquired.SettleObligation == "" {
		t.Fatal("acquire issued a token and said nothing about the remediation the chain will demand at settle; the attempt is spent by the time the operator finds out (#2912)")
	}
	// It has to be actionable, not a warning shape. The operator needs the
	// exact flag and the exact value.
	for _, want := range []string{"--remediates-evidence-revision", failedEvidence} {
		if !strings.Contains(acquired.SettleObligation, want) {
			t.Fatalf("settle_obligation does not name %q:\n%s", want, acquired.SettleObligation)
		}
	}
}

// TestAcquireNamesNoObligationOnACleanChain keeps the notice honest. A chain
// with nothing unremediated must produce no obligation at all: a field that is
// always populated is noise, and noise is how a real notice gets ignored.
func TestAcquireNamesNoObligationOnACleanChain(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "clean-chain")
	store.ReviewDisabled = true
	appendRuntimeLedgerFile(t, repo, "first work\n")

	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "clean-acquire", WorkUnit: "u", EvidenceGoal: "g",
		MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.State != CompactStateProceed {
		t.Fatalf("acquire = %#v, want state=proceed", acquired)
	}
	if acquired.SettleObligation != "" {
		t.Fatalf("acquire invented an obligation on a chain with no unremediated failure:\n%s", acquired.SettleObligation)
	}
}

// TestObligationSurvivesAPassedSettlement pins the boundary the notice is
// derived from: the first passed settlement after a failure discharges it, so
// the next acquire must go quiet. If the notice outlived the obligation it
// would train operators to ignore it.
func TestObligationSurvivesAPassedSettlementOnlyUntilItIsDischarged(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "discharged-obligation")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "first work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "d-begin-1", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 4, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "d-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}

	// While it stands, acquire names it.
	second, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "d-acquire-2", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 4, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if second.SettleObligation == "" {
		t.Fatal("acquire named no obligation while the chain holds an unremediated failure")
	}
	_ = failed

	// Discharge it the way the ledger admits: a correction that names the
	// failure it repairs.
	appendRuntimeLedgerFile(t, repo, "the correction\n")
	if _, err := store.Settle(ctx, CompactSettleRequest{
		Token: second.Token, RequestID: "d-settle-2", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "correction passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "no descendants", RemediatesEvidenceRevision: failedEvidence,
	}); err != nil {
		t.Fatal(err)
	}

	// Discharged: the next acquire is quiet.
	third, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "d-acquire-3", WorkUnit: "follow-up", EvidenceGoal: "follow-up verification",
		MaxAttempts: 4, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if third.State == CompactStateProceed && third.SettleObligation != "" {
		t.Fatalf("the obligation outlived the passed settlement that discharged it:\n%s", third.SettleObligation)
	}
}

// TestStatusAndAcquireReportTheSameObligation is the anti-drift guard, and the
// reason the obligation is derived once rather than computed on each surface.
// #2114 was two derivations of one fact disagreeing; a notice that only acquire
// carries would rebuild that defect on a new field.
func TestStatusAndAcquireReportTheSameObligation(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "obligation-agreement")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "g-begin", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 4, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "g-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "no descendants",
	}); err != nil {
		t.Fatal(err)
	}

	request := BeginAttemptRequest{
		RequestID: "g-acquire", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 4, MaxChangedLines: 400,
	}
	status, err := store.AdmissionStatus(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
	if err != nil {
		t.Fatal(err)
	}
	if status.SettleObligation != acquired.SettleObligation {
		t.Fatalf("status and acquire disagree about the obligation:\nstatus:  %q\nacquire: %q", status.SettleObligation, acquired.SettleObligation)
	}
	if status.SettleObligation == "" {
		t.Fatal("neither surface named the obligation the chain holds")
	}
}
