package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestCommittedBaseDiffLastReviewerCapturePublishesExactStatusContinuation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	const baseRef = "frozen-base"
	const lineage = "committed-last-capture-continuation"
	runReviewCLIGit(t, repo, "branch", baseRef, "HEAD")
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o755)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "wrong candidate")

	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", baseRef, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var closureOutput bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{{
		ID: "R3-001", Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
		ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &closureOutput)

	var closure struct {
		Schema             string                     `json:"schema"`
		Operation          string                     `json:"operation"`
		LineageID          string                     `json:"lineage_id"`
		State              reviewtransaction.State    `json:"state"`
		StatusContinuation *ReviewTransitionExecution `json:"status_continuation"`
	}
	if err := json.Unmarshal(closureOutput.Bytes(), &closure); err != nil {
		t.Fatalf("decode committed final capture closure: %v\n%s", err, closureOutput.String())
	}
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-result" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("committed final capture closure = %#v, want correction-required public closure", closure)
	}
	if closure.StatusContinuation == nil {
		t.Fatalf("committed correction closure lacks required status_continuation: %s", closureOutput.String())
	}

	continuation := closure.StatusContinuation
	if continuation.Operation != "review.status" || continuation.Command == "" {
		t.Fatalf("status continuation = %#v, want runnable review.status", continuation)
	}
	arguments, err := reviewTransitionArgumentMap(continuation.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := map[string]string{
		"cwd": repo, "contract": ReviewIntegrationContractV2, "next-transition": "true",
		"lineage": lineage, "base-ref": record.State.InitialSnapshot.BaseTree, "committed-only": "true",
	}
	if len(arguments) != len(wantArguments) {
		t.Fatalf("status continuation arguments = %#v, want exactly %#v", continuation.Arguments, wantArguments)
	}
	for name, want := range wantArguments {
		got, ok := arguments[name]
		if !ok || got != want {
			t.Fatalf("status continuation argument %q = %q, want %q; all=%#v", name, got, want, continuation.Arguments)
		}
	}
	for _, argument := range continuation.Arguments {
		if want := "--" + argument.Name + "=" + argument.Value; argument.Token != want {
			t.Fatalf("status continuation argument %q token = %q, want %q", argument.Name, argument.Token, want)
		}
	}
	if want := reviewTransitionCommandLine(continuation.Operation, continuation.Arguments); continuation.Command != want {
		t.Fatalf("status continuation command = %q, want %q", continuation.Command, want)
	}

	// The named branch is mutable after START. Re-entry must remain bound to the
	// frozen BaseTree carried by the continuation rather than this symbolic ref.
	runReviewCLIGit(t, repo, "branch", "-f", baseRef, "HEAD")
	statusArgs := []string{strings.TrimPrefix(continuation.Operation, "review.")}
	for _, argument := range continuation.Arguments {
		statusArgs = append(statusArgs, argument.Token)
	}
	var statusOutput bytes.Buffer
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatalf("run closure status continuation unchanged: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.ReasonCode != "correction_plan_required" {
		t.Fatalf("closure status continuation = %#v, want correction_plan_required", status.NextTransition)
	}
}

