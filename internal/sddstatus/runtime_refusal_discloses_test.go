package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// Three refusals in the settle path compare two values the operator cannot
// see, then report only that the comparison failed. Each one produced a report
// where the reporter did everything the tooling told them to and still could
// not move:
//
//   #2881 "unmanaged remediation requires the current failed evidence and a
//         direct correction attempt" — the operator DID pass the current failed
//         evidence. What they could not know is that their own previous
//         correction had already discharged it, so the chain no longer holds it
//         unremediated and this work unit owes nothing.
//   #2088 "approved SDD remediation successor does not bind the natively
//         charged candidate" — six configurations tried, all refused, and the
//         message never names which trees were compared. The reporter asked
//         literally for "which tree/identity/manifest relationship is checked".
//   #2894 "bound compact predecessor is stale or not approved" — two conditions
//         in one sentence, and it says neither which one failed nor against
//         what.
//
// A refusal that withholds the comparison it made is not a refusal, it is a
// dead end with a sentence attached. These tests demand that each one disclose
// the values and name a continuation.

// TestDischargedFailureRefusalSaysItWasDischargedAndNamesTheOrdinaryExit is
// #2881. The exit here is genuinely simple, which is what makes the silence so
// expensive: this work unit is ordinary work, and it settles by dropping
// --remediates-evidence-revision entirely.
func TestDischargedFailureRefusalSaysItWasDischargedAndNamesTheOrdinaryExit(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "discharged-failure")
	store.ReviewDisabled = true

	// A failure, then a correction that names it. That correction discharges
	// the binding.
	appendRuntimeLedgerFile(t, repo, "work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "df-begin-1", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "df-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "formatting verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "df-begin-2", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "first correction slice\n")
	discharged, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "df-finish-2", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "first slice passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The reporter's step 4: an explicit audited reset for the next work unit
	// in the same maintainer-approved correction plan.
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: discharged.Revision, RequestID: "df-reset",
		Reason: "next bounded slice of the same correction plan", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The next budget-bounded slice, still naming the same failure because it
	// is the same correction plan.
	third, err := store.Begin(ctx, BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "df-begin-3", WorkUnit: "format",
		EvidenceGoal: "formatting correction", MaxAttempts: 6, MaxChangedLines: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "second correction slice\n")
	_, err = store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: third.Revision, RequestID: "df-finish-3", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "second slice passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	})
	if err == nil {
		t.Fatal("the second slice settled while claiming to repair a failure that was already discharged")
	}
	message := err.Error()

	// It must say the failure was already repaired, and by which settlement.
	for _, want := range []string{"already", failedEvidence} {
		if !strings.Contains(message, want) {
			t.Fatalf("the refusal does not disclose that the failure was already discharged (missing %q):\n%s", want, message)
		}
	}
	// And it must name the exit, which is to stop claiming a remediation.
	if !strings.Contains(message, "--remediates-evidence-revision") {
		t.Fatalf("the refusal names no continuation; the exit is to settle without --remediates-evidence-revision:\n%s", message)
	}
	// The old text blamed the operator's input for being wrong when it was
	// exactly right, which is what sent #2881's reporter looking for authority
	// to guess at.
	if strings.Contains(message, "requires the current failed evidence and a direct correction attempt") {
		t.Fatalf("the refusal still blames the operator's input rather than naming the discharged state:\n%s", message)
	}
}

// TestChargedCandidateRefusalDisclosesBothTrees is #2088. The reporter tried
// six configurations and asked, in the issue, for "which tree/identity/manifest
// relationship is checked". That is the whole fix: print both sides.
func TestChargedCandidateRefusalDisclosesBothTrees(t *testing.T) {
	message := runtimeChargedCandidateRefusal("aaaa111122223333444455556666777788889999", "bbbb111122223333444455556666777788889999", "review-17dc02f1df033729").Error()

	for _, want := range []string{
		"aaaa1111", // the approved tree
		"bbbb1111", // the tree this attempt charged
		"review-17dc02f1df033729",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("the refusal does not disclose %q:\n%s", want, message)
		}
	}
	if !strings.Contains(message, "gentle-ai ") {
		t.Fatalf("the refusal names no runnable continuation:\n%s", message)
	}
}

// TestStalePredecessorRefusalSaysWhichConditionFailed is #2894. "stale or not
// approved" is two different problems with two different exits, reported as
// one sentence that distinguishes neither.
func TestStalePredecessorRefusalSaysWhichConditionFailed(t *testing.T) {
	stale := runtimeBoundPredecessorRefusal("review-abc", "sha256:1111", "sha256:2222", true).Error()
	if !strings.Contains(stale, "sha256:1111") || !strings.Contains(stale, "sha256:2222") {
		t.Fatalf("the stale refusal does not disclose the revisions it compared:\n%s", stale)
	}
	if strings.Contains(stale, "not approved") {
		t.Fatalf("the stale refusal still reports both conditions at once:\n%s", stale)
	}

	unapproved := runtimeBoundPredecessorRefusal("review-abc", "sha256:1111", "sha256:1111", false).Error()
	if strings.Contains(unapproved, "stale") {
		t.Fatalf("the unapproved refusal still reports both conditions at once:\n%s", unapproved)
	}
	for _, message := range []string{stale, unapproved} {
		if !strings.Contains(message, "gentle-ai ") {
			t.Fatalf("the refusal names no runnable continuation:\n%s", message)
		}
	}
}
