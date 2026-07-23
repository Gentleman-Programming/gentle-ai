package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestReviewEvidenceRuntimeHarnessReachesTerminalReceipt(t *testing.T) {
	repo, started, store, record, _ := capturedArtifact(t)
	finalize := []string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID, "--captured-results"}
	var first bytes.Buffer
	if err := RunReviewFacadeFinalize(finalize, &first); err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var stale bytes.Buffer
	err = RunReview(append([]string{"finalize"}, finalize...), &stale)
	if err == nil {
		t.Fatal("stale --captured-results finalize succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, stale.Bytes())
	if failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted || failure.NextAction != "review.status" || failure.LineageID != started.LineageID {
		t.Fatalf("stale captured-results failure = %#v", failure)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("stale captured-results mutated authority:\nbefore=%#v\nafter=%#v", before, after)
	}

	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	var waiting bytes.Buffer
	if err := RunReview(statusArgs, &waiting); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, waiting.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "verification_evidence_required" || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("validating status = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "evidence" || input.Schema != reviewVerificationEvidenceSchemaName || input.CaptureOperation != "review.capture-evidence" ||
		!reflect.DeepEqual(input.Arguments, []ReviewTransitionArgument{{Name: "lineage", Value: started.LineageID}, {Name: "expected-revision", Value: status.Authority.Revision}, {Name: "target", Value: reviewAuthorityTargetIdentity(status)}}) {
		t.Fatalf("verification evidence input = %#v", input)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, evidence, reviewVerificationEvidence{
		Schema:  reviewVerificationEvidenceSchemaName,
		Outcome: reviewVerificationPassed,
		Checks:  []reviewVerificationCheck{{Name: "focused tests", Status: reviewVerificationPassed, Command: "go test ./internal/cli", Evidence: []string{"ok github.com/gentleman-programming/gentle-ai/internal/cli"}}},
	})
	captureArgs := []string{"capture-evidence", "--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--expected-revision", status.Authority.Revision, "--input", evidence}
	var captured, recaptured bytes.Buffer
	if err := RunReview(captureArgs, &captured); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(captureArgs, &recaptured); err != nil {
		t.Fatal(err)
	}
	if captured.String() != recaptured.String() {
		t.Fatalf("idempotent evidence capture changed output:\n%s\n%s", captured.String(), recaptured.String())
	}
	var ready bytes.Buffer
	if err := RunReview(statusArgs, &ready); err != nil {
		t.Fatal(err)
	}
	decodeStrictReviewJSON(t, ready.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("evidence-ready status = %#v", status.NextTransition)
	}
	var terminal bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}, &terminal); err != nil {
		t.Fatal(err)
	}
	var finalized ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, terminal.Bytes()).Result, &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("captured evidence finalize state = %q, want approved", finalized.State)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.LineageID != started.LineageID || receipt.InitialReviewTree != record.State.InitialSnapshot.CandidateTree || receipt.FinalCandidateTree != record.State.CurrentSnapshot.CandidateTree || receipt.PathsDigest != record.State.InitialSnapshot.PathsDigest {
		t.Fatalf("terminal receipt lost lineage or target identity = %#v", receipt)
	}
	var replay bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}, &replay); err != nil {
		t.Fatal(err)
	}
	var replayed ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, replay.Bytes()).Result, &replayed)
	if replayed.State != reviewtransaction.StateApproved || replayed.StoreRevision != finalized.StoreRevision {
		t.Fatalf("terminal replay = %#v, original revision %q", replayed, finalized.StoreRevision)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	if err := RunReview([]string{"validate", "--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &bytes.Buffer{}); err != nil {
		t.Fatalf("pre-commit after captured evidence: %v", err)
	}
}

func TestFinalizeNextTransitionBindsCorrectedCurrentSnapshot(t *testing.T) {
	initialTarget := strings.Repeat("a", 64)
	currentTarget := strings.Repeat("b", 64)
	transition := reviewFinalizeNextTransition(reviewtransaction.CompactState{
		LineageID:       "corrected-validating-lineage",
		State:           reviewtransaction.StateValidating,
		RiskLevel:       reviewtransaction.RiskMedium,
		InitialSnapshot: reviewtransaction.Snapshot{Identity: initialTarget},
		CurrentSnapshot: reviewtransaction.Snapshot{Identity: currentTarget},
	}, strings.Repeat("c", 64), nil, nil)
	if transition.Kind != reviewNextTransitionCollect || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("corrected validating transition = %#v", transition)
	}
	arguments := transition.Collect.Inputs[0].Arguments
	if len(arguments) != 3 || arguments[2].Name != "target" || arguments[2].Value != currentTarget {
		t.Fatalf("corrected validating target arguments = %#v, want current snapshot %q", arguments, currentTarget)
	}
}

