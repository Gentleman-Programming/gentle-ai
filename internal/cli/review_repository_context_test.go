package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRepositoryContextCaptureFromUnrelatedCWDClosesOnLastCapture(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "repository-context-capture")
	if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
		t.Fatalf("START result = %#v", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	contextHandle := rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, Revision: started.RepositoryContext.Revision,
	})
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(resultPath, admittedReviewerPayloadForTest(t, repo, record, started.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	// The process cwd stays unrelated on purpose: the capability is that a
	// caller can capture from anywhere, and it now names the repository the
	// provider-issued digest is verified against instead of reading a path out
	// of the token.
	t.Chdir(t.TempDir())
	bindingArgs := []string{
		"--cwd", repo,
		"--repository-context", contextHandle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lens", started.SelectedLenses[0], "--order", "0",
	}
	var preflight bytes.Buffer
	if err := RunReviewCaptureResult(append(append([]string{}, bindingArgs...), "--preflight"), &preflight); err != nil {
		t.Fatal(err)
	}
	var checked reviewCapturePreflightResult
	decodeStrictReviewJSON(t, preflight.Bytes(), &checked)
	if checked.RepositoryRoot != "" || bytes.Contains(preflight.Bytes(), []byte(repo)) {
		t.Fatalf("repository-context preflight leaked repository root: %s", preflight.String())
	}
	beforeRefusal, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		replaceReviewContextArgument(t, bindingArgs, "rctx2_not-base64"),
		replaceReviewArgument(t, bindingArgs, "--expected-revision", "sha256:"+strings.Repeat("0", 64)),
	} {
		if err := RunReviewCaptureResult(append(args, "--preflight"), io.Discard); err == nil ||
			!strings.Contains(err.Error(), "repository_context_") || strings.Contains(err.Error(), repo) {
			t.Fatalf("invalid rctx2 preflight error = %v", err)
		}
		afterRefusal, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(beforeRefusal, afterRefusal) {
			t.Fatal("invalid rctx2 preflight mutated compact authority")
		}
	}

	args := append(append([]string{}, bindingArgs...), "--input", resultPath)
	missingRevision := append([]string{}, args...)
	for index := 0; index < len(missingRevision); index++ {
		if missingRevision[index] == "--expected-revision" {
			missingRevision = append(missingRevision[:index], missingRevision[index+2:]...)
			break
		}
	}
	if err := RunReviewCaptureResult(missingRevision, io.Discard); err == nil {
		t.Fatal("repository-context capture accepted a missing revision")
	}

	var captured bytes.Buffer
	if err := RunReviewCaptureResult(args, &captured); err != nil {
		t.Fatal(err)
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured.Bytes(), &closure)
	if closure.Operation != "review/capture-result" || closure.LineageID != started.LineageID ||
		closure.State != reviewtransaction.StateApproved {
		t.Fatalf("opaque terminal capture = %#v", closure)
	}
	if closure.Acknowledgement == nil {
		t.Fatalf("opaque terminal capture omitted acknowledgement: %#v", closure)
	}
	assertApprovedAcknowledgementTransition(t, closure.Acknowledgement, repo, started.LineageID, closure.Acknowledgement.Binding.TargetIdentity, closure.StoreRevision)
}

func TestOpaqueContextErrorsDoNotExposeProviderPaths(t *testing.T) {
	for _, damage := range []string{"authority"} {
		t.Run(damage, func(t *testing.T) {
			home := reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc opaqueFailure() {}\n", 0o644)
			started := runNegotiatedReviewStart(t, repo, "opaque-error-"+damage)
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(store.StatePath()); err != nil {
				t.Fatal(err)
			}
			err = RunReviewCaptureResult([]string{
				"--cwd", repo,
				"--repository-context", started.RepositoryContext.Handle,
				"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
				"--expected-revision", started.RepositoryContext.Revision,
				"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
			}, io.Discard)
			if err == nil || strings.Contains(err.Error(), repo) || strings.Contains(err.Error(), home) ||
				!strings.Contains(err.Error(), "repository_context_") || !strings.Contains(err.Error(), "refresh") {
				t.Fatalf("opaque context error = %q", err)
			}
		})
	}
}

