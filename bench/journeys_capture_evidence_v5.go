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
	targetedInspectionLineage                  = "targeted-validator-inspection"
	statusSchemaV6                             = "gentle-ai.review-integration.status/v6"
	statusSchemaV7                             = "gentle-ai.review-integration.status/v7"
	verificationEvidenceSchemaV1               = "https://gentle-ai.dev/schema/review/verification-evidence/v1"
	verificationEvidenceRecordSchemaV2         = "gentle-ai.review-verification-evidence/v2"
)

var captureEvidenceDescriptorCapability = &Capability{Verb: []string{"review", "capture-evidence"},
	Flags: []string{"--repository-context", "--lineage", "--target", "--expected-revision", "--outcome", "--input"}}
var targetedInspectionCapability = &Capability{Verb: []string{"review", "inspect-candidate"},
	Flags: []string{"--repository-context", "--lineage", "--target", "--expected-revision", "--purpose", "--request-hash", "--operation", "--path-index", "--side"}}

// captureEvidenceDescriptorJourneys proves issue #2248's V5 contract at the
// built-binary boundary. The runner executes only tokens published by STATUS;
// it supplies the outcome and raw evidence path, never an authority binding.
func captureEvidenceDescriptorJourneys() []Journey {
	return []Journey{
		{
			ID:     "j66-v5-capture-evidence-descriptors-execute",
			Review: reviewOptedIn,
			Title:  "#3417: a normal STATUS v5 evidence descriptor captures the full selected lens set for its exact active lineage",
			Source: "issue #2248 under #3417: provider-owned capture-evidence advances normal verification, then finalization burns the exact transaction",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage normal code candidate", Fixture: stageWaveCandidate},
				{Name: "start normal review with an exact active lineage", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", captureEvidenceDescriptorNormalLineage)},
				{Name: "capture the full selected lens set in STATUS order for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureExactSelectedReviewerSlots(r, captureEvidenceDescriptorNormalLineage, false)
				}},
				{Name: "finalize exact active-lineage reviewer results into validation", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", captureEvidenceDescriptorNormalLineage, "--captured-results=true"),
					After: requireReviewState("validating", captureEvidenceDescriptorNormalLineage)},
				{Name: "execute the exact active-lineage normal STATUS v5 capture-evidence descriptor", Requires: captureEvidenceDescriptorCapability, Composite: captureV5NormalEvidenceDescriptor},
				{Name: "final evidence burns the advanced normal transaction", Requires: finalizeEvidenceCapability, Composite: finalizeV5NormalReview},
			},
		},
		{
			ID:     "j67-v5-capture-evidence-correction-descriptor-executes",
			Review: reviewOptedIn,
			Title:  "#3417: a STATUS v5 correction evidence descriptor continues only the exact active lineage",
			Source: "issue #2248 under #3417: provider-owned capture-evidence advances correction verification without selectorless authority reuse",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage correction candidate", Fixture: stageCaptureEvidenceDescriptorCorrection},
				{Name: "start correction review with an exact active lineage", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", captureEvidenceDescriptorCorrectionLineage)},
				{Name: "capture correction finding and the full selected lens set for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureExactSelectedReviewerSlots(r, captureEvidenceDescriptorCorrectionLineage, true)
				}},
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
		{
			ID:     "j95-targeted-validator-inspects-provider-bound-corrected-tree",
			Review: reviewOptedIn,
			Title:  "#3417: a targeted validator inspects an exact active-lineage corrected tree through live worktree drift",
			Source: "issue #2945 under #3417: corrected targeted validation inspects only the provider-bound immutable candidate of its exact active lineage",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage correction candidate", Fixture: stageCaptureEvidenceDescriptorCorrection},
				{Name: "start correction review with an exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", targetedInspectionLineage)},
				{Name: "capture correction finding and the full selected lens set for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureExactSelectedReviewerSlots(r, targetedInspectionLineage, true)
				}},
				{Name: "finalize reviewer results into correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", targetedInspectionLineage, "--captured-results=true")},
				{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", targetedInspectionLineage, "--correction-lines", "2")},
				{Name: "fixture: correct the reviewed candidate", Fixture: writeCorrectedCandidate},
				{Name: "execute the correction STATUS v5 capture-evidence descriptor", Requires: captureEvidenceDescriptorCapability, Composite: captureJ95Evidence},
				{Name: "inspect frozen correction after live drift and refuse drifted FINALIZE", Requires: targetedInspectionCapability, Composite: inspectJ95CorrectedCandidate},
				{Name: "finalize after restoring the corrected candidate", Requires: finalizeValidationCapability, Composite: completeJ95Correction},
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

func finalizeV5NormalReview(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", captureEvidenceDescriptorNormalLineage, "--captured-evidence=true"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("finalize normal v5 evidence: %s", firstLine(observation.Stderr))
	}
	if err := requirePendingApproval(captureEvidenceDescriptorNormalLineage)(r.sandbox, observation); err != nil {
		return err
	}
	return requireAtomicLineageAcknowledged(r, captureEvidenceDescriptorNormalLineage)
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
	return captureV5CorrectionEvidenceDescriptorFor(r, captureEvidenceDescriptorCorrectionLineage)
}

