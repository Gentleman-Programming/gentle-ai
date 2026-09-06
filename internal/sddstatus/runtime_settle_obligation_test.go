package sddstatus

import (
	"context"
	"fmt"
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
// audited reset moved to a DIFFERENTLY-NAMED objective ("media verification"
// to "scoring", no rescope), and #1974 slice 2 deliberately keeps the binding
// alive across that reset. So the obligation is real, and acquire must say so
// while the attempt is still unspent.
//
// This also pins #4024 R4: lineage is keyed on the ledger's own recorded
// reset predecessor (ObjectiveID/Generation), never on the work-unit label,
// so the plain rename here must never silently drop a real obligation.
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

// TestDoubleResetRelationIsNotTransitive is #4024 R4: A fails, a reset (r1)
// opens B, B is interrupted, a second reset (r2) closes B and opens C. A
// purely structural double reset with NO declaration anywhere must never
// suppress (the headline fail-open bypass this fix closes). Independence is
// NOT transitive: a declaration on the FIRST hop (a-to-b) suppresses only
// b's own obligation, never c's -- c was opened by r2, a PLAIN reset, so it
// inherits a's unremediated failure again even though b, its immediate
// predecessor, was independent of a. Only a declaration on c's OWN opening
// hop (r2) suppresses for c.
func TestDoubleResetRelationIsNotTransitive(t *testing.T) {
	for i, test := range []struct {
		name       string
		r1, r2     RuntimeObjectiveRelation
		suppressed bool
	}{
		{"no declaration: inherits", "", "", false},
		{"a-to-b declared independent, b-to-c plain: independence does not carry past one hop, c inherits a's obligation", RuntimeObjectiveRelationIndependent, "", false},
		{"declared on second hop: suppresses only for c", "", RuntimeObjectiveRelationIndependent, true},
	} {
		ctx := context.Background()
		repo := initRuntimeLedgerRepo(t)
		store := mustRuntimeStore(t, repo, fmt.Sprintf("double-reset-%d", i))
		store.ReviewDisabled = true
		first, _ := store.Begin(ctx, BeginAttemptRequest{RequestID: "a-begin", WorkUnit: "a", EvidenceGoal: "goal-a", MaxAttempts: 1, MaxChangedLines: 400})
		failed, _ := store.Finish(ctx, FinishAttemptRequest{
			ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'),
			Diagnosis: "a failed", HarnessDisposition: HarnessReused, CleanupEvidence: "done", ProcessEvidence: "none",
		})
		reset1, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "reset-1", Reason: "open b", Actor: "m", Relation: test.r1})
		if err != nil {
			t.Fatal(err)
		}
		appendRuntimeLedgerFile(t, repo, "b work\n")
		beganB, _ := store.Begin(ctx, BeginAttemptRequest{ExpectedRevision: reset1.Revision, RequestID: "b-begin", WorkUnit: "b", EvidenceGoal: "goal-b", MaxAttempts: 1, MaxChangedLines: 400})
		interrupted, _ := store.Finish(ctx, FinishAttemptRequest{
			ExpectedRevision: beganB.Revision, RequestID: "b-finish", Outcome: AttemptInterrupted,
			Diagnosis: "b interrupted", HarnessDisposition: HarnessReused, CleanupEvidence: "done", ProcessEvidence: "none",
		})
		reset2, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: interrupted.Revision, RequestID: "reset-2", Reason: "close b", Actor: "m", Relation: test.r2})
		if err != nil {
			t.Fatal(err)
		}
		acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
			ExpectedRevision: reset2.Revision, RequestID: "c-acquire", WorkUnit: "c", EvidenceGoal: "goal-c", MaxAttempts: 2, MaxChangedLines: 400,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if test.suppressed && (acquired.SettleObligation != "" || acquired.SuppressedObligation == nil || acquired.SuppressedObligation.Reason != "declared_independent") {
			t.Fatalf("%s: acquired = %#v, want suppressed with reason declared_independent", test.name, acquired)
		}
		if !test.suppressed && (acquired.SettleObligation == "" || acquired.SuppressedObligation != nil) {
			t.Fatalf("%s: acquired = %#v, want an inherited, undisclosed-nothing obligation (#4024 R4 fail-open bypass)", test.name, acquired)
		}
	}
}