func TestNegotiatedNextTransitionDiscoversCapturedArtifactsAndAdvances(t *testing.T) {
	repo, started, _, record, _ := capturedArtifact(t)
	args := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	var first, replay bytes.Buffer
	if err := RunReview(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(args, &replay); err != nil {
		t.Fatal(err)
	}
	if first.String() != replay.String() {
		t.Fatalf("next transition changed after restart:\n%s\n%s", first.String(), replay.String())
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(first.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionExecute || transition.Execute == nil || transition.Execute.Operation != "review.finalize" ||
		len(transition.Execute.Artifacts) != len(record.State.SelectedLenses) || strings.Contains(first.String(), "reviewer-results") || strings.Contains(first.String(), repo) {
		t.Fatalf("captured result transition = %#v\n%s", transition, first.String())
	}
	var finalized bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &finalized); err != nil {
		t.Fatal(err)
	}
	result := decodeReviewOperationEnvelope(t, finalized.Bytes())
	var public ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, result.Result, &public)
	if public.NextTransition == nil || public.NextTransition.Kind != reviewNextTransitionCollect || public.NextTransition.ReasonCode != "verification_evidence_required" {
		t.Fatalf("finalize transition = %#v\n%s", public.NextTransition, finalized.String())
	}
}

func TestCorrectionNextTransitionAgreesBetweenFinalizeAndRestartStatus(t *testing.T) {
	for _, tt := range []struct {
		name, reason string
		forecast     bool
		change       bool
		kind         string
	}{
		{name: "forecast absent", reason: "correction_plan_required", kind: reviewNextTransitionCollect},
		{name: "forecast present candidate unchanged", reason: "corrected_candidate_unavailable", forecast: true, kind: reviewNextTransitionStop},
		{name: "forecast present candidate changed", reason: "targeted_validation_required", forecast: true, change: true, kind: reviewNextTransitionCollect},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			candidatePath := filepath.Join(repo, "candidate.go")
			if err := os.WriteFile(candidatePath, []byte("package candidate\n\nfunc value() int { return 1 }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			started := runNegotiatedReviewStart(t, repo, "correction-routing-"+strings.ReplaceAll(tt.name, " ", "-"))
			resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
			writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
				Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
					Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate value is wrong",
					ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
					CausalDisposition: reviewtransaction.CausalIntroduced,
				}}, Evidence: []string{"inspected exact candidate"},
			})
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if tt.forecast {
				if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "1"}, &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			if tt.change {
				if err := os.WriteFile(candidatePath, []byte("package candidate\n\nfunc value() int { return 2 }\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var directOutput bytes.Buffer
			if err := RunReviewFacadeFinalize([]string{
				"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", started.LineageID,
			}, &directOutput); err != nil {
				t.Fatalf("direct FINALIZE: %v\n%s", err, directOutput.String())
			}
			var direct ReviewIntegrationFinalizeResult
			decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, directOutput.Bytes()).Result, &direct)

			var statusOutput bytes.Buffer
			if err := RunReview([]string{
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", started.LineageID,
			}, &statusOutput); err != nil {
				t.Fatalf("restarted STATUS: %v\n%s", err, statusOutput.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
			if direct.NextTransition == nil || status.NextTransition == nil || direct.NextTransition.Kind != tt.kind ||
				direct.NextTransition.ReasonCode != tt.reason || !reflect.DeepEqual(direct.NextTransition, status.NextTransition) ||
				!reflect.DeepEqual(direct.ValidationRequest, status.ValidationRequest) {
				t.Fatalf("FINALIZE/STATUS routing mismatch:\ndirect=%#v request=%#v\nstatus=%#v request=%#v", direct.NextTransition, direct.ValidationRequest, status.NextTransition, status.ValidationRequest)
			}
		})
	}
}

