package reviewtransaction

import (
	"context"
	"testing"
)

// This file documents the STOPPED sub-path of D2 (#2471 option a, filed
// against #2388): "extend compatible base advance to pre-commit and pre-push
// by reusing the existing content checks without the CI attestation."
// Goal 1 (deriveBaseAdvanceCompatibility's requireAttestation parameter and
// receipt.go's gate-agnostic baseAdvanceStatusAllowedForGate policy) is
// implemented and covered by prepr_base_advance_characterization_test.go and
// receipt_test.go. Goal 2 -- wiring compact_gate.go's evaluateCompactGate (the
// gate evaluator the CLI actually calls for compact/modern receipts) to
// populate GateContext.BaseAdvance for GatePreCommit/GatePrePush -- is
// deliberately NOT done, because neither local gate's own target derivation
// can ever produce deriveBaseAdvanceCompatibility's required precondition
// (BaseTree diverged from the receipt while CandidateTree stayed byte-
// identical to it) without changing how that derivation works, which is out
// of scope for a change that must reuse the existing content checks
// unchanged.
//
// The two tests below are the empirical evidence for that conclusion, kept as
// permanent regression tests (not deleted after investigation) so a future
// change that touches buildPushTarget or the pre-commit target builder has a
// failing signal if it silently reintroduces or resolves this gap.
//
//   - TestCompactPrePushDisjointParentAdvanceWithoutLocalMergeAlreadyAllows:
//     buildPushTarget (gate.go:907-950) resolves TargetBaseDiff's BaseRef via
//     `git merge-base --all HEAD <boundary>` (gate.go:919), which self-heals
//     back to the true historical divergence point whenever HEAD has not
//     incorporated the advanced boundary. A pure "parent advanced on disjoint
//     paths, child untouched" #2388 scenario therefore already resolves
//     snapshot.BaseTree == receipt.BaseTree at pre-push today, with nothing to
//     fix: GateAllow, zero mismatch, zero BaseAdvance evidence needed.
//
//   - TestCompactPrePushDisjointParentAdvanceWithLocalMergeStillRefuses: the
//     only way to make snapshot.BaseTree diverge from receipt.BaseTree at
//     pre-push is for HEAD to also advance (a real local merge commit), but
//     that necessarily changes snapshot.CandidateTree away from
//     receipt.FinalCandidateTree too (compact_gate.go:422's
//     `snapshot.CandidateTree != receipt.FinalCandidateTree` check fires
//     unconditionally, with no compatibleAdvance override, exactly like it
//     does today for pre-PR -- see receipt.go's carve-out, which never
//     overrides a literal candidate-tree mismatch either). Even if this test
//     wired deriveBaseAdvanceCompatibility in on this path, the derivation's
//     own condition 3 (prepr.go's patchIdentity calls, both anchored at
//     receipt.BaseTree) would refuse it: the merged tree's diff against the
//     OLD base includes both the reviewed content and the parent's advance,
//     which is not patch-identical to the original reviewed diff. Reusing
//     that check unchanged -- the explicit instruction -- means this shape
//     can never be certified compatible without redefining patch identity to
//     be base-relative, a semantic change to a shared, separately-pinned
//     characterization (prepr_base_advance_characterization_test.go) that is
//     out of scope here.
//
// Pre-commit has the identical structural gap from the other direction:
// resolveCurrentChangesBase (snapshot.go:1110-1136) always returns HEAD's own
// tree, so TargetCurrentChanges's BaseTree can never move independently of
// CandidateTree either; see TestCompactPreCommitStagedMergeStillRefuses below
// for its own empirical proof.

// TestCompactPrePushDisjointParentAdvanceWithoutLocalMergeAlreadyAllows is the
// pre-existing-behavior half of the stop evidence: no BaseAdvance wiring is
// needed for the "child untouched" shape because pre-push already allows it.
func TestCompactPrePushDisjointParentAdvanceWithoutLocalMergeAlreadyAllows(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: state.LineageID,
	})
	if got.Result != GateAllow {
		t.Fatalf("pre-push disjoint parent advance without a local merge = %#v, want allow (merge-base already self-heals)", got)
	}
	if got.Context.BaseTree != receipt.BaseTree || got.Context.CandidateTree != receipt.FinalCandidateTree {
		t.Fatalf("pre-push context = %#v, want base/candidate unchanged from the receipt", got.Context)
	}
	if got.Context.BaseAdvance != nil {
		t.Fatalf("pre-push context carries BaseAdvance evidence = %#v, want nil: no advance was needed", got.Context.BaseAdvance)
	}
}

// TestCompactPrePushDisjointParentAdvanceWithLocalMergeStillRefuses is the
// still-refused half: once HEAD actually merges the advanced parent in,
// candidate and base move together, which the shared, unchanged content
// checks cannot certify as compatible (see file doc comment above).
func TestCompactPrePushDisjointParentAdvanceWithLocalMergeStillRefuses(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)
	gitSnapshot(t, fixture.repo, "merge", "main", "--no-edit")
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: state.LineageID,
	})
	if got.Result != GateScopeChanged || got.Context.Denial == nil || got.Context.Denial.Code != "candidate-or-paths-mismatch" {
		t.Fatalf("pre-push disjoint parent advance with a local merge = %#v, want scope-changed/candidate-or-paths-mismatch", got)
	}
}

// TestCompactPreCommitStagedMergeStillRefuses is pre-commit's own stop
// evidence: TargetCurrentChanges pins BaseTree to HEAD's own tree
// (snapshot.go's resolveCurrentChangesBase), so a staged merge of the advanced
// parent shows up entirely as a CandidateTree delta, never as a BaseTree
// movement deriveBaseAdvanceCompatibility could certify.
func TestCompactPreCommitStagedMergeStillRefuses(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)
	gitSnapshot(t, fixture.repo, "merge", "main", "--no-commit", "--no-ff")
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePreCommit, LineageID: state.LineageID,
	})
	if got.Result != GateScopeChanged || got.Context.Denial == nil || got.Context.Denial.Code != "candidate-or-paths-mismatch" {
		t.Fatalf("pre-commit staged merge of the advanced parent = %#v, want scope-changed/candidate-or-paths-mismatch", got)
	}
	if got.Context.BaseTree != receipt.FinalCandidateTree {
		t.Fatalf("pre-commit base_tree = %q, want it pinned to HEAD (== the reviewed candidate %q), proving base never moved", got.Context.BaseTree, receipt.FinalCandidateTree)
	}
}
