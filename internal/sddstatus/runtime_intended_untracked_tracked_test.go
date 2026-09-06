package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #3842: the runtime ledger records each attempt's intended-untracked
// selection at acquire time, and every later operation that re-captures the
// workspace candidate replayed that HISTORICAL list verbatim. Once the user
// legitimately committed the selected paths — the ordinary end of a work
// unit, and sometimes the work unit itself — the snapshot builder's
// "already tracked" refusal fired on every replayed capture and the runtime
// dead-ended across reset, settle, and admissibility probes. A path that
// became tracked is already part of the ordinary candidate (its bytes live
// in HEAD/index/worktree), so the fix reconciles replayed lists against the
// current index before capture: dropped from the overlay, present in the
// tree, byte-identical candidate. Only replayed history is reconciled;
// FRESH caller-supplied selections stay strict.

// The core field-report shape: a work unit selects an untracked path, passes,
// completes its objective, and the user commits the path. Reset must succeed
// and publish a reset record instead of dying on the replayed selection.
func TestRuntimeResetSucceedsAfterIntendedUntrackedBecomesTracked(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("selected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, "tracked-selected-reset")
	store.ReviewDisabled = true
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-reset-begin", WorkUnit: "apply", EvidenceGoal: "land the selected file",
		MaxAttempts: 1, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "tracked-reset-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "selected work unit passed verification",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete {
		t.Fatalf("passed selected objective did not complete: %#v", completed)
	}
	// The legitimate end of the work unit: the selected path is committed and
	// is now tracked, so the recorded selection no longer names an untracked
	// path.
	runRuntimeLedgerGit(t, repo, "add", "selected.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "land selected.txt")
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: completed.Revision, RequestID: "tracked-reset",
		Reason: "maintainer decision: open a successor scope after the selected file landed",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatalf("reset after the selected path was committed: %v", err)
	}
	if reset.LastReset == nil || reset.LastReset.Revision != reset.Revision || reset.NextAction != RuntimeActionBegin {
		t.Fatalf("reset after commit did not publish a reset record: %#v", reset)
	}
}

// Settle mid-commit: some work units require committing the selected path as
// part of the attempt itself. The finish capture replays the attempt's
// selection over the begin candidate tree and must not refuse the now-tracked
// path.
func TestRuntimeSettleSucceedsAfterIntendedUntrackedCommittedMidAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("selected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, "tracked-selected-settle")
	store.ReviewDisabled = true
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-settle-begin", WorkUnit: "apply", EvidenceGoal: "commit the selected file",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "selected.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "land selected.txt mid-attempt")
	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "tracked-settle-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "the selected file was committed as the work unit required",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatalf("settle after committing the selected path mid-attempt: %v", err)
	}
	if !settled.Complete || settled.ActiveAttempt != nil {
		t.Fatalf("mid-commit settle = %#v", settled)
	}
	// The committed bytes were already part of the begin candidate as the
	// selected overlay, so the candidate tree itself must not have moved.
	last := settled.Attempts[len(settled.Attempts)-1]
	if last.ChangedLines != 0 {
		t.Fatalf("byte-identical mid-commit candidate charged %d changed lines", last.ChangedLines)
	}
}

// Strictness preserved (#3842): reconciliation applies only to REPLAYED
// ledger history. A fresh begin whose caller explicitly selects a tracked
// path is a scope declaration error and must keep failing loudly.
func TestRuntimeFreshBeginStillRefusesTrackedIntendedUntracked(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "tracked-selected-fresh")
	_, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-fresh-begin", WorkUnit: "apply", EvidenceGoal: "reject a tracked selection",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"tracked.txt"},
	})
	if err == nil || !strings.Contains(err.Error(), "already tracked") {
		t.Fatalf("fresh begin with a tracked selection = %v, want the already-tracked refusal", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != "" || status.ActiveAttempt != nil {
		t.Fatalf("refused fresh selection mutated authority: status=%#v err=%v", status, statusErr)
	}
}

// After the selected path lands PLUS a further tracked edit, the candidate has
// genuinely drifted, so reset must be admissible and the changed-objective
// refusal must name the reset exit. Before #3842 the admissibility probes'
// captures failed on the replayed selection, both probes answered false, and
// the refusal fell back to re-offering the very begin the state refuses.
func TestRuntimeResetAdmissibleAfterIntendedUntrackedCommitDrift(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("selected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, "tracked-selected-admissible")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-admissible-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "tracked-admissible-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "verification found a defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "selected.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "land selected.txt")
	appendRuntimeLedgerFile(t, repo, "real candidate drift\n")
	// A begin naming a different work unit reaches the changed-objective
	// refusal, whose exit is chosen by the reset admissibility probe.
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "tracked-admissible-changed", WorkUnit: "other",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err == nil || !strings.Contains(err.Error(), "sdd-attempt reset") {
		t.Fatalf("changed-objective refusal after commit drift = %v, want the reset exit named", err)
	}
	// And the reset the refusal names must actually run: its drift check and
	// its fresh capture both replay the recorded selection.
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "tracked-admissible-reset",
		Reason: "maintainer decision: the selected file landed and the candidate moved on",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatalf("drift reset after the selected path was committed: %v", err)
	}
	if reset.LastReset == nil || reset.NextAction != RuntimeActionBegin {
		t.Fatalf("drift reset did not publish a reset record: %#v", reset)
	}
}

