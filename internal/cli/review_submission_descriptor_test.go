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
	reviewEnabledHome(t)
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
	evidence := filepath.Join(t.TempDir(), "correction-evidence.txt")
	if err := os.WriteFile(evidence, []byte("repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waiting := submissionDescriptorStatus(t, repo, started.LineageID)
	capture := captureEvidenceSubmissionInput(t, waiting)
	assertCaptureEvidenceSubmissionDescriptor(t, capture, waiting)
	if output, err := runCaptureEvidenceSubmission(t, *capture.Submission, map[string]string{
		"outcome": string(reviewtransaction.VerificationOutcomePassed), "input": evidence,
	}); err != nil {
		t.Fatalf("execute correction evidence descriptor: %v\n%s", err, output)
	}

	ready := submissionDescriptorStatus(t, repo, started.LineageID)
	validation := submissionDescriptorInput(t, ready)
	assertSubmissionDescriptor(t, validation, ready, "validation")
	assertSubmissionTransitionSchema(t, ready)
	if len(validation.Arguments) != 6 {
		t.Fatalf("targeted-validation provider argument count = %d, want 6: %#v", len(validation.Arguments), validation.Arguments)
	}
	validationArguments := requiredReviewTransitionArguments(t, validation.Arguments, "lineage", "expected-revision", "target", "repository-context", "purpose", "request-hash")
	contextHandle := validationArguments["repository-context"]
	if root, err := reviewtransaction.ResolveReviewRepositoryContext(context.Background(), contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: ready.Authority.LineageID, Revision: ready.Authority.Revision, TargetIdentity: ready.ValidationRequest.CorrectionTargetIdentity,
	}); err != nil || root != repo {
		t.Fatalf("targeted-validation repository context = %q, %v; want %q", root, err, repo)
	}
	wantValidationArguments := []ReviewTransitionArgument{
		{Name: "lineage", Value: ready.Authority.LineageID}, {Name: "expected-revision", Value: ready.Authority.Revision},
		{Name: "target", Value: ready.ValidationRequest.CorrectionTargetIdentity}, {Name: "repository-context", Value: contextHandle},
		{Name: "purpose", Value: reviewTargetedValidationPurpose}, {Name: "request-hash", Value: ready.ValidationRequest.RequestHash},
	}
	if validation.CaptureOperation != "external.run_targeted_validation" || !reflect.DeepEqual(validation.Arguments, wantValidationArguments) {
		t.Fatalf("targeted-validation provider arguments = %#v, want %#v", validation.Arguments, wantValidationArguments)
	}
	for name, value := range map[string]string{
		"lineage": "forged-lineage", "expected-revision": "sha256:" + strings.Repeat("a", 64),
		"target": "sha256:" + strings.Repeat("b", 64), "repository-context": "rctx1_" + strings.Repeat("e", 64),
		"purpose": reviewTargetedValidationPurpose + "-forged", "request-hash": "sha256:" + strings.Repeat("f", 64),
	} {
		original := ready.NextTransition.Collect.Inputs[0].Arguments
		ready.NextTransition.Collect.Inputs[0].Arguments = replaceReviewTransitionArgument(t, original, name, value)
		if err := ready.validateSubmissionDescriptors(); err == nil {
			t.Fatalf("targeted-validation descriptor accepted forged %q", name)
		}
		ready.NextTransition.Collect.Inputs[0].Arguments = original
	}
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

	output, err = runSubmissionDescriptor(t, *validation.Submission, validationPath)
	if err != nil {
		t.Fatalf("execute validation descriptor: %v\n%s", err, output)
	}
	terminal := assertApprovedBurnedCompactNegotiatedFinalize(t, output)
	if terminal.LineageID != started.LineageID {
		t.Fatalf("validation descriptor terminal lineage = %q, want %q", terminal.LineageID, started.LineageID)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	output, err = runSubmissionDescriptor(t, *validation.Submission, validationPath)
	if err == nil {
		t.Fatalf("burned validation descriptor succeeded: %s", output)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func submissionDescriptorCorrectionFixture(t *testing.T) (string, ReviewIntegrationStartResult, reviewtransaction.CompactStore) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
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
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo, "--lineage", lineage}, &output); err != nil {
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
	validateAgainstPublishedNextTransitionSchemaV5(t, payload)
}

func TestCaptureEvidenceDescriptorExecutesExactV5Transition(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, _, _ := capturedArtifact(t)
	if err := RunReviewFacadeFinalize([]string{
		"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo,
		"--lineage", started.LineageID, "--captured-results=true",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	status := submissionDescriptorStatus(t, repo, started.LineageID)
	capture := captureEvidenceSubmissionInput(t, status)
	assertCaptureEvidenceSubmissionDescriptor(t, capture, status)
	evidence := filepath.Join(t.TempDir(), "verification-evidence.txt")
	if err := os.WriteFile(evidence, []byte("verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runCaptureEvidenceSubmission(t, *capture.Submission, map[string]string{
		"outcome": string(reviewtransaction.VerificationOutcomePassed), "input": evidence,
	})
	if err != nil {
		t.Fatalf("execute exact evidence descriptor: %v\n%s", err, output)
	}
	var captured reviewtransaction.VerificationEvidenceRecord
	decodeStrictReviewJSON(t, output, &captured)
	if captured.Outcome != reviewtransaction.VerificationOutcomePassed {
		t.Fatalf("captured evidence = %#v", captured)
	}
	ready := submissionDescriptorStatus(t, repo, started.LineageID)
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionExecute ||
		ready.NextTransition.Execute == nil || ready.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("captured evidence was reoffered instead of advancing authority: %#v", ready.NextTransition)
	}
}

func captureEvidenceSubmissionInput(t *testing.T, status ReviewTargetStatusResult) ReviewTransitionInput {
	t.Helper()
	if status.Schema != ReviewIntegrationStatusSchemaV5 || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Submission == nil {
		t.Fatalf("capture-evidence submission transition = %#v", status.NextTransition)
	}
	return status.NextTransition.Collect.Inputs[0]
}

func assertCaptureEvidenceSubmissionDescriptor(t *testing.T, input ReviewTransitionInput, status ReviewTargetStatusResult) {
	t.Helper()
	descriptor := input.Submission
	if descriptor == nil || descriptor.OperationToken != "capture-evidence" || descriptor.Value != nil ||
		input.Schema != reviewVerificationEvidenceSchemaID || status.Authority == nil || len(descriptor.Values) != 2 {
		t.Fatalf("capture-evidence submission descriptor = %#v", descriptor)
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
	if !reflect.DeepEqual(sortedReviewStatusStrings(mapKeys(fields)), []string{"argument_tokens", "operation_token", "values"}) {
		t.Fatalf("capture-evidence descriptor fields = %s", payload)
	}
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["lineage"] != status.Authority.LineageID || arguments["expected-revision"] != status.Authority.Revision ||
		arguments["target"] == "" || reviewtransaction.ValidateReviewRepositoryContextHandle(arguments["repository-context"]) != nil {
		t.Fatalf("capture-evidence provider arguments = %#v", arguments)
	}
	for _, token := range descriptor.ArgumentTokens {
		if strings.HasPrefix(token, "--cwd=") {
			t.Fatalf("capture-evidence descriptor leaked cwd: %q", token)
		}
	}
	if descriptor.Values[0].Slot != "outcome" || descriptor.Values[0].SubstitutionLocation != 4 ||
		!reflect.DeepEqual(descriptor.Values[0].AllowedValues, []string{
			string(reviewtransaction.VerificationOutcomePassed),
			string(reviewtransaction.VerificationOutcomeFailed),
			string(reviewtransaction.VerificationOutcomeProceduralFailure),
		}) || descriptor.Values[1].Slot != "input" || descriptor.Values[1].Schema != reviewVerificationEvidenceSchemaID ||
		descriptor.Values[1].SubstitutionLocation != 5 {
		t.Fatalf("capture-evidence descriptor values = %#v", descriptor.Values)
	}
	assertSubmissionTransitionSchema(t, status)
}

func runCaptureEvidenceSubmission(t *testing.T, descriptor ReviewTransitionSubmission, values map[string]string) ([]byte, error) {
	t.Helper()
	arguments := append([]string{descriptor.OperationToken}, descriptor.ArgumentTokens...)
	for _, slot := range descriptor.Values {
		value, found := values[slot.Slot]
		if !found {
			t.Fatalf("missing consumer substitution for %q", slot.Slot)
		}
		index := slot.SubstitutionLocation + 1
		arguments[index] = strings.Replace(arguments[index], "{{"+slot.Slot+"}}", value, 1)
	}
	var output bytes.Buffer
	err := RunReview(arguments, &output)
	return output.Bytes(), err
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

func replaceReviewTransitionArgument(t *testing.T, arguments []ReviewTransitionArgument, name, value string) []ReviewTransitionArgument {
	t.Helper()
	copy := append([]ReviewTransitionArgument(nil), arguments...)
	for index := range copy {
		if copy[index].Name == name {
			copy[index].Value = value
			return copy
		}
	}
	t.Fatalf("transition arguments omit %q: %#v", name, arguments)
	return nil
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
