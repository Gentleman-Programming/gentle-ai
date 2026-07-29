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

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRepositoryContextCaptureFromUnrelatedCWDProducesFinalizeArtifact(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(resultPath, admittedReviewerPayloadForTest(t, repo, record, started.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	bindingArgs := []string{
		"--repository-context", started.RepositoryContext.Handle,
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

	var preserved bytes.Buffer
	if err := RunReviewPreserveResult(append(append([]string{}, bindingArgs...), "--input", resultPath), &preserved); err != nil {
		t.Fatalf("preserve-result from unrelated cwd: %v", err)
	}
	var preservedEnvelope map[string]json.RawMessage
	decodeStrictReviewJSON(t, preserved.Bytes(), &preservedEnvelope)
	if _, leaked := preservedEnvelope["path"]; leaked || len(preservedEnvelope["reference"]) == 0 || bytes.Contains(preserved.Bytes(), []byte(repo)) || bytes.Contains(preserved.Bytes(), []byte(os.Getenv("HOME"))) {
		t.Fatalf("opaque preserve response leaked provider paths or omitted its reference: %s", preserved.String())
	}
	var incident reviewIncidentArtifact
	decodeStrictReviewJSON(t, preserved.Bytes(), &incident)
	if incident.LineageID != started.LineageID || incident.TargetIdentity != started.RepositoryContext.TargetIdentity ||
		incident.Lens != started.SelectedLenses[0] || incident.SelectedOrder != 0 || incident.Path != "" ||
		!strings.HasPrefix(incident.Reference, reviewIncidentReferencePrefix) {
		t.Fatalf("preserved incident = %#v", incident)
	}
	missingPreserveRevision := append(append([]string{}, bindingArgs...), "--input", resultPath)
	for index := 0; index < len(missingPreserveRevision); index++ {
		if missingPreserveRevision[index] == "--expected-revision" {
			missingPreserveRevision = append(missingPreserveRevision[:index], missingPreserveRevision[index+2:]...)
			break
		}
	}
	if err := RunReviewPreserveResult(missingPreserveRevision, io.Discard); err == nil {
		t.Fatal("repository-context preserve accepted a missing revision")
	}
	if err := RunReviewPreserveResult(append(append([]string{}, bindingArgs...), "--cwd", repo, "--input", resultPath), io.Discard); err == nil {
		t.Fatal("repository-context preserve accepted an explicit cwd")
	}

	args := append(append([]string{}, bindingArgs...), "--input", resultPath)
	var captured bytes.Buffer
	if err := RunReviewCaptureResult(args, &captured); err != nil {
		t.Fatal(err)
	}
	var capturedEnvelope map[string]json.RawMessage
	decodeStrictReviewJSON(t, captured.Bytes(), &capturedEnvelope)
	if _, leaked := capturedEnvelope["path"]; leaked || len(capturedEnvelope["reference"]) == 0 || bytes.Contains(captured.Bytes(), []byte(repo)) || bytes.Contains(captured.Bytes(), []byte(os.Getenv("HOME"))) {
		t.Fatalf("opaque capture response leaked provider paths or omitted its reference: %s", captured.String())
	}
	var manifest reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &manifest)
	if manifest.LineageID != started.LineageID || manifest.TargetIdentity != started.RepositoryContext.TargetIdentity ||
		manifest.Lens != started.SelectedLenses[0] || manifest.SelectedOrder != 0 || manifest.Path != "" ||
		!strings.HasPrefix(manifest.Reference, reviewResultReferencePrefix) {
		t.Fatalf("manifest = %#v", manifest)
	}
	if results, err := readFacadeReviewerArtifacts(context.Background(), repo, []string{strings.TrimSpace(captured.String())}, store.Dir, record.State, record.Revision); err != nil || len(results) != 1 {
		t.Fatalf("provider did not resolve opaque captured-result reference: %v, %#v", err, results)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results",
	}, io.Discard); err != nil {
		t.Fatalf("FINALIZE --captured-results did not resolve the provider-owned opaque capture: %v", err)
	}

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
}

func TestPreserveResultRequiresExactLiveSelectedLensBinding(t *testing.T) {
	for _, contextMode := range []bool{false, true} {
		name := "legacy-cwd"
		if contextMode {
			name = "opaque-context"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc preserveBinding() {}\n", 0o644)
			started := runNegotiatedReviewStart(t, repo, "preserve-selected-binding-"+name)
			if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
				t.Fatalf("START result = %#v", started)
			}
			input := filepath.Join(t.TempDir(), "reviewer.json")
			writeReviewCLIJSON(t, input, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{}, Evidence: []string{"binding test"}})
			base := []string{"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity, "--input", input}
			if contextMode {
				base = append(base, "--repository-context", started.RepositoryContext.Handle, "--expected-revision", started.RepositoryContext.Revision)
			} else {
				base = append(base, "--cwd", repo)
			}
			wrongLens := reviewtransaction.LensRisk
			if wrongLens == started.SelectedLenses[0] {
				wrongLens = reviewtransaction.LensReliability
			}
			for _, binding := range []struct {
				name  string
				lens  string
				order string
			}{
				{name: "wrong lens", lens: wrongLens, order: "0"},
				{name: "wrong order", lens: started.SelectedLenses[0], order: "1"},
			} {
				t.Run(binding.name, func(t *testing.T) {
					args := append(append([]string{}, base...), "--lens", binding.lens, "--order", binding.order)
					if err := RunReviewPreserveResult(args, io.Discard); err == nil {
						t.Fatalf("preserve-result accepted unselected lens/order %q/%s", binding.lens, binding.order)
					}
				})
			}
		})
	}
}

