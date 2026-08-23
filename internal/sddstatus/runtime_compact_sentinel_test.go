//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// compactBlockedCallRegexp extracts every literal CompactBlockReason
// identifier passed as compactBlocked's first argument in runtime_compact.go.
// A non-literal call (a variable holding a reason) would not match here --
// deliberate, matching reviewStopTransitionCallRegexp's own precedent in
// internal/cli/review_next_transition_docs_test.go: the guard can only ever
// prove what it can read as plain text, and every real call site in this
// file passes a literal constant.
var compactBlockedCallRegexp = regexp.MustCompile(`compactBlocked\((CompactBlock[A-Za-z]+),`)

// compactBlockReasonConstRegexp extracts every CompactBlockReason constant
// declaration (identifier and string value) from runtime_compact.go's own
// const block, so the reason values below are derived from source too, not
// hand-copied.
var compactBlockReasonConstRegexp = regexp.MustCompile(`(CompactBlock[A-Za-z]+)\s+CompactBlockReason\s*=\s*"([a-z_]+)"`)

// TestCompactSettleRemediationRefusalIsClassifiedNotAuthorityFailure is the RED
// reproduction for #2249: a compact `sdd-attempt settle --outcome passed` with
// otherwise valid inputs, issued against an attempt whose candidate drifted
// after Begin while a review binding is in place, used to collapse into
// {"state":"blocked","reason":"authority_failure"} while status kept
// reporting outcome: running, next_action: finish — a dead end. Root cause:
// compactMutationFailure's switch had no case for
// ErrRuntimeRemediationSuccessorRequired, so it fell through to the default
// branch and threw away runtimeRemediationExitRefusal's actionable message.
func TestCompactSettleRemediationRefusalIsClassifiedNotAuthorityFailure(t *testing.T) {
	change := "compact-remediation-legibility"
	fixture := newRuntimeUnchangedBindingFixture(t, change)

	// Drift the candidate after Begin: the ordinary continuation for a
	// passing finish bound to a review is now the remediation trio, not a
	// bare pass.
	write(t, filepath.Join(fixture.store.Repo, "openspec", "changes", change, "tasks.md"), "- [x] 1.1 Done\n# post-begin drift\n")

	result, err := fixture.store.Settle(context.Background(), CompactSettleRequest{
		Token: fixture.active.Revision, RequestID: change + "-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('e'), Diagnosis: "drifted candidate settle reproduces #2249",
		HarnessDisposition: HarnessReused, CleanupEvidence: "settle cleanup completed",
		ProcessEvidence: "settle process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	// #2249's scenario no longer blocks at all: review acts after
	// implementation and verification, so a passing settle closes on the
	// attempt's own evidence and the delivery gates decide afterwards whether
	// the drifted candidate may ship. #2249's value — no opaque
	// authority_failure dead end — is preserved by the enumeration guard below.
	if result.State == CompactStateBlocked {
		t.Fatalf("drifted compact settle = %#v, want it to close: review must not gate implementation", result)
	}
}

// TestCompactMutationFailureClassifiesEveryReachableLedgerSentinel is the
// table-driven sentinel enumeration test required alongside the #2249 fix. It
// audits every ErrRuntime*/ErrBindingRevisionConflict sentinel declared in the
// var block at runtime_ledger.go:61-92 against whether Begin or Finish (the
// only two mutations Acquire/Settle drive through compactMutationFailure) can
// actually produce it, and fails if a sentinel marked reachable still lands on
// the opaque CompactBlockAuthorityFailure default. This is a genuine
// regression guard: deleting the ErrRuntimeRemediationSuccessorRequired or
// ErrBindingRevisionConflict case from compactMutationFailure's switch makes
// this test fail, not just the #2249 repro above.
func TestCompactMutationFailureClassifiesEveryReachableLedgerSentinel(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		reachable  bool // producible by Begin or Finish, and therefore by Acquire/Settle
		wantState  CompactAttemptState
		wantReason CompactBlockReason
	}{
		{name: "ErrRuntimeObjectiveDone", err: ErrRuntimeObjectiveDone, reachable: true, wantState: CompactStateComplete},
		{name: "ErrRuntimeBudgetExhausted", err: ErrRuntimeBudgetExhausted, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockMaintainerDecision},
		{name: "ErrRuntimeObjectiveChange", err: ErrRuntimeObjectiveChange, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockMaintainerDecision},
		{name: "ErrRuntimeAttemptActive", err: ErrRuntimeAttemptActive, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockActiveAttempt},
		{name: "ErrRuntimeRevisionConflict", err: ErrRuntimeRevisionConflict, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockInvalidContinuation},
		{name: "ErrRuntimeConcurrentUpdate", err: ErrRuntimeConcurrentUpdate, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockInvalidContinuation},
		{name: "ErrRuntimeRequestConflict", err: ErrRuntimeRequestConflict, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockInvalidContinuation},
		{name: "ErrRuntimeNoActiveAttempt", err: ErrRuntimeNoActiveAttempt, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockInvalidContinuation},
		{name: "ErrRuntimeWorktreeMismatch", err: ErrRuntimeWorktreeMismatch, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockWorktreeMismatch},
		{name: "ErrRuntimeCandidateUnavailable", err: ErrRuntimeCandidateUnavailable, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockCandidateUnavailable},
		{name: "ErrBindingRevisionConflict", err: ErrBindingRevisionConflict, reachable: true, wantState: CompactStateBlocked, wantReason: CompactBlockInvalidContinuation},
		// Reset is the only mutation that can produce these two; Begin/Finish
		// never do, so Acquire/Settle never route them into
		// compactMutationFailure. They stay intentionally unclassified.
		{name: "ErrRuntimeNoObjective", err: ErrRuntimeNoObjective, reachable: false},
		{name: "ErrRuntimeResetNotAllowed", err: ErrRuntimeResetNotAllowed, reachable: false},
	}

	store := RuntimeStore{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.compactMutationFailure(tt.err, false, BeginAttemptRequest{})
			if !tt.reachable {
				return
			}
			if result.Reason == CompactBlockAuthorityFailure {
				t.Fatalf("compactMutationFailure(%v) = %#v, a Begin/Finish-reachable sentinel must not fall through to authority_failure", tt.err, result)
			}
			if result.State != tt.wantState || result.Reason != tt.wantReason {
				t.Fatalf("compactMutationFailure(%v) = %#v, want state=%q reason=%q", tt.err, result, tt.wantState, tt.wantReason)
			}
			if result.Detail == "" || result.Exit == "" {
				t.Fatalf("compactMutationFailure(%v) = %#v, want non-empty Detail/Exit", tt.err, result)
			}
		})
	}
}

