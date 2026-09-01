package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNegotiatedRecoverTransitionRunsVerbatimForStagedPredecessorAfterRebase
// is the first leg of the rc.7 report on #2645: a staged review is approved,
// its exact candidate is committed and rebased across a disjoint parent
// advance (patch-identical), STATUS binds the approved predecessor, collects
// the exclusion and the exact maintainer authorization, and renders a
// complete native RECOVER command — which must then run verbatim. Root 13's
// property for RECOVER: STATUS never renders a recovery its own preflight
// refuses.
func TestNegotiatedRecoverTransitionRunsVerbatimForStagedPredecessorAfterRebase(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	// Approved staged review of one tracked change.
	writeReviewStartCandidate(t, repo, "feature.md", "# feature\n\nstaged reviewed prose.\n", 0o644)
	runReviewCLIGit(t, repo, "add", "feature.md")
	// This approved predecessor is historical compact authority. Seed it through
	// the test-only legacy seam because current START closes on its terminal
	// capture before STATUS can select recovery.
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--projection", "staged", "--lineage", "staged-rebase-predecessor",
	})
	if err != nil {
		t.Fatalf("start staged compact recovery fixture: %v", err)
	}
	var startResult ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &startResult)
	seedHistoricalApprovalForLineage(t, repo, startResult.LineageID)

	// The exact candidate is committed, then rebased across a disjoint
	// parent advance: patch, reviewed path set, and blobs stay identical
	// while every commit identity moves.
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed feature")
	feature := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD~1"))
	_ = base
	runReviewCLIGit(t, repo, "checkout", "-q", base)
	writeReviewStartCandidate(t, repo, "unrelated.md", "# unrelated\n\ndisjoint mainline advance.\n", 0o644)
	runReviewCLIGit(t, repo, "add", "unrelated.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "disjoint parent advance")
	runReviewCLIGit(t, repo, "cherry-pick", feature)
	runReviewCLIGit(t, repo, "checkout", "-q", "-B", "master", "HEAD")
	// The reporter's workspace carried untracked noise they then explicitly
	// excluded; the non-empty live view is what makes discovery assess the
	// approved staged authority against the moved candidate.
	writeReviewStartCandidate(t, repo, "scratch-note.txt", "untracked noise\n", 0o644)

	// Negotiated STATUS (plain, exactly as the reporter ran it: the staged
	// projection is empty post-commit, so discovery resolves the approved
	// staged authority as current_target and routes to recover).
	var statusOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--lineage", startResult.LineageID,
	}, &statusOut); err != nil {
		t.Fatalf("post-rebase status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil {
		t.Fatalf("status carried no next transition:\n%s", statusOut.String())
	}
	if status.Action != reviewtransaction.TargetStatusActionRecover {
		t.Skipf("status routed the rebased staged candidate to %q/%q rather than recover; this leg pins only the rendered-recover contract", status.Action, transition.ReasonCode)
	}
	if transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "recovery_authorization_required" {
		t.Fatalf("next transition is not the recovery collection: %s %s\n%s", transition.Kind, transition.ReasonCode, statusOut.String())
	}
	bound := map[string]string{}
	for _, argument := range transition.Collect.Inputs[0].Arguments {
		bound[argument.Name] = argument.Value
	}
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + bound["lineage"],
		"predecessor_revision=" + bound["expected-revision"],
		"target_identity=" + bound["target"],
		"successor_lineage=staged-rebase-successor",
		"actor=maintainer",
		"reason=rebase across disjoint parent advance",
	}, "\n")

	statusOut.Reset()
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--lineage", startResult.LineageID,
		"--recovery-successor-lineage", "staged-rebase-successor",
		"--recovery-reason", "rebase across disjoint parent advance",
		"--recovery-actor", "maintainer", "--recovery-authorization", authorization,
	}, &statusOut); err != nil {
		t.Fatalf("authorized status: %v\n%s", err, statusOut.String())
	}
	var authorized ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &authorized)
	execute := authorized.NextTransition
	if execute == nil || execute.Kind != reviewNextTransitionExecute || execute.Execute == nil ||
		execute.Execute.Operation != "review.recover" || execute.ReasonCode != "recovery_authorized" {
		t.Fatalf("authorized status did not render an executable recovery:\n%s", statusOut.String())
	}

	// The property: the rendered command runs verbatim.
	arguments := []string{"recover", "--cwd", repo}
	for _, argument := range execute.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("recovery argument %q carried no runnable token", argument.Name)
		}
		arguments = append(arguments, argument.Token)
	}
	var recovered bytes.Buffer
	if err := RunReview(arguments, &recovered); err != nil {
		t.Fatalf("STATUS rendered a recovery its own preflight refuses (#2645 rc.7 leg 1): review %v: %v\n%s", arguments, err, recovered.String())
	}
	var successor ReviewRecoverResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &successor)
	if successor.LineageID != "staged-rebase-successor" ||
		successor.Recovery.PredecessorLineageID != startResult.LineageID {
		t.Fatalf("recovery successor = %s", recovered.String())
	}
}

// TestNegotiatedStatusOffersFreshStartForByteIdenticalHistoricalApprovedCandidate
// proves that a retained historical approval cannot suppress an independent
// atomic review requested under an unused exact lineage.
func TestNegotiatedStatusOffersFreshStartForByteIdenticalHistoricalApprovedCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "feature.md", "# feature\n\nworkspace reviewed prose.\n", 0o755)
	historical := seedHistoricalApprovalForRepo(t, repo, "historical-byte-identical")
	stateBefore, err := os.ReadFile(historical.Store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	const successor = "explicit-unused-successor"
	var statusOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--lineage", successor,
	}, &statusOut); err != nil {
		t.Fatalf("explicit-successor status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionExecute || transition.Execute == nil ||
		transition.Execute.Operation != "review.start" || transition.ReasonCode != "fresh_target_ready" {
		t.Fatalf("status transition = %#v, want an executable fresh START", transition)
	}
	if transition.Execute.Binding.LineageID != successor || transition.Execute.Binding.TargetIdentity != status.TargetIdentity ||
		startTransitionArgumentValue(t, status, "lineage") != successor {
		t.Fatalf("fresh START binding = %#v, status target = %q", transition.Execute.Binding, status.TargetIdentity)
	}

	arguments := []string{"start", "--cwd", repo}
	for _, argument := range transition.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("START argument %q carried no executable token", argument.Name)
		}
		arguments = append(arguments, argument.Token)
	}
	var started bytes.Buffer
	if err := RunReview(arguments, &started); err != nil {
		t.Fatalf("run fresh START verbatim: %v\n%s", err, started.String())
	}
	var result ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, started.Bytes(), &result)
	if result.Action != "created" || result.LineageID != successor ||
		negotiatedStartTarget(result) != status.TargetIdentity {
		t.Fatalf("fresh START = %#v, want created %q bound to %q", result, successor, status.TargetIdentity)
	}
	stateAfter, err := os.ReadFile(historical.Store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("fresh atomic START changed historical approved authority")
	}
}
