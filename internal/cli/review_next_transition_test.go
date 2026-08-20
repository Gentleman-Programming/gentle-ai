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
	"regexp"
	"slices"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestValidatingEvidenceCollectionUnblocksFinalizeAndPreCommit(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record, _ := capturedArtifact(t)
	finalize := []string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID, "--captured-results"}
	var first bytes.Buffer
	if err := RunReviewFacadeFinalize(finalize, &first); err != nil {
		t.Fatal(err)
	}
	var repeated bytes.Buffer
	if err := RunReviewFacadeFinalize(finalize, &repeated); err != nil {
		t.Fatal(err)
	}
	var repeatedResult ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, repeated.Bytes()).Result, &repeatedResult)
	if repeatedResult.State != reviewtransaction.StateValidating || repeatedResult.NextTransition == nil || repeatedResult.NextTransition.Kind != reviewNextTransitionCollect || repeatedResult.NextTransition.ReasonCode != "verification_evidence_required" {
		t.Fatalf("repeated finalize made no-progress recommendation = %#v", repeatedResult)
	}

	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	var waiting bytes.Buffer
	if err := RunReview(statusArgs, &waiting); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, waiting.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != "review.capture-evidence" {
		t.Fatalf("validating status = %#v", status.NextTransition)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"capture-evidence", "--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--expected-revision", status.Authority.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidence}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
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
	if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID, "--captured-evidence"}, &terminal); err != nil {
		t.Fatal(err)
	}
	var finalized ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, terminal.Bytes()).Result, &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("captured evidence finalize state = %q, want approved", finalized.State)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	if err := RunReview([]string{"validate", "--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &bytes.Buffer{}); err != nil {
		t.Fatalf("pre-commit after captured evidence: %v", err)
	}
}

func TestFinalizeNextTransitionBindsCorrectedCurrentSnapshot(t *testing.T) {
	t.Parallel()

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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
	for _, tt := range []struct {
		name, reason          string
		forecast              bool
		change                bool
		capturePassedEvidence bool
		kind                  string
	}{
		{name: "forecast absent", reason: "correction_plan_required", kind: reviewNextTransitionCollect},
		{name: "forecast present candidate unchanged", reason: "corrected_candidate_unavailable", forecast: true, kind: reviewNextTransitionStop},
		{name: "forecast present candidate changed", reason: "correction_repository_verification_required", forecast: true, change: true, kind: reviewNextTransitionCollect},
		{name: "forecast present candidate changed evidence passed", reason: "targeted_validation_required", forecast: true, change: true, capturePassedEvidence: true, kind: reviewNextTransitionCollect},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			candidatePath := filepath.Join(repo, "candidate.go")
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
			started := runNegotiatedReviewStart(t, repo, "correction-routing-"+strings.ReplaceAll(tt.name, " ", "-"))
			resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
			writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
				Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
					Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate value is wrong",
					ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
					CausalDisposition: reviewtransaction.CausalIntroduced,
				}}, Evidence: []string{"inspected exact candidate"},
			})
			if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, &bytes.Buffer{}); err != nil {
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
			if tt.capturePassedEvidence {
				capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
			}
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
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
			directTransition, _ := json.Marshal(direct.NextTransition)
			statusTransition, _ := json.Marshal(status.NextTransition)
			directRequest, _ := json.Marshal(direct.ValidationRequest)
			statusRequest, _ := json.Marshal(status.ValidationRequest)
			if direct.NextTransition == nil || status.NextTransition == nil || direct.NextTransition.Kind != tt.kind ||
				direct.NextTransition.ReasonCode != tt.reason || !bytes.Equal(directTransition, statusTransition) ||
				!bytes.Equal(directRequest, statusRequest) {
				t.Fatalf("FINALIZE/STATUS routing mismatch:\ndirect=%s request=%#v\nstatus=%s request=%#v", directTransition, direct.ValidationRequest, statusTransition, status.ValidationRequest)
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("read-only FINALIZE/STATUS routing mutated authority: %v", err)
			}
		})
	}
}