// TestCompactBlockedNamesExitForEveryReachableReason is the enumeration-style
// sentinel guard for compactBlocked, the sibling constructor
// compactMutationFailure's #2249 fix never reached: every one of
// compactBlocked's 20 real call sites in this file emits a bare
// {"state":"blocked","reason":"<code>"} with no Exit/Detail — a token with
// nothing behind it on a surface that carries no stderr narration and no docs
// mirror at all. Unlike an earlier version of this test, the reachable
// reason set is DERIVED FROM SOURCE (every literal CompactBlockReason
// identifier actually passed to compactBlocked(...) in runtime_compact.go),
// not hand-copied: seven CompactBlockReason constants are declared and
// compactBlockedExitText's default: arm silently returns "" for any reason
// it doesn't recognize, so a hand-enumerated list would stay green forever
// after a new call site shipped with no exit text. This mirrors
// TestCompactMutationFailureClassifiesEveryReachableLedgerSentinel above and
// reviewStopTransitionCallRegexp's source-derivation precedent
// (internal/cli/review_next_transition_docs_test.go).
func TestCompactBlockedNamesExitForEveryReachableReason(t *testing.T) {
	source, err := os.ReadFile("runtime_compact.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	values := map[string]CompactBlockReason{}
	for _, m := range compactBlockReasonConstRegexp.FindAllStringSubmatch(text, -1) {
		values[m[1]] = CompactBlockReason(m[2])
	}
	if len(values) == 0 {
		t.Fatal("found no CompactBlockReason constant declarations in runtime_compact.go; the extraction regexp is stale")
	}

	calls := compactBlockedCallRegexp.FindAllStringSubmatch(text, -1)
	if len(calls) == 0 {
		t.Fatal("found no compactBlocked(CompactBlock*, ...) call sites in runtime_compact.go; the extraction regexp is stale")
	}
	reachable := map[string]bool{}
	for _, m := range calls {
		reachable[m[1]] = true
	}
	if len(reachable) < 4 {
		t.Fatalf("found only %d distinct reachable reasons; the survey found 4 (active_attempt, maintainer_decision, corrupt_authority, invalid_continuation), so the extraction is broken", len(reachable))
	}

	const sampleToken = "sha256:a1b2c3d4e5f60708091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f7"
	for identifier := range reachable {
		identifier := identifier
		reason, ok := values[identifier]
		if !ok {
			t.Fatalf("compactBlocked is called with %s, which has no CompactBlockReason constant declaration in runtime_compact.go; the extraction regexp is stale", identifier)
		}
		t.Run(identifier, func(t *testing.T) {
			token := ""
			if reason == CompactBlockActiveAttempt {
				token = sampleToken
			}
			result := compactBlocked(reason, token)
			if result.State != CompactStateBlocked || result.Reason != reason {
				t.Fatalf("compactBlocked(%q, %q) = %#v, want state=blocked reason=%q", reason, token, result, reason)
			}
			if result.Exit == "" || result.Detail == "" {
				t.Fatalf("compactBlocked(%q, %q) = %#v, want non-empty Exit/Detail — a blocked result with nothing behind its bare reason code is exactly the class this test exists to catch", reason, token, result)
			}
			if result.Token != token {
				t.Fatalf("compactBlocked(%q, %q) = %#v, want Token preserved unchanged", reason, token, result)
			}
		})
	}
}

// TestCompactMutationFailureLeavesUnexpectedErrorsAtAuthorityFailure proves
// the classification is not a blanket bypass: a genuinely unclassified error
// (unrelated to any declared ledger sentinel, e.g. a raw I/O failure) still
// lands on CompactBlockAuthorityFailure, and still carries Detail/Exit so it
// is visible instead of silently swallowed.
func TestCompactMutationFailureLeavesUnexpectedErrorsAtAuthorityFailure(t *testing.T) {
	store := RuntimeStore{}
	err := errors.New("simulated unexpected I/O failure")
	result := store.compactMutationFailure(err, false, BeginAttemptRequest{})
	if result.State != CompactStateBlocked || result.Reason != CompactBlockAuthorityFailure {
		t.Fatalf("compactMutationFailure(%v) = %#v, want state=blocked reason=authority_failure", err, result)
	}
	if result.Detail != err.Error() || result.Exit != err.Error() {
		t.Fatalf("compactMutationFailure(%v) = %#v, want Detail/Exit = %q", err, result, err.Error())
	}
}
