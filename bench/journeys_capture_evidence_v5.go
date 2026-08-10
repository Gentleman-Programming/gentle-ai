package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	captureEvidenceDescriptorNormalLineage     = "capture-evidence-v5-normal"
	captureEvidenceDescriptorCorrectionLineage = "capture-evidence-v5-correction"
	statusSchemaV5                             = "gentle-ai.review-integration.status/v5"
	verificationEvidenceSchemaV1               = "https://gentle-ai.dev/schema/review/verification-evidence/v1"
	verificationEvidenceRecordSchemaV2         = "gentle-ai.review-verification-evidence/v2"
)

var captureEvidenceDescriptorCapability = &Capability{Verb: []string{"review", "capture-evidence"},
	Flags: []string{"--repository-context", "--lineage", "--target", "--expected-revision", "--outcome", "--input"}}

// captureEvidenceDescriptorJourneys proves issue #2248's V5 contract at the
// built-binary boundary. The runner executes only tokens published by STATUS;
// it supplies the outcome and raw evidence path, never an authority binding.
func captureEvidenceDescriptorJourneys() []Journey {
	return []Journey{
		{
			ID:     "j66-v5-capture-evidence-descriptors-execute",
			Title:  "STATUS v5 capture-evidence descriptor executes normal verification without caller-built bindings",
			Source: "issue #2248: provider-owned capture-evidence descriptors must advance ordinary verification",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage normal code candidate", Fixture: stageWaveCandidate},
				{Name: "start normal review", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", captureEvidenceDescriptorNormalLineage)},
				{Name: "capture every normal-review lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize normal reviewer results into validation", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", captureEvidenceDescriptorNormalLineage, "--captured-results=true"),
					After: requireReviewState("validating", captureEvidenceDescriptorNormalLineage)},
				{Name: "execute the normal STATUS v5 capture-evidence descriptor", Requires: captureEvidenceDescriptorCapability, Composite: captureV5NormalEvidenceDescriptor},
				{Name: "finalize the advanced normal review", Requires: finalizeEvidenceCapability,
					Args:  productArgs("review", "finalize", "--lineage", captureEvidenceDescriptorNormalLineage, "--captured-evidence=true"),
					After: requireReviewState("approved", captureEvidenceDescriptorNormalLineage)},
			},
		},
		{
			ID:     "j67-v5-capture-evidence-correction-descriptor-executes",
			Title:  "STATUS v5 capture-evidence descriptor executes corrected verification without caller-built bindings",
			Source: "issue #2248: provider-owned capture-evidence descriptors must advance correction verification",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage correction candidate", Fixture: stageCaptureEvidenceDescriptorCorrection},
				{Name: "start correction review", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", captureEvidenceDescriptorCorrectionLineage)},
				{Name: "capture correction finding and complete lenses", Requires: captureResultCapability, Composite: captureCorrectableFinding},
				{Name: "finalize reviewer results into correction-required", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", captureEvidenceDescriptorCorrectionLineage, "--captured-results=true"),
					After: requireReviewState("correction_required", captureEvidenceDescriptorCorrectionLineage)},
				{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability,
					Args: productArgs("review", "finalize", "--lineage", captureEvidenceDescriptorCorrectionLineage, "--correction-lines", "2")},
				{Name: "fixture: correct the reviewed candidate", Fixture: writeCorrectedCandidate},
				{Name: "execute the correction STATUS v5 capture-evidence descriptor", Requires: captureEvidenceDescriptorCapability, Composite: captureV5CorrectionEvidenceDescriptor},
				{Name: "execute the advanced targeted-validation finalize descriptor", Requires: finalizeValidationCapability, Composite: completeV5DescriptorCorrection},
			},
		},
	}
}

func stageCaptureEvidenceDescriptorCorrection(sandbox *Sandbox) error {
	const candidate = "package candidate\n\nfunc value() int { return 3 }\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), candidate); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != "candidate.go" {
		return fmt.Errorf("v5 correction fixture paths = %q, %v", paths, err)
	}
	return nil
}