func TestConsumedHistoricalCorrectionRoutesToRecoveryOrStop(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

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
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", historicalRoutingCandidate(2), 0o644)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	request := capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
	validation := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validation, facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--validation", validation, "--captured-evidence"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load()
	record.State.State, record.State.ProposedCorrectionLines, record.State.ActualCorrectionLines = reviewtransaction.StateCorrectionRequired, proposed, nil
	record.State.FixDeltaHash, record.State.OriginalCriteria, record.State.CorrectionRegression = reviewtransaction.EmptyFixDeltaHash, nil, nil
	record.State.EvidenceHash, record.State.EvidenceRecordDigest = "", ""
	record.State.EvidenceOutcome, record.State.EvidenceTargetIdentity, record.State.EvidenceAuthorityRevision = "", "", ""
	record.State.CorrectionVerificationTarget = nil
	lastAttempt := len(record.State.CorrectionAttempts) - 1
	record.State.CorrectionAttempts[lastAttempt].OriginalCriteria.Passed = false
	record.State.CorrectionAttempts[lastAttempt].CorrectionRegression.Passed = false
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
	repo, started, _, record := newArtifactReview(t, true)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != len(record.State.SelectedLenses) {
		t.Fatalf("restart transition = %#v", status.NextTransition)
	}
	wantContext, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	for order, input := range status.NextTransition.Collect.Inputs {
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"artifact_subject", "base_tree", "candidate_tree", "changed_path_manifest"} {
			if len(document[field]) == 0 {
				t.Fatalf("restart reviewer input %d omits %q: %s", order, field, payload)
			}
		}
		var subject reviewtransaction.ArtifactSubject
		var manifest []reviewtransaction.ChangedPathManifestEntry
		if err := json.Unmarshal(document["artifact_subject"], &subject); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(document["changed_path_manifest"], &manifest); err != nil {
			t.Fatal(err)
		}
		if subject.LineageID != record.State.LineageID || subject.AuthorityRevision != record.Revision ||
			subject.TargetIdentity != record.State.InitialSnapshot.Identity || subject.Lens != record.State.SelectedLenses[order] ||
			subject.SelectedOrder != order || subject.BaseTree != wantContext.BaseTree || subject.CandidateTree != wantContext.CandidateTree {
			t.Fatalf("restart subject %d = %#v", order, subject)
		}
		if input.BaseTree != wantContext.BaseTree || input.CandidateTree != wantContext.CandidateTree || !reflect.DeepEqual(manifest, wantContext.ChangedPathManifest) {
			t.Fatalf("restart context %d differs from frozen candidate\ngot trees=%s..%s manifest=%#v\nwant trees=%s..%s manifest=%#v", order, input.BaseTree, input.CandidateTree, manifest, wantContext.BaseTree, wantContext.CandidateTree, wantContext.ChangedPathManifest)
		}
	}
}

