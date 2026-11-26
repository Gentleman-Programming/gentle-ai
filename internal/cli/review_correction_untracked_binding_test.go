package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestWorkspaceCorrectionKeepsFrozenEmptyUntrackedExclusion(t *testing.T) {
	testWorkspaceCorrectionKeepsFrozenEmptyUntrackedExclusion(t, reviewtransaction.TargetCurrentChanges)
}

func TestWorkspaceOverlayCorrectionKeepsFrozenEmptyUntrackedExclusion(t *testing.T) {
	testWorkspaceCorrectionKeepsFrozenEmptyUntrackedExclusion(t, reviewtransaction.TargetBaseWorkspaceOverlay)
}

func testWorkspaceCorrectionKeepsFrozenEmptyUntrackedExclusion(t *testing.T, kind reviewtransaction.TargetKind) {
	t.Helper()
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	const lineage = "frozen-empty-untracked-6225"
	const candidate = "candidate.go"
	const excluded = "local-only.txt"
	baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	bound := []string{"--agent=pi", "--lineage=" + lineage}
	if kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		baseCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
		bound = append(bound, "--base-ref="+baseCommit, "--workspace-overlay")
		// Make the overlay base differ from HEAD so falling back to current
		// changes could not return the same projection.
		writeReviewStartCandidate(t, repo, candidate, "package candidate\n\nfunc value() int { return 0 }\n", 0o644)
		runReviewCLIGit(t, repo, "commit", "-qm", "advance past the overlay base")
	}
	writeReviewStartCandidate(t, repo, candidate, "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, excluded, "unrelated workspace file\n", 0o644)

	run := func(args []string) []byte {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("review %q: %v\n%s", args, err, output.String())
		}
		return output.Bytes()
	}
	submit := func(input ReviewTransitionInput, value string) []byte {
		t.Helper()
		if input.Submission == nil || input.Submission.Value == nil {
			t.Fatalf("no single-value submission for %s", input.Name)
		}
		submission := input.Submission
		args := append([]string{submission.OperationToken, "--cwd=" + repo}, submission.ArgumentTokens...)
		index := 2 + submission.Value.SubstitutionLocation
		args[index] = strings.ReplaceAll(args[index], reviewSubmissionValuePlaceholder, value)
		return run(args)
	}
	encode := func(value any) []byte {
		t.Helper()
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	digest, paths := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo, bound...))
	if len(paths) != 1 || paths[0] != excluded {
		t.Fatalf("eligible untracked paths = %q, want only %q", paths, excluded)
	}
	exclusion := []string{"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest}
	ready := intendedUntrackedStatus(t, repo, append(bound, exclusion...)...)
	if ready.NextTransition == nil || ready.NextTransition.Execute == nil || ready.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("explicit exclusion did not yield START: %#v", ready.NextTransition)
	}
	startArgs := invocationArgs(t, ready.NextTransition.Execute.Command)
	started := decodeNegotiatedReviewStart(t, []byte(runReviewStartFollowingConsent(t, startArgs[1:])))
	manifest := startedChangedPathManifest(t, started)
	frozenTarget := ready.TargetIdentity
	if started.LineageID != lineage || started.Projection != reviewtransaction.ProjectionWorkspace ||
		ready.Projection.Kind != kind || started.BaseTree != baseTree || started.State != reviewtransaction.StateReviewing ||
		started.RepositoryContext == nil || started.RepositoryContext.TargetIdentity != frozenTarget ||
		len(manifest) != 1 || manifest[0].Path != candidate || manifest[0].IntendedUntracked {
		t.Fatalf("START did not freeze the tracked-only %s candidate with base %q: %#v", kind, baseTree, started)
	}

	collect := func(status ReviewTargetStatusResult, reason string) []ReviewTransitionInput {
		t.Helper()
		transition := status.NextTransition
		if transition == nil || transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != reason || transition.Collect == nil || len(transition.Collect.Inputs) == 0 {
			t.Fatalf("STATUS transition = %#v, want collect/%s", transition, reason)
		}
		return transition.Collect.Inputs
	}
	reviewing := intendedUntrackedStatus(t, repo, bound...)
	for i, input := range collect(reviewing, "reviewer_results_required") {
		findings := []facadeFinding{}
		if i == 0 {
			findings = append(findings, facadeFinding{
				ID: "R3-001", Location: candidate + ":3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{candidate + ":3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			})
		}
		if input.ArtifactSubject == nil {
			t.Fatal("reviewer slot omitted the frozen subject")
		}
		result := facadeReviewerResult{
			SubjectHash: input.ArtifactSubject.SubjectHash,
			Inspection:  reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: []string{candidate}},
			Findings:    findings, Evidence: []string{"inspected the frozen candidate"},
		}
		submit(input, writeReviewCLIRawInput(t, encode(result)))
	}
	refuter := collect(intendedUntrackedStatus(t, repo, bound...), "provider_refuter_required")[0]
	prompt := run(append([]string{"capture-refuter"}, reviewTransitionInputTokens(t, repo, refuter)...))
	refutation := facadeRefuterResult{
		RequestHash: reviewProviderRequestHashForTest(t, prompt),
		Results: []facadeRefuterOutcome{{
			FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"},
		}},
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, submit(refuter, writeReviewCLIRawInput(t, encode(refutation))), &closure)
	continuation := closure.StatusContinuation
	if closure.LineageID != lineage || closure.State != reviewtransaction.StateCorrectionRequired ||
		continuation == nil || continuation.Operation != "review.status" || continuation.Binding.TargetIdentity != frozenTarget {
		t.Fatalf("expected correction_required with a bound STATUS continuation: %#v", closure)
	}
	if kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		args, err := reviewTransitionArgumentMap(continuation.Arguments)
		if err != nil || args["base-ref"] != baseTree || args["workspace-overlay"] != "true" {
			t.Fatalf("correction continuation lost the overlay selector for base %q: %#v (%v)", baseTree, continuation.Arguments, err)
		}
	}
	statusArgs := []string{strings.TrimPrefix(continuation.Operation, "review.")}
	for _, argument := range continuation.Arguments {
		statusArgs = append(statusArgs, argument.Token)
	}
	boundStatus := func(phase string) ReviewTargetStatusResult {
		t.Helper()
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, run(statusArgs), &status)
		if err := status.Validate(); err != nil {
			t.Fatal(err)
		}
		if status.Projection.Kind != kind || status.Projection.Projection != reviewtransaction.ProjectionWorkspace || status.Projection.BaseTree != baseTree {
			t.Fatalf("%s: bound STATUS changed the %s workspace projection with base %q: %#v", phase, kind, baseTree, status.Projection)
		}
		if status.NextTransition != nil {
			t.Logf("%s: bound STATUS = %s/%s, authority=%+v", phase, status.NextTransition.Kind, status.NextTransition.ReasonCode, status.Authority)
		}
		return status
	}
	planStatus := boundStatus("before correction")
	if planStatus.Authority == nil || planStatus.Authority.LineageID != lineage ||
		planStatus.Authority.State != reviewtransaction.StateCorrectionRequired || planStatus.TargetIdentity != frozenTarget ||
		planStatus.Authority.Revision != continuation.Binding.Revision {
		t.Fatalf("correction-plan STATUS lost the frozen binding: authority=%+v, target=%q", planStatus.Authority, planStatus.TargetIdentity)
	}
	plan := collect(planStatus, "correction_plan_required")[0]
	planArgs := append(reviewTransitionInputTokens(t, repo, plan), "--correction-lines=2")
	var forecast reviewCorrectionPlanCaptureResult
	decodeStrictReviewJSON(t, run(append([]string{"capture-correction-plan"}, planArgs...)), &forecast)
	if forecast.LineageID != lineage || forecast.TargetIdentity != frozenTarget || forecast.CorrectionLines != 2 {
		t.Fatalf("accepted correction plan lost the frozen binding: %#v", forecast)
	}
	writeReviewStartCandidate(t, repo, candidate, "package candidate\n\nfunc value() int { return 2 }\n", 0o644)

	corrected := boundStatus("after correction")
	if corrected.Authority == nil || corrected.Authority.LineageID != lineage ||
		corrected.Authority.State != reviewtransaction.StateCorrectionRequired || corrected.Authority.Revision != forecast.StoreRevision ||
		corrected.AuthorityTargetIdentity != frozenTarget || len(corrected.Projection.IntendedUntracked) != 0 ||
		len(corrected.Projection.Paths) != 1 || corrected.Projection.Paths[0] != candidate {
		t.Fatalf("corrected STATUS lost the frozen binding or scope: authority=%+v, authority target=%q, projection=%+v",
			corrected.Authority, corrected.AuthorityTargetIdentity, corrected.Projection)
	}
	if corrected.EligibleUntrackedInventory != digest {
		t.Fatalf("bound STATUS live inventory = %q, want %q", corrected.EligibleUntrackedInventory, digest)
	}
	validation := collect(corrected, "targeted_validation_required")[0]
	args, err := reviewTransitionArgumentMap(validation.Arguments)
	if err != nil || validation.CaptureOperation != reviewCaptureValidationCaptureOperation || validation.ValidationRequest == nil ||
		args["lineage"] != lineage || args["target"] != validation.ValidationRequest.CorrectionTargetIdentity || args["target"] == frozenTarget {
		t.Fatalf("corrected STATUS validation input = %#v (%v), want the corrected candidate bound to %q", validation, err, lineage)
	}
}

