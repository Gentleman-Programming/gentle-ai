package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

func cliAttemptChangePath(repo, change, name string) string {
	return filepath.Join(repo, "openspec", "changes", change, name)
}

func readSharedSDDStatusContract(t *testing.T) string {
	t.Helper()
	return string(assets.MustRead("skills/_shared/sdd-status-contract.md"))
}

// TestSDDAttemptAcquireCarriesTheSettleObligationEndToEnd walks #2912's exact
// sequence through the real CLI, because the unit test proves the ledger knows
// and this proves the operator is told.
//
// The reporter's loss was not the demand, it was the timing: they failed a
// verification, took an audited reset to a distinct objective, did 118 lines
// of honest work, and only then learned the settle was bound to a failure they
// had never seen. The attempt was gone. They closed it as interrupted rather
// than record a claim they could not verify.
//
// So the assertion is about WHEN: the acquire that hands out the token, before
// a single line of the next work unit is written, names the demand and its
// exact value.
func TestSDDAttemptAcquireCarriesTheSettleObligationEndToEnd(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const change = "settle-obligation-e2e"
	disableReviewForClone(t, repo)

	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "proposal.md"), "# Proposal\n")
	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "tasks.md"), "- [x] 1.1 Work\n")
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

	// Objective A fails, spending its only attempt.
	writeCLIAttemptFile(t, cliAttemptChangePath(repo, change, "tasks.md"), "- [x] 1.1 Work\n# media\n")
	acquired, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "a1",
		"--work-unit", "media", "--evidence-goal", "verify media", "--max-attempts", "1", "--max-changed-lines", "400",
	})
	if acquired.State != "proceed" {
		t.Fatalf("first acquire = %#v", acquired)
	}
	if acquired.SettleObligation != "" {
		t.Fatalf("a clean chain produced an obligation: %s", acquired.SettleObligation)
	}
	failedEvidence := cliAttemptHash('a')
	_, _ = runCompactSDDAttempt(t, []string{
		"settle", "--cwd", repo, "--change", change, "--token", acquired.Token, "--request-id", "a2",
		"--outcome", "failed", "--evidence-revision", failedEvidence, "--diagnosis", "media verification failed",
		"--harness-disposition", "reused", "--cleanup-evidence", "cleanup done", "--process-evidence", "no descendants",
	})

	// The maintainer takes an audited reset to a distinct objective. Per
	// #1974 slice 2 that is an audit record, not a release: the binding lives.
	status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	var reset bytes.Buffer
	if err := RunSDDAttempt([]string{
		"reset", "--cwd", repo, "--change", change, "--expected-revision", status.Revision,
		"--request-id", "r1", "--reason", "media abandoned; scoring is a separate work unit", "--actor", "maintainer",
	}, &reset); err != nil {
		t.Fatalf("audited reset: %v\n%s", err, reset.String())
	}

	// THE ASSERTION: the next acquire, before any work, says what its settle
	// will owe.
	next, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "b1",
		"--work-unit", "scoring", "--evidence-goal", "verify scoring", "--max-attempts", "2", "--max-changed-lines", "400",
	})
	if next.State != "proceed" || next.Token == "" {
		t.Fatalf("the obligation must never block the launch: %#v", next)
	}
	if next.SettleObligation == "" {
		t.Fatal("acquire issued a token and said nothing about the remediation its settle is already bound to; the attempt is spent by the time the operator finds out (#2912)")
	}
	for _, want := range []string{"--remediates-evidence-revision", failedEvidence} {
		if !strings.Contains(next.SettleObligation, want) {
			t.Fatalf("settle_obligation does not name %q:\n%s", want, next.SettleObligation)
		}
	}

	// And it is the same fact the read-only surface reports, so an
	// orchestrator that polls status instead of relaying acquire sees it too.
	scoped := runSDDAttemptStatus(t, []string{
		"status", "--cwd", repo, "--change", change,
		"--work-unit", "scoring", "--evidence-goal", "verify scoring",
		"--max-attempts", "2", "--max-changed-lines", "400",
	})
	if scoped.SettleObligation != next.SettleObligation {
		t.Fatalf("status and acquire disagree:\nstatus:  %q\nacquire: %q", scoped.SettleObligation, next.SettleObligation)
	}
}

// TestSDDStatusContractRequiresRelayingTheSettleObligation is the other half of
// the fix, and the one a Go change alone cannot deliver.
//
// A field nobody renders is #2471's root 9 exactly: an obligation declared in
// text that nothing enforces. If the shared contract does not tell the
// orchestrator to relay it, #2912's reporter loses the same attempt again with
// the answer sitting unread in a JSON field.
func TestSDDStatusContractRequiresRelayingTheSettleObligation(t *testing.T) {
	contract := readSharedSDDStatusContract(t)
	if !strings.Contains(contract, "settle_obligation") {
		t.Fatal("the shared SDD status contract never mentions settle_obligation, so no runtime is told it exists")
	}
	for _, want := range []string{"RELAY IT TO THE HUMAN VERBATIM", "BEFORE LAUNCHING"} {
		if !strings.Contains(contract, want) {
			t.Fatalf("the contract does not make the relay mandatory and time-bound; it must contain %q", want)
		}
	}
	if !strings.Contains(contract, "never a block") {
		t.Fatal("the contract must say the obligation is not a block, or orchestrators will treat a proceed as a stop")
	}
}

// TestReviewLedgerContractAdmitsVerificationRefreshAfterRemediation is #2617.
//
// Its reporter had four doors and every one was shut: keeping the admitted
// failed report blocks archive; re-running SDD verification appeared to
// violate "one independent requirements/runtime verification"; review was off
// at clone scope; and the accepted remediation settlement did not by itself
// produce an archive-admissible state.
//
// The code was never the problem — native status already routes to `verify`
// once a remediation settles. The contract sentence was, because its four
// nouns (reviewer, refuter, correction, validator) are all REVIEW actors, and
// it read as forbidding SDD's own verification from ever running twice. The
// contract must now say which lifecycle it bounds.
func TestReviewLedgerContractAdmitsVerificationRefreshAfterRemediation(t *testing.T) {
	// The clarification lives in the SDD status contract, not the review
	// ledger contract: the review contract's rendered prompt sits inside a
	// deliberate token ceiling with under 15% headroom, and archive
	// eligibility is an SDD question anyway.
	if !strings.Contains(string(assets.MustRead("skills/_shared/review-ledger-contract.md")),
		"never starts another reviewer, refuter, correction, or validator") {
		t.Fatal("the one-verification sentence is gone; this guard is stale")
	}
	contract := readSharedSDDStatusContract(t)
	for _, want := range []string{
		"REVIEW actors",
		"ADMITS one verification refresh",
		"not a second independent verification",
	} {
		if !strings.Contains(contract, want) {
			t.Fatalf("the contract does not disambiguate the one-verification rule; it must contain %q", want)
		}
	}
	// And it must not be read as a licence to fabricate a pass.
	for _, want := range []string{"preserved and never erased", "no PASS is fabricated"} {
		if !strings.Contains(contract, want) {
			t.Fatalf("the clarification drops a guarantee it must keep: %q", want)
		}
	}
}
