package cli

import (
	"bytes"
	"context"
	"os"
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
// bind the approved predecessor for recovery and expose self-derived RECOVER
// with no caller-invented authorization flags.
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
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, startResult.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
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
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent ||
		status.Action != reviewtransaction.TargetStatusActionRecover ||
		status.ActionDisposition != reviewtransaction.RecoveryScopeChanged ||
		status.Authority == nil || status.Authority.LineageID != startResult.LineageID {
		t.Fatalf("status did not bind the approved predecessor for recovery:\n%s", statusOut.String())
	}
	if transition.Kind != reviewNextTransitionExecute || transition.ReasonCode != "recovery_authorized" ||
		transition.Execute == nil || transition.Execute.Operation != "review.recover" {
		t.Fatalf("next transition is not the self-derived recovery execution:\n%s", statusOut.String())
	}
	if err := transition.Validate(); err != nil {
		t.Fatal(err)
	}
	bound, err := reviewTransitionArgumentMap(transition.Execute.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 4 || bound["predecessor-lineage"] != startResult.LineageID || bound["expected-predecessor-revision"] == "" ||
		bound["successor-lineage"] == "" || bound["successor-lineage"] == startResult.LineageID ||
		bound["disposition"] != string(reviewtransaction.RecoveryScopeChanged) {
		t.Fatalf("recovery execution is not fully bound: %#v", bound)
	}
	for index, name := range []string{"predecessor-lineage", "expected-predecessor-revision", "successor-lineage", "disposition"} {
		if transition.Execute.Arguments[index].Name != name {
			t.Fatalf("recovery argument %d = %q, want %q", index, transition.Execute.Arguments[index].Name, name)
		}
	}

	// Supplying an explicitly wrong authorization must never fall through to
	// the self-derived route or create the named successor.
	var wrongOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--recovery-successor-lineage", "wrong-authorization-successor",
		"--recovery-reason", "scope changed after approval", "--recovery-actor", "maintainer",
		"--recovery-authorization", "not the exact binding",
	}, &wrongOut); err != nil {
		t.Fatalf("wrongly authorized review status: %v\n%s", err, wrongOut.String())
	}
	var wrong ReviewTargetStatusResult
	decodeStrictReviewJSON(t, wrongOut.Bytes(), &wrong)
	if wrong.NextTransition == nil || wrong.NextTransition.Kind == reviewNextTransitionExecute {
		t.Fatalf("wrong explicit authorization emitted execution: %s", wrongOut.String())
	}
	wrongStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "wrong-authorization-successor")
	if _, err := os.Stat(wrongStore.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("wrong explicit authorization created a successor: %v", err)
	}

	arguments := []string{"recover", "--cwd", repo}
	for _, argument := range transition.Execute.Arguments {
		arguments = append(arguments, argument.Token)
	}
	var recovered bytes.Buffer
	if err := RunReview(arguments, &recovered); err != nil {
		t.Fatalf("the printed recovery does not run: review %v: %v\n%s", arguments, err, recovered.String())
	}
	var successor ReviewRecoverResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &successor)
	if successor.LineageID != bound["successor-lineage"] || successor.State != reviewtransaction.StateReviewing ||
		successor.Recovery.PredecessorLineageID != startResult.LineageID ||
		successor.Recovery.Disposition != reviewtransaction.RecoveryScopeChanged {
		t.Fatalf("recovery successor = %s", recovered.String())
	}
	predecessorAfter, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil || !bytes.Equal(predecessorBefore, predecessorAfter) {
		t.Fatalf("recovery changed predecessor authority: %v", err)
	}
}

func TestNegotiatedStatusRecoversApprovedFeatureOntoCurrentBase(t *testing.T) {
	reviewModeHome(t)
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
		status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" {
		t.Fatalf("rebased status did not select scope recovery:\n%s", statusOut.String())
	}
	native, _, nativeErr := reviewtransaction.AssessTargetStatusWithSnapshot(context.Background(), repo, reviewtransaction.TargetStatusRequest{
		Target: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, BaseRef: currentBase, IntendedUntracked: []string{}}, LineageID: started.LineageID,
	})
	if nativeErr != nil || native.Decision.RecoverySelector == nil || native.Decision.RecoverySelector.Kind != reviewtransaction.TargetBaseDiff || native.Decision.RecoverySelector.BaseRef == "" {
		t.Fatalf("core did not project the native scope recovery: decision=%#v err=%v", native.Decision, nativeErr)
	}
	execute := status.NextTransition
	successorLineage := ""
	recoverArgs := []string{"recover", "--cwd", repo}
	for _, argument := range execute.Execute.Arguments {
		recoverArgs = append(recoverArgs, argument.Token)
		if argument.Name == "successor-lineage" {
			successorLineage = argument.Value
		}
	}
	var recoveredOut bytes.Buffer
	if err := RunReview(recoverArgs, &recoveredOut); err != nil {
		t.Fatalf("emitted rebased recovery failed: %v\n%s", err, recoveredOut.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successorLineage)
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

func TestNegotiatedStatusExecutesEscalatedChangedScopeRecovery(t *testing.T) {
	reviewModeHome(t)
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
		status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" {
		t.Fatalf("escalated changed-scope status did not execute native recovery:\n%s", statusOut.String())
	}
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == "actor" || argument.Name == "reason" || argument.Name == "maintainer-authorization" {
			t.Fatalf("self-derived recovery leaked %q into STATUS argv", argument.Name)
		}
	}
}
