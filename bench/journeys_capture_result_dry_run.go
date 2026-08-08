package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
	envelope, err := readStatus(r)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		return errors.New("expected a reviewer-result collect transition")
	}
	input := envelope.NextTransition.Collect.Inputs[0]
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
	if response["schema"] != "gentle-ai.review-capture-result-dry-run/v1" || response["validation"] != "accepted" {
		return fmt.Errorf("unexpected dry-run response: %#v", response)
	}
	for _, forbidden := range []string{"path", "reference", "sha256", "raw_sha256", "canonical_sha256", "result_hash"} {
		if _, found := response[forbidden]; found {
			return fmt.Errorf("dry-run response exposed forbidden field %q", forbidden)
		}
	}

	unchanged, err := readStatus(r)
	if err != nil {
		return err
	}
	if unchanged.argument("lens") != envelope.argument("lens") || unchanged.argument("order") != envelope.argument("order") {
		return errors.New("dry run advanced review authority")
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
