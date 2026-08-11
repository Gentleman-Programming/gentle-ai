package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func captureResultDryRunJourneys() []Journey {
	return []Journey{{
		ID:     "j77-capture-result-input-preflight-is-read-only",
		Title:  "Capture-result input preflight validates admission without persistence",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2630",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{Name: "fixture: stage ordinary code", Fixture: stageOrdinaryCode},
			{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
			{Name: "dry-run admission then real capture", Requires: captureResultCapability, Composite: exerciseCaptureResultDryRun},
		},
	}}
}

func exerciseCaptureResultDryRun(r *journeyRun) error {
	envelope, statusBefore, err := readCaptureResultDryRunStatus(r)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		return errors.New("expected a reviewer-result collect transition")
	}
	input := envelope.NextTransition.Collect.Inputs[0]
	gitBefore, err := gitOut(r.sandbox, r.sandbox.Repo, "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	args := []string{
		"review", "capture-result", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"), "--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"), "--lens", envelope.argument("lens"),
		"--order", envelope.argument("order"), "--preflight",
	}

	invalid, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, nil)
	if err != nil {
		return err
	}
	invalidPath, err := writeScratch(r.sandbox, "dry-run-invalid.json", invalid)
	if err != nil {
		return err
	}
	if observation := r.run(append(args, "--input", invalidPath), false); observation.ExitCode == 0 {
		return errors.New("invalid dry-run reviewer result was accepted")
	}

	valid, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, envelope.paths())
	if err != nil {
		return err
	}
	validPath, err := writeScratch(r.sandbox, "dry-run-valid.json", valid)
	if err != nil {
		return err
	}
	observation := r.run(append(args, "--input", validPath), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("valid dry run failed: %s", firstLine(observation.Stderr))
	}
	var response map[string]any
	if err := json.Unmarshal([]byte(observation.Stdout), &response); err != nil {
		return fmt.Errorf("decode dry-run response: %w", err)
	}
	order, err := strconv.Atoi(envelope.argument("order"))
	if err != nil {
		return fmt.Errorf("decode collect order: %w", err)
	}
	wantFields := map[string]bool{
		"schema": true, "operation": true, "validation": true, "lineage_id": true,
		"lens": true, "selected_order": true, "subject_hash": true, "admission_decision": true,
	}
	if response["schema"] != "gentle-ai.review-capture-result-dry-run/v1" || response["operation"] != "review/capture-result" ||
		response["validation"] != "accepted" || response["lineage_id"] != envelope.argument("lineage") ||
		response["lens"] != envelope.argument("lens") || response["selected_order"] != float64(order) ||
		response["subject_hash"] != input.ArtifactSubject.SubjectHash || response["admission_decision"] != "completed" ||
		len(response) != len(wantFields) {
		return fmt.Errorf("unexpected dry-run response: %#v", response)
	}
	for field := range response {
		if !wantFields[field] {
			return fmt.Errorf("dry-run response exposed forbidden field %q", field)
		}
	}
	gitAfter, err := gitOut(r.sandbox, r.sandbox.Repo, "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	if gitAfter != gitBefore {
		return fmt.Errorf("dry run changed Git state: before %q, after %q", gitBefore, gitAfter)
	}

	_, statusAfter, err := readCaptureResultDryRunStatus(r)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(statusAfter, statusBefore) {
		return errors.New("dry run changed the public status envelope")
	}
	realArgs := append(append([]string{}, args[:len(args)-1]...), "--input", validPath)
	if observation := r.run(realArgs, true); observation.ExitCode != 0 {
		return fmt.Errorf("real capture after dry run failed: %s", firstLine(observation.Stderr))
	}
	advanced, err := readStatus(r)
	if err != nil {
		return err
	}
	if advanced.NextTransition.Kind == "collect" && advanced.argument("lens") == envelope.argument("lens") && advanced.argument("order") == envelope.argument("order") {
		return errors.New("real capture did not persist and advance the collect binding")
	}
	return nil
}

func readCaptureResultDryRunStatus(r *journeyRun) (statusEnvelope, any, error) {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContract, "--next-transition"}, false)
	var envelope statusEnvelope
	var document any
	payload := []byte(strings.TrimSpace(observation.Stdout))
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return envelope, nil, fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return envelope, nil, fmt.Errorf("parse complete review status: %w", err)
	}
	return envelope, document, nil
}
