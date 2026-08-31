package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #2661: after the bound linked worktree is removed and pruned, every exit
// named the vanished path. Each refusal must say the worktree is gone and name
// the interrupted settle, which is admitted from any worktree of the repo.
func TestRuntimeLedgerRemovedWorktreeSettlesInterruptedFromMainWorktree(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	worktree := filepath.Join(t.TempDir(), "removed-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "removed-branch", worktree)
	change := "removed-worktree"
	linked, err := OpenRuntimeStore(context.Background(), worktree, change)
	if err != nil {
		t.Fatal(err)
	}
	began, err := linked.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "begin-linked", WorkUnit: "removed-worktree-unit",
		EvidenceGoal: "prove the removed worktree exit", MaxAttempts: 2, MaxChangedLines: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "worktree", "prune")
	main, _ := OpenRuntimeStore(context.Background(), repo, change)
	settle := func(outcome AttemptOutcome, requestID, evidence string) error {
		_, err := main.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: began.Revision, RequestID: requestID, Outcome: outcome, EvidenceRevision: evidence,
			Diagnosis: "the bound worktree was removed", HarnessDisposition: HarnessInvalidated,
			CleanupEvidence: "worktree directory deleted", ProcessEvidence: "no process remained",
		})
		return err
	}
	_, beginErr := main.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: began.Revision, RequestID: "begin-main", WorkUnit: "another-unit",
		EvidenceGoal: "prove the active-attempt exit", MaxAttempts: 2, MaxChangedLines: 200,
	})
	acquired, _ := main.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "acquire-main", WorkUnit: "another-unit", EvidenceGoal: "prove the compact exit", MaxAttempts: 2, MaxChangedLines: 200,
	}})
	_, handoffErr := main.Handoff(context.Background(), HandoffAttemptRequest{
		ExpectedRevision: began.Revision, RequestID: "handoff-main", DestinationWorktree: repo,
	})
	for _, tt := range []struct {
		label    string
		err      error
		sentinel error
	}{
		{"passed settle", settle(AttemptPassed, "finish-passed", runtimeTestHash('9')), ErrRuntimeWorktreeMismatch},
		{"begin", beginErr, ErrRuntimeAttemptActive},
		{"acquire", errors.New(acquired.Exit), nil},
		{"handoff", handoffErr, ErrRuntimeHandoffSource},
	} {
		if tt.sentinel != nil && !errors.Is(tt.err, tt.sentinel) {
			t.Fatalf("%s error = %v, want %v", tt.label, tt.err, tt.sentinel)
		}
		for _, want := range []string{"no longer exists", "--outcome interrupted", "`gentle-ai sdd-attempt settle --cwd"} {
			if !strings.Contains(tt.err.Error(), want) {
				t.Fatalf("%s refusal does not name %q:\n%s", tt.label, want, tt.err.Error())
			}
		}
	}
	if acquired.State != CompactStateBlocked || acquired.Reason != CompactBlockActiveAttempt {
		t.Fatalf("acquire = %#v, want blocked(active_attempt)", acquired)
	}
	if err := settle(AttemptInterrupted, "finish-interrupted", ""); err != nil {
		t.Fatalf("interrupted settle from the main worktree: %v", err)
	}
	status, err := main.Status()
	if err != nil || status.ActiveAttempt != nil || len(status.Attempts) != 1 || status.Attempts[0].Outcome != AttemptInterrupted || status.Attempts[0].ChangedLines != 0 {
		t.Fatalf("status after interrupted settle = %#v err = %v", status, err)
	}
}