func TestNativeNextTransitionCarriesRepositoryContextCaptureBinding(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc transition() {}\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "repository-context-transition")
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output.Bytes(), []byte(repo)) {
		t.Fatalf("native transition leaked repository path: %s", output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("next transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	// review.capture-result is an operation this product performs, so these
	// arguments are that command's own argv and carry their exact runnable
	// token alongside the name/value pair, which stays byte-identical.
	want := []ReviewTransitionArgument{
		{Name: "lineage", Value: started.LineageID, Token: "--lineage=" + started.LineageID},
		{Name: "expected-revision", Value: started.RepositoryContext.Revision, Token: "--expected-revision=" + started.RepositoryContext.Revision},
		{Name: "target", Value: started.RepositoryContext.TargetIdentity, Token: "--target=" + started.RepositoryContext.TargetIdentity},
		{Name: "repository-context", Value: started.RepositoryContext.Handle, Token: "--repository-context=" + started.RepositoryContext.Handle},
		{Name: "lens", Value: started.SelectedLenses[0], Token: "--lens=" + started.SelectedLenses[0]},
		{Name: "order", Value: "0", Token: "--order=0"},
	}
	if input.CaptureOperation != "review.capture-result" || !slices.Equal(input.Arguments, want) {
		t.Fatalf("capture input = %#v, want %#v", input, want)
	}
	wrongTransition := status
	wrongNext := *status.NextTransition
	wrongCollect := *status.NextTransition.Collect
	wrongInputs := append([]ReviewTransitionInput(nil), wrongCollect.Inputs...)
	wrongArguments := append([]ReviewTransitionArgument(nil), wrongInputs[0].Arguments...)
	for index := range wrongArguments {
		if wrongArguments[index].Name == "target" {
			wrongArguments[index].Value = "sha256:" + strings.Repeat("f", 64)
		}
	}
	wrongInputs[0].Arguments = wrongArguments
	wrongCollect.Inputs = wrongInputs
	wrongNext.Collect = &wrongCollect
	wrongTransition.NextTransition = &wrongNext
	if err := wrongTransition.Validate(); err == nil {
		t.Fatal("status accepted a capture transition bound to a different target identity")
	}
}

func TestNegotiatedStatusReturnsProviderOwnedTargetedValidationRequest(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc corrected() int { return 1 }\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "typed-validation-request")
	resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0],
		Findings: []facadeFinding{{
			Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate behavior is incorrect",
			ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"inspected frozen candidate"},
	})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{resultPath}); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 2)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc corrected() int { return 2 }\n", 0o644)
	debugStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	debugRecord, _ := debugStore.Load()
	if _, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, debugRecord.State, debugRecord.State.CapturePhaseRevision); err != nil {
		t.Fatalf("derive targeted validation request from corrected candidate: %v", err)
	}
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--lineage", started.LineageID, "--agent", "pi", "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("status after correction: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.ValidationRequest == nil || status.ValidationRequest.TargetIdentity != started.RepositoryContext.TargetIdentity ||
		status.ValidationRequest.CorrectionCandidateTree != status.Projection.CurrentCandidateTree ||
		status.ValidationRequest.CorrectionPathsDigest != status.Projection.PathsDigest ||
		transitionArgumentValue(t, status.NextTransition, "target") != status.ValidationRequest.CorrectionTargetIdentity {
		t.Fatalf("status targeted validation request = %#v; status target = %q; transition = %#v", status.ValidationRequest, status.TargetIdentity, status.NextTransition)
	}
	wrongStatusTarget := status
	wrongStatusTarget.TargetIdentity = "sha256:" + strings.Repeat("f", 64)
	if err := wrongStatusTarget.Validate(); err == nil {
		t.Fatal("status accepted a top-level target identity that differs from its current projection")
	}
	wrongInitialTarget := status
	wrongInitialTarget.Projection.InitialSnapshotIdentity = "sha256:" + strings.Repeat("e", 64)
	if err := wrongInitialTarget.Validate(); err == nil {
		t.Fatal("status accepted a validation request bound to a different initial target identity")
	}
	statusWithoutDirectRequest := status
	statusWithoutDirectRequest.ValidationRequest = nil
	if err := statusWithoutDirectRequest.Validate(); err == nil {
		t.Fatal("status accepted a transition request without the provider-owned top-level request")
	}
	statusWithoutAnyRequest := status
	transitionWithoutRequest := *status.NextTransition
	collectionWithoutRequest := *status.NextTransition.Collect
	inputsWithoutRequest := append([]ReviewTransitionInput(nil), collectionWithoutRequest.Inputs...)
	inputsWithoutRequest[0].ValidationRequest = nil
	collectionWithoutRequest.Inputs = inputsWithoutRequest
	transitionWithoutRequest.Collect = &collectionWithoutRequest
	statusWithoutAnyRequest.NextTransition = &transitionWithoutRequest
	statusWithoutAnyRequest.ValidationRequest = nil
	if err := statusWithoutAnyRequest.Validate(); err == nil {
		t.Fatal("status accepted a targeted-validation transition without its provider-owned request")
	}
	request := status.ValidationRequest
	if request.Schema != reviewtransaction.TargetedValidationRequestSchema ||
		request.LineageID != started.LineageID || request.ExpectedRevision != debugRecord.State.CapturePhaseRevision ||
		request.TargetIdentity != started.RepositoryContext.TargetIdentity || len(request.FixFindingIDs) != 1 ||
		request.CorrectionCandidateTree == "" || request.CorrectionTargetIdentity == "" ||
		reviewtransaction.ValidateTargetedValidationRequest(*request) != nil {
		t.Fatalf("validation request = %#v", request)
	}
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].ValidationRequest == nil ||
		status.NextTransition.Collect.Inputs[0].ValidationRequest.RequestHash != request.RequestHash ||
		status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation ||
		transitionArgumentValue(t, status.NextTransition, "target") != request.CorrectionTargetIdentity {
		t.Fatalf("validation transition = %#v", status.NextTransition)
	}

	captureArgs := reviewTransitionInputTokens(t, repo, status.NextTransition.Collect.Inputs[0])
	captureArgs = replaceReviewContextToken(t, captureArgs, rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: started.LineageID, TargetIdentity: request.CorrectionTargetIdentity, Revision: request.ExpectedRevision,
	}))
	previous := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, *request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	var output bytes.Buffer
	if err := RunReviewCaptureValidation(captureArgs, &output); err != nil {
		t.Fatalf("capture exact targeted-validator transition: %v\n%s", err, output.String())
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &closure)
	if closure.Operation != "review/capture-validation" || closure.State != reviewtransaction.StateApproved ||
		closure.LineageID != started.LineageID {
		t.Fatalf("targeted-validator terminal capture = %#v", closure)
	}
	if closure.Acknowledgement == nil {
		t.Fatalf("targeted-validator terminal capture omitted acknowledgement: %#v", closure)
	}
	assertApprovedAcknowledgementTransition(t, closure.Acknowledgement, repo, started.LineageID, closure.Acknowledgement.Binding.TargetIdentity, closure.StoreRevision)
}

