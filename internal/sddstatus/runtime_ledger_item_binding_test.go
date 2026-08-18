package sddstatus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLedgerUnboundBeginRetainsLegacyIdentityAndReplay(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(ctx, repo, "unbound-item-binding")
	if err != nil {
		t.Fatal(err)
	}
	request := BeginAttemptRequest{RequestID: "unbound-begin", WorkUnit: "apply", EvidenceGoal: "prove legacy identity", MaxAttempts: 1, MaxChangedLines: 20}
	started, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if started.Objective == nil || started.Objective.ID != runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, started.Objective.InitialCandidateIdentity, 1) || runtimeBeginRequestDigest(request) != runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request) {
		t.Fatalf("unbound begin changed legacy identity: %#v", started.Objective)
	}
	replayed, err := store.Begin(ctx, request)
	if err != nil || replayed.Revision != started.Revision {
		t.Fatalf("unbound replay = %#v, %v", replayed, err)
	}
	if replay, err := store.load(); err != nil || replay.itemPlan != nil {
		t.Fatalf("legacy replay established item plan: %#v, %v", replay.itemPlan, err)
	}
	record, err := store.loadRecord(started.Revision)
	if err != nil || record.Begin.ItemID != "" || len(record.Begin.ItemEditRoots) != 0 || record.RequestDigest != runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request) {
		t.Fatalf("unbound record changed: %#v, %v", record, err)
	}
}

func TestRuntimeItemPlanUsesPortableRootsAcrossLinkedWorktrees(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	storeA := mustRuntimeStore(t, repo, "item-plan-worktrees")
	plan := runtimeTestItemPlan(t)
	first := runtimePlanRequest(t, storeA, plan, "build", "portable-first")
	started, err := storeA.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: first})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.Settle(ctx, CompactSettleRequest{Token: started.Token, RequestID: "portable-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"}); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "portable-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "portable-plan", worktree)
	storeB, err := OpenRuntimeStore(ctx, worktree, "item-plan-worktrees")
	if err != nil {
		t.Fatal(err)
	}
	second := runtimePlanRequest(t, storeB, plan, "verify", "portable-second")
	if second.itemPlan.Plan.Digest != first.itemPlan.Plan.Digest || second.ItemEditRoots[0] == first.ItemEditRoots[0] {
		t.Fatalf("portable candidate/bindings = %#v %#v", first, second)
	}
	advanced, err := storeB.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: second})
	if err != nil || advanced.State != CompactStateProceed {
		t.Fatalf("linked worktree acquire = %#v, %v", advanced, err)
	}
	record, err := storeB.loadRecord(advanced.Token)
	if err != nil || !runtimeItemBindingEqual("verify", second.ItemEditRoots, record.Begin.ItemID, record.Begin.ItemEditRoots) {
		t.Fatalf("linked worktree binding = %#v, %v", record, err)
	}
}

