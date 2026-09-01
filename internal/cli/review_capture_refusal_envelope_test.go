package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// These tests prove each surviving collect-satisfying capture operation
// (`capture-result`, `capture-refuter`, `capture-validation`) emits one
// `gentle-ai.review-integration.failure/v2` envelope on stdout when it
// refuses. Machine callers can therefore classify the refusal as not started
// instead of mistaking empty stdout for an unknown mutation outcome.

// decodeCaptureRefusalEnvelope decodes the single stdout document a refused
// capture operation must emit and asserts the caller-facing typed error
// carries the same envelope.
func decodeCaptureRefusalEnvelope(t *testing.T, err error, output []byte) ReviewIntegrationFailure {
	t.Helper()
	if err == nil {
		t.Fatal("capture refusal returned success")
	}
	var typed *ReviewIntegrationFailureError
	if !errors.As(err, &typed) {
		t.Fatalf("capture refusal error is not a negotiated failure: %v", err)
	}
	if len(bytes.TrimSpace(output)) == 0 {
		t.Fatalf("capture refusal printed empty stdout; machine callers classify that as mutation outcome unknown: %v", err)
	}
	var failure ReviewIntegrationFailure
	decodeStrictReviewJSON(t, output, &failure)
	if failure.Code != typed.Failure.Code || failure.Operation != typed.Failure.Operation {
		t.Fatalf("stdout envelope %#v does not match typed error envelope %#v", failure, typed.Failure)
	}
	if failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV2 {
		t.Fatalf("capture refusal envelope identity = %q/%q, want failure/v2", failure.Schema, failure.Contract)
	}
	return failure
}

// TestCaptureResultRefusalsEmitTypedFailureEnvelopes covers the reviewer-lens
// collection: the request-shape refusal takes the shared preflight code, and
// the binding refusal earns the typed capture_binding_mismatch whose
// continuation is a fresh STATUS.
func TestCaptureResultRefusalsEmitTypedFailureEnvelopes(t *testing.T) {
	reviewEnabledHome(t)
	t.Run("missing inputs", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		var output bytes.Buffer
		err := RunReview([]string{"capture-result", "--cwd", repo}, &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Phase != "preflight" ||
			failure.Code != "invalid_request" || failure.NextAction != "correct_request" ||
			failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("missing-input refusal envelope = %#v", failure)
		}
		schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
		validatePublishedReviewSchema(t, schema, output.Bytes())
	})
	t.Run("binding mismatch", func(t *testing.T) {
		const staleTarget = "sha256:6718ffa9d77c4965113517101482479c71763d40de8c366d2ebac11a367e6e1d"
		repo, started, _, record := newArtifactReview(t, false)
		input := filepath.Join(t.TempDir(), "result.json")
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"capture-result", "--cwd", repo, "--lineage", started.LineageID,
			"--target", staleTarget, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}, &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Phase != "preflight" ||
			failure.Code != "capture_binding_mismatch" || failure.NextAction != "review.status" ||
			failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
			failure.LineageID != started.LineageID {
			t.Fatalf("binding refusal envelope = %#v", failure)
		}
		schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
		validatePublishedReviewSchema(t, schema, output.Bytes())
	})
	t.Run("occupied slot", func(t *testing.T) {
		repo, started, _, record := newArtifactReview(t, true)
		if len(record.State.SelectedLenses) < 2 {
			t.Fatalf("selected lenses = %v, want multiple lenses for a nonterminal occupied slot", record.State.SelectedLenses)
		}
		first := filepath.Join(t.TempDir(), "first.json")
		if err := os.WriteFile(first, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
			t.Fatal(err)
		}
		binding := []string{
			"capture-result", "--cwd", repo, "--lineage", started.LineageID,
			"--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0",
		}
		if err := RunReview(append(binding, "--input", first), &bytes.Buffer{}); err != nil {
			t.Fatalf("capture first selected lens: %v", err)
		}
		conflicting := filepath.Join(t.TempDir(), "conflicting.json")
		payload := admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0, "a different inspection narrative")
		if err := os.WriteFile(conflicting, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview(append(binding, "--input", conflicting), &output)
		failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
		if failure.Operation != "review.capture-result" || failure.Code != reviewerResultSlotOccupiedCode ||
			failure.NextAction != "review.status" || failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("occupied-slot refusal envelope = %#v", failure)
		}
	})
}