func TestCommittedBaseDiffCorrectionReentryRunsReturnedContinuationForAdvertisedLanes(t *testing.T) {
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentCodex} {
		t.Run(string(runtime), func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o644)
			runReviewCLIGit(t, repo, "add", "candidate.go")
			runReviewCLIGit(t, repo, "commit", "-qm", "candidate")

			lineage := "committed-" + string(runtime) + "-closure"
			var startOutput bytes.Buffer
			if err := RunReview(boundNegotiatedStartArgs(t, []string{
				"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage,
				"--base-ref", baseTree,
			}), &startOutput); err != nil {
				t.Fatal(negotiatedReviewStartFailure(err, startOutput.String()))
			}
			started := decodeNegotiatedReviewStart(t, startOutput.Bytes())
			if started.LineageID != lineage || len(started.SelectedLenses) == 0 {
				t.Fatalf("committed negotiated START = %#v", started)
			}

			store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if record.State.InitialSnapshot.BaseTree != baseTree {
				t.Fatalf("committed START base tree = %q, want frozen %q", record.State.InitialSnapshot.BaseTree, baseTree)
			}

			if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
				t.Fatalf("committed START = %#v, want one provider-owned lens", started)
			}
			reviewer := admittedReviewerResultForTest(t, repo, record, started.SelectedLenses[0], 0)
			reviewer.Findings = []facadeFinding{{
				ID: "R3-001", Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
			payload, err := json.Marshal(reviewer)
			if err != nil {
				t.Fatal(err)
			}
			var terminal []byte
			switch runtime {
			case model.AgentClaudeCode, model.AgentCodex:
				previous := reviewProviderAdapterFor
				reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
					if agent != runtime {
						return nil, errors.New("unexpected runtime")
					}
					return providerTestAdapter{raw: payload}, nil
				}
				t.Cleanup(func() { reviewProviderAdapterFor = previous })
				var output bytes.Buffer
				if err := RunReviewCaptureResult([]string{
					"--cwd", repo,
					"--repository-context", started.RepositoryContext.Handle, "--lineage", lineage,
					"--target", started.RepositoryContext.TargetIdentity, "--expected-revision", started.RepositoryContext.Revision,
					"--lens", started.SelectedLenses[0], "--order", "0", "--agent", string(runtime),
				}, &output); err != nil {
					t.Fatalf("%s final capture: %v\n%s", runtime, err, output.String())
				}
				terminal = output.Bytes()
			case model.AgentOpenCode:
				relay := startOpenCodeTransportRelay(t, repo, openCodeLensTransportStart(t, repo, record, started.SelectedLenses[0]))
				hostOutput := string(payload)
				completed, err := relay.complete(openCodeTransportEnvelope{
					Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
				})
				if err != nil || completed.Output == nil {
					t.Fatalf("%s final capture = %#v, %v", runtime, completed, err)
				}
				terminal = []byte(*completed.Output)
			}

			var closure reviewLastEventClosureResult
			decodeStrictReviewJSON(t, terminal, &closure)
			if closure.LineageID != lineage || closure.State != reviewtransaction.StateCorrectionRequired || closure.StatusContinuation == nil {
				t.Fatalf("%s terminal committed closure = %#v", runtime, closure)
			}
			continuation := closure.StatusContinuation
			if continuation.Operation != "review.status" {
				t.Fatalf("%s continuation operation = %q, want review.status", runtime, continuation.Operation)
			}
			arguments, err := reviewTransitionArgumentMap(continuation.Arguments)
			if err != nil {
				t.Fatal(err)
			}
			if arguments["agent"] != string(runtime) || arguments["base-ref"] != baseTree || arguments["committed-only"] != "true" {
				t.Fatalf("%s continuation selectors = %#v", runtime, arguments)
			}
			verb, found := reviewTransitionCommandVerb(continuation.Operation)
			if !found {
				t.Fatalf("%s continuation operation %q has no dispatched verb", runtime, continuation.Operation)
			}
			statusArgs := []string{verb}
			for _, argument := range continuation.Arguments {
				statusArgs = append(statusArgs, argument.Token)
			}
			var statusOutput bytes.Buffer
			if err := RunReview(statusArgs, &statusOutput); err != nil {
				t.Fatalf("%s closure continuation: %v\n%s", runtime, err, statusOutput.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
			if status.Authority == nil || status.Authority.LineageID != lineage || status.NextTransition == nil ||
				status.NextTransition.ReasonCode != "correction_plan_required" {
				t.Fatalf("%s closure re-entry = authority=%#v transition=%#v", runtime, status.Authority, status.NextTransition)
			}
		})
	}
}

