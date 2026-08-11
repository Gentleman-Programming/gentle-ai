package sddstatus

import (
	"strings"
	"testing"
)

// TestSDDAttemptBlockersNeverNameTheReviewKillSwitchAsTheirExit is #2913's
// reproduction, stated as a property.
//
// The reporter hit an exhausted attempt budget, got blocked(maintainer_decision),
// and was told to run `gentle-ai review mode disable --scope clone`. They ran
// it. It SUCCEEDED. Effective mode went to off, decided by clone_local. Then
// SDD status returned the same blocker and instructed them to disable the
// already-disabled mode again.
//
// The command was never capable of clearing that block. An attempt-budget
// decision is cleared by `sdd-attempt reset`, and by nothing else. Receipt-driven
// review governs DELIVERY of a finished change; it has no authority over whether
// an SDD work unit may open, so turning it off cannot open one.
//
// This file guards the boundary in the direction that actually burns people:
// an SDD-surface blocker must never advertise a receipt-driven-review command
// as its way out. The project states the cost of getting this wrong in its own
// words, in runtime_ledger.go: naming a command that runs and leaves the block
// in place is the most expensive refusal this product can emit, because the
// operator then trusts it.
func TestSDDAttemptBlockersNeverNameTheReviewKillSwitchAsTheirExit(t *testing.T) {
	const sampleToken = "sha256:a1b2c3d4e5f60708091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f7"

	// Every reason the compact SDD attempt surface can block with. These are
	// all blockers of the SDD attempt ledger, so none of them is resolved by
	// changing review mode.
	for _, reason := range []CompactBlockReason{
		CompactBlockActiveAttempt,
		CompactBlockMaintainerDecision,
		CompactBlockCorruptAuthority,
		CompactBlockInvalidContinuation,
		CompactBlockRemediationUnsatisfiable,
	} {
		t.Run(string(reason), func(t *testing.T) {
			token := ""
			if reason == CompactBlockActiveAttempt {
				token = sampleToken
			}
			exit := compactBlockedExitText(reason, token)
			if exit == "" {
				t.Fatalf("compactBlockedExitText(%q) named no continuation at all", reason)
			}
			if strings.Contains(exit, "review mode disable") {
				t.Fatalf("SDD blocker %q offers `review mode disable` as its exit:\n%s\n\nThat command succeeds and leaves this block exactly where it was (#2913). Receipt-driven review governs delivery of a finished change, not whether an SDD work unit may open.", reason, exit)
			}
			if strings.Contains(exit, "gentle-ai review ") {
				t.Fatalf("SDD blocker %q routes to a receipt-driven review command:\n%s", reason, exit)
			}
		})
	}
}

// TestMaintainerDecisionExitNamesTheOperationThatClearsIt is the positive half:
// removing the false exit is only half the fix if what remains names nothing
// the operator can run.
func TestMaintainerDecisionExitNamesTheOperationThatClearsIt(t *testing.T) {
	exit := compactBlockedExitText(CompactBlockMaintainerDecision, "")
	for _, want := range []string{"gentle-ai sdd-attempt status", "sdd-attempt reset"} {
		if !strings.Contains(exit, want) {
			t.Fatalf("maintainer_decision exit does not name %q:\n%s", want, exit)
		}
	}
}
