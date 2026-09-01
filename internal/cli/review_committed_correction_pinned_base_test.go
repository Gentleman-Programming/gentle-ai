package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestCommittedCorrectionStatusAfterCommitAnswersTheTransitionItBinds is the
// RED-first proof for #3961. A committed-only lineage started by an in-process
// capture runtime (claude-code) with a pinned base commit is driven through
// its final reviewer capture, the returned correction-plan collect, and a
// committed correction; the exact lineage-bound STATUS must then answer with
// the transition its own repository context binds instead of refusing the
// envelope it built.
//
// The pinned commit sha and the inferential refuter are the shapes the issue
// named; neither is the variable. The reproduction needs the correction to add
// an admitted companion test path (#3375): native STATUS still classified that
// as a scope-changed recovery while the validation request admitted it, so the
// envelope bound its context to a validation it then did not offer. The
// over-budget contraction is the one shape that legitimately routes to
// recovery, and it must get that collect rather than the same refusal.
func TestCommittedCorrectionStatusAfterCommitAnswersTheTransitionItBinds(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		evidence      reviewtransaction.EvidenceClass
		causal        reviewtransaction.CausalDisposition
		pinnedBaseRef bool
		companionTest bool
		contraction   bool
	}{
		{name: "deterministic/continuation-tokens", evidence: reviewtransaction.EvidenceDeterministic, causal: reviewtransaction.CausalIntroduced},
		{name: "deterministic/pinned-commit-sha", evidence: reviewtransaction.EvidenceDeterministic, causal: reviewtransaction.CausalIntroduced, pinnedBaseRef: true},
		{name: "deterministic/companion-test-added", evidence: reviewtransaction.EvidenceDeterministic, causal: reviewtransaction.CausalIntroduced, companionTest: true},
		{name: "inferential/companion-test-added/pinned-commit-sha", evidence: reviewtransaction.EvidenceInferential, causal: reviewtransaction.CausalBehaviorActivated, pinnedBaseRef: true, companionTest: true},
		{name: "deterministic/over-budget-contraction", evidence: reviewtransaction.EvidenceDeterministic, causal: reviewtransaction.CausalIntroduced, contraction: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			committedCorrectionStatusAfterCommit(t, testCase.evidence, testCase.causal, testCase.pinnedBaseRef, testCase.companionTest, testCase.contraction)
		})
	}
}