func TestCorrectionStatusContinuationUsesFrozenTargetSelectors(t *testing.T) {
	const tree = "0123456789abcdef0123456789abcdef01234567"
	for _, testCase := range []struct {
		name       string
		kind       reviewtransaction.TargetKind
		projection reviewtransaction.Projection
		want       map[string]string
		absent     []string
	}{
		{name: "workspace", kind: reviewtransaction.TargetCurrentChanges, projection: reviewtransaction.ProjectionWorkspace,
			want: map[string]string{}, absent: []string{"agent", "base-ref", "committed-only", "workspace-overlay", "projection"}},
		{name: "staged", kind: reviewtransaction.TargetCurrentChanges, projection: reviewtransaction.ProjectionStaged,
			want: map[string]string{"projection": "staged"}, absent: []string{"agent", "base-ref", "committed-only", "workspace-overlay"}},
		{name: "workspace-overlay", kind: reviewtransaction.TargetBaseWorkspaceOverlay, projection: reviewtransaction.ProjectionWorkspace,
			want: map[string]string{"base-ref": tree, "workspace-overlay": "true"}, absent: []string{"agent", "committed-only", "projection"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			continuation := reviewCorrectionStatusContinuation("/frozen/repository", reviewtransaction.CompactState{
				LineageID:       "correction-status-" + testCase.name,
				InitialSnapshot: reviewtransaction.Snapshot{Kind: testCase.kind, Projection: testCase.projection, BaseTree: tree, Identity: "sha256:" + strings.Repeat("a", 64)},
			}, "sha256:"+strings.Repeat("b", 64), "")
			arguments, err := reviewTransitionArgumentMap(continuation.Arguments)
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range testCase.want {
				if got := arguments[name]; got != want {
					t.Fatalf("%s selector %q = %q, want %q", testCase.name, name, got, want)
				}
			}
			for _, name := range testCase.absent {
				if _, found := arguments[name]; found {
					t.Fatalf("%s continuation added non-applicable selector %q: %#v", testCase.name, name, continuation.Arguments)
				}
			}
		})
	}
}

func TestCorrectionStatusContinuationRefusesUnsupportedTargetKind(t *testing.T) {
	for _, kind := range []reviewtransaction.TargetKind{
		reviewtransaction.TargetExactRevision,
		reviewtransaction.TargetFixDiff,
		reviewtransaction.TargetKind("malformed-target-kind"),
	} {
		t.Run(string(kind), func(t *testing.T) {
			continuation := reviewCorrectionStatusContinuation("/frozen/repository", reviewtransaction.CompactState{
				LineageID: "unsupported-correction-status",
				InitialSnapshot: reviewtransaction.Snapshot{
					Kind: kind, Identity: "sha256:" + strings.Repeat("a", 64),
				},
			}, "sha256:"+strings.Repeat("b", 64), model.AgentPi)
			if continuation != nil {
				t.Fatalf("unsupported target kind %q emitted selector-incomplete continuation %#v", kind, continuation)
			}
		})
	}
}

