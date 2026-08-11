package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// These helpers outlive sdd_attempt_remediation_exit_test.go and
// sdd_attempt_stranded_successor_exit_test.go, whose subject — the
// bound-passing-finish refusal and the review exits it named — no longer
// exists now that review acts after implementation and verification.
// sdd_attempt_reset_exit_test.go and review_next_transition_start_lineage_test.go
// still use them.

// namedRunnableGentleCommand extracts the first backtick-delimited
// `gentle-ai ...` command a refusal names. A refusal that names an internal
// operation identifier, or names nothing at all, is the defect this file
// exists to catch: the operator is left with a non-zero exit and no door.
func namedRunnableGentleCommand(t *testing.T, message string) []string {
	t.Helper()
	rest := message
	for {
		open := strings.Index(rest, "`")
		if open < 0 {
			break
		}
		remainder := rest[open+1:]
		closing := strings.Index(remainder, "`")
		if closing < 0 {
			break
		}
		span := remainder[:closing]
		if strings.HasPrefix(span, "gentle-ai ") {
			return splitNamedCommand(t, span)
		}
		rest = remainder[closing+1:]
	}
	t.Fatalf("refusal names no runnable `gentle-ai ...` command:\n%s", message)
	return nil
}

// splitNamedCommand tokenizes exactly like a shell would for the double-quoted
// forms these messages use, so the arguments the test dispatches are the
// arguments an operator would get by pasting the line.
func splitNamedCommand(t *testing.T, command string) []string {
	t.Helper()
	fields := []string{}
	var current strings.Builder
	quoted, started := false, false
	for _, symbol := range command {
		switch {
		case symbol == '"':
			quoted = !quoted
			started = true
		case symbol == ' ' && !quoted:
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(symbol)
			started = true
		}
	}
	if quoted {
		t.Fatalf("named command has unbalanced quoting: %s", command)
	}
	if started {
		fields = append(fields, current.String())
	}
	return fields
}

// namedCommandPlaceholders returns every `<...>` token the operator still has
// to choose. Anything else in the command must already be a concrete value the
// product knows, because a value the product knows and refuses to print is a
// round trip the operator pays for nothing.
func namedCommandPlaceholders(arguments []string) []string {
	placeholders := []string{}
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "<") && strings.HasSuffix(argument, ">") {
			placeholders = append(placeholders, argument)
		}
	}
	return placeholders
}

func fillNamedCommandPlaceholder(arguments []string, placeholder, value string) []string {
	filled := append([]string(nil), arguments...)
	for index := range filled {
		if filled[index] == placeholder {
			filled[index] = value
		}
	}
	return filled
}

// TestSDDAttemptBoundFinishNamesTheExitThatWorks is the discoverability proof
// for decode2's reported deadlock (PR #1801). The refusal a bound passing
// finish emits when the candidate moved after the attempt began used to read
// "requires an atomic approved recovery successor" and named no command at
// all — and the word "recovery" pointed at `review recover`, which is the
// first step of the cycle that has no exit.
//
// b37aa8e4 made the exit legal: the bound lineage itself is an acceptable
// --successor-lineage once the corrected candidate is approved on it. This
// test requires the refusal to NAME that exit, and proves the naming by
// running it: the command is taken out of the message the product printed,
// only the operator's own run facts are substituted, and the block must then
// clear with complete: true. Asserting on wording alone is the exact defect
// class this branch exists to kill.

// createCLIRecoverySuccessor mints an unapproved scope-changed recovery
// successor for the live candidate, which is what displaces the predecessor
// from the compact recovery leaf.
func createCLIRecoverySuccessor(t *testing.T, repo, predecessorLineage, successorLineage string) {
	t.Helper()
	ctx := context.Background()
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	switch risk {
	case reviewtransaction.RiskMedium:
		lenses = []string{reviewtransaction.LensReliability}
	case reviewtransaction.RiskHigh:
		lenses = []string{
			reviewtransaction.LensRisk, reviewtransaction.LensResilience,
			reviewtransaction.LensReadability, reviewtransaction.LensReliability,
		}
	}
	successor, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: successorLineage, Mode: reviewtransaction.ModeOrdinaryBounded,
		Generation: predecessor.State.Generation + 1, Snapshot: snapshot,
		PolicyHash: cliAttemptHash('c'), RiskLevel: risk, SelectedLenses: lenses,
		OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.RecoverCompactAuthority(ctx, repo, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: successor, Disposition: reviewtransaction.RecoveryScopeChanged,
		Reason: "the candidate moved again after approval", Actor: "cli-remediation-exit-test",
	}); err != nil {
		t.Fatal(err)
	}
}