func TestNegotiatedStatusAcceptsCorrectionSubsetDigest(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "corrected.go", "package candidate\n\nfunc correctedSubset() int { return 0 }\n", 0o644)
	writeReviewStartCandidate(t, repo, "untouched.go", "package candidate\n\nfunc untouchedSubset() int { return 0 }\n", 0o644)
	runReviewCLIGit(t, repo, "add", "corrected.go", "untouched.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add correction subset fixture")
	runReviewCLIGit(t, repo, "config", "core.trustctime", "false")
	fixed := time.Unix(1_700_000_000, 0)
	writeReviewStartCandidate(t, repo, "corrected.go", "package candidate\n\nfunc correctedSubset() int { return 1 }\n", 0o644)
	writeReviewStartCandidate(t, repo, "untouched.go", "package candidate\n\nfunc untouchedSubset() int { return 1 }\n", 0o644)
	for _, name := range []string{"corrected.go", "untouched.go"} {
		if err := os.Chtimes(filepath.Join(repo, name), fixed, fixed); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "corrected.go", "untouched.go")
	indexPath := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	if err := os.Chtimes(indexPath, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "targeted-validation-subset")
	resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0],
		Findings: []facadeFinding{{
			Location: "corrected.go:3", Severity: "CRITICAL", Claim: "corrected subset remains wrong",
			ProofRefs: []string{"corrected.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"inspected exact two-path candidate"},
	})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{resultPath}); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 1)
	writeReviewStartCandidate(t, repo, "corrected.go", "package candidate\n\nfunc correctedSubset() int { return 2 }\n", 0o644)
	if err := os.Chtimes(filepath.Join(repo, "corrected.go"), fixed, fixed); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("status rejected a correction-only subset digest: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.ValidationRequest != nil || !slices.Equal(status.Projection.Paths, []string{"corrected.go", "untouched.go"}) {
		t.Fatalf("unbound subset correction status = request %#v / projection %#v", status.ValidationRequest, status.Projection)
	}

	writeReviewStartCandidate(t, repo, "corrected.go", "package candidate\n\nfunc correctedSubset() int { return 0 }\n", 0o644)
	if err := os.Chtimes(filepath.Join(repo, "corrected.go"), fixed.Add(time.Second), fixed.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("status rejected a correction path restored exactly to base: %v\n%s", err, output.String())
	}
	status = ReviewTargetStatusResult{}
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.ValidationRequest != nil || !slices.Equal(status.Projection.Paths, []string{"untouched.go"}) {
		t.Fatalf("unbound base-equivalent correction status = request %#v / projection %#v", status.ValidationRequest, status.Projection)
	}
}