func TestRuntimeLedgerItemBindingPersistsAndGuardsIdentity(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	roots := []string{filepath.Join(repo, "a"), filepath.Join(repo, "b")}
	for _, root := range roots {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenRuntimeStore(ctx, repo, "item-binding")
	if err != nil {
		t.Fatal(err)
	}
	bound := BeginAttemptRequest{RequestID: "bound-begin", WorkUnit: "apply", EvidenceGoal: "prove item binding", MaxAttempts: 2, MaxChangedLines: 20, ItemID: "item-a", ItemEditRoots: roots}
	started, err := store.Begin(ctx, bound)
	if err != nil {
		t.Fatal(err)
	}
	if started.Objective == nil || started.ActiveAttempt == nil || started.Objective.ItemID != "item-a" || !runtimeItemBindingEqual("item-a", roots, started.ActiveAttempt.ItemID, started.ActiveAttempt.ItemEditRoots) {
		t.Fatalf("binding missing from snapshots: %#v", started)
	}
	if started.Objective.ID == runtimeObjectiveID(store.Change, bound.WorkUnit, bound.EvidenceGoal, started.Objective.InitialCandidateIdentity, 1) || started.Objective.ID == runtimeObjectiveIDForBinding(store.Change, bound.WorkUnit, bound.EvidenceGoal, started.Objective.InitialCandidateIdentity, 1, "item-b", roots) {
		t.Fatalf("bound objective identity did not bind item: %q", started.Objective.ID)
	}
	otherRoots := append([]string{}, roots...)
	otherRoots[1] = roots[0]
	if started.Objective.ID == runtimeObjectiveIDForBinding(store.Change, bound.WorkUnit, bound.EvidenceGoal, started.Objective.InitialCandidateIdentity, 1, "item-a", otherRoots) {
		t.Fatal("bound objective identity did not bind roots")
	}

	bound.ExpectedRevision = ""
	replayed, err := store.Begin(ctx, bound)
	if err != nil || replayed.Revision != started.Revision {
		t.Fatalf("exact begin replay = %#v, %v", replayed, err)
	}
	for _, changed := range []BeginAttemptRequest{{ItemID: "item-b", ItemEditRoots: roots}, {ItemID: "item-a", ItemEditRoots: []string{roots[0]}}} {
		changed.RequestID, changed.WorkUnit, changed.EvidenceGoal, changed.MaxAttempts, changed.MaxChangedLines = bound.RequestID, bound.WorkUnit, bound.EvidenceGoal, bound.MaxAttempts, bound.MaxChangedLines
		if _, err := store.Begin(ctx, changed); !errors.Is(err, ErrRuntimeRequestConflict) {
			t.Fatalf("changed request replay error = %v", err)
		}
	}
	finished, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "bound-finish", Outcome: AttemptInterrupted, Diagnosis: "interrupted", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Attempts) != 1 || finished.Attempts[0].ItemID != "item-a" {
		t.Fatalf("terminal attempt lost binding: %#v", finished.Attempts)
	}
	bound.ExpectedRevision, bound.RequestID, bound.ItemID = finished.Revision, "bound-mismatch", "item-b"
	if _, err := store.Begin(ctx, bound); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("changed subsequent binding error = %v", err)
	}
}

func TestRuntimeLedgerItemBindingRejectsMalformedAndForgedRecords(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	root := filepath.Join(repo, "item-root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(ctx, repo, "item-binding-forge")
	if err != nil {
		t.Fatal(err)
	}
	base := BeginAttemptRequest{RequestID: "invalid-item", WorkUnit: "apply", EvidenceGoal: "prove validation", MaxAttempts: 1, MaxChangedLines: 20}
	for _, invalid := range []BeginAttemptRequest{{ItemID: "item-a"}, {ItemEditRoots: []string{root}}, {ItemID: "item-a", ItemEditRoots: []string{root, root}}, {ItemID: "item-a", ItemEditRoots: []string{"relative"}}} {
		invalid.RequestID, invalid.WorkUnit, invalid.EvidenceGoal, invalid.MaxAttempts, invalid.MaxChangedLines = base.RequestID, base.WorkUnit, base.EvidenceGoal, base.MaxAttempts, base.MaxChangedLines
		if _, err := store.Begin(ctx, invalid); err == nil {
			t.Fatalf("malformed binding accepted: %#v", invalid)
		}
	}
	if records := countRuntimeRecords(t, store.Dir); records != 0 {
		t.Fatalf("invalid bindings mutated ledger: %d", records)
	}
	base.RequestID, base.ItemID, base.ItemEditRoots = "bound-record", "item-a", []string{root}
	started, err := store.Begin(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.loadRecord(started.Revision)
	if err != nil {
		t.Fatal(err)
	}
	record.Begin.ItemID = "item-b"
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
	if _, err := store.Status(); err == nil {
		t.Fatal("forged item binding replayed")
	}
}

func TestRuntimeLedgerItemBindingResetClearsAndRescopeRefuses(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	root := filepath.Join(repo, "item-root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(ctx, repo, "item-binding-transition")
	if err != nil {
		t.Fatal(err)
	}
	request := BeginAttemptRequest{RequestID: "bound-begin", WorkUnit: "apply", EvidenceGoal: "prove transition", MaxAttempts: 1, MaxChangedLines: 20, ItemID: "item-a", ItemEditRoots: []string{root}}
	started, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "bound-finish", Outcome: AttemptInterrupted, Diagnosis: "interrupted", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: terminal.Revision, RequestID: "bound-reset", Reason: "new scope", Actor: "maintainer"})
	if err != nil || reset.Objective != nil || len(reset.Attempts) != 1 || reset.Attempts[0].ItemID != "item-a" {
		t.Fatalf("reset = %#v, %v", reset, err)
	}
	request.ExpectedRevision, request.RequestID, request.MaxAttempts = reset.Revision, "bound-begin-two", 2
	started, err = store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err = store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "bound-finish-two", Outcome: AttemptInterrupted, Diagnosis: "interrupted", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Rescope(ctx, RescopeObjectiveRequest{ExpectedRevision: terminal.Revision, RequestID: "bound-rescope", WorkUnit: "narrower", EvidenceGoal: "prove narrower", MaxAttempts: 2, MaxChangedLines: 10, Reason: "narrow scope", Actor: "maintainer"})
	if !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("bound rescope error = %v", err)
	}
}

