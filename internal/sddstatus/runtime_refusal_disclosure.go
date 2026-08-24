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
