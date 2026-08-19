package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func concurrentPlan(t *testing.T) itemPlanCandidate {
	t.Helper()
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"a"}},
		{ID: "b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"b"}},
		{ID: "c", DependsOn: []string{"a", "b"}, WorkUnit: "c", EvidenceGoal: "c", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"c"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func concurrentStore(t *testing.T) (context.Context, RuntimeStore, itemPlanCandidate) {
	t.Helper()
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"a", "b", "c"} {
		mkdir(t, filepath.Join(repo, root))
	}
	return ctx, mustRuntimeStore(t, repo, "concurrent-matrix"), concurrentPlan(t)
}

func concurrentAcquire(t *testing.T, ctx context.Context, store RuntimeStore, plan itemPlanCandidate, item, request string) CompactAttemptResult {
	t.Helper()
	result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, item, request)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func concurrentSettle(t *testing.T, ctx context.Context, store RuntimeStore, token, request string) CompactAttemptResult {
	t.Helper()
	result, err := store.Settle(ctx, CompactSettleRequest{Token: token, RequestID: request, Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash(request[0]), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestConcurrentItemDependencyProvenanceMatrix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*BeginAttemptRequest)
		want   bool
	}{
		{"exact plan-bound passed predecessor", nil, true},
		{"mismatched objective contract", func(r *BeginAttemptRequest) { r.WorkUnit = "wrong" }, false},
		{"mismatched retained digest", func(r *BeginAttemptRequest) { r.itemPlan.Plan.Digest = runtimeTestHash('d') }, false},
		{"mismatched entry contract", func(r *BeginAttemptRequest) { r.itemPlan.EntryDigest = runtimeTestHash('e') }, false},
		{"mismatched roots", func(r *BeginAttemptRequest) { r.ItemEditRoots = []string{"/wrong"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, plan := concurrentStore(t)
			a := concurrentAcquire(t, ctx, store, plan, "a", "a")
			b := concurrentAcquire(t, ctx, store, plan, "b", "b")
			concurrentSettle(t, ctx, store, a.Token, "a-settle")
			concurrentSettle(t, ctx, store, b.Token, "b-settle")
			request := runtimePlanRequest(t, store, plan, "c", "c")
			if tc.mutate != nil {
				tc.mutate(&request)
			}
			before := countRuntimeRecords(t, store.Dir)
			status, err := store.AdmissionStatus(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
			if err != nil && tc.want {
				t.Fatal(err)
			}
			if tc.want != (result.State == CompactStateProceed) || tc.want != (status.BlockedReason == "") || (!tc.want && countRuntimeRecords(t, store.Dir) != before) {
				t.Fatalf("status=%#v acquire=%#v records=%d", status, result, countRuntimeRecords(t, store.Dir))
			}
		})
	}
	for _, tc := range []struct {
		name  string
		items []WorkItem
	}{
		{"unknown dependency", []WorkItem{{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"a"}}, {ID: "b", DependsOn: []string{"missing"}, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"b"}}}},
		{"duplicate dependency", []WorkItem{{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"a"}}, {ID: "b", DependsOn: []string{"a", "a"}, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"b"}}}},
		{"cyclic dependency", []WorkItem{{ID: "a", DependsOn: []string{"b"}, WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"a"}}, {ID: "b", DependsOn: []string{"a"}, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"b"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newItemPlanCandidate(tc.items, nil); err == nil {
				t.Fatal("invalid dependency plan accepted")
			}
		})
	}
}

func TestConcurrentCompatibilityProjectionAndWorkItems(t *testing.T) {
	ctx, store, plan := concurrentStore(t)
	a, b := concurrentAcquire(t, ctx, store, plan, "a", "a"), concurrentAcquire(t, ctx, store, plan, "b", "b")
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || status.ActiveAttempt.Ordinal != 1 || status.CumulativeAttempts != 1 {
		t.Fatalf("projection=%#v %v", status, err)
	}
	concurrentSettle(t, ctx, store, a.Token, "a-settle")
	if c := concurrentAcquire(t, ctx, store, plan, "c", "c-before-b"); c.State != CompactStateBlocked {
		t.Fatalf("incomplete dependency acquire=%#v", c)
	}
	if result := concurrentSettle(t, ctx, store, b.Token, "b-settle"); result.ItemSettlement == nil {
		t.Fatalf("non-projected settle=%#v", result)
	}
	if c := concurrentAcquire(t, ctx, store, plan, "c", "c"); c.State != CompactStateProceed {
		t.Fatalf("ready dependency acquire=%#v", c)
	}
	status, err = store.Status()
	if err != nil || status.runtimeActiveCount() != 1 || status.runtimeActiveAttemptForOrdinal(3) == nil {
		t.Fatalf("targeted settle=%#v %v", status, err)
	}
}