func TestRuntimeItemPlanRetainsAndRejectsDrift(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"src", "verify"} {
		mkdir(t, filepath.Join(repo, root))
	}
	store := mustRuntimeStore(t, repo, "item-plan")
	plan := runtimeTestItemPlan(t)
	first := runtimePlanRequest(t, store, plan, "build", "plan-first")
	started, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: first})
	if err != nil || started.State != CompactStateProceed {
		t.Fatalf("first acquire = %#v, %v", started, err)
	}
	record, err := store.loadRecord(started.Token)
	if err != nil || record.Begin.ItemPlan == nil || record.Begin.ItemPlanDigest != plan.Digest || record.Begin.ItemPlanEntryDigest != itemPlanEntryDigest(plan.Items[0]) {
		t.Fatalf("first plan record = %#v, %v", record, err)
	}
	if _, err := store.Settle(ctx, CompactSettleRequest{Token: started.Token, RequestID: "plan-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"}); err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func(*itemPlanCandidate){
		func(plan *itemPlanCandidate) { plan.Items[1].EditRoots = []string{"src"} },
		func(plan *itemPlanCandidate) { plan.Items[1].DependsOn = nil },
		func(plan *itemPlanCandidate) { plan.Items[1].MaxAttempts++ },
		func(plan *itemPlanCandidate) { plan.Items[1].WorkUnit = "other" },
		func(plan *itemPlanCandidate) { plan.Items[1].EvidenceGoal = "other proof" },
	} {
		drifted := *cloneItemPlan(&plan)
		mutate(&drifted)
		drifted.Digest = itemPlanDigest(drifted)
		before := countRuntimeRecords(t, store.Dir)
		result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, drifted, "verify", "plan-drift")})
		if err != nil || result.State != CompactStateBlocked || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("drift acquire = %#v, %v records=%d", result, err, countRuntimeRecords(t, store.Dir))
		}
	}

	second := runtimePlanRequest(t, store, plan, "verify", "plan-second")
	advanced, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: second})
	if err != nil || advanced.State != CompactStateProceed {
		t.Fatalf("matching successor = %#v, %v", advanced, err)
	}
	record, err = store.loadRecord(advanced.Token)
	if err != nil || record.Begin.ItemPlan != nil || record.Begin.ItemPlanDigest != plan.Digest || record.Begin.ItemPlanEntryDigest != itemPlanEntryDigest(plan.Items[1]) {
		t.Fatalf("successor plan record = %#v, %v", record, err)
	}
	if replayed, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: second}); err != nil || replayed != advanced {
		t.Fatalf("matching retry = %#v, %v", replayed, err)
	}
	drifted := *cloneItemPlan(&plan)
	drifted.Items[1].MaxChangedLines++
	drifted.Digest = itemPlanDigest(drifted)
	if result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, drifted, "verify", "plan-second")}); err != nil || result.Reason != CompactBlockInvalidContinuation {
		t.Fatalf("drifted retry = %#v, %v", result, err)
	}
}