func captureJ95Evidence(r *journeyRun) error {
	return captureV5CorrectionEvidenceDescriptorFor(r, targetedInspectionLineage)
}

func captureV5CorrectionEvidenceDescriptorFor(r *journeyRun, lineage string) error {
	after, err := executeV5CaptureEvidenceDescriptor(r, lineage, "correction-v5-evidence.txt")
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
	if status.Schema != statusSchemaV7 || status.Authority == nil || status.NextTransition == nil ||
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
	return completeBurnedV5DescriptorCorrectionFor(r, captureEvidenceDescriptorCorrectionLineage)
}

func completeJ95Correction(r *journeyRun) error {
	return completeBurnedV5DescriptorCorrectionFor(r, targetedInspectionLineage)
}

func completeBurnedV5DescriptorCorrectionFor(r *journeyRun, lineage string) error {
	if err := completeV5DescriptorCorrectionFor(r, lineage); err != nil {
		return err
	}
	return requireAtomicLineageAcknowledged(r, lineage)
}

func completeV5DescriptorCorrectionFor(r *journeyRun, lineage string) error {
	status, err := readCorrectionStatusForContract(r, lineage, reviewContractV2)
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
	if err != nil || result.State != "approved" || result.LineageID != lineage {
		return fmt.Errorf("v5 correction finalize descriptor result = %+v, %v", result, err)
	}
	return nil
}

func inspectJ95CorrectedCandidate(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, targetedInspectionLineage, reviewContractV2)
	if err != nil || status.ValidationRequest == nil || status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("targeted inspection status = %+v, %v", status, err)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "external.run_targeted_validation" || len(input.Arguments) != 6 {
		return fmt.Errorf("targeted inspection binding = %+v", input)
	}
	inspection := []string{"review", "inspect-candidate"}
	for _, argument := range input.Arguments {
		inspection = append(inspection, "--"+argument.Name, argument.Value)
	}
	inspection = append(inspection, "--operation", "object", "--path-index", "0", "--side", "candidate")
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int { return 3 }\n"); err != nil {
		return err
	}
	inspectionResult := r.runAt(r.sandbox.Root, inspection, false)
	if inspectionResult.ExitCode != 0 || !strings.Contains(inspectionResult.Stdout, "func value() int { return 2 }") {
		return fmt.Errorf("provider-bound corrected inspection = %q: %s", inspectionResult.Stdout, firstLine(inspectionResult.Stderr))
	}
	payload, err := json.Marshal(map[string]any{"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity": status.ValidationRequest.CorrectionTargetIdentity, "original_criteria": map[string]any{"passed": true, "evidence": []string{"acceptance passed"}},
		"correction_regression": map[string]any{"passed": true, "evidence": []string{"regression passed"}}, "follow_ups": []any{}})
	if err != nil {
		return err
	}
	path, err := writeScratch(r.sandbox, "j95-validation.json", payload)
	if err != nil {
		return err
	}
	arguments, err := correctionSubmissionArguments(r, status, "targeted_validation_required", "validation", path)
	if err != nil {
		return err
	}
	if observation := r.runAt(r.sandbox.Root, arguments, false); observation.ExitCode == 0 {
		return fmt.Errorf("drifted FINALIZE consumed the correction: %s", observation.Stdout)
	}
	return writeCorrectedCandidate(r.sandbox)
}
