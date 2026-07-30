package sddstatus

import (
	"strings"
	"testing"
)

// TestRuntimeLedgerErrorsNameTheStatusRoute is the RED-first proof for the
// organic-dx Phase 3b discoverability sweep (task 3b.2). The five SDD ledger
// refusals a caller can hit while driving `sdd-attempt begin/finish/reset`
// already carry every fact needed to recover — the envelope
// `sdd-attempt status` returns already derives the correct next_action for
// each of these blocked states — but until now the error text itself named
// nothing, leaving an agent that only sees the error string with no route
// back into the negotiation. Each of these five sentinel messages must
// suffix the caller-side pointer to that status route.
//
// The pointer names `--cwd <repo> --change <change>` because the bare
// `gentle-ai sdd-attempt status` command is rejected by the CLI for missing
// required flags (internal/cli/sdd_attempt.go requires --cwd and --change).
// A continuation that fails when pasted is worse than none: it costs a round
// trip to discover. `<repo>`/`<change>` mirrors the exact placeholder
// convention already used for this same command in the SDD orchestrator
// prompt assets (e.g. internal/assets/generic/sdd-orchestrator.md).
func TestRuntimeLedgerErrorsNameTheStatusRoute(t *testing.T) {
	const pointer = "run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` — its next_action names the continuation"
	for _, sentinel := range []error{
		ErrRuntimeBudgetExhausted,
		ErrRuntimeAttemptActive,
		ErrRuntimeNoActiveAttempt,
		ErrRuntimeObjectiveDone,
		ErrRuntimeNoObjective,
	} {
		if !strings.Contains(sentinel.Error(), pointer) {
			t.Errorf("sentinel %q does not name the sdd-attempt status route (want it to contain %q)", sentinel.Error(), pointer)
		}
	}
}
