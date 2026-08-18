package cli

import (
	"bytes"
	"context"
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
	reviewEnabledHome(t)
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

func TestNegotiatedStatusRecoversApprovedFeatureOntoCurrentBase(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	featureBranch := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	originalBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	attempt := filepath.Join(repo, "docs", "attempt.md")
	writeCLIAttemptFile(t, attempt, "# approved feature\n")
	runReviewCLIGit(t, repo, "add", "--", "docs/attempt.md")
	var startedOut bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo}, &startedOut); err != nil {
		t.Fatalf("review start: %v\n%s", err, startedOut.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedOut.Bytes(), &started)
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID}, &bytes.Buffer{}); err != nil {
		t.Fatalf("review finalize: %v", err)
	}
	runReviewCLIGit(t, repo, "commit", "-m", "approved feature")

	runReviewCLIGit(t, repo, "branch", "advanced-base", originalBase)
	runReviewCLIGit(t, repo, "checkout", "advanced-base")
	writeCLIAttemptFile(t, filepath.Join(repo, "base-advance.txt"), "unrelated base advance\n")
	runReviewCLIGit(t, repo, "add", "--", "base-advance.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "advance base")
	currentBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	currentBaseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", currentBase+"^{tree}"))
	runReviewCLIGit(t, repo, "checkout", featureBranch)
	runReviewCLIGit(t, repo, "rebase", "advanced-base")
	headTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))

	statusArgs := []string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--lineage", started.LineageID, "--base-ref", currentBase,
	}
	var statusOut bytes.Buffer
	if err := RunReview(statusArgs, &statusOut); err != nil {
		t.Fatalf("rebased review status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent ||
		status.Action != reviewtransaction.TargetStatusActionRecover ||
		status.ActionDisposition != reviewtransaction.RecoveryScopeChanged || status.Authority == nil ||
		status.Authority.LineageID != started.LineageID || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil {
		t.Fatalf("rebased status did not select scope recovery:\n%s", statusOut.String())
	}
	native, _, nativeErr := reviewtransaction.AssessTargetStatusWithSnapshot(context.Background(), repo, reviewtransaction.TargetStatusRequest{
		Target: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, BaseRef: currentBase, IntendedUntracked: []string{}}, LineageID: started.LineageID,
	})
	if nativeErr != nil || native.Decision.RecoverySelector == nil || native.Decision.RecoverySelector.Kind != reviewtransaction.TargetBaseDiff || native.Decision.RecoverySelector.BaseRef == "" {
		t.Fatalf("core did not project the native scope recovery: decision=%#v err=%v", native.Decision, nativeErr)
	}
	input := status.NextTransition.Collect.Inputs[0]
	bound := map[string]string{}
	for _, argument := range input.Arguments {
		bound[argument.Name] = argument.Value
	}
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + bound["lineage"],
		"predecessor_revision=" + bound["expected-revision"],
		"target_identity=" + bound["target"],
		"successor_lineage=approved-rebase-successor",
		"actor=maintainer",
		"reason=recover approved feature onto current base",
	}, "\n")
	authorizedArgs := append(append([]string{}, statusArgs...),
		"--recovery-successor-lineage", "approved-rebase-successor",
		"--recovery-reason", "recover approved feature onto current base",
		"--recovery-actor", "maintainer", "--recovery-authorization", authorization)
	statusOut.Reset()
	if err := RunReview(authorizedArgs, &statusOut); err != nil {
		t.Fatalf("authorized rebased status: %v\n%s", err, statusOut.String())
	}
	var authorized ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &authorized)
	execute := authorized.NextTransition
	if execute == nil || execute.Kind != reviewNextTransitionExecute || execute.Execute == nil ||
		execute.Execute.Operation != "review.recover" {
		t.Fatalf("authorized status did not emit recovery:\n%s", statusOut.String())
	}
	recoverArgs := []string{"recover", "--cwd", repo}
	for _, argument := range execute.Execute.Arguments {
		recoverArgs = append(recoverArgs, argument.Token)
	}
	var recoveredOut bytes.Buffer
	if err := RunReview(recoverArgs, &recoveredOut); err != nil {
		t.Fatalf("emitted rebased recovery failed: %v\n%s", err, recoveredOut.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "approved-rebase-successor")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.BaseTree != currentBaseTree || recovered.State.InitialSnapshot.CandidateTree != headTree ||
		facadeProjection(recovered.State.InitialSnapshot.Projection) != reviewtransaction.ProjectionWorkspace ||
		len(recovered.State.InitialSnapshot.Paths) != 1 || recovered.State.InitialSnapshot.Paths[0] != "docs/attempt.md" ||
		recovered.State.Recovery == nil || recovered.State.Recovery.PredecessorLineageID != started.LineageID {
		t.Fatalf("successor did not bind only the rebased feature on the current base: %#v", recovered.State)
	}
}

func TestNegotiatedStatusCollectsEscalatedChangedScopeRecovery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	attempt := filepath.Join(repo, "internal", "auth", "session.go")
	writeCLIAttemptFile(t, attempt, "package auth\n\nfunc CheckToken(token string) bool { return token != \"\" }\n")
	runReviewCLIGit(t, repo, "add", ".")
	var startedOut bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo}, &startedOut); err != nil {
		t.Fatalf("review start: %v\n%s", err, startedOut.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedOut.Bytes(), &started)
	resultPaths := make([]string, len(started.SelectedLenses))
	for index, lens := range started.SelectedLenses {
		resultPaths[index] = filepath.Join(t.TempDir(), lens+".json")
		writeReviewCLIJSON(t, resultPaths[index], facadeReviewerResult{Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate"}})
	}
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, resultPaths); err != nil {
		t.Fatalf("capture reviewer results: %v", err)
	}
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize reviewer results: %v", err)
	}
	evidence := filepath.Join(repo, "failed-verification.txt")
	writeCLIAttemptFile(t, evidence, "go test ./... FAIL\n")
	if err := RunReview([]string{"finalize", "--cwd", repo, "--lineage", started.LineageID, "--evidence", evidence, "--failed=true"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed final verification: %v", err)
	}

	writeCLIAttemptFile(t, attempt, "package auth\n\nfunc CheckToken(token string) bool { return len(token) >= 16 }\n")
	writeCLIAttemptFile(t, filepath.Join(repo, "docs", "added.md"), "# added scope\n")
	runReviewCLIGit(t, repo, "add", ".")
	var statusOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--action-eligibility", "--lineage", started.LineageID,
	}, &statusOut); err != nil {
		t.Fatalf("review status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	if status.Action != reviewtransaction.TargetStatusActionRecover || status.ActionDisposition != reviewtransaction.RecoveryEscalated ||
		status.Authority == nil || status.Authority.LineageID != started.LineageID || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "recovery_authorization_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("escalated changed-scope status did not collect recovery authorization:\n%s", statusOut.String())
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Schema != "gentle-ai.review-recovery-authorization/v1" || input.CaptureOperation != "external.authorize_recovery" {
		t.Fatalf("recovery authorization input = %#v", input)
	}
}
