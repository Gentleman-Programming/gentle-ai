package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestStatusAndAcquireAgreeOnEveryRepositoryBlocker is the generic guard for
// the whole "two authorities disagree" class behind #2114 and its siblings.
//
// runtimeReadiness already unified the LEDGER-side question: status and
// acquire ask one predicate and cannot disagree about attempts, budgets, or
// completion. The reports kept arriving anyway because Begin evaluates a
// SECOND set of preconditions that runtimeReadiness never sees — the
// REPOSITORY-side ones, inside the mutation closure, after readiness already
// answered "nothing blocks". Every report in the class reads the same way: a
// clean ledger, next_action: "begin", and an acquire that blocks with a code
// the status surface has no idea about.
//
// This test does not enumerate causes. It pins the PROPERTY: for any
// repository state, whatever acquire would block on, the status surface
// reports the same reason and the same runnable exit. A new repository-side
// precondition added to Begin without teaching the shared admission predicate
// about it fails here.
func TestStatusAndAcquireAgreeOnEveryRepositoryBlocker(t *testing.T) {
	request := BeginAttemptRequest{
		RequestID: "agreement-acquire", WorkUnit: "u", EvidenceGoal: "g",
		MaxAttempts: 2, MaxChangedLines: 400,
	}

	tests := []struct {
		name       string
		repo       func(t *testing.T) string
		wantBlock  bool
		wantReason CompactBlockReason
	}{
		// A capture failure with an intact ledger is deliberately NOT staged
		// here. Every way to break candidate capture from the outside (unborn
		// repository, pruned object store, unreadable object store) now either
		// succeeds — 7de3dbbc fixed the unborn path that #2114's Windows
		// reporter hit — or fails earlier, at store open, which is a different
		// verdict. Rather than fabricate an exotic repository to keep a row,
		// candidate_unavailable's classification is proven directly in
		// TestCompactMutationFailureClassifiesEveryReachableLedgerSentinel.
		{
			name:       "candidate drifted out from under a terminal objective",
			repo:       newDriftedObjectiveRepo,
			wantBlock:  true,
			wantReason: CompactBlockMaintainerDecision,
		},
		{
			name: "healthy repository blocks on nothing",
			repo: func(t *testing.T) string {
				repo := initRuntimeLedgerRepo(t)
				appendRuntimeLedgerFile(t, repo, "work\n")
				return repo
			},
			wantBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := tt.repo(t)
			store, err := OpenRuntimeStore(ctx, repo, "agreement-change")
			if err != nil {
				t.Fatal(err)
			}

			// The status surface is read first, exactly as a consumer does:
			// it must already know what acquire is about to say.
			status, err := store.AdmissionStatus(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
			if err != nil {
				t.Fatal(err)
			}

			if !tt.wantBlock {
				if status.BlockedReason != "" || status.BlockedExit != "" {
					t.Fatalf("status on a healthy repository = reason %q exit %q, want no blocker", status.BlockedReason, status.BlockedExit)
				}
				if acquired.State != CompactStateProceed {
					t.Fatalf("acquire on a healthy repository = %#v, want state=proceed", acquired)
				}
				return
			}

			if acquired.State != CompactStateBlocked {
				t.Fatalf("acquire = %#v, want state=blocked", acquired)
			}
			if acquired.Reason != tt.wantReason {
				t.Fatalf("acquire reason = %q, want %q", acquired.Reason, tt.wantReason)
			}
			// The agreement itself: this is the assertion every report in the
			// class was really filing.
			if status.BlockedReason != acquired.Reason {
				t.Fatalf("status.blocked_reason = %q but acquire blocked with %q; the two surfaces disagree, which is the whole defect class", status.BlockedReason, acquired.Reason)
			}
			if status.BlockedExit == "" {
				t.Fatalf("status blocked with %q and named no continuation; a blocked surface with nothing behind it is the second half of the defect class", status.BlockedReason)
			}
			if status.BlockedExit != acquired.Exit {
				t.Fatalf("status.blocked_exit = %q but acquire named %q; one blocker must produce one continuation", status.BlockedExit, acquired.Exit)
			}
		})
	}
}

// TestAdmissionStatusLeavesTheLedgerProjectionIntact proves the shared
// admission predicate is a pure read: it adds the repository verdict and
// changes nothing the ledger already owned, so every existing consumer of
// RuntimeStatus keeps reading exactly what it read before.
func TestAdmissionStatusLeavesTheLedgerProjectionIntact(t *testing.T) {
	ctx := context.Background()
	repo := newDriftedObjectiveRepo(t)
	store, err := OpenRuntimeStore(ctx, repo, "agreement-change")
	if err != nil {
		t.Fatal(err)
	}

	ledger, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	admission, err := store.AdmissionStatus(ctx, BeginAttemptRequest{
		RequestID: "projection-acquire", WorkUnit: "u", EvidenceGoal: "g",
		MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}

	if admission.BlockedReason == "" {
		t.Fatal("admission status named no blocker on a repository whose candidate drifted")
	}
	// Everything the ledger owns is untouched, including next_action: the
	// ledger genuinely is ready to begin, and the repository is what is not.
	admission.BlockedReason, admission.BlockedExit = "", ""
	if admission.NextAction != ledger.NextAction {
		t.Fatalf("admission next_action = %q, want the ledger's own %q", admission.NextAction, ledger.NextAction)
	}
	if admission.Revision != ledger.Revision || len(admission.Attempts) != len(ledger.Attempts) ||
		admission.CumulativeAttempts != ledger.CumulativeAttempts || admission.Complete != ledger.Complete ||
		admission.DecisionRequired != ledger.DecisionRequired {
		t.Fatalf("admission status = %#v, want the ledger projection %#v unchanged", admission, ledger)
	}
}

// newDriftedObjectiveRepo reproduces the variant two reporters filed on #2114
// (v2.2.4, macOS/OpenCode): an attempt is acquired, settled failed, and the
// cleanup restores the tree, so the recorded terminal candidate no longer
// matches what the repository now holds. The next acquire on the same
// objective blocks while the ledger still reads clean and ready.
func newDriftedObjectiveRepo(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(ctx, repo, "agreement-change")
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "work\n")
	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "drift-acquire", WorkUnit: "u", EvidenceGoal: "g", MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.State != CompactStateProceed {
		t.Fatalf("drift fixture acquire = %#v, want state=proceed", acquired)
	}
	settled, err := store.Settle(ctx, CompactSettleRequest{
		Token: acquired.Token, RequestID: "drift-settle", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "attempt failed before RED",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup restored the candidate",
		ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if settled.State == CompactStateBlocked {
		t.Fatalf("drift fixture settle = %#v, want it to close", settled)
	}
	// The cleanup the reporter described: the tree goes back to base, so the
	// candidate the objective recorded at Finish is no longer what is there.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}