func TestReviewNextTransitionStateTable(t *testing.T) {
	t.Parallel()

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
		{"correction required without provider request", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionStop, ""},
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
			got := newReviewNextTransition(tt.status, tt.lenses, tt.artifacts, nil, nil, input)
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

// TestReviewTransitionArgumentToken is the RED-first proof for 1745: every
// emitted Execute.Arguments entry must carry the exact, literally executable
// argv token, and Preconditions (assertions, not argv) must never carry one.
func TestReviewTransitionArgumentToken(t *testing.T) {
	t.Parallel()

	status := func(applicability reviewtransaction.TargetApplicability, state reviewtransaction.State, action reviewtransaction.TargetStatusAction, replayability reviewtransaction.Replayability) ReviewTargetStatusResult {
		return ReviewTargetStatusResult{
			Applicability: applicability, Action: action, Replayability: replayability,
			TargetIdentity: "sha256:" + strings.Repeat("b", 64), Candidates: []string{"first", "second"},
			Authority:  &ReviewTargetStatusAuthority{LineageID: "review-token", Revision: "sha256:" + strings.Repeat("a", 64), State: state},
			Frozen:     &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection: ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		}
	}
	all := []ReviewTransitionArtifact{{Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability, SHA256: "sha256:" + strings.Repeat("c", 64), LineageID: "review-token", TargetIdentity: "sha256:" + strings.Repeat("b", 64), Lens: reviewtransaction.LensReliability, SelectedOrder: 0}}
	for _, tt := range []struct {
		name       string
		status     ReviewTargetStatusResult
		lenses     []string
		artifacts  []ReviewTransitionArtifact
		input      reviewNextTransitionInput
		wantTokens map[string]string
	}{
		{
			name:   "captured results uses the real hyphenated flag",
			status: status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, reviewtransaction.TargetStatusActionFinalize, reviewtransaction.ReplayabilityNotReplayable),
			lenses: []string{reviewtransaction.LensReliability}, artifacts: all,
			input:      reviewNextTransitionInput{RepositoryContext: "rctx1_" + strings.Repeat("d", 64)},
			wantTokens: map[string]string{"lineage": "--lineage=review-token", "captured_results": "--captured-results=true"},
		},
		{
			name:       "approved receipt gate token",
			status:     status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateApproved, reviewtransaction.TargetStatusActionValidate, reviewtransaction.ReplayabilityNotReplayable),
			wantTokens: map[string]string{"lineage": "--lineage=review-token", "gate": "--gate=pre-commit"},
		},
		{
			name:   "recovery authorized token",
			status: status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateInvalidated, reviewtransaction.TargetStatusActionRecover, reviewtransaction.ReplayabilityManualActionRequired),
			input: reviewNextTransitionInput{
				Successor: "review-token-successor", Reason: "authorized recovery", Actor: "maintainer",
				Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-token\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=sha256:" + strings.Repeat("b", 64) + "\nactor=maintainer\nreason=authorized recovery",
			},
			wantTokens: map[string]string{"predecessor-lineage": "--predecessor-lineage=review-token", "successor-lineage": "--successor-lineage=review-token-successor"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status.Authority.State == reviewtransaction.StateApproved {
				tt.status.Receipt.Status = ReviewReceiptPresent
			}
			if tt.status.Authority.State == reviewtransaction.StateReviewing {
				tt.input.CaptureContext = nextTransitionTestCaptureContext(t, tt.status, tt.lenses)
			}
			got := newReviewNextTransition(tt.status, tt.lenses, tt.artifacts, nil, nil, tt.input)
			if got.Kind != reviewNextTransitionExecute || got.Execute == nil {
				t.Fatalf("next transition = %#v, want an execute transition", got)
			}
			seen := map[string]bool{}
			for _, argument := range got.Execute.Arguments {
				want, checked := tt.wantTokens[argument.Name]
				if checked {
					if argument.Token != want {
						t.Fatalf("argument %q token = %q, want %q", argument.Name, argument.Token, want)
					}
					seen[argument.Name] = true
				}
				if strings.TrimSpace(argument.Token) == "" {
					t.Fatalf("execute argument %q carries no literal argv token", argument.Name)
				}
			}
			for name := range tt.wantTokens {
				if !seen[name] {
					t.Fatalf("expected argument %q was not present in Execute.Arguments", name)
				}
			}
			for _, precondition := range got.Execute.Preconditions {
				if precondition.Token != "" {
					t.Fatalf("precondition %q carries a Token; preconditions are assertions, not argv", precondition.Name)
				}
			}
		})
	}
}

