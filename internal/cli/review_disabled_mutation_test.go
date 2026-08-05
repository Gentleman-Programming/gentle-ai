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

// The kill switch is meant to freeze review authority read-only, not to leave
// it writable. `RDDOperationMutate` was declared and documented for exactly
// that -- "advances existing authority. Disabled mode rejects it" -- and then
// never reached the CLI surface, so the only guarded operation was START. A
// lineage started before the switch went off could still be finalized to an
// approved terminal receipt while receipt-driven development was switched off,
// which also defeats the second half of the rule: re-enabling would rediscover
// approved authority and skip the re-validation that makes the switch safe.
//
// Every test here drives the real router (`RunReview`), because the defect was
// precisely that a correct internal guard was never wired to the surface: a
// unit test on AuthorizeRDDOperation passed the entire time.

// disabledReviewRepo freezes an in-flight low-risk lineage and then turns
// receipt-driven development off for this clone, which is the exact shape the
// hand reproduction hit: authority that already exists, and a switch that is
// now off.
func disabledReviewRepo(t *testing.T, lineage string) (repo string, started ReviewFacadeStartResult) {
	t.Helper()
	reviewModeHome(t)
	repo = initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "frozen.md"), []byte("frozen by the kill switch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started = startFacadeReviewResult(t, repo, lineage)
	disableReviewForClone(t, repo)
	return repo, started
}

func reviewReceiptPath(repo, lineage string) string {
	return filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", lineage, "review-receipt.json")
}

// TestDisabledReviewRefusesFinalizeThroughTheRouter is the reproduction of the
// defect itself: with the switch off, FINALIZE used to exit 0, transition the
// lineage to `approved`, and publish a terminal receipt.
func TestDisabledReviewRefusesFinalizeThroughTheRouter(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-finalize")

	var output bytes.Buffer
	err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID}, &output)
	if err == nil {
		t.Fatalf("finalize advanced review authority while the kill switch was off:\n%s", output.String())
	}
	if !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("finalize refused for the wrong reason: %v\n%s", err, output.String())
	}
	if _, statErr := os.Stat(reviewReceiptPath(repo, started.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("a terminal receipt was published while reviews were off (stat = %v)", statErr)
	}
}

// TestDisabledReviewFreezesAuthorityInsteadOfDestroyingIt holds the other half
// of "freeze": the refusal must leave the in-flight lineage exactly where it
// was, still readable, and still finishable once the operator turns reviews
// back on. Refusing must never discard work or auto-flip the switch.
func TestDisabledReviewFreezesAuthorityInsteadOfDestroyingIt(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-frozen")

	var refused bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID}, &refused); !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("finalize while disabled = %v\n%s", err, refused.String())
	}

	// The frozen authority is still fully visible while disabled.
	var status bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &status); err != nil {
		t.Fatalf("status could not read frozen authority while disabled: %v\n%s", err, status.String())
	}
	if !strings.Contains(status.String(), started.LineageID) || !strings.Contains(status.String(), "reviewing") {
		t.Fatalf("frozen lineage is no longer readable as reviewing:\n%s", status.String())
	}

	// The switch itself is untouched: refusing never re-enables anything.
	var mode bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &mode); err != nil {
		t.Fatalf("review mode status: %v\n%s", err, mode.String())
	}
	if effective := decodeReviewModeResult(t, mode.Bytes()).Status.Effective; effective != reviewtransaction.RDDModeOff {
		t.Fatalf("a refused mutation changed the kill switch to %q", effective)
	}

	// Turning reviews back on lets the same frozen lineage finish.
	enableReviewForClone(t, repo)
	var resumed bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID}, &resumed); err != nil {
		t.Fatalf("re-enabling did not release the frozen lineage: %v\n%s", err, resumed.String())
	}
	if _, statErr := os.Stat(reviewReceiptPath(repo, started.LineageID)); statErr != nil {
		t.Fatalf("re-enabled finalize published no receipt: %v", statErr)
	}
}

