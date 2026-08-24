package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The kill switch freezes review authority against every currently routed
// authority mutation. Reads and delivery validation remain available while
// disabled; mutations must refuse only after their own request validation has
// succeeded.

// disabledReviewRepo freezes an in-flight low-risk lineage and then turns
// receipt-driven development off for this clone, which is the exact shape the
// hand reproduction hit: authority that already exists, and a switch that is
// now off.
//
// Reaching that shape needs both halves of the switch in order. Receipt-driven
// development is opt-in, so the START that creates the authority to freeze only
// runs for a user who explicitly turned reviews on -- hence the enabled home.
// The clone-local disable that follows is what the tests here are actually
// about, and it still wins over that explicit global "on".
func disabledReviewRepo(t *testing.T, lineage string) (repo string, started ReviewFacadeStartResult) {
	t.Helper()
	reviewEnabledHome(t)
	repo = initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "frozen.md"), []byte("frozen by the kill switch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/frozen.md")
	started = startFacadeReviewResult(t, repo, lineage)
	disableReviewForClone(t, repo)
	return repo, started
}

// TestDisabledReviewRefusesEveryAuthorityProgressingVerb sweeps the current
// routed authority-mutation surface. Every request is well-formed, which is
// the point: the kill switch is evaluated after a verb validates its request,
// so malformed requests cannot prove the disabled-mode refusal.
func TestDisabledReviewRefusesEveryAuthorityProgressingVerb(t *testing.T) {
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	const authorization = "gentle-ai.maintainer-authorization/v1"
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		verb string
		args []string
	}{
		{verb: "start"},
		{verb: "capture-result", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--lens", "review-risk", "--order", "0", "--input", input}},
		{verb: "capture-correction-plan", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--expected-revision", digest, "--request-hash", digest, "--correction-lines", "1"}},
		{verb: "capture-refuter", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--expected-revision", digest, "--agent", "pi", "--execute"}},
		{verb: "capture-validation", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--expected-revision", digest, "--request-hash", digest, "--agent", "pi", "--execute"}},
		{verb: "preserve-result", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--lens", "review-risk", "--order", "0", "--input", input}},
		{verb: "repair", args: []string{"--contract", ReviewIntegrationContractV1}},
		{verb: "invalidate", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest}},
		{verb: "recover", args: []string{"--predecessor-lineage", "review-disabled-sweep", "--expected-predecessor-revision", digest, "--successor-lineage", "review-disabled-successor", "--disposition", "scope_changed"}},
		{verb: "reclaim", args: []string{"--lineage", "review-disabled-sweep", "--reason", "reason", "--actor", "maintainer"}},
		{verb: "dispose-result", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest, "--target", digest, "--lens", "review-risk", "--order", "0", "--artifact-digest", digest, "--class", "empty_result", "--diagnostic", "diagnostic", "--reason", "reason", "--actor", "maintainer", "--maintainer-authorization", authorization}},
		{verb: "reopen-results", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest, "--target", digest, "--reason", "reason", "--actor", "maintainer", "--maintainer-authorization", authorization}},
	} {
		t.Run(testCase.verb, func(t *testing.T) {
			repo, _ := disabledReviewRepo(t, "review-disabled-sweep")
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			var output bytes.Buffer
			err := RunReview(append([]string{testCase.verb, "--cwd", repo}, testCase.args...), &output)
			if !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
				t.Fatalf("review %s was not stopped by the kill switch: %v\n%s", testCase.verb, err, output.String())
			}
		})
	}
}