// TestNewReviewNextTransitionEscalatedRouting is the RED-first proof for
// 1800 (StateEscalated used to dead-end with Stop("escalated_authority")
// unconditionally, unlike StateInvalidated which routes to recovery) plus the
// organic-dx stop-invariant sweep's follow-up fix for the "third case" 1800
// left unsoftened: native STATUS (target_status.go:176-199) only ever sets
// Action == TargetStatusActionRecover for StateEscalated when either the
// target changed OR the authority is an accounting-only escalation eligible
// for RecoverCompactAuthority's evidence-derived edge (issue found while
// investigating "escalated_recovery_requires_changed_target" as a Phase 3
// SUSPECT stop code) — TargetStatusActionStop is the only other outcome, and
// it is routed away to native_stop_required before this switch is ever
// reached (see the TargetStatusActionStop branch above). So this switch case
// can trust status.Action unconditionally, exactly like every other case:
// StateEscalated always routes to reviewRecoveryCollection with the
// disposition forced to RecoveryEscalated, regardless of whether the target
// changed. When a Selector is present and the target has not changed, the
// generic recovery_scope_unchanged guard already inside reviewRecoveryCollection
// still applies — that guard is unrelated to StateEscalated and is not
// softened by this fix.
func TestNewReviewNextTransitionEscalatedRouting(t *testing.T) {
	t.Parallel()

	baseStatus := func(target, authorityTarget string) ReviewTargetStatusResult {
		return ReviewTargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent, Action: reviewtransaction.TargetStatusActionRecover,
			Replayability:           reviewtransaction.ReplayabilityManualActionRequired,
			TargetIdentity:          target,
			AuthorityTargetIdentity: authorityTarget,
			Authority:               &ReviewTargetStatusAuthority{LineageID: "review-escalated", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateEscalated},
			Frozen:                  &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection:              ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		}
	}
	unchangedTarget := "sha256:" + strings.Repeat("b", 64)
	changedTarget := "sha256:" + strings.Repeat("e", 64)

	t.Run("changed target routes to recovery with disposition forced to escalated", func(t *testing.T) {
		status := baseStatus(changedTarget, unchangedTarget)
		input := reviewNextTransitionInput{
			Successor: "review-escalated-successor", Reason: "authorized recovery", Actor: "maintainer",
			Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-escalated\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=" + changedTarget + "\nactor=maintainer\nreason=authorized recovery",
		}
		got := newReviewNextTransition(status, nil, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.recover" {
			t.Fatalf("escalated changed-target transition = %#v, want an execute review.recover transition", got)
		}
		arguments, err := reviewTransitionArgumentMap(got.Execute.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		if arguments["disposition"] != string(reviewtransaction.RecoveryEscalated) {
			t.Fatalf("escalated recovery disposition = %q, want %q", arguments["disposition"], reviewtransaction.RecoveryEscalated)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unchanged target without a selector still routes to recovery (accounting-only edge)", func(t *testing.T) {
		status := baseStatus(unchangedTarget, unchangedTarget)
		input := reviewNextTransitionInput{
			Successor: "review-escalated-successor", Reason: "authorized recovery", Actor: "maintainer",
			Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-escalated\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=" + unchangedTarget + "\nactor=maintainer\nreason=authorized recovery",
		}
		got := newReviewNextTransition(status, nil, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.recover" {
			t.Fatalf("escalated unchanged-target transition (no selector) = %#v, want an execute review.recover transition — status.Action already vetted this as legal (accounting-only escalation), so this switch must not re-derive a target-changed requirement", got)
		}
		arguments, err := reviewTransitionArgumentMap(got.Execute.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		if arguments["disposition"] != string(reviewtransaction.RecoveryEscalated) {
			t.Fatalf("escalated recovery disposition = %q, want %q", arguments["disposition"], reviewtransaction.RecoveryEscalated)
		}
	})

	t.Run("unchanged target with a selector still stops via the generic recovery_scope_unchanged guard", func(t *testing.T) {
		status := baseStatus(unchangedTarget, unchangedTarget)
		input := reviewNextTransitionInput{Selector: &reviewTransitionSelector{Recovery: &reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges}}}
		got := newReviewNextTransition(status, nil, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionStop || got.Execute != nil || got.Collect != nil {
			t.Fatalf("escalated unchanged-target transition (selector) = %#v, want a bare stop", got)
		}
		if got.ReasonCode != "recovery_scope_unchanged" {
			t.Fatalf("escalated unchanged-target reason (selector) = %q, want the generic reviewRecoveryCollection guard reason %q", got.ReasonCode, "recovery_scope_unchanged")
		}
	})
}

// TestReviewNextTransitionExecuteArgumentValidatesAgainstPublishedSchema is
// the RED-first proof for the 1745 follow-up: the "token" field 1745 added to
// ReviewTransitionArgument for execution arguments must be admissible under
// the published, byte-pinned review-integration/v1 status.schema.json
// contract. Before the schema fix, $defs/transition_argument declared
// "additionalProperties": false with only name/value, so this real
// "--captured-results=true" execute payload was schema-illegal even though
// the CLI has emitted it since 1745.
func TestReviewNextTransitionExecuteArgumentValidatesAgainstPublishedSchema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Action:         reviewtransaction.TargetStatusActionFinalize,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	got := newReviewNextTransition(status, nil, nil, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.finalize" {
		t.Fatalf("next transition = %#v, want an execute review.finalize transition", got)
	}
	found := false
	for _, argument := range got.Execute.Arguments {
		if argument.Name == "captured_results" {
			found = true
			if argument.Token != "--captured-results=true" {
				t.Fatalf("captured_results token = %q, want --captured-results=true", argument.Token)
			}
		}
	}
	if !found {
		t.Fatal("captured_results argument missing from execute transition")
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchema(t, payload)
}

// TestReviewNextTransitionExecuteArtifactsValidateAgainstPublishedSchema is
// the RED-first proof that a real "captured results" execute transition
// carrying artifacts is admissible under the published, byte-pinned
// review-integration/v1 status.schema.json contract. discoverCapturedReviewer
// Artifacts (internal/cli/review_artifact.go) always populates SubjectHash and
// AdmissionDecision on every ReviewTransitionArtifact it returns (no
// omitempty on either field), but $defs/transition_artifact never declared
// either property, so every execute transition carrying artifacts was
// schema-illegal under "additionalProperties": false.
func TestReviewNextTransitionExecuteArtifactsValidateAgainstPublishedSchema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Action:         reviewtransaction.TargetStatusActionFinalize,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-artifact-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	artifacts := []ReviewTransitionArtifact{{
		Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability,
		SHA256: "sha256:" + strings.Repeat("e", 64), LineageID: "review-artifact-schema",
		TargetIdentity: status.TargetIdentity, Lens: reviewtransaction.LensReliability, SelectedOrder: 0,
		SubjectHash: "sha256:" + strings.Repeat("f", 64), AdmissionDecision: reviewtransaction.ArtifactAdmissionCompleted,
	}}
	got := newReviewNextTransition(status, []string{reviewtransaction.LensReliability}, artifacts, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.finalize" || len(got.Execute.Artifacts) != 1 {
		t.Fatalf("next transition = %#v, want an execute review.finalize transition carrying one artifact", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchema(t, payload)
}

// TestReviewNextTransitionExecuteSelectorArgumentsValidateAgainstPublishedSchema
// is the RED-first proof that a real pre-PR base-ref VALIDATE execute
// transition carrying SelectorArguments is admissible under the published,
// byte-pinned review-integration/v1 status.schema.json contract.
// $defs/transition_execution never declared "selector_arguments" at all
// (unlike status-v2.schema.json, which already does), so any populated
// selector_arguments was rejected outright by "additionalProperties": false.
func TestReviewNextTransitionExecuteSelectorArgumentsValidateAgainstPublishedSchema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Action:         reviewtransaction.TargetStatusActionValidate,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-selector-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateApproved},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Receipt:        ReviewTargetStatusReceipt{Status: ReviewReceiptPresent},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, Kind: reviewtransaction.TargetBaseDiff, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	input := reviewNextTransitionInput{
		Gate:     reviewtransaction.GatePrePR,
		Selector: &reviewTransitionSelector{Kind: reviewtransaction.TargetBaseDiff, BaseRef: "main", PrePRRepresentable: true},
	}
	got := newReviewNextTransition(status, nil, nil, nil, nil, input)
	if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.SelectorArguments == nil {
		t.Fatalf("next transition = %#v, want an execute transition carrying selector arguments", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchema(t, payload)
}

// TestReviewNextTransitionExecuteArgumentValidatesAgainstPublishedV2Schema is
// the v2 sibling of TestReviewNextTransitionExecuteArgumentValidatesAgainst
// PublishedSchema: the same "token" field is emitted on every Execute
// .Arguments entry regardless of which status schema version renders it, but
// status-v2.schema.json's own $defs/transition_argument had the identical
// "additionalProperties": false gap the v1 file just had fixed.
func TestReviewNextTransitionExecuteArgumentValidatesAgainstPublishedV2Schema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Action:         reviewtransaction.TargetStatusActionFinalize,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-schema-v2", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	got := newReviewNextTransition(status, nil, nil, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.finalize" {
		t.Fatalf("next transition = %#v, want an execute review.finalize transition", got)
	}
	found := false
	for _, argument := range got.Execute.Arguments {
		if argument.Name == "captured_results" {
			found = true
			if argument.Token != "--captured-results=true" {
				t.Fatalf("captured_results token = %q, want --captured-results=true", argument.Token)
			}
		}
	}
	if !found {
		t.Fatal("captured_results argument missing from execute transition")
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV2(t, payload)
}

// validateAgainstPublishedNextTransitionSchema validates payload against the
// live $defs/next_transition subtree read straight out of the shipped
// contracts/review-integration/v1/schemas/status.schema.json, so the
// assertion stays honest as that file evolves. It deliberately compiles a
// synthetic root document ($defs/next_transition's own keys merged with the
// file's real, unmodified $defs) instead of status.schema.json's top-level
// object schema: that top-level schema's unrelated "projection" property
// resolves contracts/review-integration/v1/schemas/projection.schema.json,
// whose $defs/paths/items pattern uses a negative-lookahead regex Go's RE2
// engine cannot compile, which would fail metaschema validation for a
// reason wholly unrelated to what this test checks.
func validateAgainstPublishedNextTransitionSchema(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v1", "status.schema.json", payload)
}

// validateAgainstPublishedNextTransitionSchemaV2 performs the identical
// live-schema validation as validateAgainstPublishedNextTransitionSchema, but
// against contracts/review-integration/v1/schemas/status-v2.schema.json's own
// $defs/next_transition subtree, so the v2 wire shape stays pinned too.
func validateAgainstPublishedNextTransitionSchemaV2(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v1", "status-v2.schema.json", payload)
}

func validateAgainstPublishedNextTransitionSchemaV4(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v2", "status-v4.schema.json", payload)
}

func validateAgainstPublishedNextTransitionSchemaV5(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v2", "status-v5.schema.json", payload)
}

// validateAgainstPublishedStatusNextTransitionSchema is the shared engine
// behind both the v1 and v2 published-schema validators above. It registers
// every schema file that either version's $defs/next_transition subtree can
// $ref (targeted-validation-request.schema.json for both; artifact-subject
// .schema.json and start-v2.schema.json for v2's $defs/transition_input),
// deliberately excluding the top-level schema's "projection" property so
// compiling never touches projection.schema.json's RE2-incompatible
// negative-lookahead regex (see the comment above the v1 wrapper).
func validateAgainstPublishedStatusNextTransitionSchema(t *testing.T, version, schemaFile string, payload []byte) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration"))
	if err != nil {
		t.Fatal(err)
	}
	statusSchemaBytes, err := os.ReadFile(filepath.Join(root, version, "schemas", schemaFile))
	if err != nil {
		t.Fatal(err)
	}
	var statusSchema map[string]any
	if err := json.Unmarshal(statusSchemaBytes, &statusSchema); err != nil {
		t.Fatal(err)
	}
	defs, ok := statusSchema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs object: %#v", schemaFile, statusSchema["$defs"])
	}
	nextTransition, ok := defs["next_transition"].(map[string]any)
	if !ok {
		t.Fatalf("%s $defs.next_transition is missing or not an object: %#v", schemaFile, defs["next_transition"])
	}

	location := "https://gentle-ai.dev/contracts/review-integration/" + version + "/schemas/_test-next-transition.schema.json"
	synthetic := map[string]any{"$schema": statusSchema["$schema"], "$id": location, "$defs": defs}
	for key, value := range nextTransition {
		synthetic[key] = value
	}

	compiler := jsonschema.NewCompiler()
	resources := []struct{ version, name string }{
		{"v1", "status-v2.schema.json"},
		{"v1", "targeted-validation-request.schema.json"},
		{"v1", "correction-plan-request.schema.json"},
		{"v1", "artifact-subject.schema.json"},
		{"v1", "start-v2.schema.json"},
	}
	if version == "v2" {
		resources = append(resources,
			struct{ version, name string }{"v2", "artifact-subject.schema.json"},
			struct{ version, name string }{"v2", "start.schema.json"},
		)
	}
	for _, resource := range resources {
		refBytes, err := os.ReadFile(filepath.Join(root, resource.version, "schemas", resource.name))
		if err != nil {
			t.Fatal(err)
		}
		var refSchema any
		if err := json.Unmarshal(refBytes, &refSchema); err != nil {
			t.Fatal(err)
		}
		if resource.version == "v1" && resource.name == "status-v2.schema.json" {
			document := refSchema.(map[string]any)
			refSchema = map[string]any{"$schema": document["$schema"], "$id": document["$id"], "$defs": document["$defs"]}
		}
		if err := compiler.AddResource("https://gentle-ai.dev/contracts/review-integration/"+resource.version+"/schemas/"+resource.name, refSchema); err != nil {
			t.Fatal(err)
		}
	}
	if err := compiler.AddResource(location, synthetic); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("published next_transition schema (%s) rejected the emitted transition: %v", schemaFile, err)
	}
}

