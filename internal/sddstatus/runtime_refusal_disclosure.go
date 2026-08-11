package sddstatus

import "fmt"

// A refusal that compares two values the caller cannot see, and then reports
// only that the comparison failed, is a dead end with a sentence attached.
// Each constructor here replaces one of those, and each exists because a
// reporter did exactly what the tooling prescribed and still could not move.

// runtimeDischargedFailureRefusal replaces "unmanaged remediation requires the
// current failed evidence and a direct correction attempt" for the case where
// the named failure is real but has ALREADY been repaired (#2881).
//
// The old text blamed the operator's input. Their input was right: they passed
// the failure their correction plan targets. What they could not see is that
// their own earlier slice already discharged it — the chain holds no
// unremediated failure now, so this work unit owes nothing and is ordinary
// work. The exit is to stop claiming a remediation, which no message ever said.
func runtimeDischargedFailureRefusal(evidence string, dischargedByOrdinal int) error {
	return fmt.Errorf(
		"failed verification %s has already been repaired by the passing settlement at attempt %d, so the attempt chain holds no unremediated failure for this correction to name; this work unit is ordinary work — settle it with the same flags but WITHOUT --remediates-evidence-revision. Run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` to read the chain: a correction plan decomposed into several bounded work units names the failure once, on the slice that repairs it, and the remaining slices settle plainly",
		evidence, dischargedByOrdinal)
}

// runtimeChargedCandidateRefusal replaces "approved SDD remediation successor
// does not bind the natively charged candidate" (#2088).
//
// Its reporter tried six configurations, every one of them prescribed by the
// runtime's own dispatcher, and asked in the issue for "which
// tree/identity/manifest relationship is checked". The check is one tree
// equality, and printing both sides answers the question outright: either the
// workspace is not the approved bytes, or the approved bytes are not this
// candidate, and the operator can see at a glance which.
func runtimeChargedCandidateRefusal(approvedTree, chargedTree, lineage string) error {
	return fmt.Errorf(
		"approved review %s covers candidate tree %s, but this attempt charged tree %s, so the successor does not bind the work being settled; these two trees must be identical. Either restore the workspace to the approved bytes and settle again, or get the charged tree approved and name that lineage — `gentle-ai review status --cwd <repo> --contract %s --next-transition` names the next review action for the candidate you actually have. `git diff %s %s` shows exactly what differs",
		lineage, approvedTree, chargedTree, runtimeReviewIntegrationContract, approvedTree, chargedTree)
}

// runtimeBoundPredecessorRefusal replaces "bound compact predecessor is stale
// or not approved" (#2894), which reported two different problems, with two
// different exits, in one sentence that distinguished neither.
func runtimeBoundPredecessorRefusal(lineage, boundRevision, currentRevision string, stale bool) error {
	if stale {
		return fmt.Errorf(
			"this change is bound to review %s at revision %s, but that lineage has since advanced to %s, so the binding no longer describes its authority; re-bind to the current revision with `gentle-ai review bind-sdd --cwd <repo> --change <change> --lineage %s` and settle again",
			lineage, boundRevision, currentRevision, lineage)
	}
	return fmt.Errorf(
		"this change is bound to review %s at revision %s, and that revision is not in an approved state, so it cannot authorize a settlement; run `gentle-ai review status --cwd <repo> --contract %s --next-transition` to see what that lineage still owes before it can approve",
		lineage, boundRevision, runtimeReviewIntegrationContract)
}

// runtimeDischargedFailure answers the question #2881's refusal could not:
// was the failure this correction names real, and already repaired?
//
// It walks the same immutable chain the binding derives from, looking for the
// named failure and then for the passing settlement that discharged it. A hit
// means the operator's input was correct and merely obsolete, which is a very
// different thing to be told than "your input is wrong".
func runtimeDischargedFailure(attempts []RuntimeAttempt, named string) (string, int, bool) {
	if named == "" {
		return "", 0, false
	}
	failedAt := -1
	for index, attempt := range attempts {
		if attempt.Outcome == AttemptFailed && attempt.EvidenceRevision == named {
			failedAt = index
		}
	}
	if failedAt < 0 {
		return "", 0, false
	}
	for _, attempt := range attempts[failedAt+1:] {
		if attempt.Outcome == AttemptPassed {
			return named, attempt.Ordinal, true
		}
	}
	return "", 0, false
}