// The overlay identity binds only trees and paths, so committing the selected
// path with no other edit keeps the candidate byte-identical — that is
// rescope's zero-drift shape, not reset's. The changed-objective refusal must
// name the rescope exit, the elective reset must stay refused, and the rescope
// itself must run through both of its reconciled captures.
func TestRuntimeRescopeAdmissibleAfterIntendedUntrackedLandsByteIdentical(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("selected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, "tracked-selected-zero-drift")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-zero-drift-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "tracked-zero-drift-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "verification found a defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "selected.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "land selected.txt only")
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "tracked-zero-drift-changed", WorkUnit: "other",
		EvidenceGoal: "independent verification", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err == nil || !strings.Contains(err.Error(), "sdd-attempt rescope") {
		t.Fatalf("changed-objective refusal after a byte-identical landing = %v, want the rescope exit named", err)
	}
	// The elective reset stays refused: the reconciled drift capture must
	// succeed and answer zero drift instead of dying on the replayed
	// selection.
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "tracked-zero-drift-reset",
		Reason: "elective reset over an unmoved candidate", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeResetNotAllowed) {
		t.Fatalf("zero-drift reset after the landing = %v, want ErrRuntimeResetNotAllowed", err)
	}
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "tracked-zero-drift-rescope",
		WorkUnit: "narrower verify", EvidenceGoal: "narrower verification",
		MaxAttempts: 2, MaxChangedLines: 10,
		Reason: "maintainer decision: narrow the objective after the selected file landed",
		Actor:  "maintainer",
	})
	if err != nil {
		t.Fatalf("rescope after the byte-identical landing: %v", err)
	}
	if rescoped.LastRescope == nil || rescoped.Objective == nil || rescoped.Objective.WorkUnit != "narrower verify" {
		t.Fatalf("rescope after the landing did not open the successor: %#v", rescoped)
	}
}

// Partial landing: only one of two selected paths gets committed. The
// replayed capture must keep the still-untracked path in the candidate
// overlay while dropping only the tracked one.
func TestRuntimeReplayedCapturePreservesRemainingUntrackedIntended(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for name, content := range map[string]string{"sel-a.txt": "a\n", "sel-b.txt": "b\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := mustRuntimeStore(t, repo, "tracked-selected-partial")
	store.ReviewDisabled = true
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "tracked-partial-begin", WorkUnit: "apply", EvidenceGoal: "land one selected file",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"sel-a.txt", "sel-b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "sel-a.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "land sel-a.txt only")
	if err := os.WriteFile(filepath.Join(repo, "sel-b.txt"), []byte("b\nstill selected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settled, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "tracked-partial-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "one selected file landed, the other kept working",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatalf("settle after a partial selected landing: %v", err)
	}
	last := settled.Attempts[len(settled.Attempts)-1]
	// Exactly the still-untracked path's edit is charged: sel-a's bytes were
	// already in the begin candidate overlay, so committing them moved
	// nothing, while sel-b's one added line is real candidate change — proof
	// the remaining selection stayed in the capture.
	if last.ChangedLines != 1 {
		t.Fatalf("partial landing charged %d changed lines, want only sel-b's edit", last.ChangedLines)
	}
	// The finish record carries the selection this settlement's overlay
	// actually used (#3806 records the resolved settle-time selection, #3842
	// reconciles it): sel-a landed, so only sel-b remains. The begin record
	// upstream still holds both paths as acquired.
	if len(last.IntendedUntracked) != 1 || last.IntendedUntracked[0] != "sel-b.txt" {
		t.Fatalf("recorded selection provenance = %#v", last.IntendedUntracked)
	}
}