func TestConsumedHistoricalCorrectionRoutesToRecoveryOrStop(t *testing.T) {
	forecast := 1
	for _, proposed := range []*int{nil, &forecast} {
		for _, changed := range []bool{false, true} {
			t.Run(fmt.Sprintf("forecasted=%t/changed=%t", proposed != nil, changed), func(t *testing.T) {
				repo, lineage, store, before := historicalConsumedCorrectionRoutingFixture(t, proposed)
				if changed {
					writeReviewStartCandidate(t, repo, "candidate.go", historicalRoutingCandidate(3), 0o644)
				}
				statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", lineage}
				var first, restarted bytes.Buffer
				if err := RunReview(statusArgs, &first); err != nil {
					t.Fatal(err)
				}
				var status ReviewTargetStatusResult
				decodeStrictReviewJSON(t, first.Bytes(), &status)
				wantAction, wantKind, wantReason := reviewtransaction.TargetStatusActionStop, reviewNextTransitionStop, "unchanged_or_unverified_authority"
				if changed {
					wantAction, wantKind, wantReason = reviewtransaction.TargetStatusActionRecover, reviewNextTransitionCollect, "recovery_authorization_required"
				}
				if status.Action != wantAction || status.ValidationRequest != nil || status.NextTransition == nil || status.NextTransition.Kind != wantKind || status.NextTransition.ReasonCode != wantReason {
					t.Fatalf("historical status = action %q request %#v transition %#v", status.Action, status.ValidationRequest, status.NextTransition)
				}
				var directOutput bytes.Buffer
				if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", lineage}, &directOutput); err != nil {
					t.Fatal(err)
				}
				var direct ReviewIntegrationFinalizeResult
				decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, directOutput.Bytes()).Result, &direct)
				if direct.ValidationRequest != nil || direct.NextTransition == nil || direct.NextTransition.Kind != reviewNextTransitionStop || direct.NextTransition.ReasonCode != "unchanged_or_unverified_authority" {
					t.Fatalf("historical direct FINALIZE = request %#v transition %#v", direct.ValidationRequest, direct.NextTransition)
				}
				if err := RunReview(statusArgs, &restarted); err != nil || restarted.String() != first.String() {
					t.Fatalf("restarted STATUS changed: %v\nfirst=%s\nrestarted=%s", err, first.String(), restarted.String())
				}
				after, _ := os.ReadFile(store.StatePath())
				if !bytes.Equal(before, after) {
					t.Fatal("routing mutated historical predecessor authority")
				}
			})
		}
	}
}

func historicalConsumedCorrectionRoutingFixture(t *testing.T, proposed *int) (string, string, reviewtransaction.CompactStore, []byte) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", historicalRoutingCandidate(1), 0o644)
	started := runNegotiatedReviewStart(t, repo, "historical-consumed-routing")
	result := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate value is wrong", ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed exact candidate"}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", result, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", historicalRoutingCandidate(2), 0o644)
	validation := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validation, facadeValidationResult{OriginalCriteria: facadeValidationCheck{Evidence: []string{"acceptance still fails"}}, CorrectionRegression: facadeValidationCheck{Evidence: []string{"regression still fails"}}, FollowUps: []reviewtransaction.FollowUp{}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--validation", validation}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	record.State.State, record.State.ProposedCorrectionLines, record.State.ActualCorrectionLines = reviewtransaction.StateCorrectionRequired, proposed, nil
	record.State.FixDeltaHash, record.State.OriginalCriteria, record.State.CorrectionRegression = reviewtransaction.EmptyFixDeltaHash, nil, nil
	if err := record.State.Validate(); err != nil {
		t.Fatal(err)
	}
	record.Revision, _ = reviewtransaction.CompactRevisionForState(record.State)
	record.Schema = "gentle-ai.review-state-record/v2"
	payload, _ := json.MarshalIndent(record, "", "  ")
	payload = append(payload, '\n')
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(store.ReceiptPath())
	_ = os.Remove(filepath.Join(store.Dir, "finalize-attempt-journal.json"))
	return repo, started.LineageID, store, payload
}

func historicalRoutingCandidate(value int) string {
	return fmt.Sprintf("package candidate\n\nfunc value() int { return %d }\nfunc spare1() int { return 0 }\nfunc spare2() int { return 0 }\nfunc spare3() int { return 0 }\n", value)
}