// TestDisabledReviewLetsMalformedRequestsAnswerFirst is the ordering the sweep
// above depends on. Two things are wrong at once here -- the request is invalid
// and reviews are off -- and the usage error has to win, because a message may
// name a command only if running that command resolves the block.
func TestDisabledReviewLetsMalformedRequestsAnswerFirst(t *testing.T) {
	repo, _ := disabledReviewRepo(t, "review-disabled-malformed")

	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"invalidate", "--cwd", repo, "--nonexistent"}, want: "flag provided but not defined"},
		{name: "stray positional", args: []string{"invalidate", "--cwd", repo, "extra"}, want: "unexpected review invalidate argument"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(testCase.args, &output)
			if err == nil {
				t.Fatalf("malformed request was accepted:\n%s", output.String())
			}
			if errors.Is(err, reviewtransaction.ErrRDDDisabled) {
				t.Fatalf("the kill switch preempted a usage error and named a command that would not unblock it: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), testCase.want)
			}
		})
	}
}

// TestDisabledReviewKeepsReadOnlyInspectionAndDeliveryReachable proves that
// freezing authority does not prevent inspection or ordinary delivery.
func TestDisabledReviewKeepsReadOnlyInspectionAndDeliveryReachable(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-readable")

	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
	} {
		var output bytes.Buffer
		if err := RunReview([]string{"validate", "--cwd", repo, "--gate", string(gate)}, &output); err != nil {
			t.Fatalf("gate %s refused delivery while reviews were off: %v\n%s", gate, err, output.String())
		}
		var result ReviewValidateResult
		decodeStrictReviewJSON(t, output.Bytes(), &result)
		if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
			t.Fatalf("gate %s delivery = %q, want %q", gate, result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged)
		}
		if result.Allowed || result.Result == reviewtransaction.GateAllow {
			t.Fatalf("gate %s fabricated an approval while disabled: %#v", gate, result)
		}
	}

	for _, args := range [][]string{
		{"status", "--cwd", repo},
		{"capabilities"},
		{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID},
		{"capture-result", "--help"},
		{"abandon", "--help"},
	} {
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("read-only review %v refused while disabled: %v\n%s", args, err, output.String())
		}
	}
}

// TestDisabledReviewMutationRefusalNamesARunnableContinuation holds the branch
// rule that a block must name a command whose execution resolves it.
func TestDisabledReviewMutationRefusalNamesARunnableContinuation(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-continuation")
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	var output bytes.Buffer
	err := RunReview([]string{
		"capture-result", "--cwd", repo, "--lineage", started.LineageID,
		"--target", digest, "--lens", "review-risk", "--order", "0", "--input", input,
	}, &output)
	if err == nil {
		t.Fatalf("capture-result was not refused:\n%s", output.String())
	}
	message := err.Error()
	if !strings.Contains(message, "gentle-ai review mode enable --scope=clone") {
		t.Fatalf("mutation refusal names no runnable continuation: %s", message)
	}
	if !strings.Contains(message, "frozen") {
		t.Fatalf("mutation refusal does not say the review is frozen rather than discarded: %s", message)
	}
}

// TestDisabledReviewCaptureResultEmitsTheTypedFailure keeps the machine surface
// honest: a surviving mutation must emit the published failure envelope rather
// than a bare error, with the runnable enable continuation in its cause.
func TestDisabledReviewCaptureResultEmitsTheTypedFailure(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-negotiated")
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	var output bytes.Buffer
	err := RunReview([]string{
		"capture-result", "--cwd", repo, "--lineage", started.LineageID,
		"--target", digest, "--lens", "review-risk", "--order", "0", "--input", input,
	}, &output)
	if err == nil {
		t.Fatalf("capture-result advanced authority while disabled:\n%s", output.String())
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, output.Bytes(), &failure)
	if failure.Code != "rdd_disabled" {
		t.Fatalf("capture-result failure code = %q, want rdd_disabled\n%s", failure.Code, output.String())
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("capture-result failure claimed mutation outcome %q", failure.MutationOutcome)
	}
	if !strings.Contains(failure.Cause, "gentle-ai review mode enable --scope=clone") {
		t.Fatalf("capture-result failure carries no runnable continuation: %#v", failure)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("capture-result failure envelope is invalid: %v", err)
	}
}