func TestRuntimeItemPlanForgedRecordsFailReplay(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	mkdir(t, filepath.Join(repo, "src"))
	mkdir(t, filepath.Join(repo, "verify"))
	store := mustRuntimeStore(t, repo, "item-plan-forged")
	plan := runtimeTestItemPlan(t)
	started, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "build", "plan-forged")})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*runtimeBeginEvent){
		func(event *runtimeBeginEvent) { event.ItemPlan.Items[0].EditRoots[0] = "verify" },
		func(event *runtimeBeginEvent) { event.ItemPlan.Items[0].InitiallyDone = nil },
		func(event *runtimeBeginEvent) { event.ItemPlan.Items[0].InitiallyDone = boolPointer(true) },
		func(event *runtimeBeginEvent) { event.ItemPlanDigest = runtimeTestHash('d') },
		func(event *runtimeBeginEvent) { event.ItemPlanEntryDigest = runtimeTestHash('e') },
		func(event *runtimeBeginEvent) { event.ItemID = "verify" },
		func(event *runtimeBeginEvent) { event.ItemPlan = nil },
	} {
		record, err := store.loadRecord(started.Token)
		if err != nil {
			t.Fatal(err)
		}
		record.Begin.ItemPlan = cloneItemPlan(record.Begin.ItemPlan)
		mutate(record.Begin)
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
		if _, err := store.Status(); err == nil {
			t.Fatal("forged item plan replayed")
		}
		if err := store.publishHead(started.Token); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeItemPlanRejectsPublicTamperingAndJSONInjection(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	for _, root := range []string{"src", "verify"} {
		mkdir(t, filepath.Join(repo, root))
	}
	plan := runtimeTestItemPlan(t)
	for _, tamper := range []func(*BeginAttemptRequest){
		func(request *BeginAttemptRequest) { request.ItemID = "verify" },
		func(request *BeginAttemptRequest) { request.ItemEditRoots = []string{filepath.Join(repo, "verify")} },
		func(request *BeginAttemptRequest) { request.WorkUnit = "verify" },
		func(request *BeginAttemptRequest) { request.EvidenceGoal = "other" },
		func(request *BeginAttemptRequest) { request.MaxAttempts++ },
		func(request *BeginAttemptRequest) { request.MaxChangedLines++ },
	} {
		store := mustRuntimeStore(t, repo, "item-plan-tamper")
		request := runtimePlanRequest(t, store, plan, "build", "tampered")
		tamper(&request)
		result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
		if err != nil || result.State != CompactStateBlocked || countRuntimeRecords(t, store.Dir) != 0 {
			t.Fatalf("tampered acquire = %#v, %v", result, err)
		}
	}

	store := mustRuntimeStore(t, repo, "item-plan-json")
	payload := []byte(`{"request_id":"json-injection","work_unit":"build","evidence_goal":"compile","max_attempts":1,"max_changed_lines":20,"item_id":"build","item_edit_roots":["` + filepath.Join(repo, "src") + `"],"item_plan":{"digest":"forged"}}`)
	var injected BeginAttemptRequest
	if err := json.Unmarshal(payload, &injected); err != nil || injected.itemPlan != nil {
		t.Fatalf("JSON injected private plan: %#v, %v", injected.itemPlan, err)
	}
	started, err := store.Begin(ctx, injected)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.loadRecord(started.Revision)
	if err != nil || record.Begin.ItemPlan != nil {
		t.Fatalf("JSON established plan authority: %#v, %v", record, err)
	}
}

func TestRuntimeItemPlanRejectsPlanlessBoundBeginAndRetainsGenericSerialPath(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	mkdir(t, filepath.Join(repo, "src"))
	mkdir(t, filepath.Join(repo, "verify"))
	store := mustRuntimeStore(t, repo, "item-plan-planless")
	plan := runtimeTestItemPlan(t)
	request := runtimePlanRequest(t, store, plan, "build", "planless-first")
	started, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "planless-finish", Outcome: AttemptInterrupted,
		Diagnosis: "interrupted", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	planless := runtimePlanRequest(t, store, plan, "verify", "planless-bound")
	planless.ExpectedRevision, planless.itemPlan = terminal.Revision, nil
	before := countRuntimeRecords(t, store.Dir)
	if _, err := store.Begin(ctx, planless); err == nil || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("planless bound begin mutated authority: %v", err)
	}

	reset, err := store.Reset(ctx, ResetObjectiveRequest{ExpectedRevision: terminal.Revision, RequestID: "planless-reset", Reason: "serial compatibility", Actor: "maintainer"})
	if err != nil {
		t.Fatal(err)
	}
	generic := BeginAttemptRequest{ExpectedRevision: reset.Revision, RequestID: "planless-generic", WorkUnit: "generic", EvidenceGoal: "serial compatibility", MaxAttempts: 1, MaxChangedLines: 20}
	if _, err := store.Begin(ctx, generic); err != nil {
		t.Fatalf("generic serial begin = %v", err)
	}
	if replay, err := store.load(); err != nil || replay.itemPlan == nil || replay.itemPlan.Digest != plan.Digest {
		t.Fatalf("generic serial begin altered retained plan: %#v, %v", replay.itemPlan, err)
	}
}