// TestNegotiatedStatusPreservesUntrackedBindingThroughCorrectionLineage pins
// issue #3849: a correction_required lineage whose frozen candidate carries
// an explicit intended-untracked selection must keep offering its own
// correction transition, bound to the authority's own target identity, on a
// plain STATUS re-entry that names no untracked-scope flags -- exactly the
// shape of the exact bound continuation `review.capture-correction-plan`
// itself returns. It must not re-demand a fresh intended-untracked
// declaration and bind the resulting collect input to a different (live,
// declaration-less) target identity than the one the authority is bound to.
func TestNegotiatedStatusPreservesUntrackedBindingThroughCorrectionLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "correction-untracked-binding-3849"
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "notes.txt", "untracked but explicitly selected\n", 0o644)

	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest, "--intended-untracked", "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("started lenses = %v, want exactly one selected lens", started.SelectedLenses)
	}

	captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
		Location: "candidate.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
		ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &bytes.Buffer{})

	captureCorrectionPlanFromCurrentStatus(t, repo, lineage, 1)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.InitialSnapshot.IntendedUntracked) != 1 || record.State.InitialSnapshot.IntendedUntracked[0] != "notes.txt" {
		t.Fatalf("fixture did not freeze the declared untracked selection: %#v", record.State.InitialSnapshot)
	}

	// The exact bound STATUS continuation `review.capture-correction-plan`
	// returns carries no untracked-scope flags: it is a plain
	// `--next-transition --lineage` re-entry, exactly like the operation
	// shape issue #3849 reports.
	status := negotiatedReviewStatusForLineage(t, repo, lineage)
	if status.NextTransition == nil {
		t.Fatal("correction lineage STATUS produced no next transition")
	}
	if status.NextTransition.Kind == reviewNextTransitionCollect && status.NextTransition.ReasonCode == "intended_untracked_selection_required" {
		t.Fatalf("correction lineage dead-ended into a fresh intended-untracked declaration: %#v", status.NextTransition)
	}
	if status.NextTransition.ReasonCode != "corrected_candidate_unavailable" {
		t.Fatalf("next transition reason = %q, want corrected_candidate_unavailable", status.NextTransition.ReasonCode)
	}
	if status.TargetIdentity != record.State.CurrentSnapshot.Identity {
		t.Fatalf("status target identity = %q, want the authority's own bound target identity %q", status.TargetIdentity, record.State.CurrentSnapshot.Identity)
	}
	if status.NextTransition.Collect != nil {
		for _, input := range status.NextTransition.Collect.Inputs {
			for _, argument := range input.Arguments {
				if argument.Name == "target_identity" && argument.Value != record.State.CurrentSnapshot.Identity {
					t.Fatalf("collect input target_identity = %q, want authority target identity %q", argument.Value, record.State.CurrentSnapshot.Identity)
				}
			}
		}
	}
}

