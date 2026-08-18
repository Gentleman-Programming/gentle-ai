package sddstatus

import (
	"context"
	"path/filepath"
	"testing"
)

func TestConcurrentPlanBoundCore(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "concurrent-core")
	for _, root := range []string{"a", "b", "a/child"} {
		mkdir(t, filepath.Join(repo, root))
	}
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "a", DependsOn: []string{}, WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"a"}},
		{ID: "b", DependsOn: []string{}, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"b"}},
		{ID: "overlap", DependsOn: []string{}, WorkUnit: "overlap", EvidenceGoal: "overlap", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"a/child"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	acquire := func(item, id string) CompactAttemptResult {
		result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, item, id)})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	a, b := acquire("a", "a-acquire"), acquire("b", "b-acquire")
	if a.State != CompactStateProceed || b.State != CompactStateProceed || a.Token == b.Token {
		t.Fatalf("acquire=%#v %#v", a, b)
	}
	status, err := store.Status()
	if err != nil || status.runtimeActiveCount() != 2 || status.ActiveAttempt == nil || status.ActiveAttempt.Ordinal != 1 || status.runtimeActiveAttempt() != nil {
		t.Fatalf("owners=%#v %v", status, err)
	}
	before := countRuntimeRecords(t, store.Dir)
	if overlap := acquire("overlap", "overlap"); overlap.State != CompactStateBlocked || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("overlap=%#v", overlap)
	}
	settle := func(token, id string) CompactAttemptResult {
		result, err := store.Settle(ctx, CompactSettleRequest{Token: token, RequestID: id, Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash(id[0]), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := settle(a.Token, "a-settle")
	if first.ItemSettlement == nil || first.State == CompactStateComplete {
		t.Fatalf("first=%#v", first)
	}
	if after, err := store.Status(); err != nil || after.runtimeActiveCount() != 1 || after.runtimeActiveAttemptForOrdinal(2) == nil {
		t.Fatalf("after A=%#v %v", after, err)
	}
	if second := settle(b.Token, "b-settle"); second.ItemSettlement == nil {
		t.Fatalf("second=%#v", second)
	}
}