// TestProviderRoleCaptureRefusalsEmitTypedFailureEnvelopes covers the two
// non-lens provider role collections. A request-shape refusal is the shared
// preflight code; what matters is that stdout carries the typed envelope
// instead of nothing.
func TestProviderRoleCaptureRefusalsEmitTypedFailureEnvelopes(t *testing.T) {
	for _, tt := range []struct {
		verb      string
		operation string
	}{
		{verb: "capture-refuter", operation: "review.capture-refuter"},
		{verb: "capture-validation", operation: "review.capture-validation"},
	} {
		t.Run(tt.verb, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			var output bytes.Buffer
			err := RunReview([]string{tt.verb, "--cwd", repo}, &output)
			failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
			if failure.Operation != tt.operation || failure.Phase != "preflight" ||
				failure.Code != "invalid_request" || failure.NextAction != "correct_request" ||
				failure.MutationOutcome != ReviewMutationNotStarted {
				t.Fatalf("%s refusal envelope = %#v", tt.verb, failure)
			}
			schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
			validatePublishedReviewSchema(t, schema, output.Bytes())
		})
	}
}

// TestCaptureRefusalKeepsOperatorErrorAndKillSwitchIdentity pins two behaviors
// the envelope emission must not change: the operator-facing error text stays
// (stderr keeps its human-readable line) and a kill-switch refusal keeps its
// typed identity for errors.Is dispatch.
func TestCaptureRefusalKeepsOperatorErrorAndKillSwitchIdentity(t *testing.T) {
	repo, _ := disabledReviewRepo(t, "review-capture-envelope-disabled")
	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	var output bytes.Buffer
	err := RunReview([]string{
		"capture-result", "--cwd", repo, "--lineage", "review-capture-envelope-disabled",
		"--target", digest, "--lens", "review-risk", "--order", "0", "--input", input,
	}, &output)
	if !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("kill-switch identity lost through envelope emission: %v", err)
	}
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code != "rdd_disabled" || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("kill-switch refusal envelope = %#v", failure)
	}
}

// TestCollectCaptureOperationsStayOffTheNegotiatedSurface pins the boundary:
// the capture operations join the failure envelope vocabulary without joining
// the published capabilities operations array (its length is a pinned
// contract) and without gaining a negotiated --contract route.
func TestCollectCaptureOperationsStayOffTheNegotiatedSurface(t *testing.T) {
	for _, operation := range []string{
		"review.capture-result", "review.capture-refuter", "review.capture-validation",
	} {
		metadata, known := reviewIntegrationOperationByName(operation)
		if !known {
			t.Fatalf("collect capture operation %q is not in the operation registry", operation)
		}
		if metadata.Negotiated {
			t.Fatalf("collect capture operation %q must not join the negotiated surface", operation)
		}
		if _, negotiated := reviewIntegrationOperationByCommand(metadata.Command); negotiated {
			t.Fatalf("collect capture command %q is routed as a negotiated command", metadata.Command)
		}
		for _, published := range reviewIntegrationOperationNames() {
			if published == operation {
				t.Fatalf("collect capture operation %q leaked into the published capabilities operations array", operation)
			}
		}
		if !validReviewIntegrationFailureOperation(operation) {
			t.Fatalf("collect capture operation %q is not admitted to the failure envelope vocabulary", operation)
		}
		lineage := safeReviewIntegrationLineage(operation, []string{"--lineage", "review-capture-lineage"})
		if lineage != "review-capture-lineage" {
			t.Fatalf("collect capture operation %q drops lineage from safe argument extraction: %q", operation, lineage)
		}
	}
}

// reviewEmitFailureWriter simulates a dead stdout (closed pipe, halted host
// relay) so envelope emission fails underneath a real native refusal.
type reviewEmitFailureWriter struct{ err error }

func (w reviewEmitFailureWriter) Write([]byte) (int, error) { return 0, w.err }

// TestCaptureRefusalEmitFailurePreservesNativeRefusal pins that a failed
// envelope emission never discards the refusal: the returned chain carries
// both errors, refusal primary, so errors.Is/As dispatch keeps working.
func TestCaptureRefusalEmitFailurePreservesNativeRefusal(t *testing.T) {
	emitErr := errors.New("stdout gone: broken pipe")
	err := RunReview([]string{"capture-result", "--cwd", initReviewCLIRepo(t)}, reviewEmitFailureWriter{err: emitErr})
	var typed *ReviewIntegrationFailureError
	if !errors.As(err, &typed) || typed.Failure.Code != "invalid_request" {
		t.Fatalf("native refusal lost when envelope emission failed: %v", err)
	}
	if !errors.Is(err, emitErr) {
		t.Fatalf("emit failure missing from the error chain: %v", err)
	}
	if !strings.HasPrefix(err.Error(), typed.Error()) {
		t.Fatalf("refusal is not the primary error: %q", err.Error())
	}
}

