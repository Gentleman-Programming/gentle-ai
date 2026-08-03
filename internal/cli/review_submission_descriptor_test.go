package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestSubmissionDescriptorsAreBoundAndExecuteOneValueOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo, started, store := submissionDescriptorCorrectionFixture(t)
	status := submissionDescriptorStatus(t, repo, started.LineageID)
	correction := submissionDescriptorInput(t, status)
	assertSubmissionDescriptor(t, correction, status, "correction_lines")
	assertSubmissionTransitionSchema(t, status)

	before := readReviewOperationFile(t, store.StatePath())
	for _, test := range []struct {
		name       string
		descriptor ReviewTransitionSubmission
		value      string
	}{
		{name: "malicious correction value", descriptor: *correction.Submission, value: "1 --target=sha256:" + strings.Repeat("a", 64)},
		{name: "stale correction revision", descriptor: replaceSubmissionToken(t, *correction.Submission, "--expected-revision=", "--expected-revision=sha256:"+strings.Repeat("b", 64)), value: "1"},
		{name: "mismatched correction request", descriptor: replaceSubmissionToken(t, *correction.Submission, "--request-hash=", "--request-hash=sha256:"+strings.Repeat("c", 64)), value: "1"},
		{name: "altered correction context", descriptor: replaceSubmissionToken(t, *correction.Submission, "--repository-context=", "--repository-context=rctx1_"+strings.Repeat("d", 64)), value: "1"},
		{name: "altered correction contract", descriptor: replaceSubmissionToken(t, *correction.Submission, "--contract=", "--contract="+ReviewIntegrationContractV1), value: "1"},
		{name: "altered correction token order", descriptor: swapSubmissionTokens(t, *correction.Submission, 1, 2), value: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runSubmissionDescriptor(t, test.descriptor, test.value)
			assertSubmissionNotStarted(t, err, output, store, before)
		})
	}
	if output, err := runSubmissionDescriptor(t, *correction.Submission, "1"); err != nil {
		t.Fatalf("execute correction descriptor: %v\n%s", err, output)
	}
	forecasted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if forecasted.State.ProposedCorrectionLines == nil || *forecasted.State.ProposedCorrectionLines != 1 || len(forecasted.State.CorrectionAttempts) != 0 {
		t.Fatalf("correction descriptor state = %#v", forecasted.State)
	}
	before = readReviewOperationFile(t, store.StatePath())
	output, err := runSubmissionDescriptor(t, *correction.Submission, "1")
	assertSubmissionNotStarted(t, err, output, store, before)

	if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte("package candidate\n\nfunc value() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, forecasted.State, forecasted.Revision)
	if err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "correction-evidence.txt")
	if err := os.WriteFile(evidence, []byte("repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", forecasted.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidence,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	ready := submissionDescriptorStatus(t, repo, started.LineageID)
	validation := submissionDescriptorInput(t, ready)
	assertSubmissionDescriptor(t, validation, ready, "validation")
	assertSubmissionTransitionSchema(t, ready)
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	before = readReviewOperationFile(t, store.StatePath())
	for _, test := range []struct {
		name       string
		descriptor ReviewTransitionSubmission
		value      string
	}{
		{name: "malicious validation value", descriptor: *validation.Submission, value: validationPath + " --target=sha256:" + strings.Repeat("a", 64)},
		{name: "stale validation revision", descriptor: replaceSubmissionToken(t, *validation.Submission, "--expected-revision=", "--expected-revision=sha256:"+strings.Repeat("b", 64)), value: validationPath},
		{name: "mismatched validation request", descriptor: replaceSubmissionToken(t, *validation.Submission, "--request-hash=", "--request-hash=sha256:"+strings.Repeat("c", 64)), value: validationPath},
		{name: "altered validation context", descriptor: replaceSubmissionToken(t, *validation.Submission, "--repository-context=", "--repository-context=rctx1_"+strings.Repeat("d", 64)), value: validationPath},
		{name: "altered validation contract", descriptor: replaceSubmissionToken(t, *validation.Submission, "--contract=", "--contract="+ReviewIntegrationContractV1), value: validationPath},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runSubmissionDescriptor(t, test.descriptor, test.value)
			assertSubmissionNotStarted(t, err, output, store, before)
		})
	}
	wrongValidation := filepath.Join(t.TempDir(), "wrong-validation.json")
	writeReviewCLIJSON(t, wrongValidation, facadeValidationResult{
		TargetedValidationRequestHash: "sha256:" + strings.Repeat("d", 64),
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	output, err = runSubmissionDescriptor(t, *validation.Submission, wrongValidation)
	assertSubmissionNotStarted(t, err, output, store, before)

	if output, err := runSubmissionDescriptor(t, *validation.Submission, validationPath); err != nil {
		t.Fatalf("execute validation descriptor: %v\n%s", err, output)
	}
	terminal, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State.State != reviewtransaction.StateApproved || len(terminal.State.CorrectionAttempts) != 1 {
		t.Fatalf("validation descriptor terminal state = %#v", terminal.State)
	}
	before = readReviewOperationFile(t, store.StatePath())
	output, err = runSubmissionDescriptor(t, *validation.Submission, validationPath)
	assertSubmissionNotStarted(t, err, output, store, before)
}

func submissionDescriptorCorrectionFixture(t *testing.T) (string, ReviewIntegrationStartResult, reviewtransaction.CompactStore) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte("package candidate\n\nfunc value() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "submission-descriptor")
	result := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{
		Lens: started.SelectedLenses[0],
		Findings: []facadeFinding{{
			Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate value is wrong",
			ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"inspected exact candidate"},
	})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{result}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, started, store
}

func submissionDescriptorStatus(t *testing.T, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition", "--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatalf("descriptor status: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	return status
}

func submissionDescriptorInput(t *testing.T, status ReviewTargetStatusResult) ReviewTransitionInput {
	t.Helper()
	if status.Schema != ReviewIntegrationStatusSchema || status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Submission == nil {
		t.Fatalf("submission descriptor transition = %#v", status.NextTransition)
	}
	return status.NextTransition.Collect.Inputs[0]
}

func assertSubmissionDescriptor(t *testing.T, input ReviewTransitionInput, status ReviewTargetStatusResult, slot string) {
	t.Helper()
	descriptor := input.Submission
	if descriptor == nil || descriptor.OperationToken != "finalize" || descriptor.Value.Slot != slot || descriptor.Value.SubstitutionLocation != 6 {
		t.Fatalf("submission descriptor = %#v", descriptor)
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sortedReviewStatusStrings(mapKeys(fields)), []string{"argument_tokens", "operation_token", "value"}) {
		t.Fatalf("descriptor fields = %s", payload)
	}
	if strings.Contains(string(payload), "\"operation\"") || strings.Contains(string(payload), "\"binding\"") || strings.Contains(string(payload), "request_hash") || strings.Contains(string(payload), "\"type\"") {
		t.Fatalf("descriptor retained duplicate metadata: %s", payload)
	}
	if slot == "validation" && descriptor.Value.Schema != reviewValidatorSchemaID || slot == "correction_lines" && (descriptor.Value.Schema != "" || descriptor.Value.Maximum != status.Frozen.CorrectionBudget) {
		t.Fatalf("descriptor value = %#v", descriptor.Value)
	}
	placeholders := 0
	for index, token := range descriptor.ArgumentTokens {
		if strings.HasPrefix(token, "--cwd=") {
			t.Fatalf("descriptor leaked a path or cwd token: %q", token)
		}
		if strings.Contains(token, reviewSubmissionValuePlaceholder) {
			placeholders++
			if index != descriptor.Value.SubstitutionLocation || strings.Count(token, reviewSubmissionValuePlaceholder) != 1 {
				t.Fatalf("descriptor value slot = %q at %d", token, index)
			}
		}
	}
	if placeholders != 1 || !strings.HasPrefix(descriptor.ArgumentTokens[4], "--request-hash=") {
		t.Fatalf("descriptor argv = %#v", descriptor.ArgumentTokens)
	}
}

func assertSubmissionTransitionSchema(t *testing.T, status ReviewTargetStatusResult) {
	t.Helper()
	payload, err := json.Marshal(status.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV4(t, payload)
}

func runSubmissionDescriptor(t *testing.T, descriptor ReviewTransitionSubmission, value string) ([]byte, error) {
	t.Helper()
	arguments := submissionDescriptorArguments(t, descriptor, value)
	var output bytes.Buffer
	err := RunReview(arguments, &output)
	return output.Bytes(), err
}

func submissionDescriptorArguments(t *testing.T, descriptor ReviewTransitionSubmission, value string) []string {
	t.Helper()
	arguments := append([]string{descriptor.OperationToken}, descriptor.ArgumentTokens...)
	slot := descriptor.Value.SubstitutionLocation + 1
	arguments[slot] = strings.Replace(arguments[slot], reviewSubmissionValuePlaceholder, value, 1)
	if strings.Contains(arguments[slot], reviewSubmissionValuePlaceholder) {
		t.Fatal("submission executor did not replace exactly one value slot")
	}
	return arguments
}

func replaceSubmissionToken(t *testing.T, descriptor ReviewTransitionSubmission, prefix, replacement string) ReviewTransitionSubmission {
	t.Helper()
	copy := descriptor
	copy.ArgumentTokens = append([]string{}, descriptor.ArgumentTokens...)
	for index, token := range copy.ArgumentTokens {
		if strings.HasPrefix(token, prefix) {
			copy.ArgumentTokens[index] = replacement
			return copy
		}
	}
	t.Fatalf("descriptor tokens lack %q: %v", prefix, copy.ArgumentTokens)
	return ReviewTransitionSubmission{}
}

func swapSubmissionTokens(t *testing.T, descriptor ReviewTransitionSubmission, left, right int) ReviewTransitionSubmission {
	t.Helper()
	copy := descriptor
	copy.ArgumentTokens = append([]string{}, descriptor.ArgumentTokens...)
	if left < 0 || right < 0 || left >= len(copy.ArgumentTokens) || right >= len(copy.ArgumentTokens) {
		t.Fatalf("submission token indexes %d,%d out of range", left, right)
	}
	copy.ArgumentTokens[left], copy.ArgumentTokens[right] = copy.ArgumentTokens[right], copy.ArgumentTokens[left]
	return copy
}

func assertSubmissionNotStarted(t *testing.T, err error, output []byte, store reviewtransaction.CompactStore, before []byte) {
	t.Helper()
	if err == nil {
		t.Fatalf("submission unexpectedly succeeded: %s", output)
	}
	failure := decodeReviewIntegrationFailure(t, output)
	if failure.Operation != ReviewIntegrationOperationFinalize || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("submission failure = %#v", failure)
	}
	if after := readReviewOperationFile(t, store.StatePath()); !bytes.Equal(before, after) {
		t.Fatal("rejected submission mutated authority bytes")
	}
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
