package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// #2588: the bounded attempt budget was spent entirely on the apply worker
// failing to CONSTRUCT a valid harness — four different shell defects across
// three executions, every one of them before a single consumer command ran.
// The reporter ended at blocked(maintainer_decision) knowing nothing about
// their own change, because their change was never exercised.
//
// Two things were wrong, and only one of them is the budget.
//
// The ledger recorded provider defects as evidence about the candidate. The
// immutable chain now says "this candidate failed four bounded attempts" for a
// candidate that was never tried, and that chain is what later remediation,
// reset and archive decisions read.
//
// And maintainer_decision was a dead end in prose: it named a reset the human
// had to know about and assemble by hand from six flags. There is no reason
// the exhausted state cannot simply ASK, the way a medium-risk review START
// asks before it spends anything.
//
// Deliberately NOT a fixed exemption count. An exemption an actor can claim is
// not verifiable, so any N large enough to help is large enough to stop the
// budget bounding anything. A grant a human issues is verifiable and audited,
// and it needs no calibration.

func TestExhaustedBudgetAsksInsteadOfDeadEnding(t *testing.T) {
	envelope, err := sddstatus.BudgetConsentEnvelope(sddstatus.BudgetConsentInput{
		Repo: "/repo", Change: "demo", Revision: "sha256:" + strings.Repeat("a", 64),
		MaxAttempts: 2, CumulativeAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.Blocking {
		t.Fatal("the envelope must mark itself blocking: nothing may proceed until the human answers")
	}
	if len(envelope.Choices) != 2 || envelope.Choices[0].Answer != "granted" || envelope.Choices[1].Answer != "declined" {
		t.Fatalf("choices = %#v, want exactly granted then declined", envelope.Choices)
	}
	// The grant has to be runnable verbatim, not described. The whole defect
	// being fixed is a human being told to assemble a six-flag reset by hand.
	grant := envelope.Choices[0].Invocation
	for _, want := range []string{"gentle-ai sdd-attempt reset", "--cwd", "--change", "--expected-revision", "sha256:" + strings.Repeat("a", 64)} {
		if !strings.Contains(grant, want) {
			t.Fatalf("the grant invocation is not runnable verbatim (missing %q):\n%s", want, grant)
		}
	}
	if envelope.Choices[1].Invocation == "" {
		t.Fatal("declining must also be a runnable answer, not an absence")
	}
	// The accounting the human is deciding about has to be in front of them.
	joined := strings.Join(envelope.Evidence, "\n")
	if !strings.Contains(joined, "2") {
		t.Fatalf("the evidence does not show the attempt accounting being decided:\n%s", joined)
	}
}

// TestHarnessFailureIsTypedSoTheQuestionCanBeAnswered is the half that makes
// the question decidable. A prompt that only says "retry?" reproduces #2588
// with a click in the middle: the reporter answered the equivalent question
// four times because nothing distinguished the tool failing from their code
// failing.
func TestHarnessFailureIsTypedSoTheQuestionCanBeAnswered(t *testing.T) {
	envelope, err := sddstatus.BudgetConsentEnvelope(sddstatus.BudgetConsentInput{
		Repo: "/repo", Change: "demo", Revision: "sha256:" + strings.Repeat("a", 64),
		MaxAttempts: 2, CumulativeAttempts: 2, HarnessFailures: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := envelope.Headline + "\n" + envelope.Reason + "\n" + strings.Join(envelope.Evidence, "\n")
	if !strings.Contains(text, "harness") {
		t.Fatalf("the envelope does not say the attempts were spent on harness failures:\n%s", text)
	}
	// The second harness failure is information, not noise: it says the worker
	// cannot converge. The human must be told that rather than being asked to
	// infer it.
	if !strings.Contains(text, "never ran") && !strings.Contains(text, "not evidence about") {
		t.Fatalf("the envelope does not tell the human these attempts produced no evidence about their change:\n%s", text)
	}

	clean, err := sddstatus.BudgetConsentEnvelope(sddstatus.BudgetConsentInput{
		Repo: "/repo", Change: "demo", Revision: "sha256:" + strings.Repeat("a", 64),
		MaxAttempts: 2, CumulativeAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(clean.Headline+clean.Reason+strings.Join(clean.Evidence, "\n"), "harness") {
		t.Fatal("an ordinary exhausted budget must not claim harness failures it did not have")
	}
}

// TestBudgetConsentRefusesAnIncompleteEnvelope keeps the producer honest
// through the shared core's own completeness rule, the same one the review
// envelope answers to.
func TestBudgetConsentRefusesAnIncompleteEnvelope(t *testing.T) {
	if _, err := sddstatus.BudgetConsentEnvelope(sddstatus.BudgetConsentInput{
		Repo: "/repo", Change: "demo", MaxAttempts: 2, CumulativeAttempts: 2,
	}); err == nil {
		t.Fatal("an envelope with no revision cannot name a runnable reset, so it must refuse to be built")
	}
}

// TestExhaustedBudgetSurfacesTheQuestionEndToEnd is the half a constructor
// cannot deliver. An envelope nobody renders is #2471's root 9, and it is
// exactly how #2588's reporter ended up answering the same implicit question
// four times: the ledger knew, and never asked.
func TestExhaustedBudgetSurfacesTheQuestionEndToEnd(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const change = "budget-consent-e2e"
	disableReviewForClone(t, repo)

	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "proposal.md"), "# Proposal\n")
	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "tasks.md"), "- [x] 1.1 Work\n")
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

	// One attempt, one budget, and the harness never runs the work: the actor
	// settles it invalidated, which is the settle contract's existing way of
	// saying the harness could not be used.
	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "tasks.md"), "- [x] 1.1 Work\n# harness\n")
	acquired, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "h1",
		"--work-unit", "acceptance", "--evidence-goal", "postgres acceptance",
		"--max-attempts", "1", "--max-changed-lines", "400",
	})
	if acquired.State != "proceed" {
		t.Fatalf("acquire = %#v", acquired)
	}
	runCompactSDDAttempt(t, []string{
		"settle", "--cwd", repo, "--change", change, "--token", acquired.Token, "--request-id", "h2",
		"--outcome", "failed", "--evidence-revision", cliAttemptHash('a'),
		"--diagnosis", "the harness could not be constructed; no consumer command ran",
		"--harness-disposition", "invalidated",
		"--cleanup-evidence", "container removed", "--process-evidence", "no descendants",
	})

	// The budget is gone. Status must now ASK, with the accounting and a
	// verbatim-runnable grant.
	scoped := runSDDAttemptStatus(t, []string{
		"status", "--cwd", repo, "--change", change,
		"--work-unit", "acceptance", "--evidence-goal", "postgres acceptance",
		"--max-attempts", "1", "--max-changed-lines", "400",
	})
	if scoped.BlockedReason != "maintainer_decision" {
		t.Fatalf("blocked_reason = %q, want maintainer_decision", scoped.BlockedReason)
	}
	if scoped.Consent == nil {
		t.Fatal("the exhausted budget dead-ended instead of asking; the ledger knew it needed a decision and never put the question in front of anyone (#2588)")
	}
	if !scoped.Consent.Blocking || len(scoped.Consent.Choices) != 2 {
		t.Fatalf("consent = %#v, want a blocking two-choice question", scoped.Consent)
	}
	if !strings.Contains(scoped.Consent.Choices[0].Invocation, "gentle-ai sdd-attempt reset") ||
		!strings.Contains(scoped.Consent.Choices[0].Invocation, scoped.Revision) {
		t.Fatalf("the grant is not runnable verbatim against this exact ledger revision:\n%s", scoped.Consent.Choices[0].Invocation)
	}
	// And it must say the attempts produced no evidence about the candidate,
	// or the human cannot tell this apart from their own code failing.
	text := scoped.Consent.Headline + scoped.Consent.Reason + strings.Join(scoped.Consent.Evidence, "\n")
	if !strings.Contains(text, "harness") || !strings.Contains(text, "never ran") {
		t.Fatalf("the question does not distinguish a provider defect from the candidate failing:\n%s", text)
	}
}

// TestSDDStatusContractRequiresRelayingTheBudgetConsent closes the loop a Go
// change cannot: an envelope no runtime is told to relay is #2471's root 9.
func TestSDDStatusContractRequiresRelayingTheBudgetConsent(t *testing.T) {
	contract := readSharedSDDStatusContract(t)
	for _, want := range []string{
		"gentle-ai.sdd-integration.consent/v1",
		"Lossless Blocking Prompt",
		"never answer on their behalf",
		"non-interactive runtime, emit the complete envelope and STOP",
		"not evidence about the candidate",
	} {
		if !strings.Contains(contract, want) {
			t.Fatalf("the contract does not bind the consent relay; it must contain %q", want)
		}
	}
}