func TestConcurrentWorkItemProjectionUsesRetainedPlanAuthority(t *testing.T) {
	tasks := func(aChecked bool) string {
		checked := " "
		if aChecked {
			checked = "x"
		}
		return fmt.Sprintf("- [%s] a: A\n- [ ] b: B\n- [ ] c: C\n<!-- gentle-ai.sdd-items/v1\n{\"items\":[{\"id\":\"a\",\"dependsOn\":[],\"workUnit\":\"a\",\"editRoots\":[\"a\"],\"maxAttempts\":2,\"maxChangedLines\":20,\"evidenceGoal\":\"a\"},{\"id\":\"b\",\"dependsOn\":[],\"workUnit\":\"b\",\"editRoots\":[\"b\"],\"maxAttempts\":2,\"maxChangedLines\":20,\"evidenceGoal\":\"b\"},{\"id\":\"c\",\"dependsOn\":[\"a\",\"b\"],\"workUnit\":\"c\",\"editRoots\":[\"c\"],\"maxAttempts\":2,\"maxChangedLines\":20,\"evidenceGoal\":\"c\"}]}\n-->", checked)
	}
	project := func(t *testing.T, store RuntimeStore, runtime RuntimeStatus, checked bool) []WorkItem {
		t.Helper()
		status := itemStatus(t)
		status.ActionContext.WorkspaceRoot = store.Workspace
		status.ActionContext.AllowedEditRoots = []string{store.Workspace}
		status.RuntimeStatus = &runtime
		items, present, err := projectWorkItems(tasks(checked), status)
		if err != nil || !present {
			t.Fatalf("projection=%#v present=%v err=%v", items, present, err)
		}
		return items
	}

	t.Run("passed predecessor makes unchecked dependent ready", func(t *testing.T) {
		ctx, store, plan := concurrentStore(t)
		a, b := concurrentAcquire(t, ctx, store, plan, "a", "projection-a"), concurrentAcquire(t, ctx, store, plan, "b", "projection-b")
		concurrentSettle(t, ctx, store, a.Token, "a-settle-projection")
		concurrentSettle(t, ctx, store, b.Token, "b-settle-projection")
		runtime, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		items := project(t, store, runtime, false)
		if items[0].Done || items[0].Active || items[0].Ready || !items[0].Blocked || !items[2].Ready || items[2].Blocked {
			t.Fatalf("runtime-proven items=%#v", items)
		}
		before := countRuntimeRecords(t, store.Dir)
		aRetry := runtimePlanRequest(t, store, plan, "a", "projection-a-reacquire")
		aRetry.ExpectedRevision = runtime.Revision
		if result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: aRetry}); err != nil || result.State != CompactStateComplete || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("passed item reacquire=%#v err=%v records=%d", result, err, countRuntimeRecords(t, store.Dir))
		}
		changeRoot := filepath.Join(store.Repo, "openspec", "changes", store.Change)
		for path, content := range map[string]string{"proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/item/spec.md": "### Requirement: Item\n#### Scenario: Acquire\n", "tasks.md": tasks(false)} {
			write(t, filepath.Join(changeRoot, path), content)
		}
		if _, err := ResolveItemAcquire(ResolveOptions{CWD: store.Repo, ChangeName: store.Change, ReviewDisabled: true}, "a", "projection-a-resolve"); err == nil || !strings.Contains(err.Error(), "not ready") {
			t.Fatalf("passed item resolve error=%v", err)
		}
		if result := concurrentAcquire(t, ctx, store, plan, "c", "projection-c"); result.State != CompactStateProceed {
			t.Fatalf("runtime-proven acquire=%#v", result)
		}
	})

	t.Run("checkbox without passed provenance stays blocked", func(t *testing.T) {
		ctx, store, plan := concurrentStore(t)
		concurrentAcquire(t, ctx, store, plan, "a", "checkbox-a")
		concurrentAcquire(t, ctx, store, plan, "b", "checkbox-b")
		runtime, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if items := project(t, store, runtime, true); !items[2].Blocked || items[2].Ready {
			t.Fatalf("checkbox-only dependent=%#v", items[2])
		}
		before := countRuntimeRecords(t, store.Dir)
		if result := concurrentAcquire(t, ctx, store, plan, "c", "checkbox-c"); result.Reason != CompactBlockActiveAttempt || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("checkbox-only acquire=%#v records=%d", result, countRuntimeRecords(t, store.Dir))
		}
	})

	t.Run("legacy owner without retained provenance remains serial", func(t *testing.T) {
		ctx, store, plan := concurrentStore(t)
		a := concurrentAcquire(t, ctx, store, plan, "a", "legacy-plan-a")
		concurrentSettle(t, ctx, store, a.Token, "a-settle-legacy")
		completed, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: completed.Revision, RequestID: "legacy-reset", Reason: "open legacy scope", Actor: "maintainer"}); err != nil {
			t.Fatal(err)
		}
		reset, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Begin(ctx, BeginAttemptRequest{ExpectedRevision: reset.Revision, RequestID: "legacy-begin", WorkUnit: "legacy", EvidenceGoal: "legacy", MaxAttempts: 2, MaxChangedLines: 20}); err != nil {
			t.Fatal(err)
		}
		runtime, err := store.Status()
		if err != nil || runtime.itemPlan == nil {
			t.Fatalf("retained plan status=%#v err=%v", runtime, err)
		}
		// The persisted form is unbound because retained-plan replay rejects a
		// new planless item binding. Model the legacy item-bound projection that
		// an older status producer could still surface; it must remain serial.
		active := runtime.runtimeActiveAttemptForOrdinal(2)
		owner := runtime.ownership.objectives[active.ObjectiveID]
		active.ItemID, active.ItemEditRoots = "b", []string{filepath.Join(store.Workspace, "b")}
		owner.objective.ItemID, owner.objective.ItemEditRoots = active.ItemID, active.ItemEditRoots
		if items := project(t, store, runtime, false); !items[2].Blocked || items[2].Ready {
			t.Fatalf("legacy item-bound projection=%#v", items[2])
		}
		before := countRuntimeRecords(t, store.Dir)
		if result := concurrentAcquire(t, ctx, store, plan, "c", "legacy-c"); result.Reason != CompactBlockActiveAttempt || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("legacy active acquire=%#v records=%d", result, countRuntimeRecords(t, store.Dir))
		}
	})

	t.Run("persisted planless item owner stays serial", func(t *testing.T) {
		ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
		for _, root := range []string{"a", "b", "c"} {
			mkdir(t, filepath.Join(repo, root))
		}
		store := mustRuntimeStore(t, repo, "legacy-projection")
		active, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "legacy-a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, ItemID: "a", ItemEditRoots: []string{filepath.Join(repo, "a")}})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := store.Status()
		if err != nil || runtime.itemPlan != nil {
			t.Fatalf("legacy runtime=%#v err=%v", runtime, err)
		}
		if items := project(t, store, runtime, false); !items[0].Active || !items[1].Blocked || items[1].Ready {
			t.Fatalf("legacy projection=%#v", items)
		}
		before := countRuntimeRecords(t, store.Dir)
		request := BeginAttemptRequest{ExpectedRevision: active.Revision, RequestID: "legacy-b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, ItemID: "b", ItemEditRoots: []string{filepath.Join(repo, "b")}}
		if result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request}); err != nil || result.Reason != CompactBlockActiveAttempt || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("legacy acquire=%#v err=%v records=%d", result, err, countRuntimeRecords(t, store.Dir))
		}
	})

	t.Run("final passed item remains pending checkbox projection", func(t *testing.T) {
		ctx, store, plan := concurrentStore(t)
		a := concurrentAcquire(t, ctx, store, plan, "a", "final-a")
		concurrentSettle(t, ctx, store, a.Token, "a-settle-final")
		runtime, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if items := project(t, store, runtime, false); items[0].Done || items[0].Active || items[0].Ready || !items[0].Blocked {
			t.Fatalf("final passed projection=%#v", items[0])
		}
		before := countRuntimeRecords(t, store.Dir)
		request := runtimePlanRequest(t, store, plan, "a", "final-a-reacquire")
		request.ExpectedRevision = runtime.Revision
		if result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request}); err != nil || result.State != CompactStateComplete || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("final passed acquire=%#v err=%v records=%d", result, err, countRuntimeRecords(t, store.Dir))
		}
	})
}

