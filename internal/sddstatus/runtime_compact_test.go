package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCompactAcquireCASClaimsOneAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "compact-cas")
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	failNextCompactStoreSync(t, store)
	requests := []CompactAcquireRequest{
		{BeginAttemptRequest: BeginAttemptRequest{RequestID: "compact-cas-one", WorkUnit: "cas-unit", EvidenceGoal: "prove one compact claimant", MaxAttempts: 2, MaxChangedLines: 20}},
		{BeginAttemptRequest: BeginAttemptRequest{RequestID: "compact-cas-two", WorkUnit: "cas-unit", EvidenceGoal: "prove one compact claimant", MaxAttempts: 2, MaxChangedLines: 20}},
	}
	type outcome struct {
		result CompactAttemptResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, len(requests))
	var ready sync.WaitGroup
	ready.Add(len(requests))
	for _, request := range requests {
		go func(request CompactAcquireRequest) {
			ready.Done()
			<-start
			result, err := store.Acquire(context.Background(), request)
			outcomes <- outcome{result: result, err: err}
		}(request)
	}
	ready.Wait()
	close(start)

	proceed, blocked := 0, 0
	for range requests {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		switch outcome.result.State {
		case CompactStateProceed:
			proceed++
		case CompactStateBlocked:
			if outcome.result.Reason != CompactBlockActiveAttempt && outcome.result.Reason != CompactBlockInvalidContinuation {
				t.Fatalf("competing acquire = %#v", outcome.result)
			}
			blocked++
		default:
			t.Fatalf("competing acquire = %#v", outcome.result)
		}
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if proceed != 1 || blocked != 1 || len(status.Attempts) != 1 || status.ActiveAttempt == nil || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("compact CAS proceed=%d blocked=%d status=%#v records=%d", proceed, blocked, status, countRuntimeRecords(t, store.Dir))
	}
}

// TestCompactAcquireTokenProvesOwnershipWithoutMutation reproduces #2291's
// deadlock shape: a parent acquires (proceed + token), then launches an
// actor that is a distinct call/process and cannot re-acquire without
// colliding with the parent's own active attempt. Presenting that exact
// token as ownership proof must let the actor proceed under the SAME
// attempt without appending anything to the ledger.
func TestCompactAcquireTokenProvesOwnershipWithoutMutation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "ownership-proof")
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	parent, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "ownership-parent", WorkUnit: "ownership-unit",
		EvidenceGoal: "prove the actor can continue the parent's attempt", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if parent.ActiveAttempt == nil {
		t.Fatalf("parent begin status = %#v", parent)
	}

	before, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)

	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "ownership-actor", WorkUnit: "ownership-unit",
			EvidenceGoal: "prove the actor can continue the parent's attempt", MaxAttempts: 2, MaxChangedLines: 20,
		},
		Token: parent.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateProceed || result.Token != parent.Revision || result.Reason != "" {
		t.Fatalf("ownership-proof acquire result = %#v", result)
	}

	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("ownership-proof acquire mutated the ledger: before=%q after=%q", before.Revision, after.Revision)
	}
	if countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("ownership-proof acquire appended a record: before=%d after=%d", beforeRecords, countRuntimeRecords(t, store.Dir))
	}
}