func TestNegotiatedStartPublishesStableOpaqueRepositoryContext(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)

	args := boundNegotiatedStartArgs(t, []string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", "repository-context-start"})
	var first bytes.Buffer
	if err := RunReviewFacadeStart(args, &first); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, first.Bytes(), &started)
	startSchema := compileWholePublishedReviewSchema(t, "v2", "start-v4.schema.json")
	validatePublishedReviewSchema(t, startSchema, first.Bytes())
	if started.RepositoryContext == nil || started.RepositoryContext.Capability != reviewtransaction.ReviewRepositoryContextCapability ||
		!strings.HasPrefix(started.RepositoryContext.Handle, "rctx2_") || !validReviewCapabilitySHA256(started.RepositoryContext.Revision) {
		t.Fatalf("START emitted legacy or invalid repository context = %#v", started.RepositoryContext)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".gentle-ai", "review-contexts")); !os.IsNotExist(err) {
		t.Fatalf("START persisted a retired repository-context locator: %v", err)
	}
	if bytes.Contains(first.Bytes(), []byte(repo)) || bytes.Contains(first.Bytes(), []byte(filepath.Join(repo, ".git"))) {
		t.Fatalf("negotiated START leaked a repository path: %s", first.String())
	}
	root, err := reviewtransaction.ResolveReviewRepositoryContext(context.Background(), repo, started.RepositoryContext.Handle, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, Revision: started.RepositoryContext.Revision,
	})
	if err != nil || root != repo {
		t.Fatalf("resolved context = %q, %v; want %q", root, err, repo)
	}

	var resumed bytes.Buffer
	if err := RunReviewFacadeStart(args, &resumed); err != nil {
		t.Fatal(err)
	}
	var retry ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, resumed.Bytes(), &retry)
	if retry.Action != "replayed" || retry.RepositoryContext == nil ||
		retry.RepositoryContext.Handle != started.RepositoryContext.Handle || retry.RepositoryContext.Revision != started.RepositoryContext.Revision {
		t.Fatalf("replayed repository context = %#v", retry)
	}
	var statusOutput bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", started.LineageID, "--next-transition"}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.RepositoryContext == nil || status.RepositoryContext.Handle != started.RepositoryContext.Handle {
		t.Fatalf("status repository context = %#v", status.RepositoryContext)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if status.Authority == nil || status.RepositoryContext.Revision != record.State.CapturePhaseRevision {
		t.Fatalf("STATUS repository context is not Pn-bound: status=%#v record=%#v", status, record)
	}
	// CapturePhaseRevision is deliberately an internal producer invariant, not a
	// new v2 wire field. Restore it only for this in-memory tamper check.
	status.Authority.CapturePhaseRevision = record.State.CapturePhaseRevision
	wrongStatusRevision := status
	wrongStatusContext := *status.RepositoryContext
	wrongStatusContext.Revision = "sha256:" + strings.Repeat("e", 64)
	wrongStatusRevision.RepositoryContext = &wrongStatusContext
	if err := wrongStatusRevision.Validate(); err == nil {
		t.Fatal("STATUS accepted repository context bound to a different capture phase")
	}
	wrongCapturePhase := "sha256:" + strings.Repeat("f", 64)
	if err := RunReviewCaptureResult([]string{
		"--repository-context", status.RepositoryContext.Handle, "--lineage", status.Authority.LineageID,
		"--target", status.RepositoryContext.TargetIdentity, "--expected-revision", wrongCapturePhase,
		"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
	}, io.Discard); err == nil {
		t.Fatal("capture preflight accepted repository context bound to the wrong capture phase")
	}
	wrongTarget := status
	wrongTargetContext := *status.RepositoryContext
	wrongTargetContext.TargetIdentity = "sha256:" + strings.Repeat("f", 64)
	wrongTarget.RepositoryContext = &wrongTargetContext
	if err := wrongTarget.Validate(); err == nil {
		t.Fatal("STATUS accepted repository context bound to the wrong authority target")
	}
	wrongStartRevision := started
	wrongStartRevisionContext := *started.RepositoryContext
	wrongStartRevisionContext.Revision = "sha256:" + strings.Repeat("f", 64)
	wrongStartRevision.RepositoryContext = &wrongStartRevisionContext
	if err := wrongStartRevision.Validate(); err == nil {
		t.Fatal("START accepted repository context bound to the wrong authority revision")
	}
}