func TestConcurrentItemReplayRejectsPlanEntryProvenanceForgery(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*BeginAttemptRequest, *runtimeBeginEvent, itemPlanCandidate)
		want   string
	}{
		{"retained plan digest mismatch", func(request *BeginAttemptRequest, event *runtimeBeginEvent, plan itemPlanCandidate) {
			request.itemPlan.Plan.Digest = runtimeTestHash('d')
			event.ItemPlanDigest = request.itemPlan.Plan.Digest
		}, "item plan digest does not match replay state"},
		{"item entry digest mismatch", func(request *BeginAttemptRequest, event *runtimeBeginEvent, plan itemPlanCandidate) {
			a, _ := itemPlanEntryForID(plan, "a")
			request.itemPlan.EntryDigest = itemPlanEntryDigest(a)
			event.ItemPlanEntryDigest = request.itemPlan.EntryDigest
		}, "item plan selected entry does not match replay state"},
		{"concurrent objective generation mismatch", func(request *BeginAttemptRequest, event *runtimeBeginEvent, plan itemPlanCandidate) {
			event.ObjectiveGeneration++
			event.ObjectiveID = runtimeObjectiveIDForBinding("provenance-forgery", event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, event.ObjectiveGeneration, event.ItemID, event.ItemEditRoots)
		}, "concurrent begin record is not semantically admissible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
			for _, root := range []string{"a", "b"} {
				mkdir(t, filepath.Join(repo, root))
			}
			store := mustRuntimeStore(t, repo, "provenance-forgery")
			plan, err := newItemPlanCandidate([]WorkItem{{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"a"}}, {ID: "b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"b"}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			started := concurrentAcquire(t, ctx, store, plan, "a", "provenance-a")
			request := runtimePlanRequest(t, store, plan, "b", "provenance-b")
			request.ExpectedRevision = started.Token
			entry, _ := itemPlanEntryForID(plan, "b")
			event := &runtimeBeginEvent{ObjectiveGeneration: 2, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, ItemID: request.ItemID, ItemEditRoots: request.ItemEditRoots, ItemPlanDigest: plan.Digest, ItemPlanEntryDigest: itemPlanEntryDigest(entry), Ordinal: 2, BeginCandidateIdentity: runtimeTestHash('b'), BeginCandidateTree: strings.Repeat("b", 40), BeginWorktree: store.Workspace, EffectiveWorktree: store.Workspace}
			event.ObjectiveID = runtimeObjectiveIDForBinding(store.Change, event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, event.ObjectiveGeneration, event.ItemID, event.ItemEditRoots)
			tc.mutate(&request, event, plan)
			record := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: started.Token, Operation: runtimeOperationBegin, RequestID: request.RequestID, RequestDigest: runtimeBeginRequestDigest(request), Begin: event}
			if err := validateRuntimeRecordShape(record); err != nil {
				t.Fatalf("integrity-valid provenance forgery failed shape validation: %v", err)
			}
			revision, payload, err := runtimeRecordRevision(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.publishRecord(revision, payload); err != nil {
				t.Fatal(err)
			}
			if err := store.publishHead(revision); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Status(); err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "request digest") || strings.Contains(err.Error(), "shape") {
				t.Fatalf("provenance replay error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestConcurrentSymlinkSettlementRetryAndCLI(t *testing.T) {
	ctx, store, plan := concurrentStore(t)
	a, b := concurrentAcquire(t, ctx, store, plan, "a", "a"), concurrentAcquire(t, ctx, store, plan, "b", "b")
	first := concurrentSettle(t, ctx, store, a.Token, "a-settle")
	concurrentSettle(t, ctx, store, b.Token, "b-settle")
	before := countRuntimeRecords(t, store.Dir)
	retry := concurrentSettle(t, ctx, store, a.Token, "a-settle")
	if !reflect.DeepEqual(first.ItemSettlement, retry.ItemSettlement) || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("immutable retry first=%#v retry=%#v records=%d", first, retry, countRuntimeRecords(t, store.Dir))
	}
	for index, token := range []string{"", runtimeTestHash('f'), b.Token} {
		before := countRuntimeRecords(t, store.Dir)
		result, err := store.Settle(ctx, CompactSettleRequest{Token: token, RequestID: fmt.Sprintf("wrong-%d", index), Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
		if (token != "" && err != nil) || result.ItemSettlement != nil || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("token=%q result=%#v err=%v records=%d", token, result, err, countRuntimeRecords(t, store.Dir))
		}
	}

	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"a", "b"} {
		mkdir(t, filepath.Join(repo, root))
	}
	if err := os.Symlink(filepath.Join(repo, "a"), filepath.Join(repo, "alias")); err != nil {
		t.Fatal(err)
	}
	symlinkStore := mustRuntimeStore(t, repo, "symlink")
	symlinkPlan, err := newItemPlanCandidate([]WorkItem{{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"a"}}, {ID: "b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"alias"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	concurrentAcquire(t, ctx, symlinkStore, symlinkPlan, "a", "symlink-a")
	before = countRuntimeRecords(t, symlinkStore.Dir)
	if result := concurrentAcquire(t, ctx, symlinkStore, symlinkPlan, "b", "symlink-b"); result.Reason != CompactBlockActiveAttempt || countRuntimeRecords(t, symlinkStore.Dir) != before {
		t.Fatalf("symlink overlap=%#v records=%d", result, countRuntimeRecords(t, symlinkStore.Dir))
	}
}

func TestConcurrentItemReplayRejectsSemanticForgery(t *testing.T) {
	for _, tc := range []struct {
		name, aRoot, bRoot string
		dependsOn          []string
	}{
		{"equal roots", "a", "a", nil},
		{"descendant overlap", "a", "a/child", nil},
		{"ancestor overlap", "a/child", "a", nil},
		{"unsatisfied dependency", "a", "b", []string{"c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
			for _, root := range []string{"a", "a/child", "b", "c"} {
				mkdir(t, filepath.Join(repo, root))
			}
			store := mustRuntimeStore(t, repo, "semantic-forgery")
			plan, err := newItemPlanCandidate([]WorkItem{
				{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{tc.aRoot}},
				{ID: "b", DependsOn: tc.dependsOn, WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{tc.bRoot}},
				{ID: "c", WorkUnit: "c", EvidenceGoal: "c", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"c"}},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			started := concurrentAcquire(t, ctx, store, plan, "a", "forged-a")
			request := runtimePlanRequest(t, store, plan, "b", "forged-b")
			request.ExpectedRevision = started.Token
			event := &runtimeBeginEvent{ObjectiveGeneration: 2, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
				MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, ItemID: request.ItemID, ItemEditRoots: request.ItemEditRoots,
				ItemPlanDigest: plan.Digest, ItemPlanEntryDigest: request.itemPlan.EntryDigest, Ordinal: 2,
				BeginCandidateIdentity: runtimeTestHash('b'), BeginCandidateTree: strings.Repeat("b", 40), BeginWorktree: store.Workspace, EffectiveWorktree: store.Workspace}
			event.ObjectiveID = runtimeObjectiveIDForBinding(store.Change, event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, event.ObjectiveGeneration, event.ItemID, event.ItemEditRoots)
			record := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: started.Token, Operation: runtimeOperationBegin, RequestID: request.RequestID, RequestDigest: runtimeBeginRequestDigest(request), Begin: event}
			revision, payload, err := runtimeRecordRevision(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.publishRecord(revision, payload); err != nil || store.publishHead(revision) != nil {
				t.Fatalf("publish integrity-valid forgery: %v", err)
			}
			_, err = store.Status()
			if err == nil || !strings.Contains(err.Error(), "concurrent begin record is not semantically admissible") || strings.Contains(err.Error(), "digest") || strings.Contains(err.Error(), "shape") {
				t.Fatalf("semantic replay error=%v", err)
			}
		})
	}
}

func TestConcurrentItemReplayRejectsPlanlessPassedDependencyForgery(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"a", "b", "c"} {
		mkdir(t, filepath.Join(repo, root))
	}
	store := mustRuntimeStore(t, repo, "planless-dependency-forgery")
	roots := func(id string) []string { return []string{filepath.Join(repo, id)} }

	// A predates retained-plan provenance: it is a valid v2 item-bound attempt,
	// but cannot satisfy a later retained plan's dependency.
	a, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "legacy-a-begin", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, ItemID: "a", ItemEditRoots: roots("a")})
	if err != nil {
		t.Fatal(err)
	}
	passed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: a.Revision, RequestID: "legacy-a-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "legacy item passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "a", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"a"}},
		{ID: "b", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"b"}},
		{ID: "c", DependsOn: []string{"a"}, WorkUnit: "c", EvidenceGoal: "c", MaxAttempts: 2, MaxChangedLines: 20, EditRoots: []string{"c"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bRequest := runtimePlanRequest(t, store, plan, "b", "retained-b")
	bRequest.ExpectedRevision = passed.Revision
	b, err := store.Begin(ctx, bRequest)
	if err != nil {
		t.Fatal(err)
	}

	cRequest := runtimePlanRequest(t, store, plan, "c", "forged-c")
	cRequest.ExpectedRevision = b.Revision
	cEntry, _ := itemPlanEntryForID(plan, "c")
	event := &runtimeBeginEvent{
		ObjectiveGeneration: b.ObjectiveGeneration + 1, WorkUnit: cRequest.WorkUnit, EvidenceGoal: cRequest.EvidenceGoal,
		MaxAttempts: cRequest.MaxAttempts, MaxChangedLines: cRequest.MaxChangedLines, ItemID: cRequest.ItemID, ItemEditRoots: cRequest.ItemEditRoots,
		ItemPlanDigest: plan.Digest, ItemPlanEntryDigest: itemPlanEntryDigest(cEntry), Ordinal: b.NextOrdinal,
		BeginCandidateIdentity: runtimeTestHash('c'), BeginCandidateTree: strings.Repeat("c", 40), BeginWorktree: store.Workspace, EffectiveWorktree: store.Workspace,
	}
	event.ObjectiveID = runtimeObjectiveIDForBinding(store.Change, event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, event.ObjectiveGeneration, event.ItemID, event.ItemEditRoots)
	record := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: b.Revision, Operation: runtimeOperationBegin, RequestID: cRequest.RequestID, RequestDigest: runtimeBeginRequestDigest(cRequest), Begin: event}
	if err := validateRuntimeRecordShape(record); err != nil {
		t.Fatalf("planless dependency forgery failed shape validation: %v", err)
	}
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil || store.publishHead(revision) != nil {
		t.Fatalf("publish integrity-valid planless dependency forgery: %v", err)
	}
	status, err := store.Status()
	if err == nil || !strings.Contains(err.Error(), "concurrent begin record is not semantically admissible") || strings.Contains(err.Error(), "digest") || strings.Contains(err.Error(), "shape") || status.runtimeActiveCount() == 3 {
		t.Fatalf("planless dependency semantic replay status=%#v error=%v", status, err)
	}
}

func TestConcurrentLifecycleStructuralReplayRejectsMultipleActiveOwners(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*testing.T, RuntimeStore, RuntimeStatus) runtimeRecord
		want  string
	}{
		{"reset", func(t *testing.T, store RuntimeStore, status RuntimeStatus) runtimeRecord {
			request := ResetObjectiveRequest{ExpectedRevision: status.Revision, RequestID: "forged-reset", Reason: "forged", Actor: "maintainer"}
			return runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: status.Revision, Operation: runtimeOperationReset, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-reset-request/v1", request), Reset: &runtimeResetEvent{PreviousObjectiveID: status.Objective.ID, PreviousGeneration: status.Objective.Generation, ResetCandidateIdentity: runtimeTestHash('a'), ResetCandidateTree: strings.Repeat("a", 40), Reason: request.Reason, Actor: request.Actor}}
		}, "objective reset is not a valid successor"},
		{"rescope", func(t *testing.T, store RuntimeStore, status RuntimeStatus) runtimeRecord {
			objective := status.Objective
			request := RescopeObjectiveRequest{ExpectedRevision: status.Revision, RequestID: "forged-rescope", WorkUnit: "narrow", EvidenceGoal: "narrow", MaxAttempts: 1, MaxChangedLines: 10, Reason: "forged", Actor: "maintainer"}
			return runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: status.Revision, Operation: runtimeOperationRescope, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request), Rescope: &runtimeRescopeEvent{PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation, PreviousMaxAttempts: objective.MaxAttempts, PreviousMaxChangedLines: objective.MaxChangedLines, RescopeCandidateIdentity: runtimeTestHash('b'), RescopeCandidateTree: strings.Repeat("b", 40), ObjectiveID: runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, runtimeTestHash('b'), status.ObjectiveGeneration+1), ObjectiveGeneration: status.ObjectiveGeneration + 1, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Reason: request.Reason, Actor: request.Actor}}
		}, "objective rescope is not a valid successor"},
		{"advance enclosing begin", func(t *testing.T, store RuntimeStore, status RuntimeStatus) runtimeRecord {
			request := BeginAttemptRequest{ExpectedRevision: status.Revision, RequestID: "forged-advance", WorkUnit: "successor", EvidenceGoal: "successor", MaxAttempts: 1, MaxChangedLines: 10}
			event := &runtimeBeginEvent{ObjectiveGeneration: status.ObjectiveGeneration + 1, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Ordinal: status.NextOrdinal, BeginCandidateIdentity: runtimeTestHash('c'), BeginCandidateTree: strings.Repeat("c", 40)}
			event.ObjectiveID = runtimeObjectiveID(store.Change, event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, event.ObjectiveGeneration)
			return runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: status.Revision, Operation: runtimeOperationAdvance, RequestID: request.RequestID, RequestDigest: runtimeBeginRequestDigest(request), Begin: event, Advance: &runtimeAdvanceEvent{PreviousObjectiveID: status.Objective.ID, PreviousGeneration: status.Objective.Generation, PreviousWorkUnit: status.Objective.WorkUnit}}
		}, "objective advance is not a valid successor"},
		{"handoff", func(t *testing.T, store RuntimeStore, status RuntimeStatus) runtimeRecord {
			request := HandoffAttemptRequest{ExpectedRevision: status.Revision, RequestID: "forged-handoff", DestinationWorktree: filepath.Join(store.Workspace, "destination")}
			return runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: status.Revision, Operation: runtimeOperationHandoff, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-handoff-request/v1", request), Handoff: &RuntimeHandoff{Ordinal: 1, SourceWorktree: store.Workspace, DestinationWorktree: request.DestinationWorktree, CommonDir: store.commonDir, ExpectedRevision: status.Revision, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-handoff-request/v1", request), DestinationCandidateIdentity: runtimeTestHash('d'), DestinationCandidateTree: strings.Repeat("d", 40)}}
		}, "handoff record does not match the active attempt"},
		{"remediation finish", func(t *testing.T, store RuntimeStore, status RuntimeStatus) runtimeRecord {
			fixture := newRuntimeUnchangedBindingFixture(t, store.Change)
			binding := fixture.binding
			binding.Change, binding.Revision = store.Change, ""
			binding.Revision = bindingDigest(binding)
			request := FinishAttemptRequest{ExpectedRevision: status.Revision, RequestID: "forged-remediation", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('e'), Diagnosis: "forged", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none", ExpectedBindingRevision: binding.Revision, SuccessorLineageID: binding.Lineage, RemediatesEvidenceRevision: runtimeTestHash('f')}
			return runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: status.Revision, Operation: runtimeOperationFinishRemediation, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", request), Finish: &runtimeFinishEvent{Ordinal: 1, FinishCandidateIdentity: runtimeTestHash('e'), FinishCandidateTree: strings.Repeat("e", 40), Outcome: request.Outcome, EvidenceRevision: request.EvidenceRevision, Diagnosis: request.Diagnosis, HarnessDisposition: request.HarnessDisposition, CleanupEvidence: request.CleanupEvidence, ProcessEvidence: request.ProcessEvidence, RemediatesEvidenceRevision: request.RemediatesEvidenceRevision}, Binding: &runtimeBindingEvent{ExpectedRevision: request.ExpectedBindingRevision, Current: binding}}
		}, "atomic remediation binding does not match replay state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, plan := concurrentStore(t)
			concurrentAcquire(t, ctx, store, plan, "a", "a")
			concurrentAcquire(t, ctx, store, plan, "b", "b")
			status, err := store.Status()
			if err != nil || status.runtimeActiveCount() != 2 {
				t.Fatalf("real A/B active status=%#v err=%v", status, err)
			}
			record := tc.build(t, store, status)
			if err := validateRuntimeRecordShape(record); err != nil {
				t.Fatalf("%s record failed shape validation: %v", tc.name, err)
			}
			revision, payload, err := runtimeRecordRevision(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.publishRecord(revision, payload); err != nil {
				t.Fatal(err)
			}
			if err := store.publishHead(revision); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Status(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s replay error=%v, want %q", tc.name, err, tc.want)
			}
		})
	}

	// An exact token targets one owner and is intentionally permitted; singular
	// Finish is the enclosing serial writer and refuses multiple active owners.
	ctx, store, plan := concurrentStore(t)
	concurrentAcquire(t, ctx, store, plan, "a", "finish-a")
	concurrentAcquire(t, ctx, store, plan, "b", "finish-b")
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: status.Revision, RequestID: "singular-finish", Outcome: AttemptInterrupted, Diagnosis: "serial writer", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"}); !errors.Is(err, ErrRuntimeAttemptActive) {
		t.Fatalf("singular Finish error=%v", err)
	}
}

