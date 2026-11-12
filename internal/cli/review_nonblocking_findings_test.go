package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// runNonBlockingStatus runs the same contracted STATUS with and without the
// --non-blocking-findings opt-in and returns both outputs.
func runNonBlockingStatus(t *testing.T, repo, lineage string) (plain, opted []byte) {
	t.Helper()
	args := []string{"--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition"}
	var plainOutput, optedOutput bytes.Buffer
	if err := RunReviewStatus(args, &plainOutput); err != nil {
		t.Fatalf("STATUS without opt-in: %v\n%s", err, plainOutput.String())
	}
	if err := RunReviewStatus(append(append([]string{}, args...), "--non-blocking-findings"), &optedOutput); err != nil {
		t.Fatalf("STATUS with opt-in: %v\n%s", err, optedOutput.String())
	}
	return plainOutput.Bytes(), optedOutput.Bytes()
}

// assertOptInOnlyAddsNonBlockingFindings proves the client that did not opt
// in receives exactly what it receives without this change (no key at all),
// and that the opt-in adds that one key and changes nothing else.
func assertOptInOnlyAddsNonBlockingFindings(t *testing.T, plain, opted []byte) *reviewtransaction.NonBlockingFindingSet {
	t.Helper()
	if bytes.Contains(plain, []byte("non_blocking_findings")) {
		t.Fatalf("STATUS without opt-in carries non_blocking_findings:\n%s", plain)
	}
	optedDocument := decodeJSONObjectCopy(t, opted)
	raw, present := optedDocument["non_blocking_findings"]
	delete(optedDocument, "non_blocking_findings")
	if !reflect.DeepEqual(optedDocument, decodeJSONObjectCopy(t, plain)) {
		t.Fatalf("opt-in changed more than non_blocking_findings:\nplain %s\nopted %s", plain, opted)
	}
	if !present {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var set reviewtransaction.NonBlockingFindingSet
	if err := json.Unmarshal(encoded, &set); err != nil {
		t.Fatal(err)
	}
	return &set
}

func assertStatusSchemaKeepsNonBlockingFindingsSeparate(t *testing.T, opted []byte) {
	t.Helper()
	schema := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
	validatePublishedReviewSchema(t, schema, opted)
	for _, key := range []string{"advisory_findings", "reviewer_results"} {
		document := decodeJSONObjectCopy(t, opted)
		document[key] = document["non_blocking_findings"]
		if err := schema.Validate(document); err == nil {
			t.Fatalf("status-v9 schema accepted %s outside approved", key)
		}
	}
	document := decodeJSONObjectCopy(t, opted)
	findings := document["non_blocking_findings"].(map[string]any)["findings"].([]any)
	if len(findings) > 0 {
		delete(findings[0].(map[string]any), "claim")
		if err := schema.Validate(document); err == nil {
			t.Fatal("status-v9 schema accepted a non-blocking finding without its claim")
		}
	}
}

func TestCorrectionRequiredStatusReturnsNonBlockingFindingsOnlyToOptedInClient(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var closure bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{
		{
			ID: "R3-001", Location: "internal/auth/session.go:4", Severity: "CRITICAL",
			Claim: "the candidate introduces an observable authentication failure", ProofRefs: []string{"the changed line deterministically causes the reproduced failure"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		},
		{ID: "R3-W01", Location: "internal/auth/session.go:4", Severity: "WARNING", Claim: "the token check could be easier to read", ProofRefs: []string{"the exact changed line was inspected"}},
		{
			ID: "R3-002", Location: "internal/auth/session.go:5", Severity: "CRITICAL",
			Claim: "empty tokens were already accepted before this change", ProofRefs: []string{"the base revision has the same comparison"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalPreExisting,
		},
	}, &closure)

	// The closure is untouched: no new key, and its schema still refuses
	// reviewer_results outside approved.
	closureDocument := decodeJSONObjectCopy(t, closure.Bytes())
	if closureDocument["state"] != string(reviewtransaction.StateCorrectionRequired) {
		t.Fatalf("closure = %s, want correction_required", closure.Bytes())
	}
	for _, key := range []string{"non_blocking_findings", "advisory_findings", "reviewer_results"} {
		if _, present := closureDocument[key]; present {
			t.Fatalf("correction_required closure carries %s:\n%s", key, closure.Bytes())
		}
	}
	closureDocument["reviewer_results"] = []any{}
	if err := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json").Validate(closureDocument); err == nil {
		t.Fatal("last-event closure schema accepted reviewer_results outside approved")
	}

	plain, opted := runNonBlockingStatus(t, repo, started.LineageID)
	got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted)
	want := &reviewtransaction.NonBlockingFindingSet{Statement: reviewtransaction.NonBlockingStatement, Findings: []reviewtransaction.NonBlockingFinding{
		{ID: "R3-002", Lens: "reliability", Location: "internal/auth/session.go:5", Severity: "CRITICAL", Disposition: reviewtransaction.AdvisoryFollowUp, Claim: "empty tokens were already accepted before this change"},
		{ID: "R3-W01", Lens: "reliability", Location: "internal/auth/session.go:4", Severity: "WARNING", Disposition: reviewtransaction.AdvisoryInformational, Claim: "the token check could be easier to read"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non_blocking_findings = %#v, want %#v", got, want)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opted, &status)
	if status.NextTransition == nil || status.NextTransition.CorrectionRequest == nil ||
		!reflect.DeepEqual(status.NextTransition.CorrectionRequest.FixFindingIDs, []string{"R3-001"}) {
		t.Fatalf("correction request = %#v, want only R3-001 to correct", status.NextTransition)
	}
	assertStatusSchemaKeepsNonBlockingFindingsSeparate(t, opted)
}

func TestEscalatedStatusReturnsNonBlockingFindingsOnlyToOptedInClient(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithFindings(t, []facadeFinding{{
		Location: "tracked.txt:2", Severity: "SUGGESTION", Claim: "name the magic value", ProofRefs: []string{"tracked.txt:2 changed hunk"},
	}})
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: false, Evidence: []string{"acceptance test failed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"no regression observed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var closure bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", "pi", "--input", writeReviewCLIRawInput(t, payload),
	}, &closure); err != nil {
		t.Fatalf("capture rejected targeted validator: %v\n%s", err, closure.String())
	}
	if decodeJSONObjectCopy(t, closure.Bytes())["state"] != string(reviewtransaction.StateEscalated) || strings.Contains(closure.String(), "non_blocking_findings") {
		t.Fatalf("escalated closure = %s", closure.String())
	}

	plain, opted := runNonBlockingStatus(t, repo, lineage)
	got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted)
	if got == nil || got.Statement != reviewtransaction.NonBlockingStatement || len(got.Findings) != 1 ||
		got.Findings[0].Claim != "name the magic value" || got.Findings[0].Disposition != reviewtransaction.AdvisoryInformational {
		t.Fatalf("escalated non_blocking_findings = %#v, want the one suggestion with its claim", got)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opted, &status)
	for _, id := range status.Escalation.FindingIDs {
		if id == got.Findings[0].ID {
			t.Fatalf("escalation finding %s listed as non-blocking", id)
		}
	}
	assertStatusSchemaKeepsNonBlockingFindingsSeparate(t, opted)
}

func TestApprovedStatusIgnoresNonBlockingFindingsOptIn(t *testing.T) {
	repo, started, _, _ := startZeroLensPendingAcknowledgement(t)
	plain, opted := runNonBlockingStatus(t, repo, started.LineageID)
	if got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted); got != nil || !bytes.Equal(plain, opted) {
		t.Fatalf("approved STATUS changed with opt-in:\nplain %s\nopted %s", plain, opted)
	}
	schema := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
	validatePublishedReviewSchema(t, schema, plain)
	document := decodeJSONObjectCopy(t, plain)
	document["non_blocking_findings"] = map[string]any{"statement": reviewtransaction.NonBlockingStatement, "findings": []any{}}
	if err := schema.Validate(document); err == nil {
		t.Fatal("status-v9 schema accepted non_blocking_findings on an approved authority")
	}
}

