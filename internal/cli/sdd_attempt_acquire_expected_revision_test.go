package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunSDDAttemptAcquireAcceptsExpectedRevision is the RED reproduction for
// #4160: `sdd-attempt acquire --expected-revision <rev>` used to fail at the
// CLI flag-parsing boundary with "flag provided but not defined:
// -expected-revision", even though reset/status guidance can point a caller
// at exactly that continuation. Matching the live revision must proceed;
// naming a stale one must block (not parse-fail) with the typed
// stale-revision shape every legacy verb already returns.
func TestRunSDDAttemptAcquireAcceptsExpectedRevision(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "acquire-expected-revision"

	status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})

	matching, _ := runCompactSDDAttempt(t, append(
		compactAcquireArgs(repo, change, "acquire-revision-match", 2),
		"--expected-revision", status.Revision,
	))
	if matching.State != "proceed" || matching.Token == "" {
		t.Fatalf("acquire --expected-revision %q = %#v", status.Revision, matching)
	}
	// Close the attempt so the ledger is free again and a fresh acquire's own
	// CAS check (not the unrelated active-attempt block) is what a stale
	// --expected-revision actually hits below.
	closed, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, matching.Token, "acquire-revision-close", "failed"))
	if closed.State != "proceed" {
		t.Fatalf("settle to free the ledger = %#v", closed)
	}

	stale, _ := runCompactSDDAttempt(t, append(
		compactAcquireArgs(repo, change, "acquire-revision-stale", 2),
		"--expected-revision", cliAttemptHash('9'),
	))
	if stale.State != "blocked" || stale.Reason != "invalid_continuation" {
		t.Fatalf("acquire with a stale --expected-revision = %#v, want blocked/invalid_continuation", stale)
	}
	if !strings.Contains(stale.Detail, "--expected-revision") {
		t.Fatalf("stale --expected-revision block detail = %q, want it to name --expected-revision", stale.Detail)
	}

	// The block must not have appended to the chain: status still reports the
	// SAME single settled attempt from above, no second one.
	after := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	if len(after.Attempts) != 1 {
		t.Fatalf("stale --expected-revision acquire mutated the ledger: %#v", after)
	}
}

// TestRunSDDAttemptAcquireTokenAndExpectedRevisionMustAgree covers #4160's
// second clause at the CLI boundary: acquire now parses both --token and
// --expected-revision, and presenting two different revisions through them
// is refused rather than silently resolved one way or the other.
func TestRunSDDAttemptAcquireTokenAndExpectedRevisionMustAgree(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "acquire-token-revision-agree"

	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "agree-parent", 2))
	if started.State != "proceed" {
		t.Fatalf("parent acquire = %#v", started)
	}

	var conflict bytes.Buffer
	err := RunSDDAttempt(append(
		compactAcquireArgs(repo, change, "agree-conflict", 2),
		"--token", started.Token, "--expected-revision", cliAttemptHash('5'),
	), &conflict)
	if err == nil {
		t.Fatalf("acquire with disagreeing --token and --expected-revision = %s, want a refusal", conflict.String())
	}
	if !strings.Contains(err.Error(), "--token") && !strings.Contains(err.Error(), "token") {
		t.Fatalf("disagreeing --token/--expected-revision refusal = %v, want it to name token", err)
	}
}

// assertSDDAttemptCommandFlagsAreDefined proves the OTHER half of a named
// continuation being runnable (#4160): every `gentle-ai sdd-attempt <verb>
// ...` command extracted from a refusal must only carry `--flag` names that
// verb's own flag.FlagSet actually defines. #4160 was reachable exactly
// because nothing checked this: guidance could point a caller at a verb with
// a flag that verb's own parser rejects outright.
func assertSDDAttemptCommandFlagsAreDefined(t *testing.T, arguments []string) {
	t.Helper()
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "sdd-attempt" {
		t.Fatalf("not a runnable gentle-ai sdd-attempt command: %v", arguments)
	}
	verb := arguments[2]
	if _, ok := sddAttemptOperationDefinition(verb); !ok {
		t.Fatalf("named command uses unknown sdd-attempt operation %q: %v", verb, arguments)
	}
	for index := 3; index < len(arguments); index++ {
		argument := arguments[index]
		if !strings.HasPrefix(argument, "--") {
			continue
		}
		name := strings.TrimPrefix(argument, "--")
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
		} else if index+1 < len(arguments) {
			index++
		}
		if _, ok := sddAttemptOperationFlag(verb, name); !ok {
			t.Fatalf("gentle-ai sdd-attempt %s names --%s, which that operation does not define: %v", verb, name, arguments)
		}
	}
}

// allNamedGentleCommands extracts every backtick-delimited `gentle-ai ...`
// command a message names, in order, reusing the exact same tokenizer
// namedRunnableGentleCommand relies on so a walk of several commands in one
// message stays byte-identical to what a single extraction would produce.
func allNamedGentleCommands(t *testing.T, message string) [][]string {
	t.Helper()
	var found [][]string
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
			found = append(found, splitNamedCommand(t, span))
		}
		rest = remainder[closing+1:]
	}
	return found
}

// TestSDDAttemptGuidanceCommandsOnlyNameDefinedFlags is the guidance-side
// guard for #4160: every `gentle-ai sdd-attempt <verb>` command named by
// runtime guidance must only carry flags that verb actually defines. This
// walks the two representative shapes reset/status guidance actually
// prints -- the drifted-objective refusal (begin points at reset) and the
// exhausted-budget block (acquire points at status, then at reset) -- rather
// than re-deriving them, and would have caught #4160 if acquire's own
// guidance had ever named a flag acquire's parser did not accept.
func TestSDDAttemptGuidanceCommandsOnlyNameDefinedFlags(t *testing.T) {
	t.Run("drifted objective begin names a runnable reset", func(t *testing.T) {
		fixture := newDriftedObjectiveFixture(t, "guidance-drift")
		message := driftedBeginRefusal(t, fixture, "guidance-drift-begin-2", fixture.objective)
		for _, command := range allNamedGentleCommands(t, message) {
			assertSDDAttemptCommandFlagsAreDefined(t, command)
		}
	})

	t.Run("exhausted budget acquire names a runnable status and reset", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		const change = "guidance-budget"
		acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "guidance-budget-1", 1))
		if acquired.State != "proceed" {
			t.Fatalf("acquire = %#v", acquired)
		}
		// Spending the sole attempt allowed reaches decision-required, and the
		// settle that exhausts it already reports the maintainer-decision
		// block (compactSettleResult asks the same request-blind readiness
		// question a following acquire would), naming status and reset.
		settled, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, acquired.Token, "guidance-budget-settle-1", "failed"))
		if settled.State != "blocked" || settled.Reason != "maintainer_decision" {
			t.Fatalf("budget-exhausting settle = %#v", settled)
		}
		commands := allNamedGentleCommands(t, settled.Exit)
		if len(commands) < 2 {
			t.Fatalf("exhausted-budget exit names %d commands, want at least status and reset:\n%s", len(commands), settled.Exit)
		}
		for _, command := range commands {
			assertSDDAttemptCommandFlagsAreDefined(t, command)
		}
	})
}