func TestNegotiatedStartRepositoryContextCoversWorkspaceStagedAndOverlay(t *testing.T) {
	tests := []struct {
		name string
		args func(*testing.T, string) []string
	}{
		{name: "workspace", args: func(t *testing.T, repo string) []string {
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc workspace() {}\n", 0o644)
			return nil
		}},
		{name: "staged", args: func(t *testing.T, repo string) []string {
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc staged() {}\n", 0o644)
			runReviewCLIGit(t, repo, "add", "candidate.go")
			return []string{"--projection", "staged"}
		}},
		{name: "workspace overlay", args: func(t *testing.T, repo string) []string {
			base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			writeReviewStartCandidate(t, repo, "committed.go", "package candidate\n", 0o644)
			runReviewCLIGit(t, repo, "add", "committed.go")
			runReviewCLIGit(t, repo, "commit", "-m", "add committed candidate")
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc overlay() {}\n", 0o644)
			return []string{"--base-ref", base, "--workspace-overlay"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			args := boundNegotiatedStartArgs(t, append([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--lineage", "repository-context-" + strings.ReplaceAll(tt.name, " ", "-")}, tt.args(t, repo)...))
			var output bytes.Buffer
			if err := RunReviewFacadeStart(args, &output); err != nil {
				t.Fatal(err)
			}
			var result ReviewIntegrationStartResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if result.State != reviewtransaction.StateReviewing || result.RepositoryContext == nil {
				t.Fatalf("START result = %#v", result)
			}
			if result.TargetIdentity != "" {
				wrong := result
				contextCopy := *result.RepositoryContext
				contextCopy.TargetIdentity = "sha256:" + strings.Repeat("f", 64)
				wrong.RepositoryContext = &contextCopy
				if err := wrong.Validate(); err == nil {
					t.Fatal("START accepted repository context bound to a different top-level target identity")
				}
			}
		})
	}
}

// rctx2ReviewRepositoryContextForTest issues the same provider-owned handle the
// lifecycle issues. The handle is a digest, so a test cannot hand-assemble one:
// deriving it here is what keeps these tests honest about what a host receives.
func rctx2ReviewRepositoryContextForTest(t *testing.T, repo string, binding reviewtransaction.ReviewRepositoryContextBinding) string {
	t.Helper()
	handle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	// Surfaces that take the handle without a --cwd flag -- the OpenCode relay
	// the managed shim starts in place -- verify the digest against process
	// cwd, so a test holding this handle stands where a real host stands.
	t.Chdir(repo)
	return handle
}

func replaceReviewContextArgument(t *testing.T, args []string, handle string) []string {
	t.Helper()
	return replaceReviewArgument(t, args, "--repository-context", handle)
}

func replaceReviewArgument(t *testing.T, args []string, name, value string) []string {
	t.Helper()
	result := slices.Clone(args)
	for index, argument := range result {
		if argument == name && index+1 < len(result) {
			result[index+1] = value
			return result
		}
	}
	t.Fatalf("review arguments omit %q: %v", name, args)
	return nil
}

func replaceReviewContextToken(t *testing.T, args []string, handle string) []string {
	t.Helper()
	result := slices.Clone(args)
	for index, argument := range result {
		if strings.HasPrefix(argument, "--repository-context=") {
			result[index] = "--repository-context=" + handle
			return result
		}
	}
	t.Fatalf("review tokens omit repository context: %v", args)
	return nil
}

func TestLegacyStartBytesDoNotContainRepositoryContext(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc legacy() {}\n", 0o644)
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "legacy-no-context"}, &output); err != nil {
		t.Fatal(err)
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacy["repository_context"]; exists {
		t.Fatalf("legacy START gained negotiated repository context: %s", output.String())
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "legacy-no-context"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}
