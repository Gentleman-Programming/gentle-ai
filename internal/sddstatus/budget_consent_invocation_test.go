package sddstatus

import (
	"context"
	"strings"
	"testing"
)

func budgetConsentFixture() BudgetConsentInput {
	return BudgetConsentInput{
		Repo:     "/workspace/repo",
		Change:   "harden-login",
		Revision: "sha256:" + strings.Repeat("a", 64),

		MaxAttempts: 1, CumulativeAttempts: 1,
		MaxChangedLines: 400, CumulativeLines: 12,
	}
}

// grantInvocationFlag pulls one flag value out of the granted invocation the
// same way a relaying orchestrator would: by reading the exact string, not by
// re-deriving it from the input.
func grantInvocationFlag(t *testing.T, invocation, flag string) string {
	t.Helper()
	fields := strings.Fields(invocation)
	for index, field := range fields {
		if field == flag && index+1 < len(fields) {
			return strings.Trim(fields[index+1], `"`)
		}
	}
	t.Fatalf("granted invocation carries no %s: %s", flag, invocation)
	return ""
}

func grantInvocation(t *testing.T, in BudgetConsentInput) string {
	t.Helper()
	envelope, err := BudgetConsentEnvelope(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range envelope.Choices {
		if choice.Answer == sddBudgetConsentGranted {
			return choice.Invocation
		}
	}
	t.Fatal("envelope carries no granted choice")
	return ""
}

// TestGrantedResetInvocationIsExecutableAsReturned is #3045, which #2959
// introduced: I shipped the exhausted-budget question with literal
// "<unique-request-id>" and "<actor>" inside the granted invocation.
//
// The relay contract requires executing the provider-owned invocation exactly
// as returned, and forbids the orchestrator from rewriting it. So the one
// command a granted answer produces was rejected by this project's own CLI
// before any ledger mutation, and the consumer landed back in
// decision_required with the same revision and the same envelope. A consent
// question whose granted answer cannot be carried out is not a question.
func TestGrantedResetInvocationIsExecutableAsReturned(t *testing.T) {
	invocation := grantInvocation(t, budgetConsentFixture())

	for _, placeholder := range []string{"<unique-request-id>", "<actor>", "<", ">"} {
		if strings.Contains(invocation, placeholder) {
			t.Fatalf("granted invocation still carries the placeholder %q, so executing it verbatim fails input validation: %s", placeholder, invocation)
		}
	}

	requestID := grantInvocationFlag(t, invocation, "--request-id")
	if !runtimeRequestIDPattern.MatchString(requestID) {
		t.Fatalf("granted --request-id %q is not a canonical lowercase identifier, which is the exact refusal #3045 reported", requestID)
	}
	// The actor is audit metadata, not authenticated human authority, so it is
	// a transparent fixed runtime literal rather than a value the orchestrator
	// would have to invent. Ratified on the issue.
	if actor := grantInvocationFlag(t, invocation, "--actor"); actor != sddRuntimeAuditActor {
		t.Fatalf("granted --actor = %q, want the fixed runtime audit actor %q", actor, sddRuntimeAuditActor)
	}
}

// TestGrantedResetRequestIDIsDeterministicAndBound pins replay. The same
// objective at the same revision must produce the same request ID, so
// executing the returned command twice is idempotent rather than opening two
// budgets; a different revision must produce a different one, so a stale
// granted command cannot be replayed onto moved authority. CAS on
// --expected-revision is the enforcement; a distinct ID keeps the audit trail
// honest about which decision it was.
func TestGrantedResetRequestIDIsDeterministicAndBound(t *testing.T) {
	base := budgetConsentFixture()
	first := grantInvocationFlag(t, grantInvocation(t, base), "--request-id")
	again := grantInvocationFlag(t, grantInvocation(t, base), "--request-id")
	if first != again {
		t.Fatalf("request id is not deterministic: %q then %q; replaying the returned command would open a second budget", first, again)
	}

	moved := base
	moved.Revision = "sha256:" + strings.Repeat("b", 64)
	if drifted := grantInvocationFlag(t, grantInvocation(t, moved), "--request-id"); drifted == first {
		t.Fatalf("request id %q does not change with the expected revision, so a stale grant is indistinguishable from a current one", drifted)
	}

	other := base
	other.Change = "harden-billing"
	if elsewhere := grantInvocationFlag(t, grantInvocation(t, other), "--request-id"); elsewhere == first {
		t.Fatalf("request id %q is shared across changes, so it is not bound to this objective", elsewhere)
	}
}

// TestDeclineInvocationStaysReadOnly guards the half that was never broken:
// declining re-enters native status and mutates nothing. It is pinned here
// because the fix touches the same envelope.
func TestDeclineInvocationStaysReadOnly(t *testing.T) {
	envelope, err := BudgetConsentEnvelope(budgetConsentFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range envelope.Choices {
		if choice.Answer != sddBudgetConsentDeclined {
			continue
		}
		if !strings.HasPrefix(choice.Invocation, "gentle-ai sdd-attempt status ") {
			t.Fatalf("decline no longer re-enters through read-only status: %s", choice.Invocation)
		}
		if strings.Contains(choice.Invocation, "reset") || strings.Contains(choice.Invocation, "<") {
			t.Fatalf("decline invocation gained a mutation or a placeholder: %s", choice.Invocation)
		}
		return
	}
	t.Fatal("envelope carries no declined choice")
}

// TestStaleGrantFailsCASWithoutMutation completes #3045's closure coverage.
//
// The deterministic request ID makes an exact replay idempotent, which is what
// the relay contract needs. The other half is that a grant captured BEFORE
// authority drifted must not apply afterwards: that grant names an earlier
// expected revision and carries a different ID, so it is a different decision
// about a state that no longer exists. --expected-revision is the enforcement;
// this pins that it actually refuses, and that the ledger is untouched when it
// does.
func TestStaleGrantFailsCASWithoutMutation(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "stale-grant")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "work\n")
	begun, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID: "s-begin", WorkUnit: "u", EvidenceGoal: "g",
		MaxAttempts: 1, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: begun.Revision, RequestID: "s-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The grant the envelope would hand out at this revision.
	captured := sddBudgetConsentResetRequestID(BudgetConsentInput{
		Repo: repo, Change: "stale-grant", Revision: exhausted.Revision,
	})

	// Authority drifts for an unrelated reason: some other reset lands first.
	drifted, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: exhausted.Revision, RequestID: "someone-else",
		Reason: "another decision reached the ledger first", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if drifted.Revision == exhausted.Revision {
		t.Fatal("fixture did not drift the revision, so the stale case is not staged")
	}

	// The captured grant, executed verbatim, against the moved authority.
	_, staleErr := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: exhausted.Revision, RequestID: captured,
		Reason: "maintainer authorized a fresh budget for this objective", Actor: sddRuntimeAuditActor,
	})
	if staleErr == nil {
		t.Fatal("a grant captured before the drift applied afterwards; it names a revision that no longer exists")
	}

	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != drifted.Revision {
		t.Fatalf("refused stale grant still moved the ledger: %q -> %q", drifted.Revision, after.Revision)
	}
}
