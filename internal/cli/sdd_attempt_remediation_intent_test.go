package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// TestRunSDDAttemptAcquireRemediationIntentFailsFastAfterReset is #2564's CLI
// RED: the reporter's exact recipe (disabled review, failed verification,
// audited reset) followed by an acquire that declares its correction intent
// with --remediates-evidence-revision. Before this slice the flag was
// settle-only, so the intent was not expressible at acquire at all. With the
// chain-derived binding (#2565) the reconciled contract is: a declaration the
// immutable attempt chain does not hold unremediated earns a typed refusal
// before any correction work runs, while the chain's own failed evidence
// survives the audited reset and still issues the token.
func TestRunSDDAttemptAcquireRemediationIntentFailsFastAfterReset(t *testing.T) {
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)
	const change = "remediation-intent"

	acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "intent-acquire-verification", 1))
	if acquired.State != "proceed" {
		t.Fatalf("verification acquire = %#v", acquired)
	}
	failedEvidence := cliAttemptHash('a')
	settled, _ := runCompactSDDAttempt(t, []string{
		"settle", "--cwd", repo, "--change", change, "--token", acquired.Token,
		"--request-id", "intent-settle-verification", "--outcome", "failed",
		"--evidence-revision", failedEvidence,
		"--diagnosis", "independent verification found a correctable defect", "--harness-disposition", "reused",
		"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
	})
	if settled.State != "blocked" || settled.Reason != "maintainer_decision" {
		t.Fatalf("exhausting failed settle = %#v", settled)
	}

	var legacy bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &legacy); err != nil {
		t.Fatal(err)
	}
	var status sddstatus.RuntimeStatus
	if err := json.Unmarshal(legacy.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	var reset bytes.Buffer
	if err := RunSDDAttempt([]string{
		"reset", "--cwd", repo, "--change", change, "--expected-revision", status.Revision,
		"--request-id", "intent-reset", "--reason", "maintainer authorized bounded remediation", "--actor", "maintainer",
	}, &reset); err != nil {
		t.Fatal(err)
	}

	refused, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "intent-acquire-stale",
		"--work-unit", "remediation", "--evidence-goal", "bounded correction",
		"--max-attempts", "1", "--max-changed-lines", "200",
		"--remediates-evidence-revision", cliAttemptHash('b'),
	})
	if refused.State != "blocked" || refused.Reason != "remediation_unsatisfiable" || refused.Token != "" {
		t.Fatalf("stale post-reset remediation-intent acquire = %#v, want blocked/remediation_unsatisfiable", refused)
	}
	if !strings.Contains(refused.Exit, "gentle-ai sdd-attempt") || refused.Detail == "" {
		t.Fatalf("remediation-intent refusal does not name a runnable continuation: %#v", refused)
	}

	// The chain's own failed evidence survives the audited reset (#2565), so
	// declaring it still issues the correction token.
	bound, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "intent-acquire-correction",
		"--work-unit", "remediation", "--evidence-goal", "bounded correction",
		"--max-attempts", "1", "--max-changed-lines", "200",
		"--remediates-evidence-revision", failedEvidence,
	})
	if bound.State != "proceed" || bound.Token == "" {
		t.Fatalf("post-reset chain-bound remediation-intent acquire = %#v, want proceed", bound)
	}
}
