package cli

// Wave 5 (Gate Cutover), Slice 7: invalidation verb deletion (design
// decision 2, LANDS LAST -- the wave's only destructive authority step).
// This file carries the slice's three named RED tests (tasks 8.1-8.3).

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates is task 8.1:
// a receipt that would have been os.Remove'd under the pre-cutover
// InvalidateApprovedCompactAuthority writer stays present, byte-identical,
// on disk after a drifted approved candidate denies at every one of the
// five gates -- the gate denies with an ALREADY-derived mismatch relation
// (evaluateCompactGate's own scope/binding checks, unrelated to and
// unaffected by this slice's deletion), never a persisted write.
func TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	lineage := "review-invalidation-deletion-receipt-persists"
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	finalizeApprovedFacadeReview(t, repo, lineage)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateApproved {
		t.Fatalf("fixture did not reach approved: %#v", before.State)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatalf("approved fixture carries no receipt: %v", err)
	}

	// Drift the candidate out of scope, so every gate genuinely denies
	// (relation=changed) rather than trivially allowing. Committed, not
	// just staged: pre-push/pre-pr compare committed history against the
	// reviewed base boundary, not the uncommitted index post-commit/pre-commit does.
	writeReviewStartCandidate(t, repo, "outside.txt", "outside frozen scope\n", 0o644)
	runReviewCLIGit(t, repo, "add", "outside.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "drift out of reviewed scope")

	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		t.Run(string(gate), func(t *testing.T) {
			args := []string{"--cwd", repo, "--gate", string(gate)}
			if gate == reviewtransaction.GatePrePush || gate == reviewtransaction.GatePrePR {
				args = append(args, "--base-ref", "origin/"+branch)
			}
			var output bytes.Buffer
			err := RunReviewFacadeValidate(args, &output)
			if err == nil {
				t.Fatalf("%s: drifted approved candidate unexpectedly allowed:\n%s", gate, output.String())
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if after.State.State != reviewtransaction.StateApproved || after.Revision != before.Revision {
				t.Fatalf("%s: gate evaluation mutated persisted authority: before=%#v after=%#v", gate, before.State, after.State)
			}
			receiptAfter, err := os.ReadFile(store.ReceiptPath())
			if err != nil {
				t.Fatalf("%s: receipt was removed by gate evaluation: %v", gate, err)
			}
			if !bytes.Equal(receiptBefore, receiptAfter) {
				t.Fatalf("%s: receipt bytes changed after gate evaluation", gate)
			}
		})
	}
}

// Task 8.2's own RED/GREEN evidence (StateInvalidated / InvalidationEvidence
// parse without rewrite) lives in
// internal/reviewtransaction/compact_invalidated_readonly_test.go: it needs
// the package's own unexported write primitives (makeCompactRecord,
// writeAtomic) to construct a pre-cutover invalidated record directly,
// which -- correctly -- a _test.go file cannot export across the package
// boundary for this file to call. `review status`'s own read-only handling
// of an invalidated record is already covered generally by its existing
// test suite; nothing about Slice 7's deletion changes that read path.