// TestSelectorlessStatusPreservesIntendedUntrackedBindingBeforeCorrectionPlan
// reproduces #4435's first dead end: after a selected-untracked review enters
// correction_required, the provider-published selectorless STATUS continuation
// must retain the frozen selection rather than demanding a new declaration.
func TestSelectorlessStatusPreservesIntendedUntrackedBindingBeforeCorrectionPlan(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "correction-untracked-selectorless-status-4435"
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "notes.txt", "untracked but explicitly selected\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "local-only.txt", "eligible but deliberately unselected\n", 0o644)

	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest, "--intended-untracked", "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("started lenses = %v, want exactly one selected lens", started.SelectedLenses)
	}

	var closureOutput bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
		Location: "candidate.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
		ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &closureOutput)

	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, closureOutput.Bytes(), &closure)
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-result" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("selectorless final capture closure = %#v, want correction_required closure", closure)
	}
	if closure.StatusContinuation == nil {
		t.Fatalf("selectorless final capture closure lacks status_continuation: %s", closureOutput.String())
	}
	continuation := closure.StatusContinuation
	if continuation.Operation != "review.status" {
		t.Fatalf("selectorless status continuation operation = %q, want review.status", continuation.Operation)
	}
	for _, argument := range continuation.Arguments {
		for _, forbidden := range []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"} {
			if argument.Token == forbidden || strings.HasPrefix(argument.Token, forbidden+"=") {
				t.Fatalf("selectorless status continuation leaks %q: %#v", forbidden, continuation.Arguments)
			}
		}
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Replay only the operation and opaque tokens the final reviewer capture
	// published; reconstructing STATUS would hide a malformed continuation.
	statusArgs := []string{strings.TrimPrefix(continuation.Operation, "review.")}
	for _, argument := range continuation.Arguments {
		statusArgs = append(statusArgs, argument.Token)
	}
	var statusOutput bytes.Buffer
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatalf("run selectorless status continuation unchanged: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("selectorless STATUS authority = %#v, want correction_required authority for lineage %q", status.Authority, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.CorrectionRequest == nil {
		t.Fatalf("selectorless STATUS transition = %#v, want collect/correction_plan_required", status.NextTransition)
	}

	request := status.NextTransition.CorrectionRequest
	if status.TargetIdentity != record.State.CurrentSnapshot.Identity || request.TargetIdentity != record.State.CurrentSnapshot.Identity {
		t.Fatalf("selectorless STATUS target bindings = status %q, correction request %q, want %q", status.TargetIdentity, request.TargetIdentity, record.State.CurrentSnapshot.Identity)
	}
	if status.Authority.Revision != record.Revision {
		t.Fatalf("selectorless STATUS authority revision = %q, want %q", status.Authority.Revision, record.Revision)
	}
	if request.ExpectedRevision != record.State.CapturePhaseRevision {
		t.Fatalf("correction request capture-phase revision = %q, want %q", request.ExpectedRevision, record.State.CapturePhaseRevision)
	}
	if request.LineageID != lineage {
		t.Fatalf("correction request lineage = %q, want %q", request.LineageID, lineage)
	}

	// Losing one frozen selected path is real scope drift. Extra unselected paths
	// may remain outside the candidate, but they must not stand in for the missing
	// selected path when the same continuation is re-entered.
	if err := os.Remove(filepath.Join(repo, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	statusOutput.Reset()
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatalf("run continuation after selected path removal: %v\n%s", err, statusOutput.String())
	}
	var missingSelected ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &missingSelected)
	if missingSelected.NextTransition == nil || missingSelected.NextTransition.Kind != reviewNextTransitionCollect ||
		missingSelected.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("missing selected path transition = %#v, want intended_untracked_selection_required", missingSelected.NextTransition)
	}
}