func captureV5NormalEvidenceDescriptor(r *journeyRun) error {
	after, err := executeV5CaptureEvidenceDescriptor(r, captureEvidenceDescriptorNormalLineage, "normal-v5-evidence.txt")
	if err != nil {
		return err
	}
	if after.NextTransition == nil || after.NextTransition.Kind != "execute" || after.NextTransition.Execute == nil ||
		after.NextTransition.Execute.Operation != "review.finalize" {
		return fmt.Errorf("normal capture evidence did not advance to review.finalize: %+v", after.NextTransition)
	}
	return nil
}

func captureV5CorrectionEvidenceDescriptor(r *journeyRun) error {
	after, err := executeV5CaptureEvidenceDescriptor(r, captureEvidenceDescriptorCorrectionLineage, "correction-v5-evidence.txt")
	if err != nil {
		return err
	}
	if after.NextTransition == nil || after.NextTransition.Kind != "collect" ||
		after.NextTransition.ReasonCode != "targeted_validation_required" || after.NextTransition.Collect == nil ||
		len(after.NextTransition.Collect.Inputs) != 1 || after.NextTransition.Collect.Inputs[0].Submission == nil ||
		after.NextTransition.Collect.Inputs[0].Submission.OperationToken != "finalize" {
		return fmt.Errorf("correction capture evidence did not advance to a review.finalize descriptor: %+v", after.NextTransition)
	}
	return nil
}