func TestOpaqueContextErrorsDoNotExposeProviderPaths(t *testing.T) {
	for _, damage := range []string{"locator", "authority"} {
		t.Run(damage, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc opaqueFailure() {}\n", 0o644)
			started := runNegotiatedReviewStart(t, repo, "opaque-error-"+damage)
			if damage == "locator" {
				if err := os.RemoveAll(filepath.Join(home, ".gentle-ai", "review-contexts")); err != nil {
					t.Fatal(err)
				}
			} else {
				store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(store.StatePath()); err != nil {
					t.Fatal(err)
				}
			}
			err := RunReviewCaptureResult([]string{
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

func TestNegotiatedFinalizeReturnsProviderOwnedTargetedValidationRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc corrected() int { return 2 }\n", 0o644)
	debugStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	debugRecord, _ := debugStore.Load()
	if _, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, debugRecord.State, debugRecord.Revision); err != nil {
		t.Fatalf("derive validation request before FINALIZE: %v", err)
	}
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", started.LineageID, "--next-transition",
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
	evidencePath := filepath.Join(t.TempDir(), "correction-evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("targeted and full repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", status.ValidationRequest.CorrectionTargetIdentity,
		"--expected-revision", status.Authority.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", started.LineageID, "--captured-evidence",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var envelope ReviewIntegrationOperationResult
	decodeStrictReviewJSON(t, output.Bytes(), &envelope)
	var result ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, envelope.Result, &result)
	request := result.ValidationRequest
	if request == nil || request.Schema != reviewtransaction.TargetedValidationRequestSchema ||
		request.LineageID != started.LineageID || request.ExpectedRevision != result.StoreRevision ||
		request.TargetIdentity != started.RepositoryContext.TargetIdentity || len(request.FixFindingIDs) != 1 ||
		request.CorrectionCandidateTree == "" || request.CorrectionTargetIdentity == "" ||
		reviewtransaction.ValidateTargetedValidationRequest(*request) != nil {
		t.Fatalf("validation request = %#v", request)
	}
	if result.NextTransition == nil || result.NextTransition.Collect == nil || len(result.NextTransition.Collect.Inputs) != 1 ||
		result.NextTransition.Collect.Inputs[0].ValidationRequest == nil ||
		result.NextTransition.Collect.Inputs[0].ValidationRequest.RequestHash != request.RequestHash ||
		transitionArgumentValue(t, result.NextTransition, "target") != request.CorrectionTargetIdentity {
		t.Fatalf("validation transition = %#v", result.NextTransition)
	}
	resultWithoutDirectRequest := result
	resultWithoutDirectRequest.ValidationRequest = nil
	changedResult, err := json.Marshal(resultWithoutDirectRequest)
	if err != nil {
		t.Fatal(err)
	}
	changedEnvelope := envelope
	changedEnvelope.Result = changedResult
	if err := changedEnvelope.Validate(); err == nil {
		t.Fatal("FINALIZE accepted a transition request without the provider-owned top-level request")
	}
	if bytes.Contains(output.Bytes(), []byte(filepath.Join(repo, ".git", "gentle-ai"))) || bytes.Contains(output.Bytes(), []byte(repo)) {
		t.Fatalf("validation request leaked provider internals: %s", output.String())
	}
}

func TestNegotiatedStatusAcceptsCorrectionSubsetDigest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "1"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "corrected.go", "package candidate\n\nfunc correctedSubset() int { return 2 }\n", 0o644)
	if err := os.Chtimes(filepath.Join(repo, "corrected.go"), fixed, fixed); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("status rejected a correction-only subset digest: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.ValidationRequest == nil || status.ValidationRequest.CorrectionPathsDigest == status.Projection.PathsDigest ||
		status.ValidationRequest.CorrectionCandidateTree != status.Projection.CurrentCandidateTree ||
		!slices.Equal(status.ValidationRequest.CorrectionPaths, []string{"corrected.go"}) ||
		!slices.Equal(status.Projection.Paths, []string{"corrected.go", "untouched.go"}) {
		t.Fatalf("subset request/projection = %#v / %#v", status.ValidationRequest, status.Projection)
	}
}

func TestNegotiatedStartPublishesStableOpaqueRepositoryContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)

	args := boundNegotiatedStartArgs(t, []string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--lineage", "repository-context-start"})
	var first bytes.Buffer
	if err := RunReviewFacadeStart(args, &first); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, first.Bytes(), &started)
	if started.RepositoryContext == nil || started.RepositoryContext.Capability != reviewtransaction.ReviewRepositoryContextCapability ||
		started.RepositoryContext.Handle == "" || !validReviewCapabilitySHA256(started.RepositoryContext.Revision) {
		t.Fatalf("repository context = %#v", started.RepositoryContext)
	}
	if bytes.Contains(first.Bytes(), []byte(repo)) || bytes.Contains(first.Bytes(), []byte(filepath.Join(repo, ".git"))) {
		t.Fatalf("negotiated START leaked a repository path: %s", first.String())
	}
	root, err := reviewtransaction.ResolveReviewRepositoryContext(context.Background(), started.RepositoryContext.Handle, reviewtransaction.ReviewRepositoryContextBinding{
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
	if retry.Action != string(reviewtransaction.CompactStartResumed) || retry.RepositoryContext == nil ||
		retry.RepositoryContext.Handle != started.RepositoryContext.Handle || retry.RepositoryContext.Revision != started.RepositoryContext.Revision {
		t.Fatalf("resumed repository context = %#v", retry)
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
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

func TestLegacyStartBytesDoNotContainRepositoryContext(t *testing.T) {
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