func TestRuntimeItemPlanRejectsOpaqueOriginTransfer(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	worktree := filepath.Join(t.TempDir(), "origin-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "origin-worktree", worktree)
	plan := runtimeTestItemPlan(t)
	storeA := mustRuntimeStore(t, repo, "origin-change")
	storeB, err := OpenRuntimeStore(ctx, worktree, "origin-change")
	if err != nil {
		t.Fatal(err)
	}
	fromA := runtimePlanRequest(t, storeA, plan, "build", "origin-transfer")
	entry, _ := itemPlanEntryForID(plan, "build")
	rootsB, _ := canonicalWorkItemRoots(entry.EditRoots, storeB.Workspace)
	fromA.ItemEditRoots = rootsB
	for _, target := range []RuntimeStore{storeB, mustRuntimeStore(t, repo, "other-change"), mustRuntimeStore(t, initRuntimeLedgerRepo(t), "origin-change")} {
		before := countRuntimeRecords(t, target.Dir)
		if _, err := target.Begin(ctx, fromA); err == nil || countRuntimeRecords(t, target.Dir) != before {
			t.Fatalf("opaque origin transfer to %q accepted: %v", target.Change, err)
		}
	}
	fresh := runtimePlanRequest(t, storeB, plan, "build", "origin-fresh")
	if _, err := storeB.Begin(ctx, fresh); err != nil {
		t.Fatalf("fresh linked-worktree candidate = %v", err)
	}
}

func TestRuntimeItemPlanRequiresSealedStoreIdentity(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "sealed-plan")
	plan := runtimeTestItemPlan(t)
	request := runtimePlanRequest(t, store, plan, "build", "sealed-request")
	for _, mutate := range []func(*RuntimeStore){
		func(store *RuntimeStore) { store.Dir += "-foreign" },
		func(store *RuntimeStore) { store.Repo += "-foreign" },
		func(store *RuntimeStore) { store.Workspace += "-foreign" },
		func(store *RuntimeStore) { store.Change = "foreign-change" },
	} {
		changed := store
		mutate(&changed)
		before := countRuntimeRecords(t, store.Dir)
		if _, err := changed.Begin(ctx, request); err == nil || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("mutated sealed store accepted plan: %v", err)
		}
	}
	foreignRepo := initRuntimeLedgerRepo(t)
	foreign := mustRuntimeStore(t, foreignRepo, "foreign-plan")
	foreign.Dir, foreign.Repo, foreign.Workspace, foreign.Change = store.Dir, store.Repo, store.Workspace, store.Change
	if _, err := foreign.Begin(ctx, request); err == nil {
		t.Fatal("redirected foreign store accepted plan")
	}
	unsealed := RuntimeStore{Dir: store.Dir, Repo: store.Repo, Workspace: store.Workspace, Change: store.Change}
	if _, err := unsealed.Begin(ctx, request); err == nil {
		t.Fatal("unsealed literal store established plan")
	}
	copied, err := store.ForInstance("sealed-instance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copied.Begin(ctx, request); err != nil {
		t.Fatalf("sealed copied store rejected plan: %v", err)
	}
}

