package sddstatus

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func joinPlan(t *testing.T) itemPlanCandidate {
	t.Helper()
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "p", Done: true, WorkUnit: "p", EvidenceGoal: "prior", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"p"}},
		{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"a"}},
		{ID: "b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"b"}},
		{ID: "c", DependsOn: []string{"a", "b", "p"}, WorkUnit: "c", EvidenceGoal: "c", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"c"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestItemPlanV2SnapshotAndV1Compatibility(t *testing.T) {
	plan := joinPlan(t)
	p, ok := itemPlanEntryForID(plan, "p")
	if plan.Version != itemPlanVersionV2 || len(plan.Items) != 4 || !ok || p.InitiallyDone == nil || !*p.InitiallyDone {
		t.Fatalf("v2 plan = %#v", plan)
	}
	for _, entry := range plan.Items {
		if entry.ID == "p" {
			continue
		}
		if entry.InitiallyDone == nil || *entry.InitiallyDone {
			t.Fatalf("v2 entry = %#v", entry)
		}
	}
	drifted, err := newItemPlanCandidate([]WorkItem{
		{ID: "p", WorkUnit: "p", EvidenceGoal: "prior", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"p"}},
		{ID: "a", Done: true, WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"a"}},
		{ID: "b", Done: true, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"b"}},
		{ID: "c", Done: true, DependsOn: []string{"a", "b", "p"}, WorkUnit: "c", EvidenceGoal: "c", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"c"}},
	}, &plan)
	if err != nil || drifted.Digest != plan.Digest {
		t.Fatalf("retained snapshot drift = %#v, %v", drifted, err)
	}
	legacy := *cloneItemPlan(&plan)
	legacy.Version = itemPlanVersionV1
	for index := range legacy.Items {
		legacy.Items[index].InitiallyDone = nil
	}
	legacy.Digest = itemPlanDigest(legacy)
	if err := validateItemPlan(legacy); err != nil || strings.Contains(string(stringMustMarshal(t, legacy)), "initially_done") {
		t.Fatalf("v1 compatibility = %#v, %v", legacy, err)
	}
	for _, mutate := range []func(*itemPlanCandidate){
		func(value *itemPlanCandidate) { value.Items[0].InitiallyDone = nil },
		func(value *itemPlanCandidate) { value.Version = itemPlanVersionV1 },
	} {
		forged := *cloneItemPlan(&plan)
		mutate(&forged)
		forged.Digest = itemPlanDigest(forged)
		if err := validateItemPlan(forged); err == nil {
			t.Fatalf("forged plan accepted: %#v", forged)
		}
	}
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"a", "b", "c", "p"} {
		mkdir(t, repo+"/"+root)
	}
	store := mustRuntimeStore(t, repo, "legacy-plan")
	request := runtimePlanRequest(t, store, legacy, "a", "legacy-plan-a")
	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
	if err != nil || acquired.State != CompactStateProceed {
		t.Fatalf("v1 acquire = %#v, %v", acquired, err)
	}
	settled, err := store.Settle(ctx, CompactSettleRequest{Token: acquired.Token, RequestID: "legacy-plan-settle", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil || settled.State != CompactStateComplete || settled.ItemSettlement == nil {
		t.Fatalf("v1 settle = %#v, %v", settled, err)
	}
}

func TestV2DependenciesAndJoinRequireImmutableCompletionAndProjection(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"a", "b", "c", "p"} {
		mkdir(t, repo+"/"+root)
	}
	store, plan := mustRuntimeStore(t, repo, "join"), joinPlan(t)
	a, b := concurrentAcquire(t, ctx, store, plan, "a", "join-a"), concurrentAcquire(t, ctx, store, plan, "b", "join-b")
	concurrentSettle(t, ctx, store, a.Token, "a-settle")
	runtime, err := store.Status()
	if err != nil || runtimePlanDependenciesSatisfied(runtimeReplay{Status: runtime, itemPlan: runtime.itemPlan}, *runtime.itemPlan, "c") {
		t.Fatalf("C satisfied before B = %#v, %v", runtime, err)
	}
	concurrentSettle(t, ctx, store, b.Token, "b-settle")
	c := concurrentAcquire(t, ctx, store, plan, "c", "join-c")
	runtime, err = store.Status()
	if err != nil || !runtimePlanDependenciesSatisfied(runtimeReplay{Status: runtime, itemPlan: runtime.itemPlan}, *runtime.itemPlan, "c") {
		t.Fatalf("C not satisfied after A/B/P = %#v, %v", runtime, err)
	}
	status := Status{RuntimeStatus: &runtime, Dependencies: Dependencies{Apply: DependencyAllDone, Verify: DependencyReady, Archive: DependencyReady}, NextRecommended: "verify"}
	for _, id := range []string{"p", "a", "b", "c"} {
		status.Items = append(status.Items, WorkItem{ID: id, Done: true})
	}
	applyRetainedItemPlanJoinRouting(&status)
	if status.NextRecommended != "apply" || status.Dependencies.Verify != DependencyBlocked {
		t.Fatalf("active join routed = %#v", status)
	}
	concurrentSettle(t, ctx, store, c.Token, "c-settle")
	runtime, err = store.Status()
	if err != nil {
		t.Fatal(err)
	}
	status.RuntimeStatus, status.Dependencies, status.NextRecommended = &runtime, Dependencies{Apply: DependencyAllDone, Verify: DependencyReady, Archive: DependencyBlocked}, "verify"
	applyRetainedItemPlanJoinRouting(&status)
	if status.NextRecommended != "verify" || status.Dependencies.Verify != DependencyReady {
		t.Fatalf("joined routing = %#v", status)
	}
}

func stringMustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
