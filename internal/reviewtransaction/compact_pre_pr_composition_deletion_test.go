package reviewtransaction

// Wave 5 (Gate Cutover), Slice 5: pre-PR chain composition deletion (design
// decision 1's "DELETED edges: EvaluateCompactPrePRChain..."). This file
// carries the "after" picture of the S1 characterization corpus's DELTA row
// (TestPrePRChainCompositionRemovalDelta, formerly in the now-deleted
// compact_chain_test.go): where that row proved a divergence between the
// ordinary single-receipt path and composition for an identical live
// candidate, this file proves the divergence is now permanent — direct
// single-receipt evaluation through EvaluateCompactGate is the ONLY path,
// and there is no composition left to rescue a multi-segment delivery.
//
// The minimal 3-segment fixture below is a purpose-built subset of the
// deleted compact_chain_test.go's compactPrePRChainFixture: only what this
// one surviving test needs (a linear multi-commit publication history, each
// commit individually reviewed and approved as its own compact receipt),
// none of the generic N-segment composition-proof machinery (member
// binding, authority graphs, cycle detection) that existed solely to
// exercise EvaluateCompactPrePRChain itself.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type compactPrePRDeletionFixture struct {
	repo, remote, branch string
	states               []CompactState
	receipts             []CompactReceipt
}

func newCompactPrePRDeletionFixture(t *testing.T, count int) *compactPrePRDeletionFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	fixture := &compactPrePRDeletionFixture{repo: repo, branch: branch}
	fixture.remote = configurePublicationRemote(t, repo, branch)
	for index := 0; index < count; index++ {
		fixture.addApprovedSegment(t, index)
	}
	return fixture
}

func (fixture *compactPrePRDeletionFixture) addApprovedSegment(t *testing.T, index int) {
	t.Helper()
	logicalPath := "segment-" + string(rune('a'+index)) + ".txt"
	path := filepath.Join(fixture.repo, logicalPath)
	if err := os.WriteFile(path, []byte("reviewed segment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lineage := "compact-pre-pr-deletion-segment-" + string(rune('a'+index))
	state := newCompactStartStateForTarget(t, fixture.repo, lineage, Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{logicalPath},
	})
	state, receipt := persistApprovedCompactState(t, fixture.repo, state)
	fixture.states = append(fixture.states, state)
	fixture.receipts = append(fixture.receipts, receipt)
	gitSnapshot(t, fixture.repo, "add", "-A")
	gitSnapshot(t, fixture.repo, "commit", "-m", "deliver reviewed segment")
}

func (fixture *compactPrePRDeletionFixture) input() NativeGateRequestInput {
	return NativeGateRequestInput{Gate: GatePrePR, BaseRef: "origin/" + fixture.branch}
}

// TestPrePRChainCompositionDeletionSupersedesRemovalDelta supersedes
// TestPrePRChainCompositionRemovalDelta (S1, formerly compact_chain_test.go).
// That row proved: for a 3-segment fixture, the last segment's OWN receipt
// only covers base=segment-b's post-commit tree, so the ordinary
// single-receipt path (EvaluateCompactGate) denies it scope-changed against
// the live 3-commits-ahead candidate, while EvaluateCompactPrePRChain
// composed all three segment receipts and reached GateAllow for the
// IDENTICAL live repository state and receipt. This row proves the same
// exact fixture and denial still occurs — unchanged, since EvaluateCompactGate
// itself is untouched by this slice — and that there is no longer any
// composition path left to rescue it: the divergence the S1 row pinned as
// the "before" picture is now the permanent, sole outcome.
func TestPrePRChainCompositionDeletionSupersedesRemovalDelta(t *testing.T) {
	fixture := newCompactPrePRDeletionFixture(t, 3)
	lastSegment := fixture.states[2]
	lastReceipt := fixture.receipts[2]

	directInput := fixture.input()
	directInput.LineageID = lastSegment.LineageID
	direct := EvaluateCompactGate(context.Background(), fixture.repo, lastReceipt, directInput)
	if direct.Result != GateScopeChanged {
		t.Fatalf("expected the single-receipt path to deny scope-changed (unchanged from before this slice), got %#v", direct)
	}
	if direct.Context.Denial == nil || direct.Context.Denial.Stage != "receipt-binding" || direct.Context.Denial.Code != "candidate-or-paths-mismatch" {
		t.Fatalf("expected the bound receipt-binding/candidate-or-paths-mismatch denial the S1 row pinned, got %#v", direct.Context.Denial)
	}
	// The permanent half of the delta: EvaluateCompactPrePRChain no longer
	// exists to rescue this denial (TestPrePRComposition_ZeroCallers,
	// internal/cli, proves it by call-absence). The funnel-level proof that
	// the identical multi-segment repository state denies end-to-end through
	// RunReviewFacadeValidate lives in internal/cli
	// (TestRunReviewFacadeValidatePrePRDeniesMultiSegmentDeliveryWithoutComposition)
	// — a lower-level reviewtransaction test cannot drive the funnel's own
	// lineage-free auto-discovery, which is a package cli concern.
}