// TestOverBudgetCorrectionPlanRefusalIsClassifiedAsNotStarted pins the envelope
// an over-budget pre-edit forecast earns. The flag exists so an operator can
// declare a size BEFORE editing, and the refusal is a pure precondition on
// caller input: BeginCorrection returns before it records a proposal or
// advances the capture phase, and the command returns before its store.Replace,
// so nothing is written. Reporting that as operation_outcome_unknown with
// retry_safe:false tells the operator the store may have moved when it provably
// did not, and it routes them to recovery instead of to a smaller forecast.
func TestOverBudgetCorrectionPlanRefusalIsClassifiedAsNotStarted(t *testing.T) {
	repo, started, store, record, request := correctionRequiredForPlanCapture(t)
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"capture-correction-plan", "--cwd", repo, "--lineage", started.LineageID,
		"--target", request.TargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash, "--correction-lines", strconv.Itoa(request.CorrectionBudget + 1),
	}, &output)

	// Only an outcome that stays unknown after classification earns a defect
	// report. An operator who forecast too many lines has not found a product
	// defect, and should not be told to file one.
	reports, _ := filepath.Glob(filepath.Join(filepath.Dir(reviewCLIAuthorityRoot(t, repo)), reviewDefectReportDirName, "*"))
	if len(reports) != 0 {
		t.Fatalf("over-budget forecast wrote %d defect report(s) for ordinary caller input", len(reports))
	}

	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code == "operation_outcome_unknown" {
		t.Fatalf("over-budget forecast reported an unknown outcome for a refusal that never started: %#v", failure)
	}
	if failure.Phase != "preflight" || failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe {
		t.Fatalf("over-budget forecast envelope = %#v, want a retry-safe preflight refusal that never started", failure)
	}
	if !strings.Contains(failure.Cause, "exceeding the remaining budget") {
		t.Fatalf("over-budget forecast envelope hides its cause: %#v", failure)
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision || after.State.ProposedCorrectionLines != nil {
		t.Fatalf("over-budget forecast mutated authority: %#v, %v", after, err)
	}
}

// TestCorrectionStatusRoutesWithAnUntrackedArtifactPresent covers the way a
// correction actually happens: the operator edits, and the tools they run while
// editing leave files behind. A test artifact, a build output, a coverage
// profile -- any untracked file appearing during the correction switched STATUS
// to the untracked-selection transition, which legitimately carries no
// validation request while the status still carried one, and the consistency
// check read that as two copies disagreeing.
//
// The result was a lineage with no way forward: the exact-lineage STATUS the
// contract names as the only re-entry refused, and its read-only envelope is
// content-free by design, so the refusal advertised "retry" and named nothing.
func TestCorrectionStatusRoutesWithAnUntrackedArtifactPresent(t *testing.T) {
	repo, started, _, record, request := correctionRequiredForPlanCapture(t)
	if err := RunReview([]string{
		"capture-correction-plan", "--cwd", repo, "--lineage", started.LineageID,
		"--target", request.TargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash, "--correction-lines", "2",
	}, io.Discard); err != nil {
		t.Fatalf("capture correction plan: %v", err)
	}
	// Correct the candidate the way an operator would, then confirm the review
	// is waiting on validation: that is the state whose validation request the
	// untracked file has to survive.
	corrected := "package auth\n\n// CheckToken reports whether a session token is present.\nfunc CheckToken(token string) bool {\n\treturn len(token) > 0\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "internal", "auth", "session.go"), []byte(corrected), 0o644); err != nil {
		t.Fatal(err)
	}
	var beforeArtifact bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--lineage", started.LineageID, "--next-transition",
	}, &beforeArtifact); err != nil {
		t.Fatalf("corrected STATUS before the artifact: %v\n%s", err, beforeArtifact.String())
	}
	var beforeStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, beforeArtifact.Bytes(), &beforeStatus)
	if beforeStatus.ValidationRequest == nil {
		t.Fatalf("corrected STATUS carries no validation request, so this test proves nothing: %s", beforeArtifact.String())
	}

	// One artifact from the tools the operator ran while correcting.
	if err := os.WriteFile(filepath.Join(repo, "results.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--lineage", started.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("correction STATUS with an untracked artifact present: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil {
		t.Fatalf("correction STATUS offered no transition: %s", output.String())
	}
	if status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("correction STATUS reason = %q, want the untracked selection the new file requires", status.NextTransition.ReasonCode)
	}
	// The point of the exemption: the pending validation is still true while the
	// operator declares the file, so the status must keep reporting it.
	if status.ValidationRequest == nil {
		t.Fatal("correction STATUS dropped the pending validation request the review is still waiting on")
	}
	if !reflect.DeepEqual(*status.ValidationRequest, *beforeStatus.ValidationRequest) {
		t.Fatalf("the untracked artifact changed the pending validation request: %#v vs %#v", status.ValidationRequest, beforeStatus.ValidationRequest)
	}

	// Routing is only half of it. The lineage was stranded because nothing could
	// drive it forward, so declaring the artifact has to put the review back
	// where it was: waiting on the same validation.
	if status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("untracked selection offered no single collect input: %#v", status.NextTransition.Collect)
	}
	// This input is a decision the operator owns, so it carries values and no
	// runnable tokens: the product names the exact flags in its own refusal
	// rather than issuing an invocation. Composing them here is therefore the
	// real route, and this assertion makes a future change that starts issuing
	// tokens fail rather than let the test keep hand-building them.
	for _, argument := range status.NextTransition.Collect.Inputs[0].Arguments {
		if argument.Token != "" {
			t.Fatalf("untracked selection now issues runnable tokens; replay them instead of composing flags: %#v", argument)
		}
	}
	inventory := ""
	for _, argument := range status.NextTransition.Collect.Inputs[0].Arguments {
		if argument.Name == "expected_untracked_inventory" {
			if inventory != "" {
				t.Fatalf("untracked selection repeated its inventory argument: %#v", status.NextTransition.Collect.Inputs[0].Arguments)
			}
			inventory = argument.Value
		}
	}
	if inventory == "" {
		t.Fatalf("untracked selection offered no inventory to declare against: %#v", status.NextTransition.Collect.Inputs[0].Arguments)
	}
	var recovered bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--lineage", started.LineageID, "--next-transition",
		"--untracked-scope=exclude", "--expected-untracked-inventory=" + inventory,
	}, &recovered); err != nil {
		t.Fatalf("declaring the artifact did not recover the correction: %v\n%s", err, recovered.String())
	}
	var recoveredStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &recoveredStatus)
	if recoveredStatus.NextTransition == nil || recoveredStatus.NextTransition.ReasonCode != "targeted_validation_required" {
		t.Fatalf("declared artifact left the correction unable to proceed: %#v", recoveredStatus.NextTransition)
	}
	if recoveredStatus.ValidationRequest == nil || !reflect.DeepEqual(*recoveredStatus.ValidationRequest, *beforeStatus.ValidationRequest) {
		t.Fatalf("recovery changed the pending validation request: %#v", recoveredStatus.ValidationRequest)
	}

	// The exemption relaxes presence parity, so the gate it sits in must still
	// refuse two copies that genuinely disagree. Nothing else proves that the
	// gate is wired to the helper at all.
	// PolicyContent is chosen deliberately: the earlier submission-descriptor and
	// binding guards pin the request hash, target and trees, so tampering one of
	// those never reaches this gate. This one only the copies check compares.
	tampered := beforeStatus
	disagreeing := *beforeStatus.ValidationRequest
	disagreeing.PolicyContent = disagreeing.PolicyContent + "\ndrifted"
	tampered.ValidationRequest = &disagreeing
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "validation request copies differ") {
		t.Fatalf("disagreeing validation request copies validated = %v", err)
	}
}