func TestNegotiatedRestartStatusSuppliesFrozenContextForEveryMissingReviewer(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("provider-bound candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "review-provider-task-binding")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	resumed := runNegotiatedReviewStart(t, repo, started.LineageID)
	if resumed.Action != string(reviewtransaction.CompactStartResumed) ||
		!reflect.DeepEqual(resumed.ReviewerTaskBindings, started.ReviewerTaskBindings) {
		t.Fatalf("resumed START task bindings changed:\ncreated=%#v\nresumed=%#v", started.ReviewerTaskBindings, resumed.ReviewerTaskBindings)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	legacyPayload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var legacyStatus ReviewTargetStatusResult
	if err := json.Unmarshal(legacyPayload, &legacyStatus); err != nil {
		t.Fatal(err)
	}
	for index := range legacyStatus.NextTransition.Collect.Inputs {
		legacyStatus.NextTransition.Collect.Inputs[index].ReviewerTaskBinding = ""
	}
	if err := legacyStatus.Validate(); err != nil {
		t.Fatalf("Validate() rejected additive-minor STATUS without reviewer task bindings: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != len(record.State.SelectedLenses) {
		t.Fatalf("restart transition = %#v", status.NextTransition)
	}
	wantContext, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	for order, input := range status.NextTransition.Collect.Inputs {
		if len(started.ReviewerTaskBindings) != len(record.State.SelectedLenses) ||
			input.ReviewerTaskBinding != started.ReviewerTaskBindings[order] ||
			!strings.HasPrefix(input.ReviewerTaskBinding, "GENTLE_AI_REVIEW_BINDING {") ||
			strings.Contains(input.ReviewerTaskBinding, repo) || strings.Contains(input.ReviewerTaskBinding, "tracked.txt") {
			t.Fatalf("restart reviewer binding %d = %q, START bindings %#v", order, input.ReviewerTaskBinding, started.ReviewerTaskBindings)
		}
		var binding map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(input.ReviewerTaskBinding, "GENTLE_AI_REVIEW_BINDING ")), &binding); err != nil {
			t.Fatalf("decode reviewer binding %d: %v", order, err)
		}
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"artifact_subject", "candidate_diff", "changed_path_manifest"} {
			if len(document[field]) == 0 {
				t.Fatalf("restart reviewer input %d omits %q: %s", order, field, payload)
			}
		}
		var subject reviewtransaction.ArtifactSubject
		var diff reviewtransaction.FrozenCandidateDiff
		var manifest []reviewtransaction.ChangedPathManifestEntry
		if err := json.Unmarshal(document["artifact_subject"], &subject); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(document["candidate_diff"], &diff); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(document["changed_path_manifest"], &manifest); err != nil {
			t.Fatal(err)
		}
		if subject.LineageID != record.State.LineageID || subject.AuthorityRevision != record.Revision ||
			subject.TargetIdentity != record.State.InitialSnapshot.Identity || subject.Lens != record.State.SelectedLenses[order] ||
			subject.SelectedOrder != order || subject.CandidateDiffSHA256 != wantContext.CandidateDiff.SHA256 {
			t.Fatalf("restart subject %d = %#v", order, subject)
		}
		wantBinding := map[string]any{
			"lens": subject.Lens, "lineage": subject.LineageID, "order": float64(subject.SelectedOrder),
			"repository_context": started.RepositoryContext.Handle, "revision": subject.AuthorityRevision,
			"subject_hash": subject.SubjectHash, "target": subject.TargetIdentity,
		}
		if !reflect.DeepEqual(binding, wantBinding) {
			t.Fatalf("restart reviewer binding %d = %#v, want %#v", order, binding, wantBinding)
		}
		if !reflect.DeepEqual(diff, wantContext.CandidateDiff) || !reflect.DeepEqual(manifest, wantContext.ChangedPathManifest) {
			t.Fatalf("restart context %d differs from frozen candidate\ngot diff=%#v manifest=%#v\nwant diff=%#v manifest=%#v", order, diff, manifest, wantContext.CandidateDiff, wantContext.ChangedPathManifest)
		}
	}
}