func nextTransitionTestCaptureContext(t *testing.T, status ReviewTargetStatusResult, lenses []string) *reviewCaptureContext {
	t.Helper()
	baseTree, candidateTree := strings.Repeat("c", 40), strings.Repeat("d", 40)
	frozen := reviewtransaction.FrozenCandidateContext{
		BaseTree: baseTree, CandidateTree: candidateTree,
		ChangedPathManifest: []reviewtransaction.ChangedPathManifestEntry{{
			Path: "tracked.txt", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644",
		}},
	}
	state := reviewtransaction.CompactState{
		LineageID: status.Authority.LineageID,
		InitialSnapshot: reviewtransaction.Snapshot{
			Identity: status.TargetIdentity, BaseTree: baseTree, CandidateTree: candidateTree, Paths: []string{"tracked.txt"},
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
	got := newReviewNextTransition(status, []string{reviewtransaction.LensRisk}, nil, nil, errors.New("tampered capture"), reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionStop || got.ReasonCode != "captured_artifacts_unverifiable" || got.Execute != nil || got.Collect != nil {
		t.Fatalf("target drift transition = %#v", got)
	}
}

func TestReviewForecastMirrorsExactlyOneNextTransition(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  ReviewTargetStatusResult
		horizon ReviewForecastHorizon
	}{
		{name: "stop", status: ReviewTargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityCorrupted}, horizon: ForecastHorizonTerminal},
		{name: "execute", status: ReviewTargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityUnrelated, TargetIdentity: "sha256:" + strings.Repeat("a", 64)}, horizon: ForecastHorizonPartial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			head := newReviewNextTransition(tt.status, nil, nil, nil, nil, reviewNextTransitionInput{})
			forecast := newReviewForecast(head)
			if forecast.Horizon != tt.horizon || len(forecast.Steps) != 1 {
				t.Fatalf("forecast = %#v, want one %s step", forecast, tt.horizon)
			}
			step := forecast.Steps[0]
			if step.Step != 1 || step.Kind != head.Kind || step.ReasonCode != head.ReasonCode || strings.TrimSpace(step.Description) == "" {
				t.Fatalf("forecast step = %#v, head = %#v", step, head)
			}
		})
	}
}