func TestSelectorlessCommittedCorrectionClosesOnTargetedValidation(t *testing.T) {
	for _, amend := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "amend"}[amend], func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			repo, base, lineage := forecastCommittedCorrection(t)
			writeCommittedCorrection(t, repo, amend)
			// The mutable ref must not influence the frozen correction boundary.
			runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")
			authorityStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			authorityRecord, err := authorityStore.Load()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewtransaction.RebuildCommittedBaseDiffCorrectionCandidate(context.Background(), repo, authorityRecord.State); err != nil {
				t.Fatalf("rebuild committed correction: %v", err)
			}

			status := committedCorrectionStatus(t, repo, lineage, authorityRecord.State.InitialSnapshot.BaseTree)
			if status.Authority == nil || status.Authority.LineageID != lineage || status.ValidationRequest == nil ||
				status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
				status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
				len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
				t.Fatalf("post-commit correction status = %#v", status)
			}
			request := status.ValidationRequest
			previous := reviewProviderRoleHostAdapter
			reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
				return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
					return providerTargetedValidationPayload(t, *request), nil
				})
			}
			t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
			var terminalOutput bytes.Buffer
			if err := RunReviewCaptureValidation(reviewTransitionInputTokens(t, repo, status.NextTransition.Collect.Inputs[0]), &terminalOutput); err != nil {
				t.Fatalf("capture selector-less targeted validation: %v\n%s", err, terminalOutput.String())
			}
			var terminal reviewLastEventClosureResult
			decodeStrictReviewJSON(t, terminalOutput.Bytes(), &terminal)
			if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
				t.Fatalf("committed correction terminal capture = %#v", terminal)
			}
			assertApprovedCompactAuthorityBurned(t, authorityStore, lineage)
		})
	}
}

func TestStagedCorrectionClosesOnTargetedValidation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n", 0o755)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "staged-correction", "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate is wrong",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed frozen staged candidate"}})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{result}); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 2)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--lineage", started.LineageID, "--projection", "staged", "--agent", "pi",
	}, &statusOutput); err != nil {
		t.Fatalf("staged correction status: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.ValidationRequest == nil || status.NextTransition == nil ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
		t.Fatalf("staged correction status = %#v", status)
	}
	request := status.ValidationRequest
	previous := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, *request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	var terminalOutput bytes.Buffer
	if err := RunReviewCaptureValidation(reviewTransitionInputTokens(t, repo, status.NextTransition.Collect.Inputs[0]), &terminalOutput); err != nil {
		t.Fatalf("capture staged targeted validation: %v\n%s", err, terminalOutput.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, terminalOutput.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("staged correction terminal capture = %#v", terminal)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestSelectorlessCommittedCorrectionFailsClosedForUnreadableAuthority(t *testing.T) {
	repo, base, lineage := forecastCommittedCorrection(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeCommittedCorrection(t, repo, false)
	runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")

	statePath := filepath.Join(reviewCLICompactStoreDir(repo, "unreadable-authority"), "review-state.json")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", "unreadable-authority",
		"--base-ref", record.State.InitialSnapshot.BaseTree, "--committed-only",
	}, &output)
	if err == nil {
		t.Fatalf("selector-less status selected a recoverable lineage despite unreadable authority: %s", output.String())
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("selector-less status error = %T %[1]v, want operational path error", err)
	}
}

func TestSelectorlessCommittedCorrectionFailsClosedForOperationalReconstruction(t *testing.T) {
	repo := committedCorrectionWithOperationalReconstructionAuthority(t)

	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
	}, &output)
	assertOperationalReconstructionFailure(t, err, output.String())
}

func TestSelectorlessCommittedCorrectionFailsClosedForOverBudgetReconstruction(t *testing.T) {
	repo := committedCorrectionWithOverBudgetReconstructionAuthority(t)

	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
	}, &output)
	assertReconstructedBudgetFailure(t, err, output.String())
}

