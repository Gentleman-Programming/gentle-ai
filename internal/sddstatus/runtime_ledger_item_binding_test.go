package sddstatus

import (
	"context"
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
	record, err := store.loadRecord(started.Revision)
	if err != nil || record.Begin.ItemID != "" || len(record.Begin.ItemEditRoots) != 0 || record.RequestDigest != runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request) {
		t.Fatalf("unbound record changed: %#v, %v", record, err)
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