// TestFailureInsideIndependentSuccessorInheritsItsOwnObligation: A fails, an
// independent reset opens B, B ITSELF fails with its own evidence, a plain
// reset retries B as C. C's own opening relation (r2) is plain, not
// independent, so C inherits the chain's most recent unremediated failure --
// B's own, never A's -- regardless of the earlier A-to-B declaration.
func TestFailureInsideIndependentSuccessorInheritsItsOwnObligation(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "independent-successor-own-failure")
	store.ReviewDisabled = true
	first, _ := store.Begin(ctx, BeginAttemptRequest{RequestID: "a-begin", WorkUnit: "a", EvidenceGoal: "goal-a", MaxAttempts: 1, MaxChangedLines: 400})
	failedA, _ := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "a failed", HarnessDisposition: HarnessReused, CleanupEvidence: "done", ProcessEvidence: "none",
	})
	reset1, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: failedA.Revision, RequestID: "reset-1", Reason: "open independent b", Actor: "m", Relation: RuntimeObjectiveRelationIndependent,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "b work\n")
	beganB, _ := store.Begin(ctx, BeginAttemptRequest{ExpectedRevision: reset1.Revision, RequestID: "b-begin", WorkUnit: "b", EvidenceGoal: "goal-b", MaxAttempts: 1, MaxChangedLines: 400})
	evidenceB := runtimeTestHash('b')
	failedB, _ := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: beganB.Revision, RequestID: "b-finish", Outcome: AttemptFailed, EvidenceRevision: evidenceB,
		Diagnosis: "b failed on its own", HarnessDisposition: HarnessReused, CleanupEvidence: "done", ProcessEvidence: "none",
	})
	reset2, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: failedB.Revision, RequestID: "reset-2", Reason: "retry b", Actor: "m"})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		ExpectedRevision: reset2.Revision, RequestID: "c-acquire", WorkUnit: "b-retry", EvidenceGoal: "goal-b", MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(acquired.SettleObligation, evidenceB) {
		t.Fatalf("retry of b's own failure = %#v, want an obligation naming b's own evidence %s (not a's, and not suppressed)", acquired, evidenceB)
	}
	if acquired.SuppressedObligation != nil {
		t.Fatalf("b's own retry was wrongly suppressed by the earlier a-to-b independent declaration: %#v", acquired.SuppressedObligation)
	}
}