func TestNonBlockingFindingsOptInRequiresContractV2(t *testing.T) {
	repo := initReviewCLIRepo(t)
	reviewEnabledHome(t)
	for _, args := range [][]string{
		{"--cwd", repo, "--non-blocking-findings"},
		{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--non-blocking-findings"},
	} {
		if err := RunReviewStatus(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), reviewNonBlockingFindingsRequiresContractReason) {
			t.Fatalf("STATUS %v error = %v, want %q", args, err, reviewNonBlockingFindingsRequiresContractReason)
		}
	}
	// PRESERVE: the sibling opt-ins keep their own refusal.
	if err := RunReviewStatus([]string{"--cwd", repo, "--action-eligibility"}, &bytes.Buffer{}); err == nil || err.Error() != reviewContractRequiredForActionEligibilityReason {
		t.Fatalf("STATUS --action-eligibility without contract error = %v", err)
	}
}

func TestRefutedSevereFindingIsNonBlockingOnCorrectionRequired(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, started, store, _ := newArtifactReview(t, false)
	findings := []facadeFinding{}
	for _, id := range []string{"R3-001", "R3-002"} {
		findings = append(findings, facadeFinding{
			ID: id, Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure " + id,
			ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
			CausalDisposition: reviewtransaction.CausalBehaviorActivated,
		})
	}
	captureCLIReviewerResultWithFindings(t, repo, started, 0, findings, &bytes.Buffer{})
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	refuterInput := filepath.Join(t.TempDir(), "refuter.json")
	writeReviewCLIJSON(t, refuterInput, facadeRefuterResult{RequestHash: request.RequestHash, Results: []facadeRefuterOutcome{
		{FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"}},
		{FindingID: "R3-002", Outcome: reviewtransaction.OutcomeRefuted, ProofRefs: []string{"the guard on line 1 prevents it"}},
	}})
	if err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", "pi", "--input", refuterInput), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	plain, opted := runNonBlockingStatus(t, repo, started.LineageID)
	got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted)
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opted, &status)
	if got == nil || len(got.Findings) != 1 || got.Findings[0].ID != "R3-002" || got.Findings[0].Disposition != reviewtransaction.AdvisoryRefuted ||
		status.NextTransition == nil || status.NextTransition.CorrectionRequest == nil || !reflect.DeepEqual(status.NextTransition.CorrectionRequest.FixFindingIDs, []string{"R3-001"}) {
		t.Fatalf("STATUS = %s, want only R3-002 refuted as non-blocking and only R3-001 to correct", opted)
	}
	assertStatusSchemaKeepsNonBlockingFindingsSeparate(t, opted)
}