func TestRuntimeItemPlanReplayRejectsPlanlessBoundSuccessor(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	mkdir(t, filepath.Join(repo, "src"))
	mkdir(t, filepath.Join(repo, "verify"))
	store := mustRuntimeStore(t, repo, "item-plan-planless-replay")
	plan := runtimeTestItemPlan(t)
	first := runtimePlanRequest(t, store, plan, "build", "replay-first")
	started, err := store.Begin(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	passed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "replay-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	second := runtimePlanRequest(t, store, plan, "verify", "replay-second")
	second.ExpectedRevision = passed.Revision
	advanced, err := store.Begin(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.loadRecord(advanced.Revision)
	if err != nil {
		t.Fatal(err)
	}
	record.Begin.ItemPlan, record.Begin.ItemPlanDigest, record.Begin.ItemPlanEntryDigest = nil, "", ""
	legacy := BeginAttemptRequest{ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: record.Begin.WorkUnit,
		EvidenceGoal: record.Begin.EvidenceGoal, MaxAttempts: record.Begin.MaxAttempts, MaxChangedLines: record.Begin.MaxChangedLines,
		ItemID: record.Begin.ItemID, ItemEditRoots: record.Begin.ItemEditRoots}
	record.RequestDigest = runtimeBeginRequestDigest(legacy)
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
	if _, err := store.Status(); err == nil {
		t.Fatal("planless item-bound successor replayed")
	}
}

func runtimeTestItemPlan(t *testing.T) itemPlanCandidate {
	t.Helper()
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "build", WorkUnit: "build", EvidenceGoal: "compile", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"src"}},
		{ID: "verify", DependsOn: []string{"build"}, WorkUnit: "verify", EvidenceGoal: "test", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"verify"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func runtimePlanRequest(t *testing.T, store RuntimeStore, plan itemPlanCandidate, itemID, requestID string) BeginAttemptRequest {
	t.Helper()
	entry, ok := itemPlanEntryForID(plan, itemID)
	roots, ok := canonicalWorkItemRoots(entry.EditRoots, store.Workspace)
	if !ok {
		t.Fatal("canonical roots")
	}
	return BeginAttemptRequest{RequestID: requestID, WorkUnit: entry.WorkUnit, EvidenceGoal: entry.EvidenceGoal,
		MaxAttempts: entry.MaxAttempts, MaxChangedLines: entry.MaxChangedLines, ItemID: itemID, ItemEditRoots: roots,
		itemPlan: &itemPlanBinding{Plan: plan, ItemID: itemID, EntryDigest: itemPlanEntryDigest(entry), Workspace: store.Workspace, Change: store.Change}}
}