// TestDisabledReviewRefusesEveryAuthorityProgressingVerb sweeps the whole verb
// surface, and it is the anti-drift guard for this fix: the original defect was
// one guard wired to one verb out of twenty, so a per-verb assertion is what
// keeps the next verb from quietly reopening the hole.
//
// Every request here is well-formed, which is the point. The kill switch is
// deliberately evaluated after a verb has validated its own request -- refusing
// a malformed request with "run review mode enable" would name a command that
// does not resolve the block -- so a sweep built from malformed requests would
// prove nothing about the switch.
func TestDisabledReviewRefusesEveryAuthorityProgressingVerb(t *testing.T) {
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	const authorization = "gentle-ai.maintainer-authorization/v1"
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// retry-final-verification parses its incident before it resolves the
	// repository, so the sweep supplies one that parses. It never has to match
	// real authority: the kill switch answers first.
	incidentPayload, err := reviewtransaction.CanonicalFinalVerificationIncident(reviewtransaction.FinalVerificationIncident{
		Schema: reviewtransaction.FinalVerificationIncidentSchema, Class: reviewtransaction.FinalVerificationIncidentProceduralToolingFailure,
		LineageID: "review-disabled-sweep", TerminalRevision: digest, ValidatingRevision: digest,
		TargetIdentity: digest, FailedEvidenceHash: digest, FinalizeRequestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	incident := filepath.Join(t.TempDir(), "incident.json")
	if err := os.WriteFile(incident, incidentPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		verb string
		args []string
	}{
		{verb: "start"},
		{verb: "finalize"},
		{verb: "capture-result", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--lens", "review-risk", "--order", "0", "--input", input}},
		{verb: "capture-evidence", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--expected-revision", digest, "--outcome", "passed", "--input", input}},
		{verb: "preserve-result", args: []string{"--lineage", "review-disabled-sweep", "--target", digest, "--lens", "review-risk", "--order", "0", "--input", input}},
		{verb: "repair", args: []string{"--contract", ReviewIntegrationContractV1}},
		{verb: "invalidate", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest}},
		{verb: "recover", args: []string{"--predecessor-lineage", "review-disabled-sweep", "--expected-predecessor-revision", digest, "--successor-lineage", "review-disabled-successor", "--disposition", "scope_changed"}},
		{verb: "retry-final-verification", args: []string{"--predecessor-lineage", "review-disabled-sweep", "--expected-predecessor-revision", digest, "--successor-lineage", "review-disabled-successor", "--incident", incident, "--actor", "maintainer", "--reason", "reason", "--maintainer-authorization", authorization}},
		{verb: "reclaim", args: []string{"--lineage", "review-disabled-sweep", "--reason", "reason", "--actor", "maintainer"}},
		{verb: "dispose-result", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest, "--target", digest, "--lens", "review-risk", "--order", "0", "--artifact-digest", digest, "--class", "empty_result", "--diagnostic", "diagnostic", "--reason", "reason", "--actor", "maintainer", "--maintainer-authorization", authorization}},
		{verb: "reopen-results", args: []string{"--lineage", "review-disabled-sweep", "--expected-revision", digest, "--target", digest, "--reason", "reason", "--actor", "maintainer", "--maintainer-authorization", authorization}},
		{verb: "bind-sdd", args: []string{"--change", "some-change", "--lineage", "review-disabled-sweep", "--expected-binding-revision", digest}},
	} {
		t.Run(testCase.verb, func(t *testing.T) {
			repo, _ := disabledReviewRepo(t, "review-disabled-sweep")
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
// and reviews are off -- and the usage error has to win, because this branch's
// rule is that a message may name a command only if running that command
// resolves the block. Answering an unknown flag with "run review mode enable"
// would name a command that leaves the caller exactly as stuck.
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

// TestDisabledReviewKeepsReadOnlyInspectionAndDeliveryReachable is the other
// direction, and it is the load-bearing one: freezing authority read-only is
// worthless if the operator can no longer see it, and the entire point of the
// switch is that ordinary delivery proceeds unblocked and unmanaged. A change
// that made any lifecycle gate refuse while disabled would break the feature.
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
		{"finalize", "--help"},
		{"abandon", "--help"},
	} {
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("read-only review %v refused while disabled: %v\n%s", args, err, output.String())
		}
	}
}

// TestDisabledReviewReplaysTerminalAuthorityWhileDisabled is the boundary of
// the refusal, and it is a read, not a loophole. FINALIZE against an
// already-terminal lineage re-emits the frozen receipt byte for byte and
// advances nothing, which is exactly the "exact replay" RDDOperationRead is
// documented to cover. Freezing authority read-only has to keep it readable;
// refusing here would make an approval the operator already earned
// unreachable, and would break replay-based recovery for anyone who switched
// reviews off afterwards.
func TestDisabledReviewReplaysTerminalAuthorityWhileDisabled(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "terminal.md"), []byte("approved before the switch went off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const lineage = "review-disabled-terminal-replay"
	startFacadeReviewResult(t, repo, lineage)

	var approved bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", lineage}, &approved); err != nil {
		t.Fatalf("finalize while enabled: %v\n%s", err, approved.String())
	}

	disableReviewForClone(t, repo)

	var replayed bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", lineage}, &replayed); err != nil {
		t.Fatalf("exact replay of terminal authority refused while disabled: %v\n%s", err, replayed.String())
	}
	if replayed.String() != approved.String() {
		t.Fatalf("terminal replay changed bytes while disabled:\nfirst:\n%s\nreplay:\n%s", approved.String(), replayed.String())
	}
}

// TestDisabledReviewMutationRefusalNamesARunnableContinuation holds the
// branch rule that a block must name a command whose execution resolves it,
// and that the mutation refusal says what re-enabling will actually let the
// operator do -- they hold an in-flight lineage, not a fresh start.
func TestDisabledReviewMutationRefusalNamesARunnableContinuation(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-continuation")

	var output bytes.Buffer
	err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID}, &output)
	if err == nil {
		t.Fatalf("finalize was not refused:\n%s", output.String())
	}
	message := err.Error()
	if !strings.Contains(message, "gentle-ai review mode enable --scope=clone") {
		t.Fatalf("mutation refusal names no runnable continuation: %s", message)
	}
	if !strings.Contains(message, "frozen") {
		t.Fatalf("mutation refusal does not say the review is frozen rather than discarded: %s", message)
	}
}

// TestDisabledReviewNegotiatedMutationEmitsTheTypedFailure keeps the machine
// surface honest: a caller driving the negotiated contract must receive the
// published failure envelope, not a bare error, and it must carry the runnable
// continuation rather than only a bare `stop`.
func TestDisabledReviewNegotiatedMutationEmitsTheTypedFailure(t *testing.T) {
	repo, started := disabledReviewRepo(t, "review-disabled-negotiated")

	var output bytes.Buffer
	err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output)
	if err == nil {
		t.Fatalf("negotiated finalize advanced authority while disabled:\n%s", output.String())
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, output.Bytes(), &failure)
	if failure.Code != "rdd_disabled" {
		t.Fatalf("negotiated failure code = %q, want rdd_disabled\n%s", failure.Code, output.String())
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("negotiated failure claimed mutation outcome %q", failure.MutationOutcome)
	}
	if !strings.Contains(failure.Cause, "gentle-ai review mode enable --scope=clone") {
		t.Fatalf("negotiated failure carries no runnable continuation: %#v", failure)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("negotiated failure envelope is invalid: %v", err)
	}
}