// TestRescopeIndependentPersistsReplaysAndSuppresses is #4024's rescope arm
// (previously untested): the declared relation persists, replays, and
// suppresses like reset's own; an invalid relation names its resolution.
func TestRescopeIndependentPersistsReplaysAndSuppresses(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "rescope-independent")
	store.ReviewDisabled = true
	first, _ := store.Begin(ctx, BeginAttemptRequest{RequestID: "a-begin", WorkUnit: "a", EvidenceGoal: "goal-a", MaxAttempts: 2, MaxChangedLines: 400})
	failedA, _ := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'),
		Diagnosis: "a failed with capacity left", HarnessDisposition: HarnessReused, CleanupEvidence: "done", ProcessEvidence: "none",
	})
	if _, err := store.Rescope(ctx, RescopeObjectiveRequest{
		ExpectedRevision: failedA.Revision, RequestID: "rescope-invalid", WorkUnit: "b", EvidenceGoal: "goal-b",
		MaxAttempts: 2, MaxChangedLines: 10, Reason: "narrow", Actor: "m", Relation: "bogus",
	}); err == nil || !strings.Contains(err.Error(), "gentle-ai sdd-attempt rescope") {
		t.Fatalf("invalid objective_relation on rescope = %v, want a refusal naming its resolution", err)
	}
	rescoped, err := store.Rescope(ctx, RescopeObjectiveRequest{
		ExpectedRevision: failedA.Revision, RequestID: "rescope-1", WorkUnit: "b", EvidenceGoal: "goal-b",
		MaxAttempts: 2, MaxChangedLines: 10, Reason: "narrow independent of a", Actor: "m", Relation: RuntimeObjectiveRelationIndependent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rescoped.Objective == nil || rescoped.Objective.Relation != RuntimeObjectiveRelationIndependent ||
		rescoped.Objective.PredecessorObjectiveID != failedA.Attempts[0].ObjectiveID {
		t.Fatalf("rescope did not persist the declared relation and predecessor link: %#v", rescoped.Objective)
	}
	reopened := mustRuntimeStore(t, repo, "rescope-independent")
	replayed, err := reopened.Status()
	if err != nil || replayed.Objective == nil || replayed.Objective.Relation != RuntimeObjectiveRelationIndependent {
		t.Fatalf("rescope's declared relation did not replay: %#v err=%v", replayed.Objective, err)
	}
	acquired, err := reopened.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "b-acquire", WorkUnit: "b", EvidenceGoal: "goal-b", MaxAttempts: 2, MaxChangedLines: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.SettleObligation != "" || acquired.SuppressedObligation == nil || acquired.SuppressedObligation.Reason != "declared_independent" {
		t.Fatalf("rescope-declared independent did not suppress: %#v", acquired)
	}
}

// TestSettleAndFinishAgreeOnTheForceRefusal is #4024 R3/R4's own
// cross-layer table: compact Settle and the raw Finish transition must reach
// byte-identical verdicts and text for the same ledger state, because they
// now call the ONE shared gate (runtimeFailedAttemptInObjectiveLineage) with
// the same inputs instead of two independently maintained copies.
func TestSettleAndFinishAgreeOnTheForceRefusal(t *testing.T) {
	ctx := context.Background()

	// seed builds an identical same-lineage reset scenario (objective "verify"
	// fails, an audited reset reopens it) in a fresh repo/store, returning the
	// live token an unbound passing settle would be refused for.
	seed := func(t *testing.T, change string) (RuntimeStore, string) {
		t.Helper()
		repo := initRuntimeLedgerRepo(t)
		store := mustRuntimeStore(t, repo, change)
		store.ReviewDisabled = true
		appendRuntimeLedgerFile(t, repo, "objective work\n")
		first, err := store.Begin(ctx, BeginAttemptRequest{
			RequestID: "a-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
			MaxAttempts: 1, MaxChangedLines: 400,
		})
		if err != nil {
			t.Fatal(err)
		}
		failed, err := store.Finish(ctx, FinishAttemptRequest{
			ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash('a'), Diagnosis: "verification failed",
			HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		})
		if err != nil {
			t.Fatal(err)
		}
		reset, err := store.Reset(ctx, ResetObjectiveRequest{
			ExpectedRevision: failed.Revision, RequestID: "audited-reset",
			Reason: "retry the same verification under a fresh generation", Actor: "maintainer",
		})
		if err != nil {
			t.Fatal(err)
		}
		active, err := store.Begin(ctx, BeginAttemptRequest{
			ExpectedRevision: reset.Revision, RequestID: "b-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
			MaxAttempts: 2, MaxChangedLines: 400,
		})
		if err != nil {
			t.Fatal(err)
		}
		return store, active.Revision
	}

	rawStore, rawToken := seed(t, "agree-raw")
	appendRuntimeLedgerFile(t, rawStore.Workspace, "the retry (raw)\n")
	_, rawErr := rawStore.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: rawToken, RequestID: "b-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "retry passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if rawErr == nil {
		t.Fatal("raw Finish settled passed with no remediation claim over a same-lineage failure")
	}

	compactStore, compactToken := seed(t, "agree-compact")
	appendRuntimeLedgerFile(t, compactStore.Workspace, "the retry (compact)\n")
	compactResult, compactErr := compactStore.Settle(ctx, CompactSettleRequest{
		Token: compactToken, RequestID: "b-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "retry passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if compactErr != nil {
		t.Fatal(compactErr)
	}
	if compactResult.State != CompactStateBlocked || compactResult.Reason != CompactBlockInvalidContinuation {
		t.Fatalf("compact settle = %#v, want blocked(invalid_continuation) matching the raw Finish refusal", compactResult)
	}
	if compactResult.Detail != rawErr.Error() {
		t.Fatalf("compact settle and raw finish disagree on the refusal text:\ncompact: %q\nraw:     %q", compactResult.Detail, rawErr.Error())
	}
}