func committedCorrectionWithOperationalReconstructionAuthority(t *testing.T) string {
	t.Helper()
	repo, _, lineage := forecastCommittedCorrection(t)
	writeCommittedCorrection(t, repo, false)

	source, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	const missingPath = "missing-reconstruction.txt"
	if err := os.WriteFile(filepath.Join(repo, missingPath), []byte("reconstruction fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startCommittedCorrectionFixture(t, repo, "operational-reconstruction", record.State.InitialSnapshot.BaseTree, []string{missingPath}, 2)
	if err := os.Remove(filepath.Join(repo, missingPath)); err != nil {
		t.Fatal(err)
	}
	return repo
}

func assertOperationalReconstructionFailure(t *testing.T, err error, output string) {
	t.Helper()
	if err == nil {
		t.Fatalf("selector-less correction selected an authority despite operational reconstruction failure: %s", output)
	}
	var gitErr *reviewtransaction.GitCommandError
	if !errors.As(err, &gitErr) {
		t.Fatalf("selector-less correction error = %T %[1]v, want operational Git command error", err)
	}
}

func assertReconstructedBudgetFailure(t *testing.T, err error, output string) {
	t.Helper()
	var budgetErr *reviewtransaction.CorrectionBudgetExceededError
	if !errors.As(err, &budgetErr) || budgetErr.Actual != 3 || budgetErr.Remaining != 2 {
		t.Fatalf("selector-less correction error = %T %[1]v, want rebuilt 3/2 budget failure", err)
	}
	if strings.Contains(output, "committed-correction") || strings.Contains(output, "healthy-reconstruction") {
		t.Fatalf("selector-less correction exposed a lineage before budget refusal: %s", output)
	}
}

func committedCorrectionWithOverBudgetReconstructionAuthority(t *testing.T) string {
	t.Helper()
	repo, base, _ := forecastCommittedCorrection(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\nvar repaired = true\nvar extraOne = 1\nvar extraTwo = 2\nvar extraThree = 3\nvar extraFour = 4\nvar extraFive = 5\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "large candidate")
	startCommittedCorrectionFixture(t, repo, "healthy-reconstruction", base, nil, 5)
	writeOverBudgetCommittedCorrection(t, repo)
	runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")
	return repo
}

func startCommittedCorrectionFixture(t *testing.T, repo, lineage, baseRef string, intendedUntracked []string, correctionLines int) {
	t.Helper()
	args := []string{"--cwd", repo, "--lineage", lineage, "--base-ref", baseRef, "--committed-only"}
	if len(intendedUntracked) > 0 {
		_, inventoryDigest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, "--untracked-scope", "select", "--expected-untracked-inventory", inventoryDigest)
		for _, path := range intendedUntracked {
			args = append(args, "--intended-untracked", path)
		}
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, args)
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, lineage, correctionLines)
}

// forecastCommittedCorrection drives a real lineage from START through a
// blocking finding to an open correction budget. Every caller continues that
// lifecycle through the native CLI, so the fixture opts in the way a real user
// does: receipt-driven development is off until someone enables it, and none of
// these transitions exist for a clone that never did.
func forecastCommittedCorrection(t *testing.T) (string, string, string) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := "frozen-base"
	runReviewCLIGit(t, repo, "branch", base, "HEAD")
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o755)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "wrong candidate")

	const lineage = "committed-correction"
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", base, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	resultPaths := make([]string, len(started.SelectedLenses))
	for index, lens := range started.SelectedLenses {
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		resultPaths[index] = filepath.Join(t.TempDir(), "reviewer-"+strconv.Itoa(index)+".json")
		writeReviewCLIJSON(t, resultPaths[index], facadeReviewerResult{
			Lens: lens, Findings: findings, Evidence: []string{"reviewed frozen committed candidate"},
		})
	}
	if err := captureReviewCLIResultFiles(t, repo, lineage, resultPaths); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, lineage, 2)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateCorrectionRequired || record.State.ProposedCorrectionLines == nil || *record.State.ProposedCorrectionLines != 2 {
		t.Fatalf("committed correction fixture = %#v, want an open two-line compact correction", record.State)
	}
	return repo, base, lineage
}

func writeCommittedCorrection(t *testing.T, repo string, amend bool) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	if amend {
		runReviewCLIGit(t, repo, "commit", "--amend", "--no-edit")
		return
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate")
}

func writeOverBudgetCommittedCorrection(t *testing.T, repo string) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\nvar repaired = true\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "over-budget correct candidate")
}

func committedCorrectionStatus(t *testing.T, repo, lineage, baseTree string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage,
		"--base-ref", baseTree, "--committed-only", "--agent", "pi",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}