func TestReviewNextTransitionStateTable(t *testing.T) {
	status := func(applicability reviewtransaction.TargetApplicability, state reviewtransaction.State, action reviewtransaction.TargetStatusAction, replayability reviewtransaction.Replayability) ReviewTargetStatusResult {
		return ReviewTargetStatusResult{
			Applicability: applicability, Action: action, Replayability: replayability,
			TargetIdentity: "sha256:" + strings.Repeat("b", 64), Candidates: []string{"first", "second"},
			Authority:  &ReviewTargetStatusAuthority{LineageID: "review-next-transition", Revision: "sha256:" + strings.Repeat("a", 64), State: state},
			Frozen:     &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection: ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		}
	}
	all := []ReviewTransitionArtifact{{Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability, SHA256: "sha256:" + strings.Repeat("c", 64), LineageID: "review-next-transition", TargetIdentity: "sha256:" + strings.Repeat("b", 64), Lens: reviewtransaction.LensReliability, SelectedOrder: 0}}
	for _, tt := range []struct {
		name          string
		status        ReviewTargetStatusResult
		lenses        []string
		artifacts     []ReviewTransitionArtifact
		wantKind      string
		wantOperation string
	}{
		{"unreviewed workspace", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed staged", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed base ref", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed overlay", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"reviewing low partial", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, nil, reviewNextTransitionCollect, ""},
		{"reviewing medium all captured", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, all, reviewNextTransitionExecute, "review.finalize"},
		{"reviewing high partial", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, nil, reviewNextTransitionCollect, ""},
		{"correction required", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionCollect, ""},
		{"unchanged corrected authority", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionStop, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
		{"validating", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateValidating, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionCollect, ""},
		{"pending finalize journal", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionReconcileFinalize, reviewtransaction.ReplayabilityStatusRequired), nil, nil, reviewNextTransitionStop, ""},
		{"approved", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateApproved, reviewtransaction.TargetStatusActionValidate, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.validate"},
		{"invalidated", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateInvalidated, reviewtransaction.TargetStatusActionRecover, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionExecute, "review.recover"},
		{"escalated unchanged", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateEscalated, reviewtransaction.TargetStatusActionStop, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
		{"ambiguous", status(reviewtransaction.TargetApplicabilityAmbiguous, "", reviewtransaction.TargetStatusActionSelectLineage, reviewtransaction.ReplayabilityStatusRequired), nil, nil, reviewNextTransitionCollect, ""},
		{"corrupt", status(reviewtransaction.TargetApplicabilityCorrupted, "", reviewtransaction.TargetStatusActionRepairAuthority, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := reviewNextTransitionInput{}
			if tt.status.Authority != nil && tt.status.Authority.State == reviewtransaction.StateReviewing {
				input.RepositoryContext = "rctx1_" + strings.Repeat("d", 64)
				input.CaptureContext = nextTransitionTestCaptureContext(t, tt.status, tt.lenses)
			}
			if tt.status.Authority.State == reviewtransaction.StateApproved {
				tt.status.Receipt.Status = ReviewReceiptPresent
			}
			if tt.status.Action == reviewtransaction.TargetStatusActionRecover {
				input = reviewNextTransitionInput{Successor: "review-next-successor", Reason: "authorized recovery", Actor: "maintainer"}
				input.Authorization = "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + tt.status.Authority.LineageID + "\npredecessor_revision=" + tt.status.Authority.Revision + "\ntarget_identity=" + tt.status.TargetIdentity + "\nactor=" + input.Actor + "\nreason=" + input.Reason
			}
			got := newReviewNextTransition(tt.status, tt.lenses, tt.artifacts, false, nil, input)
			if got.Kind != tt.wantKind || got.Execute != nil && got.Execute.Operation != tt.wantOperation {
				t.Fatalf("next transition = %#v", got)
			}
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
			if got.Kind == reviewNextTransitionStop && (got.Execute != nil || got.Collect != nil) {
				t.Fatalf("stop exposed a command or template: %#v", got)
			}
		})
	}
}

func nextTransitionTestCaptureContext(t *testing.T, status ReviewTargetStatusResult, lenses []string) *reviewCaptureContext {
	t.Helper()
	diff, err := reviewtransaction.NewFrozenCandidateDiff([]byte("immutable candidate\n"))
	if err != nil {
		t.Fatal(err)
	}
	frozen := reviewtransaction.FrozenCandidateContext{
		CandidateDiff: diff,
		ChangedPathManifest: []reviewtransaction.ChangedPathManifestEntry{{
			Path: "tracked.txt", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644",
		}},
	}
	state := reviewtransaction.CompactState{
		LineageID: status.Authority.LineageID,
		InitialSnapshot: reviewtransaction.Snapshot{
			Identity: status.TargetIdentity, Paths: []string{"tracked.txt"},
		},
		SelectedLenses: append([]string{}, lenses...),
	}
	context, err := newReviewCaptureContext(state, status.Authority.Revision, frozen)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func TestReviewNextTransitionRefusesTargetDriftAndUnverifiableCaptures(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability: reviewtransaction.TargetApplicabilityCurrent, Action: reviewtransaction.TargetStatusActionFinalize,
		Authority:      &ReviewTargetStatusAuthority{LineageID: "target-drift", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		TargetIdentity: "sha256:" + strings.Repeat("b", 64), Frozen: &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskHigh},
	}
	got := newReviewNextTransition(status, []string{reviewtransaction.LensRisk}, nil, false, errors.New("tampered capture"), reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionStop || got.ReasonCode != "captured_artifacts_unverifiable" || got.Execute != nil || got.Collect != nil {
		t.Fatalf("target drift transition = %#v", got)
	}
}