func TestReviewStatusValidateRejectsMalformedForecast(t *testing.T) {
	item := ReviewForecastItem{Step: 1, Kind: "stop", ReasonCode: "corrupted_or_unverifiable_authority", Description: "desc"}
	tests := []struct {
		name, wantErr string
		forecast      ReviewForecast
		next          bool
	}{
		{"missing next", "forecast without next_transition is invalid", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{item}}, false},
		{"invalid horizon", `invalid forecast horizon "invalid_horizon"`, ReviewForecast{Horizon: "invalid_horizon", Steps: []ReviewForecastItem{item}}, true},
		{"complete horizon", `invalid forecast horizon "complete"`, ReviewForecast{Horizon: "complete", Steps: []ReviewForecastItem{item}}, true},
		{"multiple steps", "forecast must contain exactly one step", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{item, item}}, true},
		{"step is not one", "forecast step must be 1, got 2", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 2, Kind: item.Kind, ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"invalid kind", `forecast step 1 has invalid kind "invalid_kind"`, ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: "invalid_kind", ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"empty reason", "forecast step 1 has empty reason_code", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: item.Kind, Description: item.Description}}}, true},
		{"empty description", "forecast step 1 has empty description", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: item.Kind, ReasonCode: item.ReasonCode}}}, true},
		{"head divergence", "forecast head (execute/corrupted_or_unverifiable_authority) diverges", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: "execute", ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"stop partial", "stop transition requires a terminal forecast", ReviewForecast{Horizon: ForecastHorizonPartial, Steps: []ReviewForecastItem{item}}, true},
	}
	status := newReviewTargetStatusResultForContract(reviewtransaction.TargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityCorrupted, AuthorityVersion: reviewtransaction.AuthorityVersionCompact, Action: reviewtransaction.TargetStatusActionStop, Replayability: reviewtransaction.ReplayabilityNotReplayable}, ReviewIntegrationContractV2)
	status.TargetIdentity = "sha256:" + strings.Repeat("a", 64)
	status.Projection = ReviewTargetStatusProjection{Schema: ReviewIntegrationProjectionSchema, Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("a", 40), InitialReviewTree: strings.Repeat("b", 40), CurrentCandidateTree: strings.Repeat("c", 40), PathsDigest: "sha256:" + strings.Repeat("a", 64), IntendedUntrackedProof: "sha256:" + strings.Repeat("b", 64), InitialSnapshotIdentity: status.TargetIdentity, CurrentSnapshotIdentity: status.TargetIdentity, Paths: []string{"tracked.txt"}, IntendedUntracked: []string{}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status
			got.Forecast = &tt.forecast
			if tt.next {
				got.NextTransition = &ReviewNextTransition{Kind: item.Kind, ReasonCode: item.ReasonCode}
			}
			err := got.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNegotiatedStatusForecastStaysStructuralAndV1StaysFrozen(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("forecast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr := captureReviewProcessStderr(t)

	var stdout bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &stdout); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("status envelope is invalid: %v", err)
	}
	if status.NextTransition == nil || status.Forecast == nil || len(status.Forecast.Steps) != 1 {
		t.Fatalf("status forecast = %#v, transition = %#v", status.Forecast, status.NextTransition)
	}
	step := status.Forecast.Steps[0]
	if status.Forecast.Horizon != ForecastHorizonPartial || step.Step != 1 || step.Kind != status.NextTransition.Kind || step.ReasonCode != status.NextTransition.ReasonCode || strings.TrimSpace(step.Description) == "" {
		t.Fatalf("status forecast = %#v, transition = %#v", status.Forecast, status.NextTransition)
	}
	// The forecast is structural only: a successful negotiated STATUS writes
	// zero bytes to stderr (gentle-pi fails closed on any stderr a successful
	// native process writes).
	if got := stderr(); got != "" {
		t.Errorf("negotiated STATUS narrated the forecast to stderr, want zero bytes:\n%q", got)
	}

	var legacy bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &legacy); err != nil {
		t.Fatal(err)
	}
	var v1 map[string]any
	if err := json.Unmarshal(legacy.Bytes(), &v1); err != nil {
		t.Fatal(err)
	}
	if _, found := v1["forecast"]; found {
		t.Fatalf("v1 status gained forecast: %s", legacy.String())
	}
}

