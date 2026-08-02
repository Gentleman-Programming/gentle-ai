package sddstatus

import (
	"context"
	"errors"
	"path/filepath"
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
		{RequestID: "compact-cas-one", WorkUnit: "cas-unit", EvidenceGoal: "prove one compact claimant", MaxAttempts: 2, MaxChangedLines: 20},
		{RequestID: "compact-cas-two", WorkUnit: "cas-unit", EvidenceGoal: "prove one compact claimant", MaxAttempts: 2, MaxChangedLines: 20},
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

func TestCompactSettlePreservesAtomicRemediationAndReplay(t *testing.T) {
	legacyFixture := newRuntimeUnchangedBindingFixture(t, "compact-legacy-evidence")
	write(t, filepath.Join(legacyFixture.store.Repo, "openspec", "changes", "compact-legacy-evidence", "tasks.md"), "- [x] 1.1 Done\n# candidate-changing remediation\n")
	fixture := runtimeRemediationFixture{repo: legacyFixture.store.Repo, store: legacyFixture.store, predecessorBinding: legacyFixture.binding,
		failedEvidence: runtimeTestHash('b'), active: legacyFixture.active,
		successor: createRuntimeRecoverySuccessor(t, legacyFixture.store.Repo, legacyFixture.binding.Lineage, "compact-legacy-successor", true)}
	failNextCompactStoreSync(t, fixture.store)
	before := countRuntimeRecords(t, fixture.store.Dir)
	legacy := fixture.finishRequest("compact-remediation-settle")
	request := CompactSettleRequest{
		Token: fixture.active.Revision, RequestID: legacy.RequestID, Outcome: legacy.Outcome,
		EvidenceRevision: legacy.EvidenceRevision, Diagnosis: legacy.Diagnosis,
		HarnessDisposition: legacy.HarnessDisposition, CleanupEvidence: legacy.CleanupEvidence,
		ProcessEvidence: legacy.ProcessEvidence, SuccessorLineageID: legacy.SuccessorLineageID,
		RemediatesEvidenceRevision: legacy.RemediatesEvidenceRevision,
	}

	result, err := fixture.store.Settle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateComplete || result.Reason != "" || result.Token != "" {
		t.Fatalf("compact remediation result = %#v", result)
	}
	status, err := fixture.store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Complete || status.Binding == nil || status.Binding.Lineage != fixture.successor.State.LineageID ||
		status.EvidenceRevision != legacy.EvidenceRevision || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("compact remediation status = %#v records=%d", status, countRuntimeRecords(t, fixture.store.Dir))
	}

	replayed, err := fixture.store.Settle(context.Background(), request)
	if err != nil || replayed != result || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("compact remediation replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, fixture.store.Dir))
	}
	for _, test := range []struct {
		name      string
		successor string
		evidence  string
	}{
		{name: "omitted successor", evidence: request.RemediatesEvidenceRevision},
		{name: "omitted evidence", successor: request.SuccessorLineageID},
		{name: "different evidence", successor: request.SuccessorLineageID, evidence: runtimeTestHash('d')},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflict := request
			conflict.SuccessorLineageID = test.successor
			conflict.RemediatesEvidenceRevision = test.evidence
			blocked, settleErr := fixture.store.Settle(context.Background(), conflict)
			if settleErr != nil || blocked.State != CompactStateBlocked || blocked.Reason != CompactBlockInvalidContinuation ||
				countRuntimeRecords(t, fixture.store.Dir) != before+1 {
				t.Fatalf("conflicting replay = %#v err=%v records=%d", blocked, settleErr, countRuntimeRecords(t, fixture.store.Dir))
			}
		})
	}
}

func TestCompactSettleReviewDisabledClosesOrdinaryWithoutAdvancingBinding(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	legacy := fixture.finishRequest("compact-review-disabled-settle")
	store := fixture.store
	store.ReviewDisabled = true
	before := countRuntimeRecords(t, store.Dir)

	result, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: fixture.active.Revision, RequestID: legacy.RequestID, Outcome: legacy.Outcome,
		EvidenceRevision: legacy.EvidenceRevision, Diagnosis: legacy.Diagnosis,
		HarnessDisposition: legacy.HarnessDisposition, CleanupEvidence: legacy.CleanupEvidence,
		ProcessEvidence: legacy.ProcessEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, statusErr := store.Status()
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if result.State != CompactStateComplete || status.ActiveAttempt != nil || status.Binding == nil ||
		status.Binding.Revision != fixture.predecessorBinding.Revision || countRuntimeRecords(t, store.Dir) != before+1 {
		t.Fatalf("review-disabled compact settle result=%#v status=%#v records=%d", result, status, countRuntimeRecords(t, store.Dir))
	}
	record, loadErr := store.loadRecord(status.Revision)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if record.Operation != runtimeOperationFinish || record.Binding != nil {
		t.Fatalf("review-disabled compact settle record = %#v", record)
	}
}

func TestCompactSettleTokenSurvivesBindingCASAndDerivesSelfSuccessor(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	if fixture.active.Revision == fixture.postBind.Revision {
		t.Fatal("fixture did not advance runtime HEAD after acquire")
	}
	legacy := fixture.finishRequest("compact-self-remediation")
	result, err := fixture.store.Settle(context.Background(), CompactSettleRequest{
		Token: fixture.active.Revision, RequestID: legacy.RequestID, Outcome: legacy.Outcome,
		EvidenceRevision: legacy.EvidenceRevision, Diagnosis: legacy.Diagnosis,
		HarnessDisposition: legacy.HarnessDisposition, CleanupEvidence: legacy.CleanupEvidence,
		ProcessEvidence: legacy.ProcessEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, statusErr := fixture.store.Status()
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if result.State != CompactStateComplete || !status.Complete || status.Binding == nil || status.Binding.Lineage != fixture.lineage {
		t.Fatalf("compact self-remediation result=%#v status=%#v", result, status)
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
