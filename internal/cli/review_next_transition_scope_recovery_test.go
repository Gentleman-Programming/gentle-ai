package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNegotiatedStatusRoutesApprovedScopeChangeToBoundRecovery pins the closed
// negotiated loop confirmed on issue #1826: with an approved lineage and the
// operator's staged revision of the exact reviewed path set, negotiated status
// answered fresh_target_ready, the printed START answered blocked-scope-action
// at exit 0 having created nothing, and status then printed the same START
// again. The gentle-ai.review-recovery-authorization/v1 collection was
// reachable only after status had already selected recovery, which this shape
// never did.
//
// The store already refuses a fresh lineage for this shape and answers
// recover, so the negotiated surface must classify with the same predicate:
// bind the approved predecessor for recovery and expose the authorization
// collection with no caller-invented flags.
func TestNegotiatedStatusRoutesApprovedScopeChangeToBoundRecovery(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	attempt := filepath.Join(repo, "docs", "attempt.md")
	writeCLIAttemptFile(t, attempt, "# attempt\n\nplain prose.\n")
	runReviewCLIGit(t, repo, "add", ".")
	var started bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo}, &started); err != nil {
		t.Fatalf("review start: %v\n%s", err, started.String())
	}
	var startResult ReviewFacadeStartResult
	decodeStrictReviewJSON(t, started.Bytes(), &startResult)
	var finalized bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo}, &finalized); err != nil {
		t.Fatalf("review finalize: %v\n%s", err, finalized.String())
	}

	// The operator stages a revision of the exact reviewed path set: the
	// frozen delivery scope is unchanged while the candidate tree moved.
	writeCLIAttemptFile(t, attempt, "# attempt\n\nplain prose, revised after approval.\n")
	runReviewCLIGit(t, repo, "add", ".")

	var statusOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
	}, &statusOut); err != nil {
		t.Fatalf("review status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil {
		t.Fatalf("status carried no next transition:\n%s", statusOut.String())
	}
	if transition.Kind == reviewNextTransitionExecute && transition.Execute != nil &&
		transition.Execute.Operation == "review.start" {
		// Prove the loop instead of merely asserting a shape: run the printed
		// transition verbatim and show what it answers while changing nothing.
		arguments := []string{"start", "--cwd", repo}
		for _, argument := range transition.Execute.Arguments {
			arguments = append(arguments, argument.Token)
		}
		var rerun bytes.Buffer
		if err := RunReview(arguments, &rerun); err != nil {
			t.Fatalf("the printed start does not run: %v\n%s", err, rerun.String())
		}
		var result ReviewIntegrationStartResult
		decodeStrictReviewJSON(t, rerun.Bytes(), &result)
		t.Fatalf("status routed the approved scope change to a fresh start; running it verbatim answered %q against lineage %q: the issue #1826 closed loop",
			result.Action, result.LineageID)
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent ||
		status.Action != reviewtransaction.TargetStatusActionRecover ||
		status.ActionDisposition != reviewtransaction.RecoveryScopeChanged ||
		status.Authority == nil || status.Authority.LineageID != startResult.LineageID {
		t.Fatalf("status did not bind the approved predecessor for recovery:\n%s", statusOut.String())
	}
	if transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "recovery_authorization_required" ||
		transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("next transition is not the bound recovery collection:\n%s", statusOut.String())
	}
	input := transition.Collect.Inputs[0]
	if input.Schema != "gentle-ai.review-recovery-authorization/v1" || input.CaptureOperation != "external.authorize_recovery" {
		t.Fatalf("collection input = %q via %q, want the recovery authorization schema", input.Schema, input.CaptureOperation)
	}
	bound := map[string]string{}
	for _, argument := range input.Arguments {
		bound[argument.Name] = argument.Value
	}
	if bound["lineage"] != startResult.LineageID || bound["disposition"] != string(reviewtransaction.RecoveryScopeChanged) ||
		bound["target"] == "" || bound["expected-revision"] == "" {
		t.Fatalf("recovery collection is not fully bound: %#v", bound)
	}

	// Completing the collection must yield an executable transition that,
	// run verbatim, creates the generation-2 successor: the loop is closed
	// by an exit the operator can actually take.
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + bound["lineage"],
		"predecessor_revision=" + bound["expected-revision"],
		"target_identity=" + bound["target"],
		"successor_lineage=scope-loop-successor",
		"actor=maintainer",
		"reason=scope changed after approval",
	}, "\n")
	statusOut.Reset()
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--recovery-successor-lineage", "scope-loop-successor",
		"--recovery-reason", "scope changed after approval",
		"--recovery-actor", "maintainer", "--recovery-authorization", authorization,
	}, &statusOut); err != nil {
		t.Fatalf("authorized review status: %v\n%s", err, statusOut.String())
	}
	var authorized ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &authorized)
	execute := authorized.NextTransition
	if execute == nil || execute.Kind != reviewNextTransitionExecute || execute.Execute == nil ||
		execute.Execute.Operation != "review.recover" || execute.ReasonCode != "recovery_authorized" {
		t.Fatalf("authorized status did not emit an executable recovery:\n%s", statusOut.String())
	}
	arguments := []string{"recover", "--cwd", repo}
	for _, argument := range execute.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("recovery argument %q carried no runnable token", argument.Name)
		}
		arguments = append(arguments, argument.Token)
	}
	var recovered bytes.Buffer
	if err := RunReview(arguments, &recovered); err != nil {
		t.Fatalf("the printed recovery does not run: review %v: %v\n%s", arguments, err, recovered.String())
	}
	var successor ReviewRecoverResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &successor)
	if successor.LineageID != "scope-loop-successor" || successor.State != reviewtransaction.StateReviewing ||
		successor.Recovery.PredecessorLineageID != startResult.LineageID ||
		successor.Recovery.Disposition != reviewtransaction.RecoveryScopeChanged {
		t.Fatalf("recovery successor = %s", recovered.String())
	}
}