func TestNativeStatusSchemasValidateWholeForecastEnvelope(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "status-v5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"status-v4.schema.json", "status-v5.schema.json"} {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(fixture, &document); err != nil {
				t.Fatal(err)
			}
			document["schema"] = "gentle-ai.review-integration.status/v" + strings.TrimSuffix(strings.TrimPrefix(name, "status-v"), ".schema.json")
			schema := compileWholeNativeStatusSchema(t, name)
			if err := schema.Validate(document); err != nil {
				t.Fatalf("whole %s envelope rejected fixture: %v", name, err)
			}
			document["unknown"] = true
			if err := schema.Validate(document); err == nil {
				t.Fatal("whole schema accepted an unknown property")
			}
		})
	}
}

func compileWholeNativeStatusSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration"))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(reviewSchemaRegexpEngine)
	for _, version := range []string{"v1", "v2"} {
		paths, err := filepath.Glob(filepath.Join(root, version, "schemas", "*.schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			if err := compiler.AddResource(document["$id"].(string), document); err != nil {
				t.Fatal(err)
			}
		}
	}
	id := "https://gentle-ai.dev/contracts/review-integration/v2/schemas/" + name
	schema, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

type reviewSchemaRegexp struct {
	pattern string
	re      *regexp.Regexp
}