// TestCompactAcquireForeignTokenStaysBlockedWithoutMutation covers the
// converse of #2291's fix: a token that does NOT match the live active
// attempt must not be treated as ownership proof. It gets the ordinary
// active_attempt block naming the REAL active token (not the foreign one),
// with a named Exit/Detail explaining how to proceed, and — like the
// matching-token path — zero mutation for a losing ownership check.
func TestCompactAcquireForeignTokenStaysBlockedWithoutMutation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "foreign-token")
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "foreign-token-begin", WorkUnit: "foreign-unit",
		EvidenceGoal: "prove a stale token cannot hijack the active attempt", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	before, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)

	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "foreign-token-actor", WorkUnit: "foreign-unit",
			EvidenceGoal: "prove a stale token cannot hijack the active attempt", MaxAttempts: 2, MaxChangedLines: 20,
		},
		Token: runtimeTestHash('f'),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateBlocked || result.Reason != CompactBlockActiveAttempt || result.Token != active.Revision {
		t.Fatalf("foreign-token acquire result = %#v", result)
	}
	if result.Exit == "" || result.Detail == "" {
		t.Fatalf("foreign-token acquire result missing named exit: %#v", result)
	}

	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("foreign-token acquire mutated the ledger: before=%q after=%q", before.Revision, after.Revision)
	}
	if countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("foreign-token acquire appended a record: before=%d after=%d", beforeRecords, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireReplayRejectsForeignToken(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "foreign-replay-token")
	request := BeginAttemptRequest{
		RequestID: "foreign-replay-begin", WorkUnit: "foreign replay unit",
		EvidenceGoal: "refuse a foreign token for a replayed request", MaxAttempts: 2, MaxChangedLines: 20,
	}
	started, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)
	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: request, Token: runtimeTestHash('f'),
	})
	if err != nil || result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation {
		t.Fatalf("foreign replay token = %#v, err=%v", result, err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != started.Revision || countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("foreign replay token mutated authority: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactSettlePreservesFailedEvidenceAndReplay(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "compact-remediation-settle")
	failedEvidence := runtimeTestHash('a')
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "failed-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "failed-settle", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "verification found a correction",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "correction-acquire", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 20,
	}, RemediatesEvidenceRevision: failedEvidence})
	if err != nil || acquired.State != CompactStateProceed {
		t.Fatalf("correction acquire = %#v err=%v", acquired, err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded correction\n")
	failNextCompactStoreSync(t, store)
	before := countRuntimeRecords(t, store.Dir)
	request := CompactSettleRequest{
		Token: acquired.Token, RequestID: "correction-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "correction passed verification",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
		RemediatesEvidenceRevision: failedEvidence,
	}

	result, err := store.Settle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateComplete || result.Reason != "" || result.Token != "" {
		t.Fatalf("compact remediation result = %#v", result)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Complete || len(status.Attempts) != 2 || status.Attempts[1].RemediatesEvidenceRevision != failedEvidence ||
		status.EvidenceRevision != request.EvidenceRevision || countRuntimeRecords(t, store.Dir) != before+1 {
		t.Fatalf("compact remediation status = %#v records=%d", status, countRuntimeRecords(t, store.Dir))
	}

	replayed, err := store.Settle(context.Background(), request)
	if err != nil || replayed != result || countRuntimeRecords(t, store.Dir) != before+1 {
		t.Fatalf("compact remediation replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, store.Dir))
	}
	for _, test := range []struct {
		name     string
		evidence string
	}{
		{name: "omitted evidence"},
		{name: "different evidence", evidence: runtimeTestHash('d')},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflict := request
			conflict.RemediatesEvidenceRevision = test.evidence
			blocked, settleErr := store.Settle(context.Background(), conflict)
			if settleErr != nil || blocked.State != CompactStateBlocked || blocked.Reason != CompactBlockInvalidContinuation ||
				countRuntimeRecords(t, store.Dir) != before+1 {
				t.Fatalf("conflicting replay = %#v err=%v records=%d", blocked, settleErr, countRuntimeRecords(t, store.Dir))
			}
		})
	}
	if failed.Revision == "" {
		t.Fatal("failed verification did not publish its evidence record")
	}
}

func TestCompactSettleReplaysCanonicalLegacyInterruptedRequest(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "compact-legacy-interrupted-replay")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "legacy-begin", WorkUnit: "unit", EvidenceGoal: "goal", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	beginRecord, err := store.loadRecord(started.Revision)
	if err != nil {
		t.Fatal(err)
	}
	legacy := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: started.Revision,
		Operation: runtimeOperationFinish, RequestID: "legacy-finish", Finish: &runtimeFinishEvent{
			Ordinal: 1, FinishCandidateIdentity: beginRecord.Begin.BeginCandidateIdentity,
			FinishCandidateTree: beginRecord.Begin.BeginCandidateTree, Outcome: AttemptInterrupted,
			ChangedLines: 0, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "legacy interrupted record",
			HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none",
		}}
	legacy.RequestDigest = runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", FinishAttemptRequest{
		ExpectedRevision: legacy.PreviousRevision, RequestID: legacy.RequestID, Outcome: legacy.Finish.Outcome,
		EvidenceRevision: legacy.Finish.EvidenceRevision, Diagnosis: legacy.Finish.Diagnosis,
		HarnessDisposition: legacy.Finish.HarnessDisposition, CleanupEvidence: legacy.Finish.CleanupEvidence,
		ProcessEvidence: legacy.Finish.ProcessEvidence,
	})
	revision, payload, err := runtimeRecordRevision(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)

	result, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: started.Revision, RequestID: legacy.RequestID, Outcome: AttemptInterrupted,
		EvidenceRevision: legacy.Finish.EvidenceRevision, Diagnosis: legacy.Finish.Diagnosis,
		HarnessDisposition: legacy.Finish.HarnessDisposition, CleanupEvidence: legacy.Finish.CleanupEvidence,
		ProcessEvidence: legacy.Finish.ProcessEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateProceed || status.Revision != revision || countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("legacy interrupted compact replay result=%#v status=%#v records=%d", result, status, countRuntimeRecords(t, store.Dir))
	}
}