func TestInconclusiveEscalationKeepsUnresolvedFindingOutOfNonBlocking(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{
		{ID: "R3-004", Location: "internal/auth/session.go:4", Severity: "CRITICAL", Claim: "possible race", ProofRefs: []string{"the lock order could not be established"},
			EvidenceClass: reviewtransaction.EvidenceInsufficient, CausalDisposition: reviewtransaction.CausalUnknown},
		{ID: "R3-002", Location: "internal/auth/session.go:5", Severity: "BLOCKER", Claim: "the base already accepts empty tokens", ProofRefs: []string{"the base revision has the same comparison"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalBaseOnly},
	}, &bytes.Buffer{})
	plain, opted := runNonBlockingStatus(t, repo, started.LineageID)
	got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted)
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opted, &status)
	if status.Escalation == nil || !reflect.DeepEqual(status.Escalation.FindingIDs, []string{"R3-004"}) ||
		got == nil || len(got.Findings) != 1 || got.Findings[0].ID != "R3-002" || got.Findings[0].Disposition != reviewtransaction.AdvisoryFollowUp {
		t.Fatalf("escalation = %#v, non_blocking_findings = %#v, want [R3-004] and only R3-002 follow_up", status.Escalation, got)
	}
	assertStatusSchemaKeepsNonBlockingFindingsSeparate(t, opted)
}