func executeV5CaptureEvidenceDescriptor(r *journeyRun, lineage, evidenceName string) (waveCorrectionStatus, error) {
	status, err := readCorrectionStatusForContract(r, lineage, reviewContractV2)
	if err != nil {
		return waveCorrectionStatus{}, err
	}
	if status.Schema != statusSchemaV5 || status.Authority == nil || status.NextTransition == nil ||
		status.NextTransition.Kind != "collect" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != "review.capture-evidence" {
		return waveCorrectionStatus{}, fmt.Errorf("v5 status did not publish capture-evidence: %+v", status)
	}
	path, err := writeScratch(r.sandbox, evidenceName, []byte("go test ./... passed\nraw verification evidence\n"))
	if err != nil {
		return waveCorrectionStatus{}, err
	}
	arguments, err := captureEvidenceDescriptorArguments(status, "passed", path)
	if err != nil {
		return waveCorrectionStatus{}, err
	}
	observation := r.runAt(r.sandbox.Root, arguments, false)
	if observation.ExitCode != 0 {
		return waveCorrectionStatus{}, fmt.Errorf("execute v5 capture-evidence descriptor: %s", firstLine(observation.Stderr))
	}
	var captured struct {
		Schema            string `json:"schema"`
		Version           int    `json:"version"`
		LineageID         string `json:"lineage_id"`
		AuthorityRevision string `json:"authority_revision"`
		TargetIdentity    string `json:"target_identity"`
		Outcome           string `json:"outcome"`
		RawPayloadSHA256  string `json:"raw_payload_sha256"`
		RawPayloadBytes   int64  `json:"raw_payload_bytes"`
		RecordDigest      string `json:"record_digest"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &captured); err != nil {
		return waveCorrectionStatus{}, fmt.Errorf("decode captured verification-evidence record: %w", err)
	}
	target := strings.TrimPrefix(status.NextTransition.Collect.Inputs[0].Submission.ArgumentTokens[2], "--target=")
	if captured.Schema != verificationEvidenceRecordSchemaV2 || captured.Version != 2 ||
		captured.LineageID != status.Authority.LineageID || captured.AuthorityRevision != status.Authority.Revision ||
		captured.TargetIdentity != target || captured.Outcome != "passed" || captured.RawPayloadSHA256 == "" ||
		captured.RawPayloadBytes <= 0 || captured.RecordDigest == "" {
		return waveCorrectionStatus{}, fmt.Errorf("capture-evidence did not publish a bound v2 record: %+v", captured)
	}
	return readCorrectionStatusForContract(r, lineage, reviewContractV2)
}

func captureEvidenceDescriptorArguments(status waveCorrectionStatus, outcome, input string) ([]string, error) {
	descriptor := status.NextTransition.Collect.Inputs[0].Submission
	if descriptor == nil || descriptor.OperationToken != "capture-evidence" || descriptor.Value != nil ||
		len(descriptor.ArgumentTokens) != 6 || len(descriptor.Values) != 2 ||
		descriptor.ArgumentTokens[0] != "--lineage="+status.Authority.LineageID ||
		descriptor.ArgumentTokens[1] != "--expected-revision="+status.Authority.Revision ||
		!strings.HasPrefix(descriptor.ArgumentTokens[2], "--target=") ||
		!strings.HasPrefix(descriptor.ArgumentTokens[3], "--repository-context=") ||
		descriptor.ArgumentTokens[4] != "--outcome={{outcome}}" || descriptor.ArgumentTokens[5] != "--input={{input}}" {
		return nil, fmt.Errorf("capture-evidence descriptor is not provider-bound: %+v", descriptor)
	}
	if strings.TrimPrefix(descriptor.ArgumentTokens[2], "--target=") == "" ||
		strings.TrimPrefix(descriptor.ArgumentTokens[3], "--repository-context=") == "" {
		return nil, fmt.Errorf("capture-evidence descriptor omitted target or repository context: %+v", descriptor.ArgumentTokens)
	}
	for _, token := range descriptor.ArgumentTokens {
		if strings.ContainsAny(token, " \t\r\n") || strings.HasPrefix(token, "--cwd=") {
			return nil, fmt.Errorf("capture-evidence descriptor leaked a caller-owned token: %q", token)
		}
	}
	outcomeSlot, inputSlot := descriptor.Values[0], descriptor.Values[1]
	if outcomeSlot.Slot != "outcome" || outcomeSlot.Domain != "verification_outcome" || outcomeSlot.Schema != "" ||
		outcomeSlot.SubstitutionLocation != 4 || strings.Join(outcomeSlot.AllowedValues, "\x00") != "passed\x00verification_failed\x00procedural_tooling_failed" ||
		inputSlot.Slot != "input" || inputSlot.Domain != "artifact_path_or_stdin" || inputSlot.Schema != verificationEvidenceSchemaV1 ||
		len(inputSlot.AllowedValues) != 0 || inputSlot.SubstitutionLocation != 5 {
		return nil, fmt.Errorf("capture-evidence descriptor values are invalid: %+v", descriptor.Values)
	}
	arguments := append([]string{"review", descriptor.OperationToken}, descriptor.ArgumentTokens...)
	arguments[outcomeSlot.SubstitutionLocation+2] = strings.Replace(arguments[outcomeSlot.SubstitutionLocation+2], "{{outcome}}", outcome, 1)
	arguments[inputSlot.SubstitutionLocation+2] = strings.Replace(arguments[inputSlot.SubstitutionLocation+2], "{{input}}", input, 1)
	if strings.Contains(arguments[outcomeSlot.SubstitutionLocation+2], "{{outcome}}") || strings.Contains(arguments[inputSlot.SubstitutionLocation+2], "{{input}}") {
		return nil, fmt.Errorf("capture-evidence descriptor did not substitute outcome and input: %v", arguments)
	}
	return arguments, nil
}

func completeV5DescriptorCorrection(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, captureEvidenceDescriptorCorrectionLineage, reviewContractV2)
	if err != nil {
		return err
	}
	if status.ValidationRequest == nil {
		return fmt.Errorf("v5 correction has no targeted validation request: %+v", status)
	}
	payload, err := json.Marshal(map[string]any{
		"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity":       status.ValidationRequest.CorrectionTargetIdentity,
		"original_criteria":                map[string]any{"passed": true, "evidence": []string{"original acceptance check passed"}},
		"correction_regression":            map[string]any{"passed": true, "evidence": []string{"targeted regression check passed"}},
		"follow_ups":                       []any{},
	})
	if err != nil {
		return err
	}
	path, err := writeScratch(r.sandbox, "v5-targeted-validation.json", append(payload, '\n'))
	if err != nil {
		return err
	}
	arguments, err := correctionSubmissionArguments(r, status, "targeted_validation_required", "validation", path)
	if err != nil {
		return err
	}
	result, err := decodeWaveOperation(r.runAt(r.sandbox.Root, arguments, false), "v5 correction finalize descriptor")
	if err != nil || result.State != "approved" || result.LineageID != captureEvidenceDescriptorCorrectionLineage {
		return fmt.Errorf("v5 correction finalize descriptor result = %+v, %v", result, err)
	}
	return nil
}
