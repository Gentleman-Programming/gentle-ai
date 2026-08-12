package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// #1974 slice 2 (#2565): the unmanaged-remediation binding derives from the
// immutable attempt chain -- the most recent settled failed attempt with no
// passed settlement after it -- never from the live evidence pointer that
// Reset, Rescope, and Advance wipe. An audited reset or an honest interrupted
// settlement between the failure and its correction is an audit record, not a
// semantic successor, so neither severs the binding.

func TestRuntimeUnmanagedRemediationBindsAcrossAuditedReset(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "chain-binding")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "chain-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "chain-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.DecisionRequired || failed.NextAction != RuntimeActionReset {
		t.Fatalf("exhausted failed verification = %#v", failed)
	}
	// The audited reset is the documented escape from decision-required. It
	// wipes the live evidence pointer; only the attempt chain remembers.
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "chain-audited-reset",
		Reason: "maintainer decision: blockers understood, remediate under a fresh objective",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reset.EvidenceRevision != "" {
		t.Fatalf("audited reset kept the live evidence pointer: %#v", reset)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "chain-begin-correction", WorkUnit: "remediate",
		EvidenceGoal: "repair admitted verification failure", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded correction across the audited reset\n")
	request := FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "chain-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "bounded correction passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	}
	unbound := request
	unbound.RequestID = "chain-finish-unbound"
	unbound.RemediatesEvidenceRevision = ""
	if _, err := store.Finish(context.Background(), unbound); err == nil || !strings.Contains(err.Error(), failedEvidence) {
		t.Fatalf("plain pass across the reset = %v, want a refusal naming the chain's failed evidence %s", err, failedEvidence)
	}
	wrongEvidence := request
	wrongEvidence.RequestID = "chain-finish-wrong-evidence"
	wrongEvidence.RemediatesEvidenceRevision = runtimeTestHash('c')
	if _, err := store.Finish(context.Background(), wrongEvidence); err == nil {
		t.Fatal("wrong failed evidence satisfied the chain-derived binding")
	}
	completed, err := store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("correction across the audited reset was refused: %v", err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || completed.Binding != nil || len(completed.Attempts) != 2 {
		t.Fatalf("correction across the audited reset = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.RemediatesEvidenceRevision != failedEvidence || last.FinishCandidateIdentity == last.BeginCandidateIdentity ||
		last.FinishCandidateTree == last.BeginCandidateTree {
		t.Fatalf("correction did not bind the failed evidence to a changed candidate: %#v", last)
	}
	if replay, err := store.Finish(context.Background(), request); err != nil || replay.Revision != completed.Revision {
		t.Fatalf("exact correction replay = %#v err=%v", replay, err)
	}
	// The replay twin must accept the same chain the write guard admitted:
	// a fresh store replays every record from genesis.
	reopened := mustRuntimeStore(t, repo, "chain-binding")
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("full-chain replay across the audited reset = %#v err=%v", replayed, err)
	}
}

func TestRuntimeUnmanagedRemediationSkipsInterruptedAuditRecords(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "chain-interrupt")
	store.ReviewDisabled = true
	objective := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "interrupt-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 3, MaxChangedLines: 40,
	}
	first, err := store.Begin(context.Background(), objective)
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "interrupt-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	// An honest interrupted settlement between the failure and its correction
	// is an audit record, not a semantic successor.
	stalled := objective
	stalled.ExpectedRevision = failed.Revision
	stalled.RequestID = "interrupt-begin-stalled"
	stalledStatus, err := store.Begin(context.Background(), stalled)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: stalledStatus.Revision, RequestID: "interrupt-finish-stalled", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "transport ended before the correction settled",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "stalled process group cleanup completed",
		ProcessEvidence: "stalled process scan found no surviving descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction := objective
	correction.ExpectedRevision = interrupted.Revision
	correction.RequestID = "interrupt-begin-correction"
	active, err := store.Begin(context.Background(), correction)
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded correction after an interrupted audit record\n")
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "interrupt-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "bounded correction passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatalf("correction after an interrupted audit record was refused: %v", err)
	}
	if !completed.Complete || len(completed.Attempts) != 3 ||
		completed.Attempts[2].RemediatesEvidenceRevision != failedEvidence {
		t.Fatalf("correction after an interrupted audit record = %#v", completed)
	}
	reopened := mustRuntimeStore(t, repo, "chain-interrupt")
	replayed, err := reopened.Status()
	if err != nil || replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("full-chain replay across the interrupted record = %#v err=%v", replayed, err)
	}
}

// The anti-laundering budget stays exact: the first passed settlement after a
// failure is the one correction that failed evidence admits. Walking the chain
// backward, that pass is reached before the failure, so a second correction
// claiming the same revision finds no failed evidence and is refused.
func TestRuntimeUnmanagedRemediationRefusesASecondCorrectionForSettledEvidence(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "chain-launder")
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "launder-begin-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "launder-finish-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "independent verification found a correctable defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "launder-begin-correction", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "first bounded correction\n")
	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "launder-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "bounded correction passed focused checks",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settled.Complete {
		t.Fatalf("first bounded correction did not settle: %#v", settled)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: settled.Revision, RequestID: "launder-reset",
		Reason: "attempt to reopen the settled correction",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "launder-begin-second", WorkUnit: "remediate",
		EvidenceGoal: "repair admitted verification failure", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "second correction attempt\n")
	before := countRuntimeRecords(t, store.Dir)
	_, err = store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "launder-finish-second", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "second correction claims the settled evidence",
		HarnessDisposition: HarnessReused, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan completed", RemediatesEvidenceRevision: failedEvidence,
	})
	// The anti-laundering property is unchanged: a second correction may not
	// claim evidence a passing settlement already repaired. What changed is
	// that the refusal now says so (#2881). Asserting the property and the
	// disclosure beats pinning the prose, which is what made this brittle.
	if err == nil {
		t.Fatal("second correction against settled evidence was admitted; a passing settlement already repaired it")
	}
	for _, want := range []string{"already been repaired", failedEvidence, "--remediates-evidence-revision"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the laundering refusal does not disclose %q:\n%v", want, err)
		}
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != second.Revision || status.ActiveAttempt == nil ||
		countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("laundering refusal mutated runtime: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}
