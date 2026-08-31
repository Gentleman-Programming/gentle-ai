package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Tests for #3881: every settlementUntrackedSelection refusal is a
// caller-actionable demand for an untracked ruling, not an authority failure,
// so each one must carry ErrRuntimeUndeclaredUntracked for
// compactMutationFailure to classify. Without the sentinel they all fall
// through to the opaque authority_failure default the contract reserves for
// what its name says -- exactly the fallthrough class candidate_unavailable
// (#2114) and worktree_mismatch (#2296 part 1) were added to close.

func TestSettlementUntrackedSelectionRefusalsCarryTheUndeclaredSentinel(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "born.txt"), []byte("born\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staleDigest := "sha256:" + strings.Repeat("0", 64)
	selection := func(paths ...string) *[]string { return &paths }

	tests := []struct {
		name    string
		active  RuntimeAttempt
		request FinishAttemptRequest
	}{
		{
			name:    "legacy record refuses any declaration",
			active:  RuntimeAttempt{},
			request: FinishAttemptRequest{IntendedUntracked: selection()},
		},
		{
			name:   "undecided born-during path",
			active: RuntimeAttempt{EligibleUntrackedInventory: staleDigest},
		},
		{
			name:   "declaration against a stale inventory",
			active: RuntimeAttempt{EligibleUntrackedInventory: staleDigest},
			request: FinishAttemptRequest{
				IntendedUntracked: selection("born.txt"), ExpectedUntrackedInventory: staleDigest,
			},
		},
		{
			name:   "selected path outside the eligible inventory",
			active: RuntimeAttempt{EligibleUntrackedInventory: digest},
			request: FinishAttemptRequest{
				IntendedUntracked: selection("missing.txt"), ExpectedUntrackedInventory: digest,
			},
		},
		{
			name:   "narrowing a begin selection",
			active: RuntimeAttempt{EligibleUntrackedInventory: digest, IntendedUntracked: []string{"born.txt"}},
			request: FinishAttemptRequest{
				IntendedUntracked: selection(), ExpectedUntrackedInventory: digest,
			},
		},
	}
	store := RuntimeStore{Repo: repo}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := store.settlementUntrackedSelection(ctx, tt.active, tt.request)
			if err == nil {
				t.Fatal("settlement untracked refusal did not refuse")
			}
			if !errors.Is(err, ErrRuntimeUndeclaredUntracked) {
				t.Fatalf("refusal %v does not carry ErrRuntimeUndeclaredUntracked, so Settle reports it as authority_failure", err)
			}
		})
	}
}

// The eligible-inventory read is a Git capture with the attempt authority
// intact and unmutated, which is the classification candidate_unavailable
// already owns; authority_failure would be exactly the misreport its own
// contract comment rules out.
func TestSettlementInventoryReadFailureClassifiesCandidateUnavailable(t *testing.T) {
	store := RuntimeStore{Repo: t.TempDir()}
	active := RuntimeAttempt{EligibleUntrackedInventory: "sha256:" + strings.Repeat("a", 64)}
	_, _, err := store.settlementUntrackedSelection(context.Background(), active, FinishAttemptRequest{})
	if err == nil {
		t.Fatal("inventory read against a non-repository did not refuse")
	}
	if !errors.Is(err, ErrRuntimeCandidateUnavailable) {
		t.Fatalf("inventory read failure %v does not carry ErrRuntimeCandidateUnavailable", err)
	}
}
