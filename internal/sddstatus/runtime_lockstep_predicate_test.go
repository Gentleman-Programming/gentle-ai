package sddstatus

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// #3816 change 2. The premise this change started from was wrong and is worth
// stating: replay does NOT merely re-derive what the writer already enforced.
// Fourteen sites in the ledger document that replay must not trust a record's
// own claims -- applyRuntimeRescopeEvent rejects "a corrupted/forged record
// claiming a widened ceiling ... on replay, not merely refused at write time".
// Writer and replay have different threat models: the writer validates the
// request it is handed, replay validates that the committed chain is coherent
// without believing any record about itself. Deleting replay's side would not
// remove duplication, it would remove the only defence against a record lying
// about its own derived fields.
//
// What #2830 actually exposed is DRIFT: the two sides disagreeing. The cure is
// to make the lockstep provable rather than conventional. The ledger already
// does this for the harder rules -- runtimeEvidenceOnlyRetryAuthorized,
// runtimeChainFailedAttempt, runtimeResetStructurallyPermitted and
// runtimeRemediationCandidateUnchanged are each called from both sides. These
// guards finish that job and stop the extracted predicates from being inlined
// back into one side.

func productionLedgerSource(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("runtime_ledger.go")
	if err != nil {
		t.Fatalf("read ledger source: %v", err)
	}
	return string(content)
}

// TestChangedLineBudgetDecisionHasOneOwner pins that the changed-line budget
// predicate is computed in exactly one place. It was two literal copies: the
// writer built the record's ChangedLineBudgetExceeded field, and replay
// recomputed the same expression to check the record had not lied about it.
// Both still happen -- that is the point -- but from one definition.
func TestChangedLineBudgetDecisionHasOneOwner(t *testing.T) {
	source := productionLedgerSource(t)
	// Exactly one occurrence is expected: the body of the shared predicate
	// itself. Any additional occurrence is a call site that inlined the rule
	// again instead of calling it.
	inline := regexp.MustCompile(`CumulativeChangedLines\s*\+\s*\w+(?:\.\w+)*\s*>\s*\S*Objective\.MaxChangedLines`)
	if got := len(inline.FindAllString(source, -1)); got != 1 {
		t.Errorf("the changed-line budget expression appears %d time(s), want exactly 1 (inside runtimeChangedLineBudgetExceeded); every other site must call it", got)
	}
	if strings.Count(source, "func runtimeChangedLineBudgetExceeded(") != 1 {
		t.Error("runtimeChangedLineBudgetExceeded is not declared exactly once")
	}
	if got := strings.Count(source, "runtimeChangedLineBudgetExceeded("); got < 3 {
		t.Errorf("runtimeChangedLineBudgetExceeded has %d references; the writer and replay must both call it", got)
	}
}

// TestLockstepPredicatesStayShared pins that the predicates already extracted
// for both sides keep more than one caller. A predicate with a single caller
// has silently stopped being a lockstep twin.
func TestLockstepPredicatesStayShared(t *testing.T) {
	source := productionLedgerSource(t)
	for _, predicate := range []string{
		"runtimeEvidenceOnlyRetryAuthorized",
		"runtimeChainFailedAttempt",
		"runtimeResetStructurallyPermitted",
		"runtimeRemediationCandidateUnchanged",
		"runtimeChangedLineBudgetExceeded",
	} {
		if got := strings.Count(source, predicate+"("); got < 3 {
			t.Errorf("%s has %d occurrences (declaration plus callers); it must be called from both the write and replay paths", predicate, got)
		}
	}
}