func (r reviewSchemaRegexp) String() string { return r.pattern }

func (r reviewSchemaRegexp) MatchString(value string) bool {
	if r.re == nil {
		return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\n") && !slices.Contains(strings.Split(value, "/"), "..")
	}
	return r.re.MatchString(value)
}

func reviewSchemaRegexpEngine(pattern string) (jsonschema.Regexp, error) {
	if pattern == "^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$)).+$" {
		return reviewSchemaRegexp{pattern: pattern}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return reviewSchemaRegexp{pattern: pattern, re: re}, nil
}

// TestReviewStopTransitionEmitsStructuredContinuations verifies that every stop
// transition emitted by reviewStopTransition carries structured continuations
// (statement, runnable commands, and/or off-path exit) so consuming adapters
// and runtime agents do not invent their own menus (#3384).
func TestReviewStopTransitionEmitsStructuredContinuations(t *testing.T) {
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := reviewStopTransitionCallRegexp.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("found no reviewStopTransition calls")
	}
	for _, match := range matches {
		code := match[1]
		transition := reviewStopTransition(code)
		if transition.Kind != reviewNextTransitionStop {
			t.Errorf("stop reason %q: kind = %q, want stop", code, transition.Kind)
		}
		if transition.ReasonCode != code {
			t.Errorf("stop reason %q: reason_code = %q, want %q", code, transition.ReasonCode, code)
		}
		if transition.Stop == nil {
			t.Errorf("stop reason %q: Stop is nil, want structured continuation", code)
			continue
		}
		if strings.TrimSpace(transition.Stop.Statement) == "" {
			t.Errorf("stop reason %q: Stop.Statement is empty", code)
		}
		hasContinuation := (transition.Stop.OffPath != nil && transition.Stop.OffPath.Command != "") || len(transition.Stop.Commands) > 0
		if !hasContinuation {
			t.Errorf("stop reason %q: Stop has neither OffPath nor Commands", code)
		}
		if transition.Stop.OffPath != nil {
			if transition.Stop.OffPath.Command != reviewModeDisableCloneCommand {
				t.Errorf("stop reason %q: OffPath.Command = %q, want %q", code, transition.Stop.OffPath.Command, reviewModeDisableCloneCommand)
			}
			if strings.TrimSpace(transition.Stop.OffPath.Note) == "" {
				t.Errorf("stop reason %q: OffPath.Note is empty", code)
			}
		}

		// Ensure clean JSON serialization and deserialization
		payload, err := json.Marshal(transition)
		if err != nil {
			t.Errorf("stop reason %q: json.Marshal failed: %v", code, err)
			continue
		}
		var decoded ReviewNextTransition
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Errorf("stop reason %q: json.Unmarshal failed: %v", code, err)
			continue
		}
		if decoded.Stop == nil || decoded.Stop.Statement != transition.Stop.Statement {
			t.Errorf("stop reason %q: roundtrip mismatch: got %#v, want %#v", code, decoded.Stop, transition.Stop)
		}
	}
}