func failNextCompactStoreSync(t *testing.T, store RuntimeStore) {
	t.Helper()
	original := runtimeSyncDirectory
	var once sync.Once
	runtimeSyncDirectory = func(path string) error {
		failed := false
		if path == store.Dir {
			once.Do(func() { failed = true })
		}
		if failed {
			return errors.New("simulated compact store sync failure")
		}
		return original(path)
	}
	t.Cleanup(func() { runtimeSyncDirectory = original })
}

// TestUnreadableAuthorityBlockNamesItsCause is #2829, and root 8's shape on a
// surface root 8 never reached.
//
// A user hit `the attempt ledger for this work unit cannot be read as valid
// authority` on a published build and neither they nor anyone triaging it
// could tell an invalid HEAD encoding from a broken chain link from a
// permissions refusal. Those need different repairs and produced identical
// output, because seven call sites discarded the error `store.load()` had
// just returned.
//
// The exit text is deliberately NOT part of this assertion: callers route on
// it and it must keep its exact bytes. Only Detail gains the cause.
func TestUnreadableAuthorityBlockNamesItsCause(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "unreadable-cause")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "cause-begin", WorkUnit: "apply", EvidenceGoal: "prove it",
		MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}

	// Damage the ledger the way an interrupted write does: a HEAD that is not
	// the exact `sha256:<64hex>\n` encoding readRuntimeHead requires.
	head := filepath.Join(store.Dir, "HEAD")
	if err := os.WriteFile(head, []byte("sha256:not-a-digest\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "cause-acquire", WorkUnit: "apply", EvidenceGoal: "prove it",
			MaxAttempts: 2, MaxChangedLines: 20,
		},
	})
	if err != nil {
		t.Fatalf("acquire over a damaged ledger returned a hard error rather than a block: %v", err)
	}
	if result.State != CompactStateBlocked || result.Reason != CompactBlockCorruptAuthority {
		t.Fatalf("result = %#v, want blocked/corrupt_authority", result)
	}
	if !strings.Contains(result.Detail, "cause:") {
		t.Fatalf("blocked detail discards the load error, so no reader can tell which damage this is: %q", result.Detail)
	}
	if !strings.Contains(result.Exit, "sdd-attempt status") {
		t.Fatalf("exit text lost its runnable continuation: %q", result.Exit)
	}
}