func committedCorrectionStatusAfterCommit(t *testing.T, evidence reviewtransaction.EvidenceClass, causal reviewtransaction.CausalDisposition, pinnedBaseRef, companionTest, contraction bool) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	baseCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o644)
	if contraction {
		writeReviewStartCandidate(t, repo, "helper.go", "package candidate\nvar one = 1\nvar two = 2\nvar three = 3\n", 0o644)
	}
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "wrong candidate")

	const lineage = "committed-pinned-base-correction"
	started := runNegotiatedReviewStartWith(t, repo, lineage, "--agent", string(model.AgentClaudeCode), "--base-ref", baseCommit)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff || started.RepositoryContext == nil {
		t.Fatalf("pinned-base START = kind=%q context=%#v, want a committed base diff with a repository context", record.State.InitialSnapshot.Kind, started.RepositoryContext)
	}

	var payload []byte
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			if bytes.Contains(invocation.Prompt(), []byte(`"claims"`)) {
				return []byte(`{"refuter_request_hash":"` + reviewProviderRequestHashForTest(t, invocation.Prompt()) + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["independent reproduction"]}]}`), nil
			}
			return payload, nil
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })

	var closure reviewLastEventClosureResult
	for order, lens := range started.SelectedLenses {
		reviewer := admittedReviewerResultForTest(t, repo, record, lens, order)
		if order == len(started.SelectedLenses)-1 {
			reviewer.Findings = []facadeFinding{{
				ID: "R3-001", Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: evidence,
				CausalDisposition: causal,
			}}
		}
		if payload, err = json.Marshal(reviewer); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo,
			"--repository-context", started.RepositoryContext.Handle, "--lineage", lineage,
			"--target", started.RepositoryContext.TargetIdentity, "--expected-revision", started.RepositoryContext.Revision,
			"--lens", lens, "--order", strconv.Itoa(order), "--agent", string(model.AgentClaudeCode),
		}, &output); err != nil {
			t.Fatalf("capture lens %q: %v\n%s", lens, err, output.String())
		}
		if order == len(started.SelectedLenses)-1 {
			decodeStrictReviewJSON(t, output.Bytes(), &closure)
		}
	}
	if closure.State != reviewtransaction.StateCorrectionRequired || closure.StatusContinuation == nil {
		t.Fatalf("final capture closure = %#v, want correction_required with a status continuation", closure)
	}

	continuation := closure.StatusContinuation
	verb, found := reviewTransitionCommandVerb(continuation.Operation)
	if !found {
		t.Fatalf("continuation operation %q has no dispatched verb", continuation.Operation)
	}
	statusArgs := []string{verb}
	for _, argument := range continuation.Arguments {
		if pinnedBaseRef && argument.Name == "base-ref" {
			// The issue shape: the operator re-runs STATUS with the commit sha
			// START was pinned to rather than the frozen tree the continuation
			// carries.
			statusArgs = append(statusArgs, "--base-ref="+baseCommit)
			continue
		}
		statusArgs = append(statusArgs, argument.Token)
	}
	boundStatus := func(label string) ReviewTargetStatusResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(statusArgs, &output); err != nil {
			t.Fatalf("bound STATUS %s: %v (cause: %v)\n%s", label, err, errors.Unwrap(err), output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if err := status.Validate(); err != nil {
			t.Fatalf("bound STATUS %s validate: %v", label, err)
		}
		return status
	}

	planStatus := boundStatus("before the correction plan")
	if planStatus.NextTransition == nil || planStatus.NextTransition.Kind != reviewNextTransitionCollect ||
		planStatus.NextTransition.ReasonCode != "correction_plan_required" || planStatus.NextTransition.Collect == nil ||
		len(planStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("bound STATUS before the correction plan = %#v, want correction_plan_required", planStatus.NextTransition)
	}
	planArgs := append(reviewTransitionInputTokens(t, repo, planStatus.NextTransition.Collect.Inputs[0]), "--correction-lines", "2")
	var planOutput bytes.Buffer
	if err := RunReviewCaptureCorrectionPlan(planArgs, &planOutput); err != nil {
		t.Fatalf("capture correction plan: %v\n%s", err, planOutput.String())
	}

	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\n", 0o644)
	uncommitted := boundStatus("with the uncommitted correction")
	if uncommitted.NextTransition == nil || uncommitted.NextTransition.Kind != reviewNextTransitionStop ||
		uncommitted.NextTransition.ReasonCode != "corrected_candidate_unavailable" {
		t.Fatalf("bound STATUS with the uncommitted correction = %#v, want stop corrected_candidate_unavailable", uncommitted.NextTransition)
	}

	if companionTest {
		// The admitted companion test path (#3375): a correction may add a
		// test file beside a reviewed path without leaving the lineage.
		writeReviewStartCandidate(t, repo, "candidate_test.go", "package candidate\n", 0o644)
	}
	if contraction {
		// Dropping a reviewed path entirely contracts the genesis scope, and
		// the deleted lines exceed the two-line budget: native routes this
		// lineage to recovery, so STATUS must answer with that collect
		// instead of binding a validation it does not offer.
		runReviewCLIGit(t, repo, "rm", "-q", "helper.go")
	}
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate")
	committed := boundStatus("after the committed correction")
	transition := committed.NextTransition
	if contraction {
		if committed.Action != reviewtransaction.TargetStatusActionRecover || committed.ValidationRequest != nil || committed.RepositoryContext == nil ||
			committed.RepositoryContext.TargetIdentity != reviewAuthorityTargetIdentity(committed) ||
			transition == nil || transition.Kind != reviewNextTransitionCollect || !strings.HasPrefix(transition.ReasonCode, "recovery_") {
			t.Fatalf("bound STATUS after the over-budget contraction = action=%q validation=%#v context=%#v transition=%#v, want the recovery collect bound to the authority target", committed.Action, committed.ValidationRequest, committed.RepositoryContext, transition)
		}
		return
	}
	if committed.ValidationRequest == nil || transition == nil || transition.Kind != reviewNextTransitionCollect ||
		transition.ReasonCode != "targeted_validation_required" || transition.Collect == nil || len(transition.Collect.Inputs) != 1 ||
		transition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
		t.Fatalf("bound STATUS after the committed correction = %#v, want the review.capture-validation collect", transition)
	}
	arguments, err := reviewTransitionArgumentMap(transition.Collect.Inputs[0].Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["agent"] != string(model.AgentClaudeCode) || arguments["target"] != committed.ValidationRequest.CorrectionTargetIdentity {
		t.Fatalf("targeted validation input = %#v, want bound to %s and the corrected candidate", transition.Collect.Inputs[0], model.AgentClaudeCode)
	}
}