// TestReviewValidationRequestCopiesAgree pins exactly how far the untracked
// exemption reaches. It relaxes presence parity and nothing else, so a
// transition that does carry a request is still held to matching the status
// one, and a transition that carries one the status does not is still wrong
// however it collects.
func TestReviewValidationRequestCopiesAgree(t *testing.T) {
	one := reviewtransaction.TargetedValidationRequest{LineageID: "review-copies", RequestHash: "sha256:" + strings.Repeat("a", 64)}
	// A separate value that equals `one`. Passing the same pointer twice would
	// make the equal cases hold on identity and prove nothing about comparison.
	sameAsOne := reviewtransaction.TargetedValidationRequest{LineageID: "review-copies", RequestHash: "sha256:" + strings.Repeat("a", 64)}
	other := reviewtransaction.TargetedValidationRequest{LineageID: "review-copies", RequestHash: "sha256:" + strings.Repeat("b", 64)}
	for _, tt := range []struct {
		name                  string
		transition, status    *reviewtransaction.TargetedValidationRequest
		collectsSomethingElse bool
		agree                 bool
	}{
		{name: "neither present", agree: true},
		{name: "both present and equal", transition: &one, status: &sameAsOne, agree: true},
		{name: "both present and different", transition: &one, status: &other},
		{name: "status only, ordinary transition", status: &one},
		{name: "status only, transition collecting something else", status: &one, collectsSomethingElse: true, agree: true},
		{name: "transition only", transition: &one},
		{name: "transition only, collecting something else", transition: &one, collectsSomethingElse: true},
		{name: "both present and different, collecting something else", transition: &one, status: &other, collectsSomethingElse: true},
		{name: "both present and equal, collecting something else", transition: &one, status: &sameAsOne, collectsSomethingElse: true, agree: true},
		{name: "neither present, collecting something else", collectsSomethingElse: true, agree: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := reviewValidationRequestCopiesAgree(tt.transition, tt.status, tt.collectsSomethingElse); got != tt.agree {
				t.Fatalf("reviewValidationRequestCopiesAgree() = %v, want %v", got, tt.agree)
			}
		})
	}
}