func TestConcurrentLifecycleRefusesMultipleActiveOwners(t *testing.T) {
	ctx, store, plan := concurrentStore(t)
	concurrentAcquire(t, ctx, store, plan, "a", "a")
	concurrentAcquire(t, ctx, store, plan, "b", "b")
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	// Advance is emitted only by Begin's successor branch; exercising Begin here
	// proves that enclosing writer refuses before it can choose an objective.
	// Remediation is Finish with binding/remediates fields, not a separate API.
	for _, call := range []struct {
		name string
		call func() error
	}{
		{"finish", func() error {
			_, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: status.Revision, RequestID: "finish", Outcome: AttemptInterrupted, Diagnosis: "stop", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"})
			return err
		}},
		{"remediation finish", func() error {
			_, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: status.Revision, RequestID: "remediation", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "repair", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none", ExpectedBindingRevision: runtimeTestHash('b'), SuccessorLineageID: "successor", RemediatesEvidenceRevision: runtimeTestHash('f')})
			return err
		}},
		{"reset", func() error {
			_, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: status.Revision, RequestID: "reset", Reason: "stop", Actor: "maintainer"})
			return err
		}},
		{"rescope", func() error {
			_, err := store.Rescope(ctx, RescopeObjectiveRequest{ExpectedRevision: status.Revision, RequestID: "rescope", WorkUnit: "narrow", EvidenceGoal: "narrow", MaxAttempts: 1, MaxChangedLines: 1, Reason: "stop", Actor: "maintainer"})
			return err
		}},
		{"advance begin", func() error {
			_, err := store.Begin(ctx, BeginAttemptRequest{ExpectedRevision: status.Revision, RequestID: "advance", WorkUnit: "advance", EvidenceGoal: "advance", MaxAttempts: 1, MaxChangedLines: 1})
			return err
		}},
		{"handoff", func() error {
			_, err := store.Handoff(ctx, HandoffAttemptRequest{ExpectedRevision: status.Revision, RequestID: "handoff", DestinationWorktree: store.Workspace})
			return err
		}},
	} {
		if err := call.call(); !errors.Is(err, ErrRuntimeAttemptActive) || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("%s err=%v records=%d", call.name, err, countRuntimeRecords(t, store.Dir))
		}
	}
}
