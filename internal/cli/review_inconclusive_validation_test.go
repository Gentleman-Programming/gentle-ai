package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// inconclusiveValidationFieldEvidence is verbatim field evidence from issue
// #3378: a targeted validator that could not reach the frozen trees and said
// so in the passive voice. It carries no verdict in either direction.
const inconclusiveValidationFieldEvidence = "Immutable correction candidate tree sha256:9f2c could not be inspected with read-only Git access; " +
	"the supplied diff appears to remove the last-component omission, but that is insufficient for a verified verdict."

// providerInconclusiveValidationPayload reproduces the exact shape the
// maintainer's stuck lineage holds: both checks reported passed:false, but
// their evidence reports the immutable candidate was never inspected, so
// neither "false" is an observation.
func providerInconclusiveValidationPayload(t *testing.T, request reviewtransaction.TargetedValidationRequest) []byte {
	t.Helper()
	payload, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: false, Evidence: []string{inconclusiveValidationFieldEvidence}},
		CorrectionRegression:          facadeValidationCheck{Passed: false, Evidence: []string{inconclusiveValidationFieldEvidence}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// seedCapturedValidatorSlot reproduces a validator slot published by an
// earlier build whose detector still missed the passive phrasing: the bytes
// and their digest sidecar are internally consistent (so the slot is neither
// corrupt nor partially published), and only their semantics are unusable.
// The slot is first filled through the real capture path so its directories
// and file modes are exactly the ones native publication creates.
func seedCapturedValidatorSlot(t *testing.T, repo, lineage string, request reviewtransaction.TargetedValidationRequest, payload []byte) reviewtransaction.CompactStore {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision, providerTargetedValidationPayload(t, request)); err != nil {
		t.Fatal(err)
	}
	slotPath := filepath.Join(store.Dir, "targeted-validator-results",
		strings.TrimPrefix(request.CorrectionTargetIdentity, "sha256:"),
		strings.TrimPrefix(request.ExpectedRevision, "sha256:"), "result.json")
	if err := os.WriteFile(slotPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if err := os.WriteFile(slotPath+".sha256", []byte("sha256:"+hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return store
}

func readInconclusiveRoutingStatus(t *testing.T, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--next-transition",
	}, &output); err != nil {
		t.Fatalf("inconclusive captured-validation STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("inconclusive captured-validation STATUS validation: %v", err)
	}
	return status
}

// TestReviewStatusRoutesInconclusiveCapturedValidationToRetryableCapture is
// the RED-first proof for issue #3378's routing defect. A captured targeted
// validator result whose evidence reports the frozen trees could not be
// inspected is not a verdict and not corruption: no state transitioned and
// the correction attempt was never consumed. STATUS used to route it through
// the "unreadable or inadmissible captured slot" default branch into the
// terminal captured_artifacts_unverifiable stop, whose only documented
// continuations (inspect the authority store, or disable review for the
// clone) are both wrong here. It must instead re-offer the validation capture.
func TestReviewStatusRoutesInconclusiveCapturedValidationToRetryableCapture(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	store := seedCapturedValidatorSlot(t, repo, lineage, request, providerInconclusiveValidationPayload(t, request))

	status := readInconclusiveRoutingStatus(t, repo, lineage)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "targeted_validation_inconclusive_recapture_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("inconclusive captured-validation transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "external.run_targeted_validation" || input.Submission == nil ||
		input.ValidationRequest == nil || input.ValidationRequest.RequestHash != request.RequestHash {
		t.Fatalf("inconclusive captured-validation input = %#v", input)
	}
	transitionPayload, err := json.Marshal(status.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)
	if status.Forecast == nil || len(status.Forecast.Steps) != 1 || status.Forecast.Horizon != ForecastHorizonPartial {
		t.Fatalf("inconclusive captured-validation forecast = %#v", status.Forecast)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateCorrectionRequired || len(after.State.CorrectionAttempts) != 0 {
		t.Fatalf("inconclusive validator slot changed authority: %#v, %v", after, err)
	}
}

// TestReviewInconclusiveCapturedValidationReachesApprovedReceipt is the
// acceptance case for the maintainer's preserved lineage: open correction,
// unconsumed attempt, and an occupied validator slot holding an inconclusive
// result. This is the caller-authored route (no host-mediated runtime bound):
// its submission descriptor hands the fresh validation straight to finalize,
// so it never touches the occupied slot at all. Executing exactly what STATUS
// emits must drive the lineage all the way to an approved receipt.
func TestReviewInconclusiveCapturedValidationReachesApprovedReceipt(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	inconclusive := providerInconclusiveValidationPayload(t, request)
	store := seedCapturedValidatorSlot(t, repo, lineage, request, inconclusive)

	status := readInconclusiveRoutingStatus(t, repo, lineage)
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].Submission == nil {
		t.Fatalf("inconclusive captured-validation transition = %#v", status.NextTransition)
	}
	submission := status.NextTransition.Collect.Inputs[0].Submission
	if submission.Value == nil {
		t.Fatalf("inconclusive captured-validation submission = %#v", submission)
	}
	validationPath := filepath.Join(t.TempDir(), "conclusive-validation.json")
	if err := os.WriteFile(validationPath, providerTargetedValidationPayload(t, request), 0o600); err != nil {
		t.Fatal(err)
	}
	tokens := slices.Clone(submission.ArgumentTokens)
	slot := submission.Value.SubstitutionLocation
	tokens[slot] = strings.Replace(tokens[slot], reviewSubmissionValuePlaceholder, validationPath, 1)

	if err := RunReviewFacadeFinalize(tokens, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize the recaptured conclusive validation: %v", err)
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateApproved || len(terminal.State.CorrectionAttempts) != 1 {
		t.Fatalf("recaptured conclusive validation terminal authority = %#v, %v", terminal, err)
	}
	// The inconclusive bytes are still preserved exactly as captured: this
	// recovery never destroys immutable provider output.
	slotPath := filepath.Join(store.Dir, "targeted-validator-results",
		strings.TrimPrefix(request.CorrectionTargetIdentity, "sha256:"),
		strings.TrimPrefix(request.ExpectedRevision, "sha256:"), "result.json")
	preserved, err := os.ReadFile(slotPath)
	if err != nil || !bytes.Equal(preserved, inconclusive) {
		t.Fatalf("inconclusive validator payload was not preserved: %v", err)
	}
}

// TestReviewInconclusiveCapturedValidationCompletesTheHostRelayRoute is the
// live-route proof for the runtime that actually drives these lineages. A
// host-mediated runtime may never author a verdict itself, so handing it the
// generic external.run_targeted_validation form leaves it with a transition it
// has no executor for; it must receive the same Go-issued provider role task
// an unoccupied slot receives. This test executes exactly the transition
// STATUS emits -- the provider role capture funnel `review capture-validation`
// uses -- and then the finalize STATUS emits next, all the way to the receipt.
func TestReviewInconclusiveCapturedValidationCompletesTheHostRelayRoute(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	inconclusive := providerInconclusiveValidationPayload(t, request)
	store := seedCapturedValidatorSlot(t, repo, lineage, request, inconclusive)

	readHostStatus := func() ReviewTargetStatusResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview([]string{
			"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
			"--agent", string(model.AgentOpenCode), "--next-transition",
		}, &output); err != nil {
			var direct bytes.Buffer
			directErr := runReviewStatus(t.Context(), []string{
				"--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
				"--agent", string(model.AgentOpenCode), "--next-transition",
			}, &direct)
			t.Fatalf("host-relay inconclusive STATUS: %v\ndirect=%v\n%s", err, directErr, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if err := status.Validate(); err != nil {
			t.Fatalf("host-relay inconclusive STATUS validation: %v", err)
		}
		return status
	}

	before := readHostStatus()
	if before.NextTransition == nil || before.NextTransition.Kind != reviewNextTransitionCollect ||
		before.NextTransition.ReasonCode != "targeted_validation_inconclusive_recapture_required" ||
		before.NextTransition.Collect == nil || len(before.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("host-relay inconclusive transition = %#v", before.NextTransition)
	}
	input := before.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "external.run_provider_role" || input.ProviderTask == nil ||
		input.ProviderTask.Role != string(reviewerprovider.RoleTargetedValidator) || input.Submission != nil {
		t.Fatalf("host-relay inconclusive input = %#v: a host-mediated runtime must receive a Go-issued validator task, never a form that asks it to author the verdict", input)
	}
	transitionPayload, err := json.Marshal(before.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)

	// Exactly the funnel `review capture-validation` drives.
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision,
		providerTargetedValidationPayload(t, request)); err != nil {
		t.Fatalf("recapture the validation the host relay just ran: %v", err)
	}

	after := readHostStatus()
	if after.NextTransition == nil || after.NextTransition.Kind != reviewNextTransitionExecute ||
		after.NextTransition.Execute == nil || after.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("post-recapture transition = %#v", after.NextTransition)
	}
	tokens := make([]string, len(after.NextTransition.Execute.Arguments))
	for index, argument := range after.NextTransition.Execute.Arguments {
		tokens[index] = argument.Token
	}
	if err := RunReviewFacadeFinalize(tokens, &bytes.Buffer{}); err != nil {
		t.Fatalf("execute the finalize STATUS emitted after the recapture: %v", err)
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateApproved || len(terminal.State.CorrectionAttempts) != 1 {
		t.Fatalf("host-relay recapture terminal authority = %#v, %v", terminal, err)
	}

	// The superseded non-verdict is archived, never destroyed.
	archived := reviewQuarantinedTargetedValidatorPath(store.Dir, request, "")
	preserved, err := os.ReadFile(archived)
	if err != nil || !bytes.Equal(preserved, inconclusive) {
		t.Fatalf("superseded inconclusive payload was not preserved at %s: %v", archived, err)
	}
}

// TestReviewStatusStopsInconclusiveRecaptureAtItsBound drives the live route
// rather than seeding the ledger: it re-reads STATUS and submits a real
// recapture through the capture path all three entry points funnel into, so it
// fails if the ledger reads but never advances. The occupant itself must be
// injected -- this build refuses to publish a non-verdict, so a legacy artifact
// is the only way one exists -- but every attempt after it is genuine.
func TestReviewStatusStopsInconclusiveRecaptureAtItsBound(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	inconclusive := providerInconclusiveValidationPayload(t, request)
	store := seedCapturedValidatorSlot(t, repo, lineage, request, inconclusive)

	// The occupant is attempt 1; every later attempt must still be offered and
	// must still be a blind validator refusing to invent a verdict.
	for attempt := 2; attempt <= maxInconclusiveTargetedValidations; attempt++ {
		status := readInconclusiveRoutingStatus(t, repo, lineage)
		if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "targeted_validation_inconclusive_recapture_required" {
			t.Fatalf("attempt %d is not still offered a recapture: %#v", attempt, status.NextTransition)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision,
			inconclusive); !errors.Is(err, errReviewTargetedValidationInconclusive) {
			t.Fatalf("recapture %d = %v, want the inconclusive refusal", attempt, err)
		}
		if recorded := reviewInconclusiveTargetedValidationAttempts(store.Dir, request); recorded != attempt-1 {
			t.Fatalf("ledger after recapture %d = %d, want %d", attempt, recorded, attempt-1)
		}
	}

	status := readInconclusiveRoutingStatus(t, repo, lineage)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "captured_artifacts_unverifiable" || status.NextTransition.Collect != nil {
		t.Fatalf("exhausted inconclusive recapture transition = %#v", status.NextTransition)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateCorrectionRequired || len(after.State.CorrectionAttempts) != 0 {
		t.Fatalf("exhausted inconclusive recapture changed authority: %#v, %v", after, err)
	}
}

// TestReviewCapturedVerdictStaysUnreplaceable is the guard on the supersede
// path. Only a captured non-verdict may be moved aside; a real verdict --
// here a genuine failed one, the exact shape a widened detector must never
// misread as inconclusive -- keeps the immutable slot it already owns.
func TestReviewCapturedVerdictStaysUnreplaceable(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	failed, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria: facadeValidationCheck{Passed: false, Evidence: []string{
			"Inspected the correction candidate tree: the loop still stops one entry short, so the finding is not fixed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"inspected the frozen candidate tree; no regression"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := seedCapturedValidatorSlot(t, repo, lineage, request, failed)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision,
		providerTargetedValidationPayload(t, request)); !errors.Is(err, reviewtransaction.ErrCapturedReviewerResultSlotConflict) {
		t.Fatalf("recapture over a real verdict = %v, want a slot conflict", err)
	}
	slot, err := reviewtransaction.ReadCompactTargetedValidatorResultSlot(store.Dir, request)
	if err != nil || !slot.Occupied || !bytes.Equal(slot.Payload, failed) {
		t.Fatalf("the real verdict no longer occupies its slot: %v", err)
	}
}

// TestReviewStatusKeepsTerminalStopForInadmissibleValidatorSlot is the
// negative half of the same distinction. A slot whose bytes and digest agree
// but whose content does not bind the provider-owned request is inadmissible,
// not inconclusive: it keeps the terminal captured_artifacts_unverifiable stop
// it has today. Only the "no verdict was produced" case becomes retryable.
func TestReviewStatusKeepsTerminalStopForInadmissibleValidatorSlot(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	foreign, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: "sha256:" + strings.Repeat("a", 64),
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"inspected the frozen candidate tree; original criteria hold"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"inspected the frozen candidate tree; no regression"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := seedCapturedValidatorSlot(t, repo, lineage, request, foreign)

	status := readInconclusiveRoutingStatus(t, repo, lineage)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "captured_artifacts_unverifiable" ||
		status.NextTransition.Execute != nil || status.NextTransition.Collect != nil {
		t.Fatalf("inadmissible validator slot transition = %#v", status.NextTransition)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateCorrectionRequired || len(after.State.CorrectionAttempts) != 0 {
		t.Fatalf("inadmissible validator slot changed authority: %#v, %v", after, err)
	}
}
